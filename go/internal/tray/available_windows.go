//go:build windows

package tray

// Available is true: Windows always has a notification area.
func Available() bool { return true }

// CanRun is true on Windows.
func CanRun() bool { return true }
