//go:build linux

package audio

import (
	"bytes"
	"os/exec"
)

// Play plays path at volume and waits for the player to finish.
func Play(path string, volume float64) error {
	name, args, stdin, err := LinuxCommand(FindPlayers(exec.LookPath), path, volume)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	return cmd.Run()
}
