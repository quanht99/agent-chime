//go:build windows

package app

import "golang.org/x/sys/windows"

const asfwAny = 0xFFFFFFFF

// allowForeground lets the helper bring its window to the front.
func allowForeground() {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")
	_, _, _ = proc.Call(uintptr(asfwAny))
}
