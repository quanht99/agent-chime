//go:build windows

package tray

import (
	"time"

	"fyne.io/systray"
)

// Quit removes the notify icon (fyne deletes it only in its own quit path, which os.Exit skips) and
// waits up to wait for the tray loop to finish. It does nothing if Run never ran.
func Quit(wait time.Duration) {
	if !running.Load() {
		return
	}
	systray.Quit()
	select {
	case <-exited:
	case <-time.After(wait):
	}
}
