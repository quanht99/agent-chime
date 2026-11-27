//go:build windows

package audio

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sndNoDefault = 0x0002
	sndMemory    = 0x0004
	sndSync      = 0x0000
	sndFilename  = 0x00020000
)

var procPlaySound = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

// Play plays path at volume and waits for it to finish. Volume is applied by
// scaling the samples; the system mixer volume is never touched.
func Play(path string, volume float64) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if scaled, ok := ScaleWAV(b, volume); ok {
		r, _, callErr := procPlaySound.Call(uintptr(unsafe.Pointer(&scaled[0])), 0, sndMemory|sndSync|sndNoDefault)
		runtime.KeepAlive(scaled)
		if r == 0 {
			return fmt.Errorf("PlaySoundW: %w", callErr)
		}
		return nil
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, callErr := procPlaySound.Call(uintptr(unsafe.Pointer(p)), 0, sndFilename|sndSync|sndNoDefault)
	runtime.KeepAlive(p)
	if r == 0 {
		return fmt.Errorf("PlaySoundW: %w", callErr)
	}
	return nil
}
