//go:build unix

package settingswin

import (
	"os"

	"golang.org/x/sys/unix"
)

// DetachStdout points fd 1 at /dev/null so a write after the parent closed
// the pipe cannot kill the helper with SIGPIPE.
func DetachStdout() error {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.Dup2(int(f.Fd()), 1)
}
