// Package policy decides when an alert is skipped.
package policy

import (
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/model"
)

// DedupeWindow is how long a repeat of the same alert is ignored.
func DedupeWindow(k model.AlertKind) time.Duration {
	if k == model.Attention {
		return 3 * time.Second
	}
	return 60 * time.Second
}

// SkipReasonFor returns why the alert should not sound, or nil when it should.
// headless is the already-computed "is this run headless" flag.
func SkipReasonFor(ev model.AgentEvent, c config.Config, now time.Time, headless bool) *model.SkipReason {
	if c.IsMuted(now) {
		until := *c.MutedUntil
		return &model.SkipReason{Kind: "muted", Until: &until}
	}
	if headless && !c.AlertHeadless {
		return &model.SkipReason{Kind: "headless"}
	}
	if ev.Kind == model.Done && ev.Duration != nil && c.DoneThresholdSeconds > 0 && *ev.Duration < c.DoneThresholdSeconds {
		return &model.SkipReason{Kind: "shortTurn", Duration: *ev.Duration, Threshold: c.DoneThresholdSeconds}
	}
	return nil
}
