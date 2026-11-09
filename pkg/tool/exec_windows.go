package tool

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const procThreadAttributeJobList = 0x0002000d

var updateWindowsProcessAttribute = windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")

func resolveLocalShell() (string, error) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	return selectWindowsShell(exec.LookPath, system)
}

func selectWindowsShell(find func(string) (string, error), system string) (string, error) {
	for _, candidate := range []string{
		"pwsh.exe",
		filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"),
		filepath.Join(system, "cmd.exe"),
	} {
		path, err := find(candidate)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		return path, err
	}
	return "", errors.New("no supported Windows shell is available (pwsh.exe, powershell.exe, cmd.exe)")
}

func windowsShellCommand(shell, command string) string {
	if strings.EqualFold(filepath.Base(shell), "cmd.exe") {
		chcp := filepath.Join(filepath.Dir(shell), "chcp.com")
		return windows.EscapeArg(shell) + ` /D /S /V:OFF /C ""` + chcp + `" 65001 >nul 2>&1 & ` + command + `"`
	}
	// An encoded script preserves quotes, newlines and Unicode in both PowerShell versions.
	// Capture $? immediately. A native command failure can supply its exact exit status.
	script := "$OutputEncoding = [Text.UTF8Encoding]::new(); [Console]::OutputEncoding = $OutputEncoding\n" +
		"$global:LASTEXITCODE = 0\n" + command +
		"\nif (-not $?) { if ($LASTEXITCODE) { exit $LASTEXITCODE }; exit 1 }; exit 0"
	var encoded []byte
	for _, char := range utf16.Encode([]rune(script)) {
		encoded = binary.LittleEndian.AppendUint16(encoded, char)
	}
	return windows.ComposeCommandLine([]string{shell, "-NoLogo", "-NoProfile", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded)})
}

type windowsProcess struct {
	mu  sync.Mutex
	job windows.Handle
}

func (p *windowsProcess) kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(p.job, 1)
}

func (p *windowsProcess) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := windows.CloseHandle(p.job)
	p.job = 0
	return err
}

func startLocalProcess(_ context.Context, shell string, spec Spec, output io.Writer) (io.WriteCloser, func() (int, error), func() error, error) {
	if strings.EqualFold(filepath.Base(shell), "cmd.exe") && strings.HasPrefix(filepath.ToSlash(spec.Workdir), "//") {
		return nil, nil, nil, errors.New("cmd.exe requires a drive-backed working directory; map the UNC share or install PowerShell")
	}
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, err
	}
	defer stdinRead.Close()
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		stdinWrite.Close()
		return nil, nil, nil, err
	}
	defer stdoutWrite.Close()

	drained := make(chan error, 1)
	go func() {
		_, err := io.Copy(output, stdoutRead)
		drained <- errors.Join(err, stdoutRead.Close())
	}()
	process, console, err := launchWindowsProcess(shell, spec, stdinRead, stdoutWrite)
	if err != nil {
		stdinWrite.Close()
		stdoutWrite.Close()
		<-drained
		return nil, nil, nil, err
	}

	job := &windowsProcess{job: process.job}
	wait := func() (int, error) {
		defer stdinWrite.Close()
		_, waitErr := windows.WaitForSingleObject(process.handle, windows.INFINITE)
		var code uint32
		exitErr := windows.GetExitCodeProcess(process.handle, &code)
		closeErr := errors.Join(job.close(), windows.CloseHandle(process.handle))
		if console != 0 {
			// ConPTY can produce a final frame while closing. Keep draining on
			// the separate goroutine until the pseudoconsole closes its pipe.
			windows.ClosePseudoConsole(console)
		}
		return int(code), errors.Join(waitErr, exitErr, closeErr, <-drained)
	}
	return stdinWrite, wait, job.kill, nil
}

type windowsChild struct {
	handle, job windows.Handle
}

func launchWindowsProcess(shell string, spec Spec, stdin, stdout *os.File) (child windowsChild, console windows.Handle, err error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return child, 0, err
	}
	defer func() {
		if err != nil {
			_ = windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return child, 0, err
	}

	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return child, 0, err
	}
	defer attributes.Delete()
	// Associate the job at creation, before the shell can start descendants.
	if err = attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return child, 0, err
	}

	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	inputHandle, outputHandle := windows.Handle(stdin.Fd()), windows.Handle(stdout.Fd())
	if spec.TTY {
		if err = windows.CreatePseudoConsole(windows.Coord{X: 80, Y: 24}, inputHandle, outputHandle, 0, &console); err != nil {
			return child, 0, err
		}
		pseudoconsole := console
		defer func() {
			if err != nil {
				// Release the parent copy so the reader sees EOF after closing ConPTY.
				_ = stdout.Close()
				windows.ClosePseudoConsole(pseudoconsole)
			}
		}()
		// HPCON is the attribute value itself. Passing &console would pass
		// the address of a Go variable instead of the native handle.
		ok, _, updateErr := updateWindowsProcessAttribute.Call(
			uintptr(unsafe.Pointer(attributes.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
			uintptr(pseudoconsole), unsafe.Sizeof(pseudoconsole), 0, 0)
		runtime.KeepAlive(attributes)
		if ok == 0 {
			return child, 0, updateErr
		}
	} else {
		flags |= windows.CREATE_NEW_CONSOLE
		startup.ShowWindow = windows.SW_HIDE
		current := windows.CurrentProcess()
		var inherited [2]windows.Handle
		for i, handle := range [...]windows.Handle{inputHandle, outputHandle} {
			if err = windows.DuplicateHandle(current, handle, current, &inherited[i], 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
				return child, 0, err
			}
			defer windows.CloseHandle(inherited[i])
		}
		if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherited[0]), unsafe.Sizeof(inherited)); err != nil {
			return child, 0, err
		}
		startup.Flags = windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW
		startup.StdInput, startup.StdOutput, startup.StdErr = inherited[0], inherited[1], inherited[1]
	}

	application, err := windows.UTF16PtrFromString(shell)
	if err != nil {
		return child, 0, err
	}
	commandLine, err := windows.UTF16PtrFromString(windowsShellCommand(shell, spec.Command))
	if err != nil {
		return child, 0, err
	}
	var cwd *uint16
	if spec.Workdir != "" {
		cwd, err = windows.UTF16PtrFromString(spec.Workdir)
		if err != nil {
			return child, 0, err
		}
	}
	// Cmd.Environ applies Go's case-insensitive environment deduplication on Windows.
	command := exec.Cmd{Env: environment(spec.Env), Dir: spec.Workdir}
	var env []uint16
	for _, variable := range command.Environ() {
		entry, err := windows.UTF16FromString(variable)
		if err != nil {
			return child, 0, err
		}
		env = append(env, entry...)
	}
	if len(env) == 0 {
		env = append(env, 0)
	}
	env = append(env, 0)
	var info windows.ProcessInformation
	err = windows.CreateProcess(application, commandLine, nil, nil, !spec.TTY, flags, &env[0], cwd, &startup.StartupInfo, &info)
	if err != nil {
		return child, 0, fmt.Errorf("start %s: %w", filepath.Base(shell), err)
	}
	_ = windows.CloseHandle(info.Thread)
	return windowsChild{handle: info.Process, job: job}, console, nil
}
