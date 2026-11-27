// Package hook is `agentchime hook <sub>`: Claude and Codex call it with a
// JSON payload on stdin. It must always return quickly and never panic out.
package hook

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/hookcmd"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/notifier"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/runctx"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/state"
)

const (
	stdinTimeout = 300 * time.Millisecond
	stdinMax     = 1 << 20
)

type Env struct {
	P         paths.Paths
	Stdin     io.Reader
	Getenv    func(string) string
	Environ   []string
	Now       func() time.Time
	Ancestors func() [][]string
	Play      notifier.Player
	Lib       sounds.Library
}

// DefaultEnv wires the real process environment. Play spawns this executable
// detached in play mode so the hook returns immediately.
func DefaultEnv(p paths.Paths) Env {
	return Env{
		P: p, Stdin: os.Stdin, Getenv: os.Getenv, Environ: os.Environ(),
		Now: time.Now, Ancestors: runctx.Ancestors,
		Lib: sounds.Library{P: p, GOOS: runtime.GOOS, Players: audio.FindPlayers(exec.LookPath)},
		Play: func(path string, volume float64) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			return audio.SpawnDetached(exe, "play", "--volume", fmt.Sprintf("%.2f", volume), path)
		},
	}
}

// ReadStdin returns what arrived on r within timeout (at most 1 MiB). A nil
// reader, a terminal, or an unstat-able file gives nothing.
func ReadStdin(r io.Reader, timeout time.Duration) []byte {
	if r == nil {
		return nil
	}
	if f, ok := r.(*os.File); ok {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice != 0 {
			return nil
		}
	}
	var (
		mu   sync.Mutex
		buf  []byte
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		chunk := make([]byte, 32*1024)
		for {
			n, err := r.Read(chunk)
			mu.Lock()
			if n > 0 {
				buf = append(buf, chunk[:n]...)
			}
			full := len(buf) >= stdinMax
			if full {
				buf = buf[:stdinMax]
			}
			mu.Unlock()
			if err != nil || full {
				return
			}
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]byte(nil), buf...)
}

// Run handles one hook invocation. It never panics out.
func Run(sub string, e Env) {
	defer func() {
		if r := recover(); r != nil {
			applog.Write(e.P, fmt.Sprintf("hook %s crashed: %v", sub, r))
		}
	}()
	run(sub, e)
}

func run(sub string, e Env) {
	p := e.P
	cmd, ok := hookcmd.ParseCommand(sub)
	if !ok {
		applog.Write(p, "unknown subcommand "+sub)
		return
	}
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.Getenv == nil {
		e.Getenv = func(string) string { return "" }
	}
	payload := ReadStdin(e.Stdin, stdinTimeout)

	var (
		ancestors [][]string
		scanned   bool
	)
	getAncestors := func() [][]string {
		if !scanned {
			scanned = true
			if e.Ancestors != nil {
				ancestors = e.Ancestors()
			}
		}
		return ancestors
	}

	if _, err := os.Stat(p.DebugEnvFlag); err == nil {
		applog.Write(p, "env-probe "+runctx.ProbeLineFull(sub, e.Environ, getAncestors(), payload))
	}

	agent := cmd.Agent()
	action := hookcmd.Parse(cmd, payload)
	now := e.Now()
	store := state.Store{P: p}

	if runctx.IsNested(agent, e.Getenv) {
		if action.Kind == hookcmd.Alert {
			ev := action.Event
			store.RecordSkip(model.SkipRecord{Agent: ev.Agent, Kind: ev.Kind,
				Reason: model.SkipReason{Kind: "nested"}, Date: model.Time{Time: now}})
		}
		applog.Write(p, "hook "+sub+" skipped: nested")
		return
	}

	switch action.Kind {
	case hookcmd.TurnStarted:
		store.RecordTurnStart(agent, action.SessionID, now)
	case hookcmd.TurnContinued:
		kept := store.ContinueTurn(agent, action.SessionID, now)
		what := "new turn"
		if kept {
			what = "turn continues"
		}
		applog.Write(p, "hook "+sub+": background work handed back, "+what)
	case hookcmd.TurnHeld:
		store.HoldTurn(agent, action.SessionID, now)
		applog.Write(p, fmt.Sprintf("hook %s held: %d background task(s) still running", sub, action.Running))
	case hookcmd.TurnCancelled:
		store.ClearTurnStart(agent, action.SessionID, now)
	case hookcmd.Ignore:
		applog.Write(p, "hook "+sub+" ignored: "+action.Reason)
	case hookcmd.Alert:
		ev := action.Event
		if ev.Kind == model.Done && action.HasSession {
			if d := store.TakeTurnStart(agent, ev.SessionID, now); d != nil {
				ev.Duration = d
			} else {
				applog.Write(p, "no start time")
			}
		}
		c := config.Load(p)
		if agent == model.Codex && !c.AlertHeadless {
			getAncestors()
		}
		headless := runctx.IsHeadless(agent, e.Getenv, ancestors)
		n := notifier.Notifier{P: p, Store: store, Lib: e.Lib, Play: e.Play}
		n.Fire(ev, c, now, headless)
	}
}
