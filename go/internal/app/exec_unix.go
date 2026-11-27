//go:build unix

package app

import (
	"os"
	"syscall"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// runDataCopy replaces this process with the installed copy.
func runDataCopy(p paths.Paths, args []string) error {
	return syscall.Exec(p.Program, append([]string{p.Program}, args...), os.Environ())
}
