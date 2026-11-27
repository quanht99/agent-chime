// Package model holds the alert data types shared by every component.
package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/quanht99/agent-chime/go/internal/i18n"
)

type Agent string

const (
	Claude Agent = "claude"
	Codex  Agent = "codex"
)

func (a Agent) DisplayName() string {
	switch a {
	case Claude:
		return "Claude Code"
	case Codex:
		return "Codex"
	}
	return string(a)
}

type AlertKind string

const (
	Done      AlertKind = "done"
	Attention AlertKind = "attention"
)

// Time marshals as float seconds since 1970 with millisecond precision.
type Time struct{ time.Time }

// Forever is the "muted until I turn it back on" sentinel.
var Forever = time.Unix(64092211200, 0)

func (t Time) MarshalJSON() ([]byte, error) {
	ms := t.UnixMilli()
	return []byte(strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64)), nil
}

func (t *Time) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return fmt.Errorf("invalid time %v", f)
	}
	t.Time = time.UnixMilli(int64(math.Round(f * 1000)))
	return nil
}

type AgentEvent struct {
	Agent     Agent
	Kind      AlertKind
	SessionID string
	Detail    string
	Duration  *float64 // seconds
}

type SkipReason struct {
	Kind      string  `json:"kind"` // muted, headless, duplicate, nested, shortTurn
	Until     *Time   `json:"until,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

func (r SkipReason) Text(lang i18n.Lang) string {
	switch r.Kind {
	case "muted":
		if r.Until == nil || r.Until.Equal(Forever) {
			return i18n.T("reasonMuted", lang)
		}
		return i18n.T("reasonMutedUntil", lang, HHMM(r.Until.Time))
	case "headless":
		return i18n.T("reasonHeadless", lang)
	case "duplicate":
		return i18n.T("reasonDuplicate", lang)
	case "nested":
		return i18n.T("reasonNested", lang)
	case "shortTurn":
		return i18n.T("reasonShortTurn", lang, DurationShort(r.Duration), DurationShort(r.Threshold))
	}
	return r.Kind
}

type AlertRecord struct {
	Agent    Agent     `json:"agent"`
	Kind     AlertKind `json:"kind"`
	Duration *float64  `json:"duration,omitempty"`
	Date     Time      `json:"date"`
}

func (r AlertRecord) Summary(lang i18n.Lang) string {
	if r.Kind == Attention {
		return i18n.T("summaryAttention", lang, r.Agent.DisplayName())
	}
	if r.Duration != nil {
		return i18n.T("summaryDoneDuration", lang, r.Agent.DisplayName(), DurationShort(*r.Duration))
	}
	return i18n.T("summaryDone", lang, r.Agent.DisplayName())
}

type SkipRecord struct {
	Agent  Agent      `json:"agent"`
	Kind   AlertKind  `json:"kind"`
	Reason SkipReason `json:"reason"`
	Date   Time       `json:"date"`
}

func (r SkipRecord) Summary(lang i18n.Lang) string {
	key := i18n.Key("summaryAttention")
	if r.Kind == Done {
		key = "summaryDone"
	}
	return i18n.T(key, lang, r.Agent.DisplayName())
}

type SoundWarning struct {
	Kind AlertKind `json:"kind"`
	Path string    `json:"path"`
}

// DurationShort formats seconds as "42s", "3m12s" or "1h05m".
func DurationShort(seconds float64) string {
	total := int(math.Round(seconds))
	if total < 0 {
		total = 0
	}
	switch {
	case total < 60:
		return fmt.Sprintf("%ds", total)
	case total < 3600:
		return fmt.Sprintf("%dm%02ds", total/60, total%60)
	}
	return fmt.Sprintf("%dh%02dm", total/3600, total%3600/60)
}

// HHMM formats t in local time as "15:04".
func HHMM(t time.Time) string { return t.Local().Format("15:04") }
