package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

var linux = Format{GOOS: "linux", Program: "/home/example/.local/share/agentchime/agentchime"}

var t0 = time.Date(2026, 11, 27, 10, 0, 0, 123_000_000, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	d, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil || len(d) == 0 {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return d
}

func seed(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	d, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func parsed(t *testing.T, path string) jsonv.Value {
	t.Helper()
	v, err := ReadJSONFile(path)
	if err != nil || v == nil {
		t.Fatalf("ReadJSONFile(%s): %v", path, err)
	}
	return *v
}

func keys(v jsonv.Value) []string {
	var ks []string
	for _, m := range v.O {
		ks = append(ks, m.Key)
	}
	return ks
}

func eq(t *testing.T, got, want any, msg string) {
	t.Helper()
	if g, w := toS(got), toS(want); g != w {
		t.Errorf("%s: got %s, want %s", msg, g, w)
	}
}

func toS(v any) string {
	switch x := v.(type) {
	case jsonv.Value:
		return x.Serialize()
	case []jsonv.Value:
		return jsonv.Arr(x...).Serialize()
	}
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(fmtAny(v), "\n", " ")), " "))
}

func fmtAny(v any) string { return strings.TrimSpace(sprint(v)) }

func installClaude(t *testing.T, p paths.Paths) bool {
	t.Helper()
	r, err := Install(p.ClaudeSettings, model.Claude, linux, p.BackupsDir, t0)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func countOurs(hooks jsonv.Value) int {
	n := 0
	for _, m := range hooks.O {
		for _, g := range m.Val.A {
			if IsOurGroup(g) {
				n++
			}
		}
	}
	return n
}

func TestClaudeInstallAppendsSixGroupsAndKeepsEverythingElse(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "claude-settings.json")
	seed(t, p.ClaudeSettings, orig)
	before, _ := jsonv.Parse(orig)
	if installClaude(t, p) {
		t.Error("replacedExisting should be false")
	}
	after := parsed(t, p.ClaudeSettings)
	eq(t, keys(after), keys(before), "top-level keys")
	bh, _ := before.Get("hooks")
	ah, _ := after.Get("hooks")
	for _, m := range bh.O {
		old := m.Val.A
		cur, _ := ah.Get(m.Key)
		if len(cur.A) < len(old) {
			t.Fatalf("%s shrank", m.Key)
		}
		eq(t, cur.A[:len(old)], old, m.Key)
		for _, g := range cur.A[len(old):] {
			if !IsOurGroup(g) {
				t.Errorf("%s: appended group not ours", m.Key)
			}
		}
	}
	if k := keys(ah); k[len(k)-1] != "Notification" {
		t.Errorf("last event = %v", k)
	}
	if _, ok := bh.Get("Notification"); ok {
		t.Error("fixture already has Notification")
	}
	if n := countOurs(ah); n != 6 {
		t.Errorf("ours = %d", n)
	}
	for _, m := range before.O {
		if m.Key != "hooks" {
			a, _ := after.Get(m.Key)
			eq(t, a, m.Val, m.Key)
		}
	}
}

func TestClaudeGroupsAreAsyncWithExpectedMatchers(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, fixture(t, "claude-settings.json"))
	installClaude(t, p)
	hooks, _ := parsed(t, p.ClaudeSettings).Get("hooks")
	for _, s := range ClaudeSpecs {
		cur, _ := hooks.Get(s.Event)
		var g jsonv.Value
		for _, c := range cur.A {
			if IsOurGroup(c) {
				g = c
			}
		}
		m, has := g.Get("matcher")
		if has != (s.Matcher != "") || m.S != s.Matcher {
			t.Errorf("%s matcher = %q", s.Event, m.S)
		}
		hs, _ := g.Get("hooks")
		h := hs.A[0]
		if a, _ := h.Get("async"); a.Kind != jsonv.KindBool || !a.B {
			t.Errorf("%s not async", s.Event)
		}
		if ty, _ := h.Get("type"); ty.S != "command" {
			t.Errorf("%s type", s.Event)
		}
		c, _ := h.Get("command")
		want := `"/home/example/.local/share/agentchime/agentchime" hook ` + string(s.Command)
		if c.S != want {
			t.Errorf("%s command = %q", s.Event, c.S)
		}
	}
}

func TestClaudeInstallIsIdempotent(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, fixture(t, "claude-settings.json"))
	installClaude(t, p)
	first := readBytes(t, p.ClaudeSettings)
	if installClaude(t, p) {
		t.Error("second install reported a replacement")
	}
	if !bytes.Equal(readBytes(t, p.ClaudeSettings), first) {
		t.Error("second install changed the file")
	}
}

func TestClaudeReinstallReplacesOldCommandInPlace(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, fixture(t, "claude-settings.json"))
	installClaude(t, p)
	root := parsed(t, p.ClaudeSettings)
	hooks, _ := root.Get("hooks")
	stopV, _ := hooks.Get("Stop")
	stop := append([]jsonv.Value{}, stopV.A...)
	idx := -1
	for i, g := range stop {
		if IsOurGroup(g) {
			idx = i
		}
	}
	old, _ := Format{GOOS: "linux", Program: "/old/agentchime"}.Group(model.Claude, ClaudeSpecs[1])
	stop[idx] = old
	stranger := jsonv.Obj(jsonv.Member{Key: "hooks", Val: jsonv.Arr(jsonv.Obj(
		jsonv.Member{Key: "type", Val: jsonv.Str("command")},
		jsonv.Member{Key: "command", Val: jsonv.Str("/home/example/bin/stranger")}))})
	stop = append(stop, stranger)
	hooks.Set("Stop", jsonv.Arr(stop...))
	root.Set("hooks", hooks)
	seed(t, p.ClaudeSettings, []byte(root.Serialize()+"\n"))

	if !installClaude(t, p) {
		t.Error("expected replacedExisting")
	}
	h2, _ := parsed(t, p.ClaudeSettings).Get("hooks")
	ns, _ := h2.Get("Stop")
	if len(ns.A) != len(stop) {
		t.Fatalf("count %d != %d", len(ns.A), len(stop))
	}
	want, _ := linux.Group(model.Claude, ClaudeSpecs[1])
	eq(t, ns.A[idx], want, "replaced group")
	eq(t, ns.A[len(ns.A)-1], stranger, "foreign group")
}

func TestClaudeInstallThenUninstallIsByteIdentical(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "claude-settings.json")
	seed(t, p.ClaudeSettings, orig)
	installClaude(t, p)
	if bytes.Equal(readBytes(t, p.ClaudeSettings), orig) {
		t.Fatal("install changed nothing")
	}
	if err := Uninstall(p.ClaudeSettings, model.Claude, p.BackupsDir, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readBytes(t, p.ClaudeSettings), orig) {
		t.Error("not byte-identical after uninstall")
	}
}

func TestClaudeInstallIntoMissingFile(t *testing.T) {
	p := testenv.Home(t)
	installClaude(t, p)
	root := parsed(t, p.ClaudeSettings)
	eq(t, keys(root), []string{"hooks"}, "keys")
	if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != Installed || st.Installed != 6 {
		t.Errorf("status %+v", st)
	}
}

func TestInvalidJSONIsNeverOverwritten(t *testing.T) {
	p := testenv.Home(t)
	broken := []byte("{ broken")
	seed(t, p.ClaudeSettings, broken)
	_, err := Install(p.ClaudeSettings, model.Claude, linux, p.BackupsDir, t0)
	var ue *UnreadableError
	if !errors.As(err, &ue) || ue.Path != p.ClaudeSettings {
		t.Errorf("install err = %v", err)
	}
	if err := Uninstall(p.ClaudeSettings, model.Claude, p.BackupsDir, t0); !errors.As(err, &ue) {
		t.Errorf("uninstall err = %v", err)
	}
	if !bytes.Equal(readBytes(t, p.ClaudeSettings), broken) {
		t.Error("file modified")
	}
	if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != Unreadable || st.Path != p.ClaudeSettings {
		t.Errorf("status %+v", st)
	}
}

func TestFilePermissionsPreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permissions")
	}
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, fixture(t, "claude-settings.json"))
	if err := os.Chmod(p.ClaudeSettings, 0o600); err != nil {
		t.Fatal(err)
	}
	installClaude(t, p)
	fi, _ := os.Stat(p.ClaudeSettings)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v", fi.Mode().Perm())
	}
}

func TestMissingFileGetsDefaultPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permissions")
	}
	p := testenv.Home(t)
	installClaude(t, p)
	fi, _ := os.Stat(p.ClaudeSettings)
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("perm = %v", fi.Mode().Perm())
	}
}

func TestSymlinkStaysSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges")
	}
	p := testenv.Home(t)
	target := filepath.Join(p.Home, "dotfiles", "settings.json")
	seed(t, target, fixture(t, "claude-settings.json"))
	if err := os.MkdirAll(filepath.Dir(p.ClaudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p.ClaudeSettings); err != nil {
		t.Fatal(err)
	}
	installClaude(t, p)
	if _, err := os.Readlink(p.ClaudeSettings); err != nil {
		t.Errorf("no longer a symlink: %v", err)
	}
	if st := StatusOf(target, model.Claude, linux); st.Kind != Installed {
		t.Errorf("target status %+v", st)
	}
}

func listBackups(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var n []string
	for _, e := range es {
		n = append(n, e.Name())
	}
	return n
}

func TestBackupWritten(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "claude-settings.json")
	seed(t, p.ClaudeSettings, orig)
	installClaude(t, p)
	files := listBackups(t, p.BackupsDir)
	if len(files) != 1 || files[0] != "settings.json.20261127-100000-123" {
		t.Fatalf("backups = %v", files)
	}
	if !bytes.Equal(readBytes(t, filepath.Join(p.BackupsDir, files[0])), orig) {
		t.Error("backup differs from original")
	}
}

func TestBackupNameAndPrune(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, []byte("{\"a\": 0}\n"))
	for i := 0; i < 11; i++ {
		v := jsonv.Obj(jsonv.Member{Key: "a", Val: jsonv.Value{Kind: jsonv.KindNumber, Num: string(rune('1' + i%9))}},
			jsonv.Member{Key: "i", Val: jsonv.Value{Kind: jsonv.KindNumber, Num: itoa(i)}})
		if err := WriteJSONFile(v, p.ClaudeSettings, p.BackupsDir, t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	files := listBackups(t, p.BackupsDir)
	if len(files) != 10 {
		t.Fatalf("%d backups: %v", len(files), files)
	}
	for _, f := range files {
		if f == "settings.json.20261127-100000-123" {
			t.Error("oldest backup was not pruned")
		}
	}
}

func TestBackupsPrunedToTenWithPreexisting(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(p.BackupsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		name := "settings.json.20200101-0000" + pad2(i)
		if err := os.WriteFile(filepath.Join(p.BackupsDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seed(t, p.ClaudeSettings, fixture(t, "claude-settings.json"))
	installClaude(t, p)
	files := listBackups(t, p.BackupsDir)
	if len(files) != 10 {
		t.Fatalf("%d backups", len(files))
	}
	for _, f := range files {
		if f == "settings.json.20200101-000000" {
			t.Error("oldest kept")
		}
	}
}

func TestBackupsInSameInstantDoNotOverwriteEachOther(t *testing.T) {
	p := testenv.Home(t)
	orig := []byte("{\n  \"a\": 1\n}\n")
	seed(t, p.ClaudeSettings, orig)
	for _, n := range []string{"2", "3", "4"} {
		v, _ := jsonv.Parse([]byte(`{"a": ` + n + `}`))
		if err := WriteJSONFile(v, p.ClaudeSettings, p.BackupsDir, t0); err != nil {
			t.Fatal(err)
		}
	}
	files := listBackups(t, p.BackupsDir)
	if len(files) != 3 {
		t.Fatalf("backups = %v", files)
	}
	found := false
	for _, f := range files {
		if bytes.Equal(readBytes(t, filepath.Join(p.BackupsDir, f)), orig) {
			found = true
		}
	}
	if !found {
		t.Error("pristine backup did not survive")
	}
	if files[0] != "settings.json.20261127-100000-123" || files[1] != "settings.json.20261127-100000-123-2" {
		t.Errorf("names = %v", files)
	}
}

func TestWriteLeavesNoTmpFileAndSkipsUnchanged(t *testing.T) {
	p := testenv.Home(t)
	v, _ := jsonv.Parse([]byte(`{"a": 1}`))
	if err := WriteJSONFile(v, p.ClaudeSettings, p.BackupsDir, t0); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSONFile(v, p.ClaudeSettings, p.BackupsDir, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.BackupsDir); err == nil {
		t.Error("unchanged write made a backup")
	}
	for _, n := range listBackups(t, filepath.Dir(p.ClaudeSettings)) {
		if strings.HasSuffix(n, ".tmp") {
			t.Errorf("tmp left: %s", n)
		}
	}
}

func TestInstallIntoBOMFile(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "claude-settings.json")
	seed(t, p.ClaudeSettings, append([]byte{0xEF, 0xBB, 0xBF}, orig...))
	before, _ := jsonv.Parse(orig)
	installClaude(t, p)
	out := readBytes(t, p.ClaudeSettings)
	if bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("output has a BOM")
	}
	after := parsed(t, p.ClaudeSettings)
	bh, _ := before.Get("hooks")
	ah, _ := after.Get("hooks")
	for _, m := range bh.O {
		cur, _ := ah.Get(m.Key)
		eq(t, cur.A[:len(m.Val.A)], m.Val.A, m.Key)
	}
	if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != Installed {
		t.Errorf("status %+v", st)
	}
}

func TestClaudeStatusTransitions(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, fixture(t, "claude-settings.json"))
	if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != NotInstalled || st.Total != 6 {
		t.Errorf("before: %+v", st)
	}
	installClaude(t, p)
	if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != Installed || st.Installed != 6 || st.Warn {
		t.Errorf("after: %+v", st)
	}
	// drop our Stop group
	root := parsed(t, p.ClaudeSettings)
	hooks, _ := root.Get("hooks")
	stop, _ := hooks.Get("Stop")
	var kept []jsonv.Value
	for _, g := range stop.A {
		if !IsOurGroup(g) {
			kept = append(kept, g)
		}
	}
	hooks.Set("Stop", jsonv.Arr(kept...))
	root.Set("hooks", hooks)
	seed(t, p.ClaudeSettings, []byte(root.Serialize()+"\n"))
	if st := StatusOf(p.ClaudeSettings, model.Claude, linux); st.Kind != Partial || st.Installed != 5 || st.Total != 6 {
		t.Errorf("partial: %+v", st)
	}
}

func TestStatusPartialWhenHandlerDiffers(t *testing.T) {
	p := testenv.Home(t)
	installClaude(t, p)
	other := Format{GOOS: "linux", Program: "/new/place/agentchime"}
	if st := StatusOf(p.ClaudeSettings, model.Claude, other); st.Kind != Partial || st.Installed != 0 {
		t.Errorf("status %+v", st)
	}
}

func TestStatusWarnFollowsFormat(t *testing.T) {
	p := testenv.Home(t)
	w := Format{GOOS: "windows", Program: `C:\Users\Nguyen A\AgentChime.exe`}
	w.ShortPath = func(string) (string, error) { return `C:\Users\NGUYEN~1`, nil }
	if st := StatusOf(p.CodexHooks, model.Codex, w); st.Warn || st.Kind != NotInstalled {
		t.Errorf("missing file: %+v", st)
	}
	w.ShortPath = func(string) (string, error) { return "", errors.New("none") }
	if st := StatusOf(p.CodexHooks, model.Codex, w); !st.Warn {
		t.Error("expected warn")
	}
	if st := StatusOf(p.ClaudeSettings, model.Claude, w); st.Warn {
		t.Error("claude exec form never warns")
	}
}

// fakeGetShortPathName shortens every component that is not already 8.3, the way Windows does,
// including the file name. alias picks the ~N suffix.
func fakeGetShortPathName(alias int) func(string) (string, error) {
	return func(p string) (string, error) {
		parts := strings.Split(p, `\`)
		for i, c := range parts {
			name, ext := c, ""
			if j := strings.LastIndex(c, "."); j > 0 {
				name, ext = c[:j], c[j:]
			}
			if i == 0 || (len(name) <= 8 && len(ext) <= 4 && !strings.ContainsAny(c, " +,;=[]")) {
				continue
			}
			short := strings.ToUpper(strings.NewReplacer(" ", "", "'", "").Replace(name))
			if len(short) > 6 {
				short = short[:6]
			}
			parts[i] = fmt.Sprintf("%s~%d%s", short, alias, strings.ToUpper(ext))
		}
		return strings.Join(parts, `\`), nil
	}
}

func TestCodexWindowsShortPathIsOursAndRemoves(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "codex-hooks.json")
	seed(t, p.CodexHooks, orig)
	w := Format{GOOS: "windows", Program: `C:\Users\Nguyen A\AppData\Local\AgentChime\AgentChime.exe`, ShortPath: fakeGetShortPathName(1)}
	if _, err := Install(p.CodexHooks, model.Codex, w, p.BackupsDir, t0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(readBytes(t, p.CodexHooks), []byte(`C:/Users/NGUYEN~1/AppData/Local/AGENTC~1/AgentChime.exe hook codex-stop`)) {
		t.Fatalf("command not shortened by directory only:\n%s", readBytes(t, p.CodexHooks))
	}
	if st := StatusOf(p.CodexHooks, model.Codex, w); st.Kind != Installed || st.Warn {
		t.Errorf("status %+v", st)
	}
	// A new 8.3 alias replaces our groups in place instead of adding a second set.
	w2 := w
	w2.ShortPath = fakeGetShortPathName(2)
	if _, err := Install(p.CodexHooks, model.Codex, w2, p.BackupsDir, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(readBytes(t, p.CodexHooks), []byte("hook codex-stop")); n != 1 {
		t.Errorf("codex-stop handlers = %d, want 1", n)
	}
	if err := Uninstall(p.CodexHooks, model.Codex, p.BackupsDir, t0.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readBytes(t, p.CodexHooks), orig) {
		t.Errorf("remove left our groups:\n%s", readBytes(t, p.CodexHooks))
	}
	if st := StatusOf(p.CodexHooks, model.Codex, w); st.Kind != NotInstalled {
		t.Errorf("after remove: %+v", st)
	}
}

func TestUninstallKeepsForeignOnlyFileAndDropsEmptyHooks(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.ClaudeSettings, []byte("{\n  \"model\": \"x\"\n}\n"))
	installClaude(t, p)
	if err := Uninstall(p.ClaudeSettings, model.Claude, p.BackupsDir, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := string(readBytes(t, p.ClaudeSettings)); got != "{\n  \"model\": \"x\"\n}\n" {
		t.Errorf("got %q", got)
	}
}

func TestUninstallMissingFileIsNoop(t *testing.T) {
	p := testenv.Home(t)
	if err := Uninstall(p.ClaudeSettings, model.Claude, p.BackupsDir, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.ClaudeSettings); err == nil {
		t.Error("file created")
	}
}

// Codex

func installCodex(t *testing.T, p paths.Paths) bool {
	t.Helper()
	r, err := Install(p.CodexHooks, model.Codex, linux, p.BackupsDir, t0)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCodexInstallAppendsAfterExistingGroups(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "codex-hooks.json")
	seed(t, p.CodexHooks, orig)
	bv, _ := jsonv.Parse(orig)
	before, _ := bv.Get("hooks")
	installCodex(t, p)
	after, _ := parsed(t, p.CodexHooks).Get("hooks")
	eq(t, keys(after), keys(before), "events")
	touched := map[string]bool{"UserPromptSubmit": true, "Stop": true, "PermissionRequest": true, "Interrupt": true}
	for _, m := range before.O {
		cur, _ := after.Get(m.Key)
		if !touched[m.Key] {
			eq(t, cur, m.Val, m.Key)
			continue
		}
		if len(cur.A) != 2 {
			t.Fatalf("%s count %d", m.Key, len(cur.A))
		}
		eq(t, cur.A[0], m.Val.A[0], m.Key)
		if !IsOurGroup(cur.A[1]) {
			t.Errorf("%s: not ours", m.Key)
		}
		hs, _ := cur.A[1].Get("hooks")
		if a, _ := hs.A[0].Get("async"); !a.B {
			t.Errorf("%s: not async", m.Key)
		}
	}
}

func TestCodexInstallIsIdempotent(t *testing.T) {
	p := testenv.Home(t)
	seed(t, p.CodexHooks, fixture(t, "codex-hooks.json"))
	installCodex(t, p)
	once := readBytes(t, p.CodexHooks)
	if installCodex(t, p) {
		t.Error("replaced on second install")
	}
	if !bytes.Equal(readBytes(t, p.CodexHooks), once) {
		t.Error("changed on second install")
	}
	if st := StatusOf(p.CodexHooks, model.Codex, linux); st.Kind != Installed || st.Total != 4 {
		t.Errorf("status %+v", st)
	}
}

func TestCodexInstallThenUninstallIsByteIdentical(t *testing.T) {
	p := testenv.Home(t)
	orig := fixture(t, "codex-hooks.json")
	seed(t, p.CodexHooks, orig)
	installCodex(t, p)
	if bytes.Equal(readBytes(t, p.CodexHooks), orig) {
		t.Fatal("install changed nothing")
	}
	if err := Uninstall(p.CodexHooks, model.Codex, p.BackupsDir, t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readBytes(t, p.CodexHooks), orig) {
		t.Error("not byte-identical")
	}
}

func TestCodexInvalidHooksJSONIsNeverOverwritten(t *testing.T) {
	p := testenv.Home(t)
	bad := []byte("{ not json")
	seed(t, p.CodexHooks, bad)
	if _, err := Install(p.CodexHooks, model.Codex, linux, p.BackupsDir, t0); err == nil {
		t.Error("install should fail")
	}
	if err := Uninstall(p.CodexHooks, model.Codex, p.BackupsDir, t0); err == nil {
		t.Error("uninstall should fail")
	}
	if !bytes.Equal(readBytes(t, p.CodexHooks), bad) {
		t.Error("file modified")
	}
	if st := StatusOf(p.CodexHooks, model.Codex, linux); st.Kind != Unreadable || st.Path != p.CodexHooks {
		t.Errorf("status %+v", st)
	}
}

func TestCodexMissingFileCreatesMinimal(t *testing.T) {
	p := testenv.Home(t)
	installCodex(t, p)
	root := parsed(t, p.CodexHooks)
	eq(t, keys(root), []string{"hooks"}, "root")
	h, _ := root.Get("hooks")
	eq(t, keys(h), []string{"UserPromptSubmit", "Stop", "PermissionRequest", "Interrupt"}, "events")
}

func itoa(i int) string { return strconv.Itoa(i) }

func pad2(i int) string { return fmt.Sprintf("%02d", i) }

func sprint(v any) string { return fmt.Sprint(v) }
