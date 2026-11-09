package coding

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ExecACP runs in Clara's stdio wrapper process. The wrapper owns the job, so
// terminating it also terminates the server and its descendants.
func ExecACP(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("external ACP command is missing")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if cmd.Err != nil {
		return cmd.Err
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer null.Close()

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return errors.Join(err, windows.CloseHandle(job))
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		return errors.Join(err, windows.CloseHandle(job))
	}
	// Leave the non-inheritable job handle open until this wrapper exits.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, null
	return cmd.Run()
}
