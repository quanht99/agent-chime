package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/config"
)

// ---- upload ----

const maxNameRunes = 100

// sanitizeName keeps the base name only and replaces anything but letters,
// digits and "._ -" so the stored name is safe on every system.
func sanitizeName(raw string) string {
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	var sb strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == ' ' || r == '-' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	name := strings.TrimLeft(sb.String(), ".")
	if utf8.RuneCountInString(name) > maxNameRunes {
		ext := filepath.Ext(name)
		stem := []rune(strings.TrimSuffix(name, ext))
		keep := maxNameRunes - utf8.RuneCountInString(ext)
		if keep < 1 {
			return ""
		}
		name = string(stem[:min(keep, len(stem))]) + ext
	}
	return name
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	data, ok := readBody(w, r, maxUploadBody)
	if !ok {
		return
	}
	kind, ok := kindOf(r.URL.Query().Get("kind"))
	if !ok {
		errJSON(w, http.StatusBadRequest, "bad_kind")
		return
	}
	raw, err := url.PathUnescape(r.Header.Get("X-File-Name"))
	if err != nil {
		errJSON(w, http.StatusBadRequest, "bad_name")
		return
	}
	name := sanitizeName(raw)
	ext := strings.ToLower(filepath.Ext(name))
	if name == "" || ext == "" {
		errJSON(w, http.StatusBadRequest, "bad_name")
		return
	}
	allowed := false
	for _, a := range audio.AllowedExtensions(s.d.GOOS, s.d.Lib.Players) {
		allowed = allowed || a == ext
	}
	if !allowed {
		errJSON(w, http.StatusBadRequest, "bad_extension")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	root := s.d.P.CustomSoundsDir
	dir := filepath.Join(root, string(kind))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		errJSON(w, http.StatusInternalServerError, "save_failed")
		return
	}
	tmpDir, err := os.MkdirTemp(root, ".upload-")
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "save_failed")
		return
	}
	defer os.RemoveAll(tmpDir)
	tmp := filepath.Join(tmpDir, name)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		errJSON(w, http.StatusInternalServerError, "save_failed")
		return
	}
	if !audio.IsPlayable(tmp, s.d.GOOS, s.d.Lib.Players) {
		errJSON(w, http.StatusBadRequest, "unplayable")
		return
	}
	dest := filepath.Join(dir, name)
	if err := os.Rename(tmp, dest); err != nil {
		applog.Write(s.d.P, "settings: moving upload failed: "+err.Error())
		errJSON(w, http.StatusInternalServerError, "save_failed")
		return
	}
	cfg := config.Load(s.d.P)
	prev := cfg.Sound(kind)
	cfg.SetSound(kind, config.SoundChoice{Source: config.File, Value: dest})
	if !s.saveConfig(w, cfg) {
		// config.json still points at the old file, so keep it and drop the new one.
		if prev.Source != config.File || prev.Value != dest {
			_ = os.Remove(dest)
		}
		return
	}
	// Only now that config.json points at dest are the older files removed.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.Name() != name {
				_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
	s.respondState(w)
}
