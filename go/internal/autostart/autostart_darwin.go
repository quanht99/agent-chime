//go:build darwin

package autostart

import (
	"errors"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// WindowsRunKey is unused on macOS.
var WindowsRunKey RunKey

func enabled(paths.Paths) bool  { return false }
func enable(paths.Paths) error  { return errors.ErrUnsupported }
func disable(paths.Paths) error { return errors.ErrUnsupported }
