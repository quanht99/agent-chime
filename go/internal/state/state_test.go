package state

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

var t0 = time.UnixMilli(1_800_000_000_000)

func at(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }

func newStore(t *testing.T) Store { return Store{P: testenv.Home(t)} }

func wantElapsed(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("elapsed = %v, want %v", got, want)
	}
}

func wantNil(t *testing.T, got *float64) {
	t.Helper()
	if got != nil {
		t.Fatalf("elapsed = %v, want nil", *got)
	}
}

func TestKeys(t *testing.T) {
	if Key(model.Claude, "s1") != "claude|s1" {
		t.Error("Key")
	}
	if FiredKey(model.Codex, "", model.Done) != "codex|-|done" || FiredKey(model.Claude, "s", model.Attention) != "claude|s|attention" {
		t.Error("FiredKey")
	}
}

func TestTurnStartRoundTrip(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	wantElapsed(t, s.TakeTurnStart(model.Claude, "s1", at(90)), 90)
	wantNil(t, s.TakeTurnStart(model.Claude, "s1", at(91)))
}

func TestKeyedByAgentAndSession(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	wantNil(t, s.TakeTurnStart(model.Codex, "s1", t0))
	wantNil(t, s.TakeTurnStart(model.Claude, "s2", t0))
	wantElapsed(t, s.TakeTurnStart(model.Claude, "s1", t0), 0)
}

func TestStaleTurnStartDropped(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	wantNil(t, s.TakeTurnStart(model.Claude, "s1", at(24*3600+1)))
	if len(s.Load().TurnStarts) != 0 {
		t.Error("turnStarts not empty")
	}
}

func TestClearTurnStart(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	s.ClearTurnStart(model.Claude, "s1", t0)
	wantNil(t, s.TakeTurnStart(model.Claude, "s1", t0))
}

func TestRecordPrunesOldEntries(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "old", t0)
	s.ShouldFire(model.AgentEvent{Agent: model.Claude, Kind: model.Done, SessionID: "old"}, 60*time.Second, t0)
	s.RecordTurnStart(model.Claude, "new", at(24*3600+1))
	st := s.Load()
	if len(st.TurnStarts) != 1 || st.TurnStarts["claude|new"].IsZero() {
		t.Errorf("turnStarts = %v", st.TurnStarts)
	}
	if len(st.LastFired) != 0 {
		t.Errorf("lastFired = %v", st.LastFired)
	}
}

func TestPrunesOlderThan24h(t *testing.T) {
	s := newStore(t)
	s.Update(t0, func(st *State) {
		st.TurnStarts["a|old"] = model.Time{Time: at(-1)}
		st.HeldTurns["a|old"] = model.Time{Time: at(-1)}
		st.LastFired["a|-|done"] = model.Time{Time: at(-1)}
		st.TurnStarts["a|edge"] = model.Time{Time: at(-24 * 3600)}
		st.TurnStarts["a|new"] = model.Time{Time: at(1)}
	})
	st := s.Load()
	if len(st.TurnStarts) != 3 {
		t.Fatalf("setup: %v", st.TurnStarts)
	}
	s.Update(at(24*3600), func(*State) {})
	st = s.Load()
	if len(st.TurnStarts) != 1 || st.TurnStarts["a|new"].IsZero() || len(st.HeldTurns) != 0 || len(st.LastFired) != 0 {
		t.Errorf("after prune: %+v", st)
	}
	if _, ok := st.TurnStarts["a|edge"]; ok {
		t.Error("edge entry older than 24h should be gone")
	}
}

func TestAttentionDedupeWindow(t *testing.T) {
	s := newStore(t)
	e := model.AgentEvent{Agent: model.Claude, Kind: model.Attention, SessionID: "s1"}
	w := 3 * time.Second
	if !s.ShouldFire(e, w, t0) {
		t.Error("first")
	}
	if s.ShouldFire(e, w, at(2)) {
		t.Error("within window")
	}
	if !s.ShouldFire(e, w, at(3.5)) {
		t.Error("after window")
	}
	other := e
	other.SessionID = "s2"
	if !s.ShouldFire(other, w, at(2)) {
		t.Error("other session")
	}
}

func TestDoneDedupeWindow(t *testing.T) {
	s := newStore(t)
	e := model.AgentEvent{Agent: model.Codex, Kind: model.Done}
	w := 60 * time.Second
	if !s.ShouldFire(e, w, t0) || s.ShouldFire(e, w, at(59)) || !s.ShouldFire(e, w, at(61)) {
		t.Error("done window")
	}
	if _, ok := s.Load().LastFired["codex|-|done"]; !ok {
		t.Error("missing codex|-|done")
	}
}

func TestConcurrentUpdatesKeepAll(t *testing.T) {
	s := newStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RecordTurnStart(model.Claude, fmt.Sprintf("s%d", i), t0)
		}()
	}
	wg.Wait()
	if n := len(s.Load().TurnStarts); n != 20 {
		t.Fatalf("got %d turn starts, want 20", n)
	}
}

func TestUnreadableStateIsDefaults(t *testing.T) {
	s := newStore(t)
	if err := os.MkdirAll(s.P.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.P.State, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := s.Load()
	if len(st.TurnStarts) != 0 || st.TurnStarts == nil || st.LastAlert != nil {
		t.Errorf("got %+v", st)
	}
	s.RecordTurnStart(model.Claude, "s1", t0)
	if len(s.Load().TurnStarts) != 1 {
		t.Error("not replaced")
	}
	b, _ := os.ReadFile(s.P.State)
	if !strings.HasSuffix(string(b), "}\n") || !strings.Contains(string(b), "\n  \"turnStarts\"") {
		t.Errorf("format: %q", b)
	}
}

func TestMissingStateFileLoadsDefaults(t *testing.T) {
	st := newStore(t).Load()
	if st.TurnStarts == nil || st.HeldTurns == nil || st.LastFired == nil || len(st.TurnStarts) != 0 {
		t.Errorf("got %+v", st)
	}
}

func TestRecordsAlertSkipAndWarning(t *testing.T) {
	s := newStore(t)
	d := 75.0
	alert := model.AlertRecord{Agent: model.Claude, Kind: model.Done, Duration: &d, Date: model.Time{Time: t0}}
	skip := model.SkipRecord{Agent: model.Codex, Kind: model.Attention, Reason: model.SkipReason{Kind: "headless"}, Date: model.Time{Time: t0}}
	w := &model.SoundWarning{Kind: model.Done, Path: "/Users/example/x.wav"}
	s.RecordAlert(alert)
	s.RecordSkip(skip)
	s.SetSoundWarning(w)
	st := s.Load()
	if st.LastAlert == nil || *st.LastAlert.Duration != 75 || !st.LastAlert.Date.Equal(t0) || st.LastAlert.Agent != model.Claude {
		t.Errorf("alert %+v", st.LastAlert)
	}
	if st.LastSkip == nil || st.LastSkip.Reason.Kind != "headless" || st.LastSkip.Agent != model.Codex {
		t.Errorf("skip %+v", st.LastSkip)
	}
	if st.SoundWarning == nil || *st.SoundWarning != *w {
		t.Errorf("warning %+v", st.SoundWarning)
	}
	s.SetSoundWarning(nil)
	st = s.Load()
	if st.SoundWarning != nil || st.LastAlert == nil {
		t.Errorf("after clear %+v", st)
	}
}

func TestHeldTurnKeepsTheStartAcrossHandBacks(t *testing.T) {
	s := newStore(t)
	c := model.Claude
	s.RecordTurnStart(c, "s1", t0)
	s.HoldTurn(c, "s1", at(5))
	if !s.ContinueTurn(c, "s1", at(28)) {
		t.Fatal("first continue")
	}
	s.HoldTurn(c, "s1", at(30))
	if !s.ContinueTurn(c, "s1", at(37)) {
		t.Fatal("second continue")
	}
	wantElapsed(t, s.TakeTurnStart(c, "s1", at(78)), 78)
	if len(s.Load().HeldTurns) != 0 {
		t.Error("held not empty")
	}
}

func TestHandBackWithoutHoldStartsFresh(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	if s.ContinueTurn(model.Claude, "s1", at(28)) {
		t.Fatal("should not keep")
	}
	wantElapsed(t, s.TakeTurnStart(model.Claude, "s1", at(28)), 0)
}

func TestHoldIsUsedOnce(t *testing.T) {
	s := newStore(t)
	c := model.Claude
	s.RecordTurnStart(c, "s1", t0)
	s.HoldTurn(c, "s1", at(5))
	if !s.ContinueTurn(c, "s1", at(10)) {
		t.Fatal("first")
	}
	if s.ContinueTurn(c, "s1", at(20)) {
		t.Fatal("second should start fresh")
	}
	wantElapsed(t, s.TakeTurnStart(c, "s1", at(20)), 0)
}

func TestTypedPromptReplacesAHeldStart(t *testing.T) {
	s := newStore(t)
	c := model.Claude
	s.RecordTurnStart(c, "s1", t0)
	s.HoldTurn(c, "s1", at(5))
	s.RecordTurnStart(c, "s1", at(600))
	if len(s.Load().HeldTurns) != 0 {
		t.Error("held not cleared")
	}
	if s.ContinueTurn(c, "s1", at(609)) {
		t.Fatal("should start fresh")
	}
	wantElapsed(t, s.TakeTurnStart(c, "s1", at(609)), 0)
}

func TestHoldWithoutStartContinuesFresh(t *testing.T) {
	s := newStore(t)
	s.HoldTurn(model.Claude, "s1", t0)
	if s.ContinueTurn(model.Claude, "s1", at(28)) {
		t.Fatal("should start fresh")
	}
	wantElapsed(t, s.TakeTurnStart(model.Claude, "s1", at(28)), 0)
}

func TestHoldsAreKeyedByAgentAndSession(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	s.RecordTurnStart(model.Claude, "s2", t0)
	s.HoldTurn(model.Claude, "s1", t0)
	if s.ContinueTurn(model.Claude, "s2", at(9)) {
		t.Error("s2")
	}
	if s.ContinueTurn(model.Codex, "s1", at(9)) {
		t.Error("codex")
	}
	if !s.ContinueTurn(model.Claude, "s1", at(9)) {
		t.Error("s1")
	}
}

func TestClearTurnStartDropsTheHold(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	s.HoldTurn(model.Claude, "s1", t0)
	s.ClearTurnStart(model.Claude, "s1", t0)
	if len(s.Load().HeldTurns) != 0 {
		t.Error("held not cleared")
	}
}

func TestStaleHoldsArePruned(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "old", t0)
	s.HoldTurn(model.Claude, "old", t0)
	s.RecordTurnStart(model.Claude, "new", at(24*3600+1))
	if len(s.Load().HeldTurns) != 0 {
		t.Error("held not pruned")
	}
}

func TestHeldStartOlderThanADayIsNotKept(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	s.HoldTurn(model.Claude, "s1", t0)
	later := at(24*3600 + 1)
	if s.ContinueTurn(model.Claude, "s1", later) {
		t.Fatal("should not keep")
	}
	wantElapsed(t, s.TakeTurnStart(model.Claude, "s1", later), 0)
}

func TestStateFileFromBeforeHeldTurnsStillLoads(t *testing.T) {
	s := newStore(t)
	if err := os.MkdirAll(s.P.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.P.State, []byte(`{"turnStarts":{"claude|s1":1800000000}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := s.Load()
	if len(st.TurnStarts) != 1 || st.HeldTurns == nil {
		t.Errorf("got %+v", st)
	}
}

func TestLockFileIs0600(t *testing.T) {
	s := newStore(t)
	s.RecordTurnStart(model.Claude, "s1", t0)
	fi, err := os.Stat(s.P.StateLock)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}
