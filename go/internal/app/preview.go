package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

// preview starts a child process that makes the sound itself, so stop can
// kill it. The child is always reaped.
func preview(p paths.Paths, players audio.Players, path string, volume float64) (func(), error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		name, args, stdin, err := audio.LinuxCommand(players, path, volume)
		if err != nil {
			return nil, err
		}
		cmd = exec.Command(name, args...)
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
	case "darwin":
		cmd = exec.Command("/usr/bin/afplay", "-v", fmt.Sprintf("%.2f", volume), path)
	default: // windows: PlaySound runs inside our own play child
		prog := p.Program
		if _, err := os.Stat(prog); err != nil {
			if prog, err = os.Executable(); err != nil {
				return nil, err
			}
		}
		cmd = exec.Command(prog, "play", "--volume", fmt.Sprintf("%.2f", volume), path)
		hideWindow(cmd)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return func() { _ = cmd.Process.Kill() }, nil
}
