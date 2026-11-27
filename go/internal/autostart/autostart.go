// Package autostart manages start-at-login for each OS.
package autostart

import (
	"runtime"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// RunKey is a tiny view of the Windows HKCU Run registry key.
type RunKey interface {
	Get(name string) (string, bool)
	Set(name, value string) error
	Delete(name string) error
}

const runValueName = "AgentChime"

// Supported reports whether autostart exists on this OS.
func Supported() bool { return runtime.GOOS == "linux" || runtime.GOOS == "windows" }

func Enabled(p paths.Paths) bool  { return enabled(p) }
func Enable(p paths.Paths) error  { return enable(p) }
func Disable(p paths.Paths) error { return disable(p) }

// Control adapts the package functions to an interface.
type Control struct{ P paths.Paths }

func (c Control) Supported() bool { return Supported() }
func (c Control) Enabled() bool   { return Enabled(c.P) }
func (c Control) Enable() error   { return Enable(c.P) }
func (c Control) Disable() error  { return Disable(c.P) }
