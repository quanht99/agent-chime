package deploy

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func mkSelf(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "self")
	if err := os.WriteFile(f, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mapFS(m map[string]string) fstest.MapFS {
	f := fstest.MapFS{}
	for k, v := range m {
		f[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return f
}

func TestCopiesSoundsWhenMissing(t *testing.T) {
	p := testenv.Home(t)
	changed, err := Sync(p, mkSelf(t, "bin"), mapFS(map[string]string{"a.wav": "A", "b.wav": "B"}))
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(p.SoundsDir, "a.wav")) != "A" || read(t, filepath.Join(p.SoundsDir, "b.wav")) != "B" {
		t.Fatal("sounds not copied")
	}
	sort.Strings(changed)
	if len(changed) != 3 {
		t.Fatalf("changed = %v", changed)
	}
}

func TestSkipsIdentical(t *testing.T) {
	p := testenv.Home(t)
	self, snd := mkSelf(t, "bin"), mapFS(map[string]string{"a.wav": "A"})
	if _, err := Sync(p, self, snd); err != nil {
		t.Fatal(err)
	}
	changed, err := Sync(p, self, snd)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestReplacesChanged(t *testing.T) {
	p := testenv.Home(t)
	self := mkSelf(t, "bin")
	if _, err := Sync(p, self, mapFS(map[string]string{"a.wav": "A"})); err != nil {
		t.Fatal(err)
	}
	changed, err := Sync(p, self, mapFS(map[string]string{"a.wav": "A2"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != filepath.Join(p.SoundsDir, "a.wav") {
		t.Fatalf("changed = %v", changed)
	}
	if read(t, changed[0]) != "A2" {
		t.Fatal("not replaced")
	}
}

func TestRemovesStaleSounds(t *testing.T) {
	p := testenv.Home(t)
	self := mkSelf(t, "bin")
	if _, err := Sync(p, self, mapFS(map[string]string{"a.wav": "A", "old.wav": "O"})); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(p.SoundsDir, "keep")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "x"), []byte("x"), 0o644)
	changed, err := Sync(p, self, mapFS(map[string]string{"a.wav": "A"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || filepath.Base(changed[0]) != "old.wav" {
		t.Fatalf("changed = %v", changed)
	}
	if _, err := os.Stat(filepath.Join(sub, "x")); err != nil {
		t.Fatal("subdirectory content must be untouched")
	}
}

func TestDeployCreatesOnlyExpectedEntries(t *testing.T) {
	p := testenv.Home(t)
	self := mkSelf(t, "bin")
	if _, err := Sync(p, self, mapFS(map[string]string{"a.wav": "A"})); err != nil {
		t.Fatal(err)
	}
	es, err := os.ReadDir(p.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := []string{filepath.Base(p.Program), filepath.Base(p.SoundsDir)}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("data dir holds %v, want %v", got, want)
	}
}

func TestSelfCopy(t *testing.T) {
	p := testenv.Home(t)
	p.Program = filepath.Join(t.TempDir(), "nested", "agentchime")
	self := mkSelf(t, "binary-v1")
	changed, err := Sync(p, self, mapFS(nil))
	if err != nil || len(changed) != 1 || changed[0] != p.Program {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if read(t, p.Program) != "binary-v1" {
		t.Fatal("not copied")
	}
	// Same size, different bytes still copies.
	self2 := mkSelf(t, "binary-v2")
	if changed, _ = Sync(p, self2, mapFS(nil)); len(changed) != 1 {
		t.Fatalf("changed = %v", changed)
	}
	if read(t, p.Program) != "binary-v2" {
		t.Fatal("not replaced")
	}
	// Running from the installed copy is a no-op.
	if changed, err = Sync(p, p.Program, mapFS(nil)); err != nil || len(changed) != 0 {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestSelfCopyIsAtomic(t *testing.T) {
	p := testenv.Home(t)
	os.MkdirAll(filepath.Dir(p.Program), 0o755)
	os.WriteFile(p.Program, []byte("old"), 0o600)
	if _, err := Sync(p, mkSelf(t, "new bytes"), mapFS(nil)); err != nil {
		t.Fatal(err)
	}
	if read(t, p.Program) != "new bytes" {
		t.Fatal("not replaced")
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(p.Program)
		if fi.Mode().Perm() != 0o755 {
			t.Fatalf("mode = %v", fi.Mode().Perm())
		}
	}
	es, _ := os.ReadDir(filepath.Dir(p.Program))
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover %s", e.Name())
		}
	}
}

func TestOldCopyCleanup(t *testing.T) {
	p := testenv.Home(t)
	p.Program = filepath.Join(t.TempDir(), "AgentChime.exe")
	os.WriteFile(p.Program, []byte("old"), 0o755)
	stale := p.Program + ".old-7"
	os.WriteFile(stale, []byte("x"), 0o755)
	var pp paths.Paths = p
	changed, err := syncWithOS(pp, mkSelf(t, "new"), mapFS(nil), "windows")
	if err != nil || len(changed) != 1 {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if read(t, p.Program) != "new" {
		t.Fatal("not replaced")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale .old not removed at start")
	}
	// On macOS/Linux removing the renamed copy succeeds immediately; either way
	// no tmp files are left.
	es, _ := os.ReadDir(filepath.Dir(p.Program))
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover %s", e.Name())
		}
	}
}
