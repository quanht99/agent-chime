//go:build linux

package autostart

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// WindowsRunKey is unused on Linux.
var WindowsRunKey RunKey

func desktopEntry(p paths.Paths) []byte {
	return []byte("[Desktop Entry]\nType=Application\nName=AgentChime\nExec=" + desktopExec(p.Program) + "\nX-GNOME-Autostart-enabled=true\n")
}

func enabled(p paths.Paths) bool {
	b, err := os.ReadFile(p.AutostartFile)
	return err == nil && bytes.Equal(b, desktopEntry(p))
}

func enable(p paths.Paths) error {
	if err := os.MkdirAll(filepath.Dir(p.AutostartFile), 0o755); err != nil {
		return err
	}
	tmp := p.AutostartFile + ".tmp"
	if err := os.WriteFile(tmp, desktopEntry(p), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.AutostartFile); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func disable(p paths.Paths) error {
	if err := os.Remove(p.AutostartFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
