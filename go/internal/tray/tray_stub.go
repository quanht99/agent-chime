//go:build darwin

package tray

// Run is never called on macOS (the Swift app owns the menu bar icon).
func Run(model func() Model, act Actions, ready func()) {}

// Available is always false on macOS.
func Available() bool { return false }

// CanRun is always false on macOS.
func CanRun() bool { return false }
