// Package state is the small JSON file hooks share: turn starts, dedupe
// timestamps and the last alert. Writers are serialised with a file lock.
package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

const maxAge = 24 * time.Hour

type State struct {
	TurnStarts   map[string]model.Time `json:"turnStarts"`
	HeldTurns    map[string]model.Time `json:"heldTurns"`
	LastFired    map[string]model.Time `json:"lastFired"`
	LastAlert    *model.AlertRecord    `json:"lastAlert,omitempty"`
	LastSkip     *model.SkipRecord     `json:"lastSkip,omitempty"`
	SoundWarning *model.SoundWarning   `json:"soundWarning,omitempty"`
}

func (s *State) fill() {
	if s.TurnStarts == nil {
		s.TurnStarts = map[string]model.Time{}
	}
	if s.HeldTurns == nil {
		s.HeldTurns = map[string]model.Time{}
	}
	if s.LastFired == nil {
		s.LastFired = map[string]model.Time{}
	}
}

// Key is the turnStarts/heldTurns key.
func Key(a model.Agent, session string) string { return string(a) + "|" + session }

// FiredKey is the lastFired key; an empty session becomes "-".
func FiredKey(a model.Agent, session string, k model.AlertKind) string {
	if session == "" {
		session = "-"
	}
	return string(a) + "|" + session + "|" + string(k)
}

type Store struct{ P paths.Paths }

// Load is lock-free. A missing or unreadable file yields defaults.
func (s Store) Load() State {
	var st State
	data, err := os.ReadFile(s.P.State)
	if err != nil {
		st.fill()
		return st
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(data, &st); err != nil {
		applog.Write(s.P, "state.json is unreadable, using defaults: "+err.Error())
		st = State{}
	}
	st.fill()
	return st
}

func prune(m map[string]model.Time, cutoff time.Time) {
	for k, v := range m {
		if v.Before(cutoff) {
			delete(m, k)
		}
	}
}

// Update runs fn on the current state under an exclusive file lock, prunes
// entries older than 24h and writes atomically. If the lock cannot be taken
// it runs anyway (fail open).
func (s Store) Update(now time.Time, fn func(*State)) {
	if err := os.MkdirAll(s.P.DataDir, 0o755); err != nil {
		applog.Write(s.P, "cannot create data dir: "+err.Error())
	}
	f, err := os.OpenFile(s.P.StateLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		applog.Write(s.P, "cannot open state.lock, writing state without a lock: "+err.Error())
	} else {
		defer f.Close()
		if err := fsx.Lock(f); err != nil {
			applog.Write(s.P, "cannot lock state.lock, writing state without a lock: "+err.Error())
		} else {
			defer fsx.Unlock(f)
		}
	}

	st := s.Load()
	cutoff := now.Add(-maxAge)
	prune(st.TurnStarts, cutoff)
	prune(st.HeldTurns, cutoff)
	prune(st.LastFired, cutoff)
	fn(&st)
	st.fill()
	if err := s.write(st); err != nil {
		applog.Write(s.P, "cannot write state.json: "+err.Error())
	}
}

// write uses the same formatting as config.Save: sorted keys, 2-space indent,
// trailing newline.
func (s Store) write(st State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.P.State), "state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := fsx.WriteSyncFile(tmp, buf.Bytes()); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := fsx.Rename(name, s.P.State); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func (s Store) RecordTurnStart(a model.Agent, session string, now time.Time) {
	s.Update(now, func(st *State) {
		k := Key(a, session)
		st.TurnStarts[k] = model.Time{Time: now}
		delete(st.HeldTurns, k)
	})
}

// HoldTurn marks a turn that ended while background work still runs.
func (s Store) HoldTurn(a model.Agent, session string, now time.Time) {
	s.Update(now, func(st *State) { st.HeldTurns[Key(a, session)] = model.Time{Time: now} })
}

// ContinueTurn handles a hand-back prompt: right after a hold it keeps the
// turn's start (using up the hold) and returns true; otherwise it starts the
// turn now and returns false.
func (s Store) ContinueTurn(a model.Agent, session string, now time.Time) bool {
	kept := false
	s.Update(now, func(st *State) {
		k := Key(a, session)
		_, held := st.HeldTurns[k]
		delete(st.HeldTurns, k)
		if _, ok := st.TurnStarts[k]; held && ok {
			kept = true
			return
		}
		st.TurnStarts[k] = model.Time{Time: now}
	})
	return kept
}

// TakeTurnStart clears the hold and start and returns the elapsed seconds,
// or nil when there is no start or it is more than 24h old.
func (s Store) TakeTurnStart(a model.Agent, session string, now time.Time) *float64 {
	var out *float64
	s.Update(now, func(st *State) {
		k := Key(a, session)
		delete(st.HeldTurns, k)
		start, ok := st.TurnStarts[k]
		delete(st.TurnStarts, k)
		if !ok || now.Sub(start.Time) > maxAge {
			return
		}
		e := now.Sub(start.Time).Seconds()
		out = &e
	})
	return out
}

func (s Store) ClearTurnStart(a model.Agent, session string, now time.Time) {
	s.Update(now, func(st *State) {
		k := Key(a, session)
		delete(st.TurnStarts, k)
		delete(st.HeldTurns, k)
	})
}

// ShouldFire reports whether the event is outside its dedupe window and, if
// so, records it.
func (s Store) ShouldFire(ev model.AgentEvent, window time.Duration, now time.Time) bool {
	fire := false
	s.Update(now, func(st *State) {
		k := FiredKey(ev.Agent, ev.SessionID, ev.Kind)
		if last, ok := st.LastFired[k]; ok && now.Sub(last.Time) < window {
			return
		}
		st.LastFired[k] = model.Time{Time: now}
		fire = true
	})
	return fire
}

func (s Store) RecordAlert(r model.AlertRecord) {
	s.Update(r.Date.Time, func(st *State) { st.LastAlert = &r })
}

func (s Store) RecordSkip(r model.SkipRecord) {
	s.Update(r.Date.Time, func(st *State) { st.LastSkip = &r })
}

func (s Store) SetSoundWarning(w *model.SoundWarning) {
	s.Update(time.Now(), func(st *State) { st.SoundWarning = w })
}
