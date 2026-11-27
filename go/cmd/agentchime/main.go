// Command agentchime is the tray app and its hook and play helper modes.
package main

import (
	"os"
	"strconv"

	"github.com/quanht99/agent-chime/go/internal/app"
	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/hook"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "hook":
			runHook(args[1:])
			os.Exit(0)
		case "play":
			runPlay(args[1:])
			os.Exit(0)
		case "--background":
			app.Run(true)
			return
		}
	}
	app.Run(false)
}

func runHook(args []string) {
	defer func() { _ = recover() }()
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	hook.Run(sub, hook.DefaultEnv(paths.Current()))
}

// runPlay handles `play --volume <v> <path>`; bad arguments do nothing.
func runPlay(args []string) {
	defer func() { _ = recover() }()
	if len(args) != 3 || args[0] != "--volume" {
		return
	}
	v, err := strconv.ParseFloat(args[1], 64)
	if err != nil || args[2] == "" {
		return
	}
	_ = audio.Play(args[2], v)
}
