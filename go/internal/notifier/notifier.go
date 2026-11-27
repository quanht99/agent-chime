// Package notifier decides whether an alert plays or is skipped and records
// the outcome in state.json and the log.
package notifier

import (
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/policy"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/state"
)

// Player plays path at volume (0..1).
type Player func(path string, volume float64) error

type Notifier struct {
	P     paths.Paths
	Store state.Store
	Lib   sounds.Library
	Play  Player
}

// Outcome is either the played path or the skip reason.
type Outcome struct {
	Played  string
	Skipped *model.SkipReason
}

// Fire runs the alert pipeline for one event.
func (n Notifier) Fire(ev model.AgentEvent, c config.Config, now time.Time, headless bool) Outcome {
	if r := policy.SkipReasonFor(ev, c, now, headless); r != nil {
		return n.skip(ev, *r, now)
	}
	if !n.Store.ShouldFire(ev, policy.DedupeWindow(ev.Kind), now) {
		return n.skip(ev, model.SkipReason{Kind: "duplicate"}, now)
	}

	choice := c.Sound(ev.Kind)
	path, warning := n.Lib.ResolvePlayable(choice, ev.Kind)
	n.Store.SetSoundWarning(warning)

	rec := model.AlertRecord{Agent: ev.Agent, Kind: ev.Kind, Duration: ev.Duration, Date: model.Time{Time: now}}
	if n.Play != nil {
		if err := n.Play(path, c.Volume(ev.Kind)); err != nil {
			applog.Write(n.P, "sound player failed: "+err.Error())
		}
	}
	n.Store.RecordAlert(rec)
	applog.Write(n.P, "alert played: "+rec.Summary(i18n.EN)+" sound="+path)
	return Outcome{Played: path}
}

func (n Notifier) skip(ev model.AgentEvent, r model.SkipReason, now time.Time) Outcome {
	rec := model.SkipRecord{Agent: ev.Agent, Kind: ev.Kind, Reason: r, Date: model.Time{Time: now}}
	n.Store.RecordSkip(rec)
	applog.Write(n.P, "alert skipped: "+rec.Summary(i18n.EN)+" reason="+r.Text(i18n.EN))
	return Outcome{Skipped: &r}
}
