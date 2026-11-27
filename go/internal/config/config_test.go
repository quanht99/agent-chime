package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

var t0 = time.UnixMilli(1_800_000_000_000)

func write(t *testing.T, p paths.Paths, s string) {
	t.Helper()
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Config, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaults(t *testing.T) {
	c := Default()
	if c.SoundDone != (SoundChoice{Bundled, "Chime.wav"}) || c.SoundAttention != (SoundChoice{Bundled, "Knock.wav"}) {
		t.Errorf("sounds %+v %+v", c.SoundDone, c.SoundAttention)
	}
	if c.VolumeDone != 1 || c.VolumeAttention != 1 || c.DoneThresholdSeconds != 60 || c.MutedUntil != nil ||
		c.AlertHeadless || c.Language != i18n.EN || c.SettingsShownOnce || !c.CheckForUpdates {
		t.Errorf("defaults %+v", c)
	}
}

func TestEmptyJSONGivesDefaults(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, "{}")
	if got := Load(p); got != Default() {
		t.Errorf("got %+v", got)
	}
}

func TestBadFieldKeepsOthers(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"language":"fr","doneThresholdSeconds":30,"volumeDone":"x","volumeAttention":7}`)
	c := Load(p)
	if c.Language != i18n.EN || c.DoneThresholdSeconds != 30 || c.VolumeDone != 1 || c.VolumeAttention != 1 {
		t.Errorf("got %+v", c)
	}
}

func TestClamps(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"volumeDone":-3,"volumeAttention":2,"doneThresholdSeconds":-10}`)
	c := Load(p)
	if c.VolumeDone != 0 || c.VolumeAttention != 1 || c.DoneThresholdSeconds != 0 {
		t.Errorf("got %+v", c)
	}
}

func TestUnknownSourceFallsBackToDefault(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"soundDone":{"source":"weird","value":"x"},"soundAttention":{"source":"weird","value":"y"}}`)
	c := Load(p)
	if c.SoundDone != (SoundChoice{Bundled, "Chime.wav"}) || c.SoundAttention != (SoundChoice{Bundled, "Knock.wav"}) {
		t.Errorf("got %+v %+v", c.SoundDone, c.SoundAttention)
	}
}

func TestMissingFileGivesDefaultsSilently(t *testing.T) {
	p := testenv.Home(t)
	if Load(p) != Default() {
		t.Error("not defaults")
	}
	if _, err := os.Stat(p.Log); err == nil {
		t.Error("missing config must not log")
	}
}

func TestCorruptFileGivesDefaultsAndLogs(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, "not json")
	if Load(p) != Default() {
		t.Error("not defaults")
	}
	b, _ := os.ReadFile(p.Log)
	if !strings.Contains(string(b), "config.json is unreadable, using defaults: ") {
		t.Errorf("log = %q", b)
	}
}

func TestUnreadableFileGivesDefaultsAndLogs(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(p.Config, 0o755); err != nil { // a directory, so reading fails
		t.Fatal(err)
	}
	if Load(p) != Default() {
		t.Error("not defaults")
	}
	b, _ := os.ReadFile(p.Log)
	if !strings.Contains(string(b), "config.json is unreadable, using defaults: ") {
		t.Errorf("log = %q", b)
	}
}

func TestBOMTolerated(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, "\xef\xbb\xbf"+`{"alertHeadless":true}`)
	if !Load(p).AlertHeadless {
		t.Error("BOM file not read")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := testenv.Home(t)
	c := Default()
	c.SoundDone = SoundChoice{File, "/Users/example/x.mp3"}
	c.SoundAttention = SoundChoice{Bundled, "Knock.wav"}
	c.VolumeDone, c.VolumeAttention = 0.4, 0.7
	c.DoneThresholdSeconds = 120
	c.MutedUntil = &model.Time{Time: t0}
	c.AlertHeadless, c.SettingsShownOnce = true, true
	c.Language = i18n.VI
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	got := Load(p)
	if got.MutedUntil == nil || !got.MutedUntil.Equal(t0) {
		t.Fatalf("mutedUntil %v", got.MutedUntil)
	}
	got.MutedUntil, c.MutedUntil = nil, nil
	if got != c {
		t.Errorf("got %+v want %+v", got, c)
	}
}

func TestUnknownSourceKeepsOtherKeys(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"soundDone":{"source":"bogus","value":"Aurora.wav"},"volumeDone":0.5}`)
	c := Load(p)
	if c.SoundDone != DefaultChoice(model.Done) {
		t.Errorf("got %+v", c.SoundDone)
	}
	if c.VolumeDone != 0.5 {
		t.Errorf("other keys must survive, volume %v", c.VolumeDone)
	}
}

func TestSaveFormat(t *testing.T) {
	p := testenv.Home(t)
	if err := Save(p, Default()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p.Config)
	want := `{
  "alertHeadless": false,
  "checkForUpdates": true,
  "doneThresholdSeconds": 60,
  "language": "en",
  "settingsShownOnce": false,
  "soundAttention": {
    "source": "bundled",
    "value": "Knock.wav"
  },
  "soundDone": {
    "source": "bundled",
    "value": "Chime.wav"
  },
  "volumeAttention": 1,
  "volumeDone": 1
}
`
	if string(b) != want {
		t.Errorf("got:\n%s", b)
	}
	entries, _ := os.ReadDir(p.DataDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
	// mutedUntil is sorted between language and settingsShownOnce
	c := Default()
	c.MutedUntil = &model.Time{Time: model.Forever}
	_ = Save(p, c)
	b, _ = os.ReadFile(p.Config)
	if !strings.Contains(string(b), `"language": "en",`+"\n"+`  "mutedUntil": 64092211200,`+"\n"+`  "settingsShownOnce"`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestMuteForever(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"mutedUntil":64092211200}`)
	c := Load(p)
	if c.MutedUntil == nil || !c.MutedUntil.Equal(model.Forever) || !c.IsMuted(t0) {
		t.Errorf("got %+v", c.MutedUntil)
	}
}

func TestMuteAndUnmute(t *testing.T) {
	c := Default()
	d := 1800 * time.Second
	c.Mute(&d, t0)
	if !c.IsMuted(t0.Add(1799*time.Second)) || c.IsMuted(t0.Add(1800*time.Second)) {
		t.Error("timed mute")
	}
	c.Mute(nil, t0)
	if !c.MutedUntil.Equal(model.Forever) {
		t.Error("forever")
	}
	c.Unmute()
	if c.MutedUntil != nil || c.IsMuted(t0) {
		t.Error("unmute")
	}
}

func TestSoundChoiceHelpers(t *testing.T) {
	cases := map[SoundChoice]string{
		{Bundled, "Soft Chime.wav"}:      "Soft Chime",
		{File, "/a/b/x.mp3"}:             "x",
		{File, `C:\Users\example\y.wav`}: "y",
		{System, "Glass"}:                "Glass",
	}
	for in, want := range cases {
		if got := in.DisplayName(); got != want {
			t.Errorf("%+v: %q want %q", in, got, want)
		}
	}
}

func TestAccessors(t *testing.T) {
	c := Default()
	c.VolumeAttention = 0.5
	c.SetSound(model.Attention, SoundChoice{System, "Ding"})
	if c.Sound(model.Attention).Value != "Ding" || c.Sound(model.Done).Value != "Chime.wav" ||
		c.Volume(model.Attention) != 0.5 || c.Volume(model.Done) != 1 {
		t.Errorf("%+v", c)
	}
	c.SetSound(model.Done, SoundChoice{System, "Pop"})
	if c.SoundDone.Value != "Pop" {
		t.Error("SetSound done")
	}
}

func TestNullMutedUntilIgnored(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"mutedUntil":null}`)
	if Load(p).MutedUntil != nil {
		t.Error("null should leave mutedUntil nil")
	}
}

func TestCheckForUpdatesDefaultsOn(t *testing.T) {
	p := testenv.Home(t)
	write(t, p, `{"language":"en"}`)
	if !Load(p).CheckForUpdates {
		t.Error("missing field should default to true")
	}
}

func TestCheckForUpdatesOffSurvivesReload(t *testing.T) {
	p := testenv.Home(t)
	c := Default()
	c.CheckForUpdates = false
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	if Load(p).CheckForUpdates {
		t.Error("false was not kept")
	}
}
