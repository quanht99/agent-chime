//go:build windows

package app

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDetachKeepsWindowVisible(t *testing.T) {
	cmd := exec.Command("x")
	detach(cmd)
	if cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow must be false")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Error("CREATE_NO_WINDOW missing")
	}
}
