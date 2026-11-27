package web

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func wavBytes() []byte { return testenv.WAV16([]int16{0, 1000, -1000, 0, 500, -500}, 8000) }

func TestStateShape(t *testing.T) {
	h := newHarness(t)
	r := h.req("GET", "/api/state", nil)
	if r.status != 200 {
		t.Fatalf("status %d", r.status)
	}
	m := r.json()
	for _, k := range []string{"version", "lang", "texts", "config", "groups", "hooks", "autostart", "muted", "status", "noPlayer", "noTray"} {
		if _, ok := m[k]; !ok {
			t.Errorf("state lacks %q", k)
		}
	}
	hk := m["hooks"].(map[string]any)
	if hk["claude"].(map[string]any)["kind"] != "notInstalled" || hk["codex"].(map[string]any)["trust"] != "notInstalled" {
		t.Errorf("hooks: %v", hk)
	}
	if m["noTray"] != false || m["noPlayer"] != false {
		t.Errorf("flags: %v %v", m["noTray"], m["noPlayer"])
	}
	if m["muted"].(map[string]any)["on"] != false {
		t.Errorf("muted: %v", m["muted"])
	}
	groups := m["groups"].([]any)
	if len(groups) != 3 {
		t.Errorf("groups: %d", len(groups))
	}
}

func TestStateKeys(t *testing.T) {
	h := newHarness(t)
	m := h.req("GET", "/api/state", nil).json()
	want := []string{"autostart", "config", "groups", "hooks", "lang", "muted", "noPlayer", "noTray", "status", "texts", "version"}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state keys %v, want %v", got, want)
	}
	var ids []string
	for _, g := range m["groups"].([]any) {
		ids = append(ids, g.(map[string]any)["id"].(string))
	}
	if !reflect.DeepEqual(ids, []string{"system", "builtin", "file"}) {
		t.Fatalf("group ids %v", ids)
	}
}

func TestNoTray(t *testing.T) {
	h := newHarness(t, func(h *harness, d *Deps) { d.TrayAvailable = func() bool { return false } })
	if m := h.req("GET", "/api/state", nil).json(); m["noTray"] != true {
		t.Fatal("noTray")
	}
}

func TestUnknownAPIRouteIs404(t *testing.T) {
	h := newHarness(t)
	if r := h.post("/api/nope", map[string]any{}); r.status != 404 {
		t.Fatalf("status %d", r.status)
	}
}

func TestStateTextsFollowLanguage(t *testing.T) {
	h := newHarness(t)
	if r := h.req("GET", "/api/state", nil).json(); r["texts"].(map[string]any)["menuQuit"] != "Quit" {
		t.Fatal("english default")
	}
	r := h.post("/api/config", map[string]any{"language": "vi"})
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	m := r.json()
	if m["lang"] != "vi" || m["texts"].(map[string]any)["menuQuit"] != "Thoát" {
		t.Fatalf("lang %v quit %v", m["lang"], m["texts"].(map[string]any)["menuQuit"])
	}
	if h.cfg().Language != "vi" {
		t.Fatal("not saved")
	}
	if r := h.post("/api/config", map[string]any{"language": "fr"}); r.status != 400 {
		t.Fatalf("bad language: %d", r.status)
	}
}

func TestConfigClampsAndSaves(t *testing.T) {
	h := newHarness(t)
	r := h.post("/api/config", map[string]any{"volumeDone": 5, "volumeAttention": -2, "doneThresholdSeconds": 99999, "alertHeadless": true})
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	c := h.cfg()
	if c.VolumeDone != 1 || c.VolumeAttention != 0 || c.DoneThresholdSeconds != 7200 || !c.AlertHeadless {
		t.Fatalf("%+v", c)
	}
	h.post("/api/config", map[string]any{"doneThresholdSeconds": -5})
	if h.cfg().DoneThresholdSeconds != 0 {
		t.Fatal("negative threshold")
	}
	if r := h.post("/api/config", "{not json"); r.status != 400 {
		t.Fatalf("bad json: %d", r.status)
	}
	r = h.post("/api/config", map[string]any{"soundDone": map[string]string{"source": "bundled", "value": "Ding.wav"}})
	if r.status != 200 || h.cfg().SoundDone.Value != "Ding.wav" {
		t.Fatalf("%d %s", r.status, r.body)
	}
	r = h.post("/api/config", map[string]any{"soundDone": map[string]string{"source": "bundled", "value": "Nope.wav"}})
	if r.status != 400 || r.json()["error"] != "unknown_sound" {
		t.Fatalf("%d %s", r.status, r.body)
	}
}

func TestConfigSetsCheckForUpdates(t *testing.T) {
	h := newHarness(t)
	h.post("/api/config", map[string]any{"volumeDone": 0.5})
	before := h.cfg()
	if !before.CheckForUpdates {
		t.Fatal("default should be on")
	}
	if r := h.post("/api/config", map[string]any{"checkForUpdates": false}); r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	got := h.cfg()
	if got.CheckForUpdates {
		t.Fatal("not saved")
	}
	want := before
	want.CheckForUpdates = false
	if got != want {
		t.Fatalf("other fields changed: %+v vs %+v", got, want)
	}
}

func TestUnknownSourceRejected(t *testing.T) {
	h := newHarness(t)
	h.post("/api/config", map[string]any{"volumeDone": 0.5})
	before := h.readConfigFile()
	c := map[string]string{"source": "bogus", "value": "Aurora.wav"}
	r := h.post("/api/config", map[string]any{"volumeDone": 0.1, "soundAttention": c})
	if r.status != 400 || r.json()["error"] != "unknown_sound" || h.readConfigFile() != before {
		t.Fatalf("%d %s, config changed", r.status, r.body)
	}
	if r := h.post("/api/preview", map[string]any{"kind": "done", "choice": c}); r.status != 400 {
		t.Fatalf("preview: %d", r.status)
	}
}

func TestFileChoiceAccepted(t *testing.T) {
	h := newHarness(t)
	inside := filepath.Join(h.p.CustomSoundsDir, "done", "mine.wav")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, wavBytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	r := h.post("/api/config", map[string]any{"soundDone": map[string]string{"source": "file", "value": inside}})
	if r.status != 200 || h.cfg().SoundDone.Value != inside {
		t.Fatalf("%d %s", r.status, r.body)
	}
}

func TestFileChoiceMustBeInCustomSounds(t *testing.T) {
	h := newHarness(t)
	outside := filepath.Join(h.p.DataDir, "outside.wav")
	if err := os.MkdirAll(h.p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, wavBytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(h.p.CustomSoundsDir, "done", "mine.wav")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, wavBytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := []string{
		outside,
		filepath.Join(h.p.CustomSoundsDir, "..", "outside.wav"),
		filepath.Join(h.p.CustomSoundsDir, "done", "missing.wav"),
		h.p.CustomSoundsDir,
		"relative.wav",
		"",
	}
	for _, v := range bad {
		r := h.post("/api/config", map[string]any{"soundDone": map[string]string{"source": "file", "value": v}})
		if r.status != 400 {
			t.Errorf("file %q: %d", v, r.status)
		}
		r = h.post("/api/preview", map[string]any{"kind": "done", "choice": map[string]string{"source": "file", "value": v}})
		if r.status != 400 {
			t.Errorf("preview file %q: %d", v, r.status)
		}
	}
	if _, err := os.Stat(h.p.Config); err == nil {
		t.Fatal("config written for rejected choices")
	}
	r := h.post("/api/config", map[string]any{"soundDone": map[string]string{"source": "file", "value": inside}})
	if r.status != 200 || h.cfg().SoundDone.Value != inside {
		t.Fatalf("%d %s", r.status, r.body)
	}
}

func TestTraversalValueRejected(t *testing.T) {
	h := newHarness(t)
	for _, src := range []string{"system", "bundled"} {
		c := map[string]string{"source": src, "value": "../../x.wav"}
		if r := h.post("/api/config", map[string]any{"soundDone": c}); r.status != 400 {
			t.Errorf("config %s: %d", src, r.status)
		}
		if r := h.post("/api/preview", map[string]any{"kind": "done", "choice": c}); r.status != 400 {
			t.Errorf("preview %s: %d", src, r.status)
		}
	}
	if len(h.calls) != 0 {
		t.Fatal("preview was called")
	}
	if _, err := os.Stat(h.p.Config); err == nil {
		t.Fatal("config written")
	}
}

func TestPreview(t *testing.T) {
	h := newHarness(t)
	h.post("/api/config", map[string]any{"volumeDone": 0.25, "volumeAttention": 0.75})
	sys := map[string]string{"source": "system", "value": "Glass.wav"}
	r := h.post("/api/preview", map[string]any{"kind": "done", "choice": sys})
	if r.status != 200 || len(h.calls) != 1 || h.calls[0].vol != 0.25 || h.calls[0].path != filepath.Join(h.p.SystemSoundsDir, "Glass.wav") {
		t.Fatalf("%d %+v", r.status, h.calls)
	}
	// A second preview stops the previous one
	r = h.post("/api/preview", map[string]any{"kind": "attention", "choice": map[string]string{"source": "bundled", "value": "Aurora.wav"}})
	if r.status != 200 || len(h.calls) != 2 || h.calls[1].vol != 0.75 || h.stops != 1 {
		t.Fatalf("%d calls=%d stops=%d", r.status, len(h.calls), h.stops)
	}
	if r := h.post("/api/preview/stop", nil); r.status != 200 || h.stops != 2 {
		t.Fatalf("stop: %d stops=%d", r.status, h.stops)
	}
	h.post("/api/preview/stop", nil)
	if h.stops != 2 {
		t.Fatal("stop called twice for one preview")
	}
	if r := h.post("/api/preview", map[string]any{"kind": "bogus", "choice": sys}); r.status != 400 {
		t.Fatalf("bad kind: %d", r.status)
	}
}

func TestPreviewNoPlayerAndFailure(t *testing.T) {
	h := newHarness(t)
	sys := map[string]any{"kind": "done", "choice": map[string]string{"source": "system", "value": "Glass.wav"}}
	h.pErr = audio.ErrNoPlayer
	r := h.post("/api/preview", sys)
	if r.status != 200 || r.json()["noPlayer"] != true {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if h.req("GET", "/api/state", nil).json()["noPlayer"] != true {
		t.Fatal("noPlayer not remembered")
	}
	h.pErr = errors.New("boom")
	r = h.post("/api/preview", sys)
	if r.status != 500 || r.json()["error"] != "preview_failed" {
		t.Fatalf("%d %s", r.status, r.body)
	}
	h.pErr = nil
	if h.post("/api/preview", sys).json()["noPlayer"] != false {
		t.Fatal("noPlayer should clear after a working preview")
	}
}

func TestHooksInstallUninstall(t *testing.T) {
	h := newHarness(t)
	r := h.post("/api/hooks/claude/install", nil)
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	hk := r.json()["hooks"].(map[string]any)["claude"].(map[string]any)
	if hk["kind"] != "installed" || hk["text"] != "✔ Hooks installed" {
		t.Fatalf("%v", hk)
	}
	if _, err := os.Stat(h.p.ClaudeSettings); err != nil {
		t.Fatal(err)
	}
	r = h.post("/api/hooks/codex/install", nil)
	cx := r.json()["hooks"].(map[string]any)["codex"].(map[string]any)
	if r.status != 200 || cx["kind"] != "installed" || cx["trust"] == "" || cx["needsRetrust"] != false {
		t.Fatalf("%d %v", r.status, cx)
	}
	// installing again replaces nothing, so no re-trust note
	// a stale handler replaced by Install sets needsRetrust
	stale := strings.ReplaceAll(readFile(t, h.p.CodexHooks), "/home/example/agentchime", "/home/other/agentchime")
	if err := os.WriteFile(h.p.CodexHooks, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	r = h.post("/api/hooks/codex/install", nil)
	if r.status != 200 || r.json()["hooks"].(map[string]any)["codex"].(map[string]any)["needsRetrust"] != true {
		t.Fatalf("needsRetrust: %d %s", r.status, r.body)
	}
	r = h.post("/api/hooks/codex/uninstall", nil)
	cx = r.json()["hooks"].(map[string]any)["codex"].(map[string]any)
	if cx["kind"] != "notInstalled" || cx["needsRetrust"] != false {
		t.Fatalf("%v", cx)
	}
	if r := h.post("/api/hooks/claude/uninstall", nil); r.status != 200 || r.json()["hooks"].(map[string]any)["claude"].(map[string]any)["kind"] != "notInstalled" {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if r := h.post("/api/hooks/other/install", nil); r.status != 404 {
		t.Fatalf("unknown agent: %d", r.status)
	}
	if r := h.post("/api/hooks/claude/frobnicate", nil); r.status != 404 {
		t.Fatalf("unknown action: %d", r.status)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHooksUnreadableAndNoProgram(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Dir(h.p.ClaudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.p.ClaudeSettings, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, act := range []string{"install", "uninstall"} {
		r := h.post("/api/hooks/claude/"+act, nil)
		want := "Can't read " + h.p.ClaudeSettings + " (invalid JSON). AgentChime won't modify this file."
		if r.status != 409 || r.json()["error"] != "unreadable" || r.json()["message"] != want {
			t.Fatalf("%s: %d %s", act, r.status, r.body)
		}
	}
	if readFile(t, h.p.ClaudeSettings) != "{not json" {
		t.Fatal("unreadable file was modified")
	}
	st := h.req("GET", "/api/state", nil).json()["hooks"].(map[string]any)["claude"].(map[string]any)
	if st["kind"] != "unreadable" {
		t.Fatalf("%v", st)
	}
	h.progOK = false
	r := h.post("/api/hooks/codex/install", nil)
	if r.status != 409 || r.json()["error"] != "no_program" {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if _, err := os.Stat(h.p.CodexHooks); err == nil {
		t.Fatal("hooks written without a program")
	}
}

func TestMuteUnmute(t *testing.T) {
	h := newHarness(t)
	r := h.post("/api/mute", map[string]any{"minutes": 30})
	m := r.json()["muted"].(map[string]any)
	if r.status != 200 || m["on"] != true || !strings.HasPrefix(m["text"].(string), "○ Muted until ") {
		t.Fatalf("%d %v", r.status, m)
	}
	if got := h.cfg().MutedUntil.Time; !got.Equal(h.now.Add(30 * time.Minute)) {
		t.Fatalf("until %v", got)
	}
	r = h.post("/api/mute", map[string]any{"minutes": nil})
	m = r.json()["muted"].(map[string]any)
	if m["on"] != true || m["text"] != "○ Muted" {
		t.Fatalf("%v", m)
	}
	m = h.post("/api/unmute", nil).json()["muted"].(map[string]any)
	if m["on"] != false || m["text"] != "● On" || h.cfg().MutedUntil != nil {
		t.Fatalf("%v", m)
	}
	for _, b := range []any{map[string]any{"minutes": 45}, map[string]any{}, map[string]any{"minutes": "x"}} {
		if r := h.post("/api/mute", b); r.status != 400 {
			t.Errorf("%v: %d", b, r.status)
		}
	}
}

func TestAutostart(t *testing.T) {
	h := newHarness(t)
	r := h.post("/api/autostart", map[string]any{"enabled": true})
	a := r.json()["autostart"].(map[string]any)
	if r.status != 200 || a["enabled"] != true || a["supported"] != true {
		t.Fatalf("%d %s", r.status, r.body)
	}
	h.auto.err = errors.New("denied")
	if r := h.post("/api/autostart", map[string]any{"enabled": false}); r.status != 500 || !strings.Contains(r.json()["message"].(string), "denied") {
		t.Fatalf("%d %s", r.status, r.body)
	}
	h.auto.supported = false
	if r := h.post("/api/autostart", map[string]any{"enabled": true}); r.status != 400 {
		t.Fatalf("unsupported: %d", r.status)
	}
}

func TestOpenLogAndQuit(t *testing.T) {
	h := newHarness(t)
	if r := h.post("/api/open-log", nil); r.status != 200 || len(h.opens) != 1 || h.opens[0] != h.p.Log {
		t.Fatalf("%d %v", r.status, h.opens)
	}
	if r := h.post("/api/quit", nil); r.status != 200 {
		t.Fatalf("%d", r.status)
	}
	select {
	case <-h.quit:
	case <-time.After(2 * time.Second):
		t.Fatal("Quit not called")
	}
}

func TestStatusLines(t *testing.T) {
	h := newHarness(t)
	s := h.req("GET", "/api/state", nil).json()["status"].(map[string]any)
	if s["lastAlert"] != "No alerts yet" || s["lastSkip"] != "" || s["soundWarning"] != "" {
		t.Fatalf("%v", s)
	}
}

// ---- upload ----

func (h *harness) upload(kind, name string, data []byte) resp {
	return h.req("POST", "/api/upload?kind="+kind, data, func(r *http.Request) {
		r.Header.Set("Content-Type", "application/octet-stream")
		r.Header.Set("X-File-Name", name)
	})
}

func TestUploadFlow(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.p.CustomSoundsDir, "done")

	r := h.upload("done", url.PathEscape("../My Sound!.wav"), wavBytes())
	want := filepath.Join(dir, "My Sound_.wav")
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("not stored: %v", err)
	}
	if got := h.cfg().SoundDone; got.Source != config.File || got.Value != want {
		t.Fatalf("choice %+v", got)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("dir perms: %v %v", fi, err)
	}

	r = h.upload("done", "Second.wav", wavBytes())
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if _, err := os.Stat(want); err == nil {
		t.Fatal("first upload not removed")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "Second.wav" {
		t.Fatalf("%v", entries)
	}
	if left, _ := os.ReadDir(h.p.CustomSoundsDir); len(left) != 1 {
		t.Fatalf("temp dir left behind: %v", left)
	}

	if r := h.upload("done", "notes.txt", []byte("hello")); r.status != 400 {
		t.Fatalf("text file: %d %s", r.status, r.body)
	}
	if r := h.upload("done", "fake.wav", []byte("this is not a wav file at all")); r.status != 400 || r.json()["error"] != "unplayable" {
		t.Fatalf("fake wav: %d %s", r.status, r.body)
	}
	if r := h.upload("done", "empty.wav", nil); r.status != 400 {
		t.Fatalf("empty: %d", r.status)
	}
	if r := h.upload("done", "%zz.wav", wavBytes()); r.status != 400 {
		t.Fatalf("bad escape: %d", r.status)
	}
	if r := h.upload("bogus", "a.wav", wavBytes()); r.status != 400 {
		t.Fatalf("bad kind: %d", r.status)
	}
	if got := h.cfg().SoundDone.Value; got != filepath.Join(dir, "Second.wav") {
		t.Fatalf("failed uploads changed the choice: %s", got)
	}

	r = h.upload("attention", "%C3%82m%20b%C3%A1o.wav", wavBytes())
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if _, err := os.Stat(filepath.Join(h.p.CustomSoundsDir, "attention", "Âm báo.wav")); err != nil {
		t.Fatalf("unicode name: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Second.wav")); err != nil {
		t.Fatal("uploading for attention removed the done file")
	}
}

func TestUploadSavesChoice(t *testing.T) {
	h := newHarness(t)
	r := h.upload("done", "mine.wav", wavBytes())
	want := filepath.Join(h.p.CustomSoundsDir, "done", "mine.wav")
	if r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if got := h.cfg().SoundDone; got.Source != config.File || got.Value != want {
		t.Fatalf("choice %+v", got)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		`../My Sound!.wav`:                "My Sound_.wav",
		`C:\x\y\..\evil.wav`:              "evil.wav",
		`...hidden.wav`:                   "hidden.wav",
		`a/b/c.wav`:                       "c.wav",
		`ok-name_1.2.wav`:                 "ok-name_1.2.wav",
		`<>:"|?*.wav`:                     "_______.wav",
		strings.Repeat("a", 150) + ".wav": strings.Repeat("a", 96) + ".wav",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestUploadSaveFailureKeepsTheOldFile(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.p.CustomSoundsDir, "done")
	if r := h.upload("done", "Old.wav", wavBytes()); r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	old := filepath.Join(dir, "Old.wav")
	if h.cfg().SoundDone.Value != old {
		t.Fatalf("choice %+v", h.cfg().SoundDone)
	}
	// A non-empty directory where config.json goes makes config.Save fail (the rename cannot
	// replace it). It works the same as root, where a read-only data dir would not.
	cfgBytes, err := os.ReadFile(h.p.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(h.p.Config); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.p.Config, "block"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := h.upload("done", "New.wav", wavBytes())
	if r.status != 500 || r.json()["error"] != "save_failed" {
		t.Fatalf("%d %s", r.status, r.body)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("old file removed before the config was saved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "New.wav")); err == nil {
		t.Fatal("new file left behind although config.json does not name it")
	}
	// Once the config can be saved again, the next upload replaces the old file as usual.
	if err := os.RemoveAll(h.p.Config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.p.Config, cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := h.upload("done", "New.wav", wavBytes()); r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "New.wav" {
		t.Fatalf("%v", entries)
	}
}
