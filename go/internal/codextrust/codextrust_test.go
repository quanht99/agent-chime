package codextrust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/hooks"
	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

const hp = "/Users/example/.codex/hooks.json"

var ourEvents = []string{"user_prompt_submit", "stop", "permission_request", "interrupt"}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// installedHooks installs our Codex hooks into a copy of the fixture and returns the parsed file.
func installedHooks(t *testing.T) *jsonv.Value {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(f, []byte(readFile(t, "../hooks/testdata/codex-hooks.json")), 0o600); err != nil {
		t.Fatal(err)
	}
	fm := hooks.Format{GOOS: "linux", Program: "/home/example/.local/share/agentchime/agentchime"}
	if _, err := hooks.Install(f, model.Codex, fm, filepath.Join(dir, "bk"), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	v, err := hooks.ReadJSONFile(f)
	if err != nil || v == nil {
		t.Fatal(err)
	}
	return v
}

func fixtureHooks(t *testing.T) *jsonv.Value {
	t.Helper()
	v, err := hooks.ReadJSONFile("../hooks/testdata/codex-hooks.json")
	if err != nil || v == nil {
		t.Fatal(err)
	}
	return v
}

func fixtureTOML(t *testing.T) string { return readFile(t, "testdata/codex-config-hooks-state.toml") }

func entry(event string, hash, enabled bool) string {
	s := "\n[hooks.state.\"" + hp + ":" + event + ":1:0\"]\nenabled = " + map[bool]string{true: "true", false: "false"}[enabled] + "\n"
	if hash {
		s += "trusted_hash = \"sha256:fake\"\n"
	}
	return s
}

func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"UserPromptSubmit": "user_prompt_submit", "Stop": "stop",
		"PermissionRequest": "permission_request", "Interrupt": "interrupt",
	} {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStateFor(t *testing.T) {
	h := installedHooks(t)
	toml := fixtureTOML(t)
	all := toml
	for _, e := range ourEvents {
		all += entry(e, true, true)
	}
	cases := []struct {
		name string
		hk   *jsonv.Value
		toml string
		want State
	}{
		{"untrusted no entries", h, toml, State{Untrusted, 4}},
		{"partial", h, toml + entry("user_prompt_submit", true, true) + entry("stop", true, true), State{Untrusted, 2}},
		{"trusted", h, all, State{Trusted, 0}},
		{"disabled", h, toml + entry("user_prompt_submit", true, true) + entry("stop", true, false), State{Disabled, 0}},
		{"not installed no our groups", fixtureHooks(t), toml, State{NotInstalled, 0}},
		{"not installed nil hooks", nil, toml, State{NotInstalled, 0}},
		{"missing toml", h, "", State{Untrusted, 4}},
		{"hash without section ignored", h, "trusted_hash = \"x\"\n" + toml, State{Untrusted, 4}},
		{"padded header", h, "  [hooks.state.\"" + hp + ":stop:1:0\"]  \ntrusted_hash = \"a\"\n", State{Untrusted, 3}},
		{"other section ends ours", h, "[hooks.state.\"" + hp + ":stop:1:0\"]\n[other]\ntrusted_hash = \"a\"\n", State{Untrusted, 4}},
	}
	for _, c := range cases {
		if got := StateFor(hp, c.hk, c.toml); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestWindowsKeyForms(t *testing.T) {
	h := installedHooks(t)
	win := `C:\Users\example\.codex\hooks.json`
	forms := []string{
		`[hooks.state."C:\\Users\\example\\.codex\\hooks.json:stop:1:0"]`,
		`[hooks.state.'c:/users/example/.codex/hooks.json:stop:1:0']`,
		`[hooks.state."\\\\?\\C:\\Users\\example\\.codex\\hooks.json:stop:1:0"]`,
	}
	for _, form := range forms {
		toml := form + "\ntrusted_hash = \"sha256:x\"\n"
		if got := StateFor(win, h, toml); got != (State{Untrusted, 3}) {
			t.Errorf("%s: got %+v, want 3 pending (stop trusted)", form, got)
		}
	}
	// Every Windows form of all four events is fully trusted.
	var toml string
	for i, e := range ourEvents {
		toml += strings.Replace(forms[i%3], "stop", e, 1) + "\ntrusted_hash = \"sha256:x\"\n"
	}
	if got := StateFor(win, h, toml); got.Kind != Trusted {
		t.Errorf("all forms: got %+v, want trusted", got)
	}
	// A different path, event or index does not count.
	for _, bad := range []string{
		`[hooks.state."C:\\Users\\other\\.codex\\hooks.json:stop:1:0"]`,
		`[hooks.state."C:\\Users\\example\\.codex\\hooks.json:stop:2:0"]`,
		`[hooks.state."C:\\Users\\example\\.codex\\hooks.json:stop:1:1"]`,
		`[hooks.state."C:\\Users\\example\\.codex\\hooks.json:session_start:1:0"]`,
	} {
		if got := StateFor(win, h, bad+"\ntrusted_hash = \"x\"\n"); got != (State{Untrusted, 4}) {
			t.Errorf("%s: got %+v, want 4 pending", bad, got)
		}
	}
}

func TestCombined(t *testing.T) {
	u := func(n int) State { return State{Untrusted, n} }
	d, tr, ni := State{Kind: Disabled}, State{Kind: Trusted}, State{Kind: NotInstalled}
	for _, c := range []struct {
		in   []State
		want State
	}{
		{[]State{u(4), tr}, tr}, {[]State{d, tr}, tr}, {[]State{u(4), d}, d},
		{[]State{u(4), u(1)}, u(1)}, {[]State{ni, ni}, ni}, {nil, ni}, {[]State{ni, u(2)}, u(2)},
	} {
		if got := Combined(c.in); got != c.want {
			t.Errorf("Combined(%v) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func setupHome(t *testing.T) (paths.Paths, string) {
	t.Helper()
	p := testenv.Home(t)
	dir := filepath.Dir(p.CodexHooks)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fm := hooks.Format{GOOS: "linux", Program: "/home/example/.local/share/agentchime/agentchime"}
	if err := os.WriteFile(p.CodexHooks, []byte(readFile(t, "../hooks/testdata/codex-hooks.json")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hooks.Install(p.CodexHooks, model.Codex, fm, p.BackupsDir, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.CodexConfig, []byte(fixtureTOML(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p, dir
}

func trustTOML(p paths.Paths, events []string) string {
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString("[hooks.state.\"" + strings.ReplaceAll(p.CodexHooks, `\`, `\\`) + ":" + e + ":1:0\"]\ntrusted_hash = \"sha256:fake\"\n")
	}
	return sb.String()
}

func TestCurrentProfiles(t *testing.T) {
	p, dir := setupHome(t)
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := Current(p); got != (State{Untrusted, 4}) {
		t.Fatalf("no profile: %+v", got)
	}
	write("notes.toml", trustTOML(p, ourEvents))
	if got := Current(p); got != (State{Untrusted, 4}) {
		t.Fatalf("other toml must be ignored: %+v", got)
	}
	write("work.config.toml", trustTOML(p, []string{"stop"}))
	if got := Current(p); got != (State{Untrusted, 3}) {
		t.Fatalf("partial profile: %+v", got)
	}
	write("work.config.toml", trustTOML(p, ourEvents))
	if got := Current(p); got.Kind != Trusted {
		t.Fatalf("profile trust: %+v", got)
	}
}

func TestCurrentMissingFilesAreEmpty(t *testing.T) {
	p, _ := setupHome(t)
	if err := os.Remove(p.CodexConfig); err != nil {
		t.Fatal(err)
	}
	if got := Current(p); got != (State{Untrusted, 4}) {
		t.Fatalf("got %+v", got)
	}
	if err := os.Remove(p.CodexHooks); err != nil {
		t.Fatal(err)
	}
	if got := Current(p); got.Kind != NotInstalled {
		t.Fatalf("got %+v", got)
	}
}

func TestConfigFilesListsMainThenProfiles(t *testing.T) {
	p, dir := setupHome(t)
	for _, n := range []string{"b.config.toml", "a.config.toml", "notes.toml"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	for _, f := range ConfigFiles(p) {
		names = append(names, filepath.Base(f))
	}
	if strings.Join(names, ",") != "config.toml,a.config.toml,b.config.toml" {
		t.Fatalf("got %v", names)
	}
}

func TestPackageHasNoWriteCalls(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("no sources", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src := readFile(t, f)
		for _, bad := range []string{"os.WriteFile", "os.Create", "O_WRONLY", "O_RDWR", "os.Rename"} {
			if strings.Contains(src, bad) {
				t.Errorf("%s contains %s", f, bad)
			}
		}
	}
}
