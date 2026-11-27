package hook

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

type rig struct {
	p      paths.Paths
	played *[]string
	scans  *int32
	env    map[string]string
	now    time.Time
}

func newRig(t *testing.T, muted bool) *rig {
	t.Helper()
	p := testenv.Home(t)
	if muted {
		c := config.Default()
		c.Mute(nil, time.Now())
		if err := config.Save(p, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"Chime.wav", "Knock.wav"} {
		path := filepath.Join(p.SoundsDir, f)
		_ = os.MkdirAll(p.SoundsDir, 0o755)
		if err := os.WriteFile(path, testenv.WAV16([]int16{1, 2, 3, 4}, 8000), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &rig{p: p, played: &[]string{}, scans: new(int32), env: map[string]string{}, now: time.Unix(1_800_000_000, 0)}
}

func (r *rig) run(sub, stdin string) {
	r.now = r.now.Add(time.Second)
	Run(sub, Env{
		P: r.p, Stdin: strings.NewReader(stdin),
		Getenv:    func(k string) string { return r.env[k] },
		Now:       func() time.Time { return r.now },
		Ancestors: func() [][]string { atomic.AddInt32(r.scans, 1); return nil },
		Play:      func(path string, v float64) error { *r.played = append(*r.played, path); return nil },
		Lib:       sounds.Library{P: r.p, GOOS: "windows"},
	})
}

func (r *rig) log() string {
	b, _ := os.ReadFile(r.p.Log)
	return string(b)
}

func (r *rig) state() state.State { return state.Store{P: r.p}.Load() }

func TestPromptThenStop(t *testing.T) {
	r := newRig(t, false)
	r.run("claude-prompt", `{"session_id":"s1","hook_event_name":"x"}`)
	if _, ok := r.state().TurnStarts["claude|s1"]; !ok {
		t.Fatal("no turn start")
	}
	c := config.Default()
	c.DoneThresholdSeconds = 0
	_ = config.Save(r.p, c)
	r.run("claude-stop", `{"session_id":"s1"}`)
	if len(*r.played) != 1 {
		t.Fatalf("played: %v", *r.played)
	}
	s := r.state()
	if _, ok := s.TurnStarts["claude|s1"]; ok {
		t.Fatal("turn start not consumed")
	}
	if s.LastAlert == nil || s.LastAlert.Duration == nil {
		t.Fatalf("lastAlert: %+v", s.LastAlert)
	}
}

func TestEmptySessionIDStillTakesTheTurnStart(t *testing.T) {
	r := newRig(t, false)
	r.run("claude-prompt", `{"session_id":""}`)
	if _, ok := r.state().TurnStarts["claude|"]; !ok {
		t.Fatal("no turn start for the empty session id")
	}
	r.run("claude-stop", `{"session_id":""}`) // 1 s turn, below the default threshold
	if len(*r.played) != 0 {
		t.Fatalf("short turn played: %v", *r.played)
	}
	if _, ok := r.state().TurnStarts["claude|"]; ok {
		t.Fatal("turn start not consumed")
	}
	if strings.Contains(r.log(), "no start time") {
		t.Fatalf("log: %s", r.log())
	}
}

func TestUnknownSub(t *testing.T) {
	r := newRig(t, true)
	r.run("no-such-command", "")
	if !strings.Contains(r.log(), "unknown subcommand no-such-command") {
		t.Fatalf("log: %s", r.log())
	}
}

func TestGarbagePayload(t *testing.T) {
	r := newRig(t, true)
	r.run("claude-stop", "not json \x00\x01")
	if s := r.state().LastSkip; s == nil || s.Reason.Kind != "muted" {
		t.Fatalf("lastSkip: %+v", s)
	}
	r.run("claude-prompt", "not json")
	if !strings.Contains(r.log(), "hook claude-prompt ignored: invalid payload") {
		t.Fatalf("log: %s", r.log())
	}
	if len(*r.played) != 0 {
		t.Fatal("played")
	}
}

func TestBackgroundWorkHoldsTheTurnUntilTheLastStop(t *testing.T) {
	r := newRig(t, true)
	r.run("claude-prompt", `{"session_id":"s1","prompt":"start two background jobs"}`)
	start, ok := r.state().TurnStarts["claude|s1"]
	if !ok {
		t.Fatal("no start")
	}
	r.run("claude-stop", `{"session_id":"s1","background_tasks":[{"type":"shell","status":"running"}]}`)
	if got := r.state().TurnStarts["claude|s1"]; !got.Equal(start.Time) {
		t.Fatal("start changed by held stop")
	}
	if r.state().LastSkip != nil {
		t.Fatal("held stop must not alert")
	}
	if !strings.Contains(r.log(), "hook claude-stop held: 1 background task(s) still running") {
		t.Fatalf("log: %s", r.log())
	}
	r.run("claude-prompt", `{"session_id":"s1","prompt":"<task-notification>\n<task-id>b1</task-id>"}`)
	if got := r.state().TurnStarts["claude|s1"]; !got.Equal(start.Time) {
		t.Fatal("start not kept")
	}
	if !strings.Contains(r.log(), "background work handed back, turn continues") {
		t.Fatalf("log: %s", r.log())
	}
	r.run("claude-stop", `{"session_id":"s1","background_tasks":[]}`)
	s := r.state()
	if _, ok := s.TurnStarts["claude|s1"]; ok {
		t.Fatal("start remains")
	}
	if s.LastSkip == nil || s.LastSkip.Kind != "done" {
		t.Fatalf("lastSkip: %+v", s.LastSkip)
	}
}

func TestNestedSkipsAndRecords(t *testing.T) {
	r := newRig(t, false)
	r.env["CODEX_THREAD_ID"] = "x"
	r.run("claude-stop", `{"session_id":"s1"}`)
	if s := r.state().LastSkip; s == nil || s.Reason.Kind != "nested" {
		t.Fatalf("lastSkip: %+v", s)
	}
	if !strings.Contains(r.log(), "hook claude-stop skipped: nested") || len(*r.played) != 0 {
		t.Fatalf("log: %s", r.log())
	}
}

func TestNoStartTimeLogged(t *testing.T) {
	r := newRig(t, true)
	r.run("claude-stop", `{"session_id":"s9"}`)
	if !strings.Contains(r.log(), "no start time") {
		t.Fatalf("log: %s", r.log())
	}
}

func TestEnvProbeScansAncestorsOnce(t *testing.T) {
	r := newRig(t, true)
	if err := os.WriteFile(r.p.DebugEnvFlag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r.run("codex-stop", `{"session_id":"s1"}`) // probe + codex alert with !AlertHeadless
	if *r.scans != 1 {
		t.Fatalf("scans: %d", *r.scans)
	}
	if !strings.Contains(r.log(), "env-probe ") {
		t.Fatalf("log: %s", r.log())
	}
}

func TestClaudeAlertDoesNotScanAncestors(t *testing.T) {
	r := newRig(t, true)
	r.run("claude-stop", `{"session_id":"s1"}`)
	if *r.scans != 0 {
		t.Fatalf("scans: %d", *r.scans)
	}
}

func TestRunRecoversFromPanic(t *testing.T) {
	r := newRig(t, true)
	Run("claude-prompt", Env{P: r.p, Stdin: strings.NewReader(`{"session_id":"s1"}`),
		Getenv: func(string) string { panic("boom") }})
	if !strings.Contains(r.log(), "hook claude-prompt crashed: boom") {
		t.Fatalf("log: %s", r.log())
	}
}

func TestReadStdinTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	start := time.Now()
	got := ReadStdin(pr, 300*time.Millisecond)
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("took %v", d)
	}
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestReadStdinReturnsArrivedBytes(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	go func() { _, _ = pw.Write([]byte("abc")) }()
	if got := ReadStdin(pr, 150*time.Millisecond); string(got) != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestReadStdinNilAndEOF(t *testing.T) {
	if len(ReadStdin(nil, time.Second)) != 0 {
		t.Fatal("nil")
	}
	if got := ReadStdin(strings.NewReader("hello"), time.Second); string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestReadStdinCapsAt1MiB(t *testing.T) {
	got := ReadStdin(strings.NewReader(strings.Repeat("a", 2<<20)), time.Second)
	if len(got) != 1<<20 {
		t.Fatalf("len %d", len(got))
	}
}
