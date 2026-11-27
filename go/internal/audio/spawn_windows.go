//go:build windows

package audio

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// SpawnDetached starts program detached from this process and its console. If
// the job forbids breakaway (access denied) it retries once without it.
func SpawnDetached(program string, args ...string) error {
	flags := uint32(windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS)
	start := func(f uint32) (*exec.Cmd, error) {
		cmd := exec.Command(program, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: f, HideWindow: true}
		return cmd, cmd.Start()
	}
	cmd, err := start(flags | windows.CREATE_BREAKAWAY_FROM_JOB)
	if err != nil && errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		cmd, err = start(flags)
	}
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}
