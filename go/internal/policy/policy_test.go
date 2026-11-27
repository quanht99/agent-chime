package policy

import (
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/model"
)

var t0 = time.Unix(1_800_000_000, 0)

func ev(k model.AlertKind, d *float64) model.AgentEvent {
	return model.AgentEvent{Agent: model.Claude, Kind: k, SessionID: "s1", Duration: d}
}

func f(v float64) *float64 { return &v }

func TestDoneShorterThanThresholdIsSkipped(t *testing.T) {
	r := SkipReasonFor(ev(model.Done, f(59)), config.Default(), t0, false)
	if r == nil || r.Kind != "shortTurn" || r.Duration != 59 || r.Threshold != 60 {
		t.Fatalf("got %+v", r)
	}
}

func TestDoneAtExactlyThresholdPlays(t *testing.T) {
	if r := SkipReasonFor(ev(model.Done, f(60)), config.Default(), t0, false); r != nil {
		t.Fatalf("got %+v", r)
	}
}

func TestThresholdZeroAlwaysPlays(t *testing.T) {
	c := config.Default()
	c.DoneThresholdSeconds = 0
	if r := SkipReasonFor(ev(model.Done, f(1)), c, t0, false); r != nil {
		t.Fatalf("got %+v", r)
	}
}

func TestDoneWithoutStartTimePlays(t *testing.T) {
	if r := SkipReasonFor(ev(model.Done, nil), config.Default(), t0, false); r != nil {
		t.Fatalf("got %+v", r)
	}
}

func TestAttentionIgnoresThreshold(t *testing.T) {
	c := config.Default()
	c.DoneThresholdSeconds = 600
	if r := SkipReasonFor(ev(model.Attention, f(1)), c, t0, false); r != nil {
		t.Fatalf("got %+v", r)
	}
}

func TestMuteFuturePastAndForever(t *testing.T) {
	c := config.Default()
	e := ev(model.Attention, nil)
	until := model.Time{Time: t0.Add(60 * time.Second)}
	c.MutedUntil = &until
	r := SkipReasonFor(e, c, t0, false)
	if r == nil || r.Kind != "muted" || r.Until == nil || !r.Until.Equal(until.Time) {
		t.Fatalf("future: %+v", r)
	}
	past := model.Time{Time: t0.Add(-time.Second)}
	c.MutedUntil = &past
	if r := SkipReasonFor(e, c, t0, false); r != nil {
		t.Fatalf("past: %+v", r)
	}
	c.MutedUntil = &model.Time{Time: model.Forever}
	r = SkipReasonFor(e, c, t0, false)
	if r == nil || r.Kind != "muted" || !r.Until.Equal(model.Forever) {
		t.Fatalf("forever: %+v", r)
	}
}

func TestHeadlessSkippedUnlessEnabled(t *testing.T) {
	c := config.Default()
	e := ev(model.Attention, nil)
	if r := SkipReasonFor(e, c, t0, true); r == nil || r.Kind != "headless" {
		t.Fatalf("got %+v", r)
	}
	c.AlertHeadless = true
	if r := SkipReasonFor(e, c, t0, true); r != nil {
		t.Fatalf("got %+v", r)
	}
}

func TestMutedWinsOverShortTurn(t *testing.T) {
	c := config.Default()
	until := model.Time{Time: t0.Add(time.Minute)}
	c.MutedUntil = &until
	if r := SkipReasonFor(ev(model.Done, f(1)), c, t0, true); r == nil || r.Kind != "muted" {
		t.Fatalf("got %+v", r)
	}
}

func TestHeadlessWinsOverShortTurn(t *testing.T) {
	if r := SkipReasonFor(ev(model.Done, f(1)), config.Default(), t0, true); r == nil || r.Kind != "headless" {
		t.Fatalf("got %+v", r)
	}
}

func TestDedupeWindows(t *testing.T) {
	if DedupeWindow(model.Attention) != 3*time.Second || DedupeWindow(model.Done) != 60*time.Second {
		t.Error("windows")
	}
}
