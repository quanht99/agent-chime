package sounds

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func values(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Choice.Value)
	}
	return out
}

func put(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSystemListFiltersByOS(t *testing.T) {
	p := testenv.Home(t)
	wav := testenv.WAV16([]int16{1, 2, 3}, 8000)
	put(t, filepath.Join(p.SystemSoundsDir, "a.wav"), wav)
	put(t, filepath.Join(p.SystemSoundsDir, "B.OGA"), []byte("x"))
	put(t, filepath.Join(p.SystemSoundsDir, "c.txt"), []byte("x"))
	put(t, filepath.Join(p.SystemSoundsDir, "sub", "d.wav"), wav)
	if err := os.Mkdir(filepath.Join(p.SystemSoundsDir, "dir.wav"), 0o755); err != nil {
		t.Fatal(err)
	}

	win := Library{P: p, GOOS: "windows"}
	if got := values(win.System()); !reflect.DeepEqual(got, []string{"a.wav"}) {
		t.Errorf("windows: %v", got)
	}
	lin := Library{P: p, GOOS: "linux", Players: audio.Players{PwPlay: "/usr/bin/pw-play"}}
	if got := values(lin.System()); !reflect.DeepEqual(got, []string{"a.wav", "B.OGA"}) {
		t.Errorf("linux: %v", got)
	}
	if lin.System()[0].Name != "a" {
		t.Errorf("name: %q", lin.System()[0].Name)
	}
	missing := Library{P: testenv.Home(t), GOOS: "linux"}
	if got := missing.System(); got == nil || len(got) != 0 {
		t.Errorf("missing dir: %#v", got)
	}
}

func TestBuiltinList(t *testing.T) {
	got := values(Library{}.Builtin())
	if !reflect.DeepEqual(got, []string{"Aurora.wav", "Bloom.wav", "Breeze.wav", "Chime.wav", "Crystal.wav", "Ding.wav", "Drift.wav", "Ember.wav", "Knock.wav", "Lumen.wav", "Pebble.wav", "Ripple.wav", "Rise.wav", "Tide.wav"}) {
		t.Errorf("%v", got)
	}
}

func TestResolvePlayableOK(t *testing.T) {
	p := testenv.Home(t)
	l := Library{P: p, GOOS: "linux"}
	f := filepath.Join(p.Home, "mine.wav")
	put(t, f, testenv.WAV16([]int16{1, 2, 3, 4}, 8000))
	path, w := l.ResolvePlayable(config.SoundChoice{Source: config.File, Value: f}, model.Done)
	if path != f || w != nil {
		t.Errorf("%q %v", path, w)
	}
}

func TestResolvePlayableFallsBack(t *testing.T) {
	p := testenv.Home(t)
	l := Library{P: p, GOOS: "linux"}
	missing := filepath.Join(p.Home, "missing.wav")
	path, w := l.ResolvePlayable(config.SoundChoice{Source: config.File, Value: missing}, model.Done)
	if path != filepath.Join(p.SoundsDir, "Chime.wav") {
		t.Errorf("path %q", path)
	}
	if w == nil || w.Kind != model.Done || w.Path != missing {
		t.Errorf("warning %+v", w)
	}
	path, _ = l.ResolvePlayable(config.SoundChoice{Source: config.File, Value: missing}, model.Attention)
	if path != filepath.Join(p.SoundsDir, "Knock.wav") {
		t.Errorf("attention path %q", path)
	}
}

func TestUnknownSourceIsInvalid(t *testing.T) {
	p := testenv.Home(t)
	l := Library{P: p, GOOS: "linux"}
	c := config.SoundChoice{Source: "bogus", Value: "Aurora.wav"}
	if l.ValidName(c) {
		t.Error("an unknown source must be invalid")
	}
	path, w := l.ResolvePlayable(c, model.Done)
	if path != filepath.Join(p.SoundsDir, "Chime.wav") || w == nil || w.Kind != model.Done {
		t.Errorf("got %q %v", path, w)
	}
}

func TestBuiltinGroupHasAll14Sorted(t *testing.T) {
	l := Library{P: testenv.Home(t), GOOS: "linux"}
	var names []string
	for _, g := range l.Groups(config.Default()) {
		if g.ID != "builtin" {
			continue
		}
		for _, it := range g.Items {
			names = append(names, it.Name)
		}
	}
	want := []string{"Aurora", "Bloom", "Breeze", "Chime", "Crystal", "Ding", "Drift", "Ember", "Knock", "Lumen", "Pebble", "Ripple", "Rise", "Tide"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v want %v", names, want)
	}
}

func TestTraversalValueRejected(t *testing.T) {
	p := testenv.Home(t)
	l := Library{P: p, GOOS: "linux"}
	// A real playable file reachable through the traversal.
	target := filepath.Join(filepath.Dir(p.SystemSoundsDir), "home", "example", "my.wav")
	put(t, target, testenv.WAV16([]int16{1, 2, 3, 4}, 8000))
	if err := os.MkdirAll(p.SystemSoundsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := []config.SoundChoice{
		{Source: config.System, Value: "../home/example/my.wav"},
		{Source: config.System, Value: "../../home/example/my.wav"},
		{Source: config.Bundled, Value: ".."},
		{Source: config.Bundled, Value: "../system-sounds/../home/example/my.wav"},
		{Source: "bogus", Value: "."},
	}
	for _, c := range bad {
		if l.ValidName(c) {
			t.Errorf("%+v should be invalid", c)
		}
		path, w := l.ResolvePlayable(c, model.Done)
		if path != filepath.Join(p.SoundsDir, "Chime.wav") || w == nil {
			t.Errorf("%+v: %q %v", c, path, w)
		}
	}
	if !l.ValidName(config.SoundChoice{Source: config.File, Value: "/any/path.wav"}) {
		t.Error("file choices are always valid")
	}
	if l.ValidName(config.SoundChoice{Source: config.Bundled, Value: "Unlisted.wav"}) {
		t.Error("unlisted bundled name accepted")
	}
	if !l.ValidName(config.SoundChoice{Source: config.Bundled, Value: "Chime.wav"}) {
		t.Error("Chime.wav should be valid")
	}
}

func TestGroups(t *testing.T) {
	p := testenv.Home(t)
	l := Library{P: p, GOOS: "linux"}
	c := config.Default()
	g := l.Groups(c)
	ids := []string{}
	for _, x := range g {
		ids = append(ids, x.ID)
		if x.Items == nil {
			t.Errorf("%s items nil", x.ID)
		}
	}
	if !reflect.DeepEqual(ids, []string{"system", "builtin", "file"}) {
		t.Errorf("%v", ids)
	}
	if len(g[2].Items) != 0 {
		t.Error("file group should be empty")
	}

	c.SoundDone = config.SoundChoice{Source: config.File, Value: "/home/example/a.wav"}
	c.SoundAttention = config.SoundChoice{Source: config.File, Value: "/home/example/b.wav"}
	if got := values(l.Groups(c)[2].Items); !reflect.DeepEqual(got, []string{"/home/example/a.wav", "/home/example/b.wav"}) {
		t.Errorf("%v", got)
	}
	c.SoundAttention = c.SoundDone
	if got := l.Groups(c)[2].Items; len(got) != 1 || got[0].Name != "a" {
		t.Errorf("dedupe: %+v", got)
	}
}
