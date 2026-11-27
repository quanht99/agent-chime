// Package config loads and saves config.json with the same leniency as the macOS app.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

type Source string

const (
	System  Source = "system"
	Bundled Source = "bundled"
	File    Source = "file"
)

func (s Source) valid() bool {
	switch s {
	case System, Bundled, File:
		return true
	}
	return false
}

type SoundChoice struct {
	Source Source `json:"source"`
	Value  string `json:"value"`
}

// DisplayName is the base file name without its extension; it understands
// both / and \ so a Windows path displays correctly anywhere.
func (s SoundChoice) DisplayName() string {
	v := s.Value
	if i := strings.LastIndexAny(v, `/\`); i >= 0 {
		v = v[i+1:]
	}
	return strings.TrimSuffix(v, filepath.Ext(v))
}

func DefaultChoice(kind model.AlertKind) SoundChoice {
	if kind == model.Done {
		return SoundChoice{Bundled, "Chime.wav"}
	}
	return SoundChoice{Bundled, "Knock.wav"}
}

type Config struct {
	SoundDone            SoundChoice `json:"soundDone"`
	SoundAttention       SoundChoice `json:"soundAttention"`
	VolumeDone           float64     `json:"volumeDone"`
	VolumeAttention      float64     `json:"volumeAttention"`
	DoneThresholdSeconds float64     `json:"doneThresholdSeconds"`
	MutedUntil           *model.Time `json:"mutedUntil,omitempty"`
	AlertHeadless        bool        `json:"alertHeadless"`
	Language             i18n.Lang   `json:"language"`
	SettingsShownOnce    bool        `json:"settingsShownOnce"`
	CheckForUpdates      bool        `json:"checkForUpdates"`
}

func Default() Config {
	return Config{
		SoundDone:            DefaultChoice(model.Done),
		SoundAttention:       DefaultChoice(model.Attention),
		VolumeDone:           1,
		VolumeAttention:      1,
		DoneThresholdSeconds: 60,
		Language:             i18n.EN,
		CheckForUpdates:      true,
	}
}

func clamp01(v float64) float64 { return min(1, max(0, v)) }

// field decodes raw[key] into v; any problem leaves v untouched and reports false.
func field(raw map[string]json.RawMessage, key string, v any) bool {
	b, ok := raw[key]
	if !ok {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func sound(raw map[string]json.RawMessage, key string, kind model.AlertKind) SoundChoice {
	var s SoundChoice
	if field(raw, key, &s) && s.Source.valid() {
		return s
	}
	return DefaultChoice(kind)
}

// decode applies per-key leniency: a bad key falls back to its default and
// never affects the others. Only a non-object document is an error.
func decode(data []byte) (Config, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Default(), err
	}
	c := Default()
	c.SoundDone = sound(raw, "soundDone", model.Done)
	c.SoundAttention = sound(raw, "soundAttention", model.Attention)
	field(raw, "volumeDone", &c.VolumeDone)
	field(raw, "volumeAttention", &c.VolumeAttention)
	c.VolumeDone, c.VolumeAttention = clamp01(c.VolumeDone), clamp01(c.VolumeAttention)
	if field(raw, "doneThresholdSeconds", &c.DoneThresholdSeconds) {
		c.DoneThresholdSeconds = max(0, c.DoneThresholdSeconds)
	}
	var until model.Time
	if field(raw, "mutedUntil", &until) && !until.IsZero() {
		c.MutedUntil = &until
	}
	field(raw, "alertHeadless", &c.AlertHeadless)
	var lang i18n.Lang
	if field(raw, "language", &lang) && (lang == i18n.EN || lang == i18n.VI) {
		c.Language = lang
	}
	field(raw, "settingsShownOnce", &c.SettingsShownOnce)
	field(raw, "checkForUpdates", &c.CheckForUpdates)
	return c, nil
}

// Load never fails: a missing file gives defaults silently, anything else
// that goes wrong gives defaults plus a log line.
func Load(p paths.Paths) Config {
	data, err := os.ReadFile(p.Config)
	if errors.Is(err, fs.ErrNotExist) {
		return Default()
	}
	if err == nil {
		var c Config
		if c, err = decode(data); err == nil {
			return c
		}
	}
	applog.Write(p, "config.json is unreadable, using defaults: "+err.Error())
	return Default()
}

// Save writes sorted keys, 2-space indent and a trailing newline, atomically.
func Save(p paths.Paths, c Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var m map[string]any // marshalling a map sorts the keys
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
	if err := os.MkdirAll(filepath.Dir(p.Config), 0o755); err != nil {
		return err
	}
	tmp := p.Config + ".tmp"
	if err := fsx.WriteSync(tmp, buf.Bytes(), 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := fsx.Rename(tmp, p.Config); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (c Config) IsMuted(now time.Time) bool {
	return c.MutedUntil != nil && c.MutedUntil.After(now)
}

// Mute with nil mutes until Unmute.
func (c *Config) Mute(d *time.Duration, now time.Time) {
	t := model.Time{Time: model.Forever}
	if d != nil {
		t = model.Time{Time: now.Add(*d)}
	}
	c.MutedUntil = &t
}

func (c *Config) Unmute() { c.MutedUntil = nil }

func (c Config) Sound(k model.AlertKind) SoundChoice {
	if k == model.Done {
		return c.SoundDone
	}
	return c.SoundAttention
}

func (c Config) Volume(k model.AlertKind) float64 {
	if k == model.Done {
		return c.VolumeDone
	}
	return c.VolumeAttention
}

func (c *Config) SetSound(k model.AlertKind, s SoundChoice) {
	if k == model.Done {
		c.SoundDone = s
	} else {
		c.SoundAttention = s
	}
}
