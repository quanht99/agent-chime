//go:build darwin

package audio

import (
	"fmt"
	"math"
	"os/exec"
)

// Play plays path with afplay (development only) and waits for it to finish.
func Play(path string, volume float64) error {
	if math.IsNaN(volume) {
		volume = 1
	}
	v := math.Min(1, math.Max(0, volume))
	return exec.Command("/usr/bin/afplay", "-v", fmt.Sprintf("%.2f", v), path).Run()
}
