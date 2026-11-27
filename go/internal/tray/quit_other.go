//go:build !windows

package tray

import "time"

// Quit does nothing outside Windows: on Linux the StatusNotifierItem goes away with the process's
// D-Bus connection.
func Quit(time.Duration) {}
