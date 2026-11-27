//go:build !windows

package app

import (
	"os/exec"
	"syscall"
)

func hideWindow(*exec.Cmd) {}

// detach starts the helper in its own session so it outlives this process.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
