//go:build windows

package app

import (
	"os/exec"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// runDataCopy starts the installed copy; the caller then exits 0.
func runDataCopy(p paths.Paths, args []string) error {
	cmd := exec.Command(p.Program, args...)
	detach(cmd)
	allowForeground()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
