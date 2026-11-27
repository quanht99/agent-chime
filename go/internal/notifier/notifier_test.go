package notifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

var t0 = time.Unix(1_800_000_000, 0)

type call struct {
	path string
	vol  float64
}

type fixture struct {
	n     Notifier
	p     paths.Paths
	calls *[]call
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, testenv.WAV16([]int16{1, 2, 3, 4}, 8000), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) fixture {
	t.Helper()
	p := testenv.Home(t)
	for _, f := range []string{"Chime.wav", "Knock.wav"} {
		write(t, filepath.Join(p.SoundsDir, f))
	}
	calls := &[]call{}
	return fixture{p: p, calls: calls, n: Notifier{
		P: p, Store: state.Store{P: p},
		Lib:  sounds.Library{P: p, GOOS: "windows"},
		Play: func(path string, v float64) error { *calls = append(*calls, call{path, v}); return nil },
	}}
}

func ev(k model.AlertKind, session string, d *float64) model.AgentEvent {
	return model.AgentEvent{Agent: model.Claude, Kind: k, SessionID: session, Duration: d}
}

func f(v float64) *float64 { return &v }

func logText(t *testing.T, p paths.Paths) string {
	b, _ := os.ReadFile(p.Log)
	return string(b)
}

func TestFireTwiceWithin3sPlaysOnce(t *testing.T) {
	x := setup(t)
	c := config.Default()
	first := x.n.Fire(ev(model.Attention, "s1", nil), c, t0, false)
	second := x.n.Fire(ev(model.Attention, "s1", nil), c, t0.Add(time.Second), false)
	if first.Played == "" {
		t.Fatal("first should play")
	}
	if second.Skipped == nil || second.Skipped.Kind != "duplicate" {
		t.Fatalf("second: %+v", second)
	}
	if len(*x.calls) != 1 {
		t.Fatalf("calls: %d", len(*x.calls))
	}
}

func TestDoneAndAttentionDoNotDedupeEachOther(t *testing.T) {
	x := setup(t)
	c := config.Default()
	x.n.Fire(ev(model.Done, "s1", f(100)), c, t0, false)
	x.n.Fire(ev(model.Attention, "s1", nil), c, t0.Add(time.Second), false)
	if len(*x.calls) != 2 {
		t.Fatalf("calls: %d", len(*x.calls))
	}
}

func TestSkipIsRecordedAndNothingPlays(t *testing.T) {
	x := setup(t)
	c := config.Default()
	c.Mute(nil, t0)
	out := x.n.Fire(ev(model.Attention, "s1", nil), c, t0, false)
	if out.Skipped == nil || out.Skipped.Kind != "muted" {
		t.Fatalf("out: %+v", out)
	}
	if s := x.n.Store.Load().LastSkip; s == nil || s.Reason.Kind != "muted" {
		t.Fatalf("lastSkip: %+v", s)
	}
	if len(*x.calls) != 0 {
		t.Fatal("played")
	}
	if !strings.Contains(logText(t, x.p), "alert skipped: Claude Code") {
		t.Fatalf("log: %s", logText(t, x.p))
	}
}

func TestPlayRecordsLastAlert(t *testing.T) {
	x := setup(t)
	out := x.n.Fire(ev(model.Done, "s1", f(90)), config.Default(), t0, false)
	if out.Played == "" {
		t.Fatal("should play")
	}
	a := x.n.Store.Load().LastAlert
	if a == nil || a.Agent != model.Claude || a.Kind != model.Done || a.Duration == nil || *a.Duration != 90 || !a.Date.Equal(t0) {
		t.Fatalf("lastAlert: %+v", a)
	}
	if !strings.Contains(logText(t, x.p), "alert played: ") {
		t.Fatal("no played log")
	}
}

func TestWarningClearsAfterGoodSound(t *testing.T) {
	x := setup(t)
	bad := config.Default()
	bad.SoundAttention = config.SoundChoice{Source: config.File, Value: "/nonexistent.mp3"}
	x.n.Fire(ev(model.Attention, "a", nil), bad, t0, false)
	w := x.n.Store.Load().SoundWarning
	if w == nil || w.Kind != model.Attention || w.Path != "/nonexistent.mp3" {
		t.Fatalf("warning: %+v", w)
	}
	x.n.Fire(ev(model.Attention, "b", nil), config.Default(), t0.Add(10*time.Second), false)
	if x.n.Store.Load().SoundWarning != nil {
		t.Fatal("warning not cleared")
	}
}

func TestUnknownSourceFallsBackToDefault(t *testing.T) {
	x := setup(t)
	c := config.Default()
	c.SoundDone = config.SoundChoice{Source: "bogus", Value: "Aurora.wav"}
	out := x.n.Fire(ev(model.Done, "s1", f(100)), c, t0, false)
	if !strings.HasSuffix(filepath.ToSlash(out.Played), "Sounds/Chime.wav") {
		t.Fatalf("played %q", out.Played)
	}
	if w := x.n.Store.Load().SoundWarning; w == nil || w.Kind != model.Done {
		t.Fatalf("warning: %+v", w)
	}
}
