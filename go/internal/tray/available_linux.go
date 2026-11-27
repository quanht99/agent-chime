//go:build linux

package tray

import "github.com/godbus/dbus/v5"

// Available reports whether a StatusNotifierWatcher owns its name on the
// session bus. The shared session bus connection is left open.
func Available() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}

// CanRun reports whether the tray can run at all: a session bus is reachable. The icon then shows as
// soon as a StatusNotifierWatcher exists, even one that starts later.
func CanRun() bool {
	_, err := dbus.SessionBus()
	return err == nil
}
