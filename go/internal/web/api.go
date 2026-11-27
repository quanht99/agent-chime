package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/codextrust"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/hooks"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/version"
)

type obj = map[string]any

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, obj{"error": code})
}

func errMsg(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, obj{"error": code, "message": msg})
}

type handlerFunc func(http.ResponseWriter, *http.Request, []byte)

// serveAPI runs after the Host and token checks.
func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch path {
	case "/api/ping":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, obj{"app": "agentchime"})
		return
	case "/api/state":
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, s.buildState())
		return
	}

	var h handlerFunc
	switch {
	case path == "/api/config":
		h = s.handleConfig
	case path == "/api/preview":
		h = s.handlePreview
	case path == "/api/preview/stop":
		h = s.handlePreviewStop
	case strings.HasPrefix(path, "/api/hooks/"):
		h = s.handleHooks
	case path == "/api/mute":
		h = s.handleMute
	case path == "/api/unmute":
		h = s.handleUnmute
	case path == "/api/autostart":
		h = s.handleAutostart
	case path == "/api/upload":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		s.handleUpload(w, r)
		return
	case path == "/api/open-log":
		h = s.handleOpenLog
	case path == "/api/quit":
		h = s.handleQuit
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	// Even bodiless calls must be JSON: a cross-site form cannot send that content type.
	if !isJSONContentType(r) {
		errJSON(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	body, ok := readBody(w, r, maxJSONBody)
	if !ok {
		return
	}
	h(w, r, body)
}

// decode parses body into v; an empty body counts as {}.
func decode(w http.ResponseWriter, body []byte, v any) bool {
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}
	if err := json.Unmarshal(body, v); err != nil {
		errJSON(w, http.StatusBadRequest, "bad_json")
		return false
	}
	return true
}

// ---- state ----

func hookKind(k hooks.StatusKind) string {
	switch k {
	case hooks.Installed:
		return "installed"
	case hooks.Partial:
		return "partial"
	case hooks.Unreadable:
		return "unreadable"
	}
	return "notInstalled"
}

func trustKind(k codextrust.Kind) string {
	switch k {
	case codextrust.Untrusted:
		return "untrusted"
	case codextrust.Trusted:
		return "trusted"
	case codextrust.Disabled:
		return "disabled"
	}
	return "notInstalled"
}

// buildState assembles the /api/state body. Enumerations, all lowercase camel strings:
//
//	hooks.*.kind:      "notInstalled" | "installed" | "partial" | "unreadable"
//	hooks.codex.trust: "notInstalled" | "untrusted" | "trusted" | "disabled"
func (s *Server) buildState() obj {
	d := s.d
	cfg := config.Load(d.P)
	lang := cfg.Language
	now := d.Now()
	t := func(k i18n.Key, args ...string) string { return i18n.T(k, lang, args...) }

	texts := obj{}
	for _, k := range i18n.Keys() {
		texts[string(k)] = i18n.T(k, lang)
	}

	cst := hooks.StatusOf(d.P.ClaudeSettings, model.Claude, d.Hooks)
	xst := hooks.StatusOf(d.P.CodexHooks, model.Codex, d.Hooks)
	trust := trustKind(codextrust.Current(d.P).Kind)

	autoSupported, autoEnabled := false, false
	if d.Autostart != nil {
		autoSupported = d.Autostart.Supported()
		autoEnabled = autoSupported && d.Autostart.Enabled()
	}

	muted := cfg.IsMuted(now)
	mutedText := t("statusOn")
	var until any
	if muted {
		if cfg.MutedUntil.Equal(model.Forever) {
			mutedText = t("statusMuted")
		} else {
			mutedText = t("statusMutedUntil", model.HHMM(cfg.MutedUntil.Time))
			until = float64(cfg.MutedUntil.UnixMilli()) / 1000
		}
	}

	st := state.Store{P: d.P}.Load()
	lastAlert := t("menuNoAlert")
	if a := st.LastAlert; a != nil {
		lastAlert = t("statusLastAlert", a.Summary(lang), model.HHMM(a.Date.Time))
	}
	lastSkip := ""
	if sk := st.LastSkip; sk != nil {
		lastSkip = t("statusLastSkip", sk.Summary(lang), sk.Reason.Text(lang), model.HHMM(sk.Date.Time))
	}
	warning := ""
	if w := st.SoundWarning; w != nil {
		warning = t("soundWarning", w.Path)
	}

	return obj{
		"version": version.Version,
		"lang":    string(lang),
		"texts":   texts,
		"config":  cfg,
		"groups":  d.Lib.Groups(cfg),
		"hooks": obj{
			"claude": obj{"kind": hookKind(cst.Kind), "text": hookText(cst, t, "")},
			"codex": obj{
				"kind": hookKind(xst.Kind), "text": hookText(xst, t, trust), "trust": trust,
				"needsRetrust": s.needsRetrust.Load(), "warn": xst.Warn,
			},
		},
		"autostart": obj{"supported": autoSupported, "enabled": autoEnabled},
		"muted":     obj{"on": muted, "until": until, "text": mutedText},
		"status": obj{
			"statusLine": mutedText, "lastAlert": lastAlert, "lastSkip": lastSkip, "soundWarning": warning,
		},
		"noPlayer": s.noPlayer.Load(),
		"noTray":   d.TrayAvailable != nil && !d.TrayAvailable(),
	}
}

// hookText mirrors SettingsView.statusText; trust is "" for Claude.
func hookText(st hooks.Status, t func(i18n.Key, ...string) string, trust string) string {
	switch st.Kind {
	case hooks.Installed:
		switch trust {
		case "":
			return t("hooksInstalled")
		case "trusted":
			return t("codexTrusted")
		case "disabled":
			return t("codexDisabled")
		}
		return t("codexUntrusted")
	case hooks.Partial:
		return t("hooksPartial")
	case hooks.Unreadable:
		return t("unreadableConfig", st.Path)
	}
	return t("hooksNotInstalled")
}

func (s *Server) respondState(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, s.buildState())
}

// ---- config ----

func kindOf(v string) (model.AlertKind, bool) {
	switch v {
	case "done":
		return model.Done, true
	case "attention":
		return model.Attention, true
	}
	return "", false
}

// insideCustomSounds reports whether v is an existing file under CustomSoundsDir.
func (s *Server) insideCustomSounds(v string) bool {
	if v == "" {
		return false
	}
	rel, err := filepath.Rel(s.d.P.CustomSoundsDir, filepath.Clean(v))
	if err != nil || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return false
	}
	st, err := os.Stat(v)
	return err == nil && st.Mode().IsRegular()
}

// checkChoice validates c; on failure it has answered.
func (s *Server) checkChoice(w http.ResponseWriter, c config.SoundChoice) bool {
	switch c.Source {
	case config.System, config.Bundled:
		if !s.d.Lib.ValidName(c) {
			errJSON(w, http.StatusBadRequest, "unknown_sound")
			return false
		}
	case config.File:
		if !s.insideCustomSounds(c.Value) {
			errJSON(w, http.StatusBadRequest, "unknown_sound")
			return false
		}
	default:
		errJSON(w, http.StatusBadRequest, "unknown_sound")
		return false
	}
	return true
}

func clamp(v, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, v)) }

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request, body []byte) {
	var req struct {
		SoundDone            *config.SoundChoice `json:"soundDone"`
		SoundAttention       *config.SoundChoice `json:"soundAttention"`
		VolumeDone           *float64            `json:"volumeDone"`
		VolumeAttention      *float64            `json:"volumeAttention"`
		DoneThresholdSeconds *float64            `json:"doneThresholdSeconds"`
		AlertHeadless        *bool               `json:"alertHeadless"`
		Language             *i18n.Lang          `json:"language"`
	}
	if !decode(w, body, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range []*config.SoundChoice{req.SoundDone, req.SoundAttention} {
		if c != nil && !s.checkChoice(w, *c) {
			return
		}
	}
	if req.Language != nil && *req.Language != i18n.EN && *req.Language != i18n.VI {
		errJSON(w, http.StatusBadRequest, "bad_language")
		return
	}
	cfg := config.Load(s.d.P)
	if req.SoundDone != nil {
		cfg.SoundDone = *req.SoundDone
	}
	if req.SoundAttention != nil {
		cfg.SoundAttention = *req.SoundAttention
	}
	if req.VolumeDone != nil {
		cfg.VolumeDone = clamp(*req.VolumeDone, 0, 1)
	}
	if req.VolumeAttention != nil {
		cfg.VolumeAttention = clamp(*req.VolumeAttention, 0, 1)
	}
	if req.DoneThresholdSeconds != nil {
		cfg.DoneThresholdSeconds = clamp(*req.DoneThresholdSeconds, 0, 7200)
	}
	if req.AlertHeadless != nil {
		cfg.AlertHeadless = *req.AlertHeadless
	}
	if req.Language != nil {
		cfg.Language = *req.Language
	}
	if !s.saveConfig(w, cfg) {
		return
	}
	s.respondState(w)
}

func (s *Server) saveConfig(w http.ResponseWriter, cfg config.Config) bool {
	if err := config.Save(s.d.P, cfg); err != nil {
		applog.Write(s.d.P, "settings: saving config failed: "+err.Error())
		errJSON(w, http.StatusInternalServerError, "save_failed")
		return false
	}
	return true
}

// ---- preview ----

func (s *Server) handlePreview(w http.ResponseWriter, _ *http.Request, body []byte) {
	var req struct {
		Kind   string             `json:"kind"`
		Choice config.SoundChoice `json:"choice"`
	}
	if !decode(w, body, &req) {
		return
	}
	kind, ok := kindOf(req.Kind)
	if !ok {
		errJSON(w, http.StatusBadRequest, "bad_kind")
		return
	}
	if !s.checkChoice(w, req.Choice) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopPreviewLocked()
	if s.d.Preview == nil {
		errJSON(w, http.StatusInternalServerError, "preview_failed")
		return
	}
	stop, err := s.d.Preview(s.d.Lib.Path(req.Choice), config.Load(s.d.P).Volume(kind))
	switch {
	case errors.Is(err, audio.ErrNoPlayer):
		s.noPlayer.Store(true)
	case err != nil:
		applog.Write(s.d.P, "settings: preview failed: "+err.Error())
		errJSON(w, http.StatusInternalServerError, "preview_failed")
		return
	default:
		s.noPlayer.Store(false)
		s.stopPrev = stop
	}
	s.respondState(w)
}

func (s *Server) handlePreviewStop(w http.ResponseWriter, _ *http.Request, _ []byte) {
	s.mu.Lock()
	s.stopPreviewLocked()
	s.mu.Unlock()
	s.respondState(w)
}

// ---- hooks ----

func (s *Server) handleHooks(w http.ResponseWriter, r *http.Request, _ []byte) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/hooks/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	var agent model.Agent
	var file string
	switch parts[0] {
	case "claude":
		agent, file = model.Claude, s.d.P.ClaudeSettings
	case "codex":
		agent, file = model.Codex, s.d.P.CodexHooks
	default:
		http.NotFound(w, r)
		return
	}
	lang := config.Load(s.d.P).Language
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	switch parts[1] {
	case "install":
		if s.d.ProgramExists != nil && !s.d.ProgramExists() {
			errMsg(w, http.StatusConflict, "no_program", i18n.T("helperMissing", lang))
			return
		}
		var replaced bool
		replaced, err = hooks.Install(file, agent, s.d.Hooks, s.d.P.BackupsDir, s.d.Now())
		if err == nil && agent == model.Codex {
			s.needsRetrust.Store(replaced)
		}
	case "uninstall":
		err = hooks.Uninstall(file, agent, s.d.P.BackupsDir, s.d.Now())
		if err == nil && agent == model.Codex {
			s.needsRetrust.Store(false)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		var ue *hooks.UnreadableError
		if errors.As(err, &ue) {
			errMsg(w, http.StatusConflict, "unreadable", i18n.T("unreadableConfig", lang, ue.Path))
			return
		}
		applog.Write(s.d.P, "settings: hooks "+parts[1]+" failed: "+err.Error())
		errMsg(w, http.StatusInternalServerError, "hooks_failed", err.Error())
		return
	}
	s.respondState(w)
}

// ---- mute ----

func (s *Server) handleMute(w http.ResponseWriter, _ *http.Request, body []byte) {
	var raw map[string]json.RawMessage
	if !decode(w, body, &raw) {
		return
	}
	minutes, present := raw["minutes"]
	if !present {
		errJSON(w, http.StatusBadRequest, "bad_minutes")
		return
	}
	var dur *time.Duration
	if string(bytes.TrimSpace(minutes)) != "null" {
		var m int
		if json.Unmarshal(minutes, &m) != nil || (m != 30 && m != 60) {
			errJSON(w, http.StatusBadRequest, "bad_minutes")
			return
		}
		d := time.Duration(m) * time.Minute
		dur = &d
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := config.Load(s.d.P)
	cfg.Mute(dur, s.d.Now())
	if s.saveConfig(w, cfg) {
		s.respondState(w)
	}
}

func (s *Server) handleUnmute(w http.ResponseWriter, _ *http.Request, _ []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := config.Load(s.d.P)
	cfg.Unmute()
	if s.saveConfig(w, cfg) {
		s.respondState(w)
	}
}

// ---- autostart ----

func (s *Server) handleAutostart(w http.ResponseWriter, _ *http.Request, body []byte) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, body, &req) {
		return
	}
	a := s.d.Autostart
	if a == nil || !a.Supported() {
		errJSON(w, http.StatusBadRequest, "unsupported")
		return
	}
	if req.Enabled == nil {
		errJSON(w, http.StatusBadRequest, "bad_json")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if *req.Enabled {
		err = a.Enable()
	} else {
		err = a.Disable()
	}
	if err != nil {
		lang := config.Load(s.d.P).Language
		applog.Write(s.d.P, "settings: autostart failed: "+err.Error())
		errMsg(w, http.StatusInternalServerError, "autostart_failed", i18n.T("loginItemError", lang, err.Error()))
		return
	}
	s.respondState(w)
}

// ---- misc ----

func (s *Server) handleOpenLog(w http.ResponseWriter, _ *http.Request, _ []byte) {
	p := s.d.P
	if _, err := os.Stat(p.Log); err != nil {
		// Nothing logged yet: create an empty file so the viewer has something to open.
		if os.MkdirAll(filepath.Dir(p.Log), 0o755) == nil {
			if f, err := os.OpenFile(p.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				f.Close()
			}
		}
	}
	if s.d.OpenURL == nil {
		errJSON(w, http.StatusInternalServerError, "open_failed")
		return
	}
	if err := s.d.OpenURL(p.Log); err != nil {
		applog.Write(p, "settings: open log failed: "+err.Error())
		errJSON(w, http.StatusInternalServerError, "open_failed")
		return
	}
	s.respondState(w)
}

func (s *Server) handleQuit(w http.ResponseWriter, _ *http.Request, _ []byte) {
	s.respondState(w)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	if s.d.Quit != nil {
		go s.d.Quit()
	}
}
