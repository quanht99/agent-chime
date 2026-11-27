//go:build !windows

package audio

import (
	"os"
	"os/exec"
	"syscall"
)

// SpawnDetached starts program in its own session with stdio on /dev/null and
// returns without waiting for it.
func SpawnDetached(program string, args ...string) error {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd := exec.Command(program, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
