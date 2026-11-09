package tool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsShellSelection(t *testing.T) {
	system := `C:\Windows\System32`
	powershell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := filepath.Join(system, "cmd.exe")
	for _, want := range []string{"pwsh.exe", powershell, cmd, ""} {
		got, err := selectWindowsShell(func(candidate string) (string, error) {
			if candidate == want {
				return candidate, nil
			}
			return "", exec.ErrNotFound
		}, system)
		if got != want || (err != nil) != (want == "") {
			t.Errorf("shell %q: got %q, %v", want, got, err)
		}
	}
	_, err := selectWindowsShell(func(string) (string, error) { return "", os.ErrPermission }, system)
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("shell lookup should surface access errors: %v", err)
	}
}

func TestWindowsJobChild(t *testing.T) {
	if os.Getenv("CLARA_TEST_JOB_CHILD") != "1" {
		t.Skip("subprocess fixture")
	}
	time.Sleep(time.Minute)
}

func TestWindowsProcessTreeAndConPTY(t *testing.T) {
	shell, err := resolveLocalShell()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(filepath.Base(shell), "cmd.exe") {
		t.Skip("this test uses the standard Windows PowerShell installation")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tty := range []bool{false, true} {
		t.Run(fmt.Sprintf("tty=%t", tty), func(t *testing.T) {
			process, err := NewLocal().Start(t.Context(), Spec{
				Command: "$p = Start-Process -FilePath $env:CLARA_TEST_EXE -ArgumentList '-test.run=^TestWindowsJobChild$' -PassThru; Write-Output ('CLARA-PID=' + $p.Id + ':'); Start-Sleep 60",
				Workdir: t.TempDir(), TTY: tty, Timeout: 10 * time.Second,
				Env: map[string]string{"CLARA_TEST_EXE": executable, "CLARA_TEST_JOB_CHILD": "1"},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer process.Close()
			var output string
			for {
				poll, err := process.Poll(t.Context(), 100*time.Millisecond)
				if err != nil {
					t.Fatal(err)
				}
				output += poll.Output
				_, suffix, found := strings.Cut(output, "CLARA-PID=")
				pidText, _, complete := strings.Cut(suffix, ":")
				if found && complete {
					pid, err := strconv.ParseUint(pidText, 10, 32)
					if err != nil {
						t.Fatal(err, output)
					}
					child, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
					if err != nil {
						t.Fatal(err)
					}
					defer windows.CloseHandle(child)
					if err := process.Close(); err != nil {
						t.Fatal(err)
					}
					if state, err := windows.WaitForSingleObject(child, 3000); err != nil || state != windows.WAIT_OBJECT_0 {
						t.Fatalf("descendant remained running: %d, %v", state, err)
					}
					break
				}
				if !poll.Running {
					t.Fatal("child PID was not captured", output)
				}
			}
		})
	}
}

func TestWindowsShellExitAndUnicode(t *testing.T) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ shell, command string }{
		{filepath.Join(system, "cmd.exe"), `echo "中文 & space" & exit /b 23`},
		{filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), `Write-Output '中文 & space'; exit 23`},
	} {
		t.Run(filepath.Base(fixture.shell), func(t *testing.T) {
			if _, err := os.Stat(fixture.shell); errors.Is(err, os.ErrNotExist) {
				t.Skip("shell unavailable")
			}
			output := newCapture(4096)
			stdin, wait, kill, err := startLocalProcess(t.Context(), fixture.shell, Spec{Command: fixture.command, Workdir: t.TempDir()}, output)
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			defer kill()
			code, err := wait()
			if got := output.drain(); err != nil || code != 23 || !strings.Contains(got, "中文 & space") {
				t.Fatalf("code %d, output %q, error %v", code, got, err)
			}
		})
	}
}
