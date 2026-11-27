package hooks

import (
	"errors"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/hookcmd"
	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/model"
)

func mustParse(t *testing.T, s string) jsonv.Value {
	t.Helper()
	v, err := jsonv.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func cmdOf(h jsonv.Value) string {
	c, _ := h.Get("command")
	return c.S
}

func TestLinuxHandlerShape(t *testing.T) {
	h, warn := linux.Handler(model.Claude, hookcmd.ClaudeStop)
	want := `{"type":"command","command":"\"/home/example/.local/share/agentchime/agentchime\" hook claude-stop","async":true}`
	eq(t, h, mustParse(t, want), "handler")
	if warn {
		t.Error("warn")
	}
	if got := h.Serialize(); got != mustParse(t, want).Serialize() {
		t.Error("key order differs")
	}
	darwin := Format{GOOS: "darwin", Program: linux.Program}
	d, _ := darwin.Handler(model.Codex, hookcmd.CodexStop)
	if cmdOf(d) != `"/home/example/.local/share/agentchime/agentchime" hook codex-stop` {
		t.Errorf("darwin: %s", cmdOf(d))
	}
}

func TestLinuxEscaping(t *testing.T) {
	f := Format{GOOS: "linux", Program: "/a\"b$c`d\\e"}
	h, _ := f.Handler(model.Claude, hookcmd.ClaudeStop)
	if got, want := cmdOf(h), `"/a\"b\$c\`+"`"+`d\\e" hook claude-stop`; got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestWindowsClaudeExecForm(t *testing.T) {
	f := Format{GOOS: "windows", Program: `C:\Users\example\AppData\Local\AgentChime\AgentChime.exe`}
	h, warn := f.Handler(model.Claude, hookcmd.ClaudeStop)
	want := mustParse(t, `{"type":"command","command":"C:\\Users\\example\\AppData\\Local\\AgentChime\\AgentChime.exe","args":["hook","claude-stop"],"async":true}`)
	if h.Serialize() != want.Serialize() {
		t.Errorf("got %s", h.Serialize())
	}
	if warn {
		t.Error("warn")
	}
	g, _ := f.Group(model.Claude, ClaudeSpecs[1])
	if !IsOurGroup(g) {
		t.Error("not ours")
	}
}

func TestWindowsCodexCommand(t *testing.T) {
	f := Format{GOOS: "windows", Program: `C:\Users\example\AppData\Local\AgentChime\AgentChime.exe`}
	h, warn := f.Handler(model.Codex, hookcmd.CodexStop)
	if got, want := cmdOf(h), "C:/Users/example/AppData/Local/AgentChime/AgentChime.exe hook codex-stop"; got != want {
		t.Errorf("got %s", got)
	}
	if warn {
		t.Error("warn")
	}
	if _, has := h.Get("args"); has {
		t.Error("codex handler has args")
	}
	g, _ := f.Group(model.Codex, CodexSpecs[1])
	if !IsOurGroup(g) {
		t.Error("not ours")
	}
}

func TestHandlerPathsWithSpacesAndUnicode(t *testing.T) {
	// Linux, space.
	lf := Format{GOOS: "linux", Program: "/home/example user/.local/share/agentchime/agentchime"}
	g, _ := lf.Group(model.Claude, ClaudeSpecs[1])
	hs, _ := g.Get("hooks")
	if cmdOf(hs.A[0]) != `"/home/example user/.local/share/agentchime/agentchime" hook claude-stop` {
		t.Errorf("linux: %s", cmdOf(hs.A[0]))
	}
	if !IsOurGroup(g) {
		t.Error("linux group not ours")
	}

	// Windows Claude, unicode round trip.
	wf := Format{GOOS: "windows", Program: `C:\Users\Nguyễn A\AppData\Local\AgentChime\AgentChime.exe`}
	g, _ = wf.Group(model.Claude, ClaudeSpecs[1])
	back := mustParse(t, g.Serialize())
	if !IsOurGroup(back) || back.Serialize() != g.Serialize() {
		t.Error("windows claude round trip")
	}
	hs, _ = back.Get("hooks")
	if cmdOf(hs.A[0]) != wf.Program {
		t.Errorf("command = %s", cmdOf(hs.A[0]))
	}

	// Windows Codex, short path.
	cf := wf
	var asked string
	cf.ShortPath = func(s string) (string, error) {
		asked = s
		return `C:\Users\NGUYNA~1\AppData\Local\AGENTC~1`, nil
	}
	h, warn := cf.Handler(model.Codex, hookcmd.CodexStop)
	if asked != `C:\Users\Nguyễn A\AppData\Local\AgentChime` {
		t.Errorf("ShortPath asked for %q, want the directory only", asked)
	}
	if cmdOf(h) != "C:/Users/NGUYNA~1/AppData/Local/AGENTC~1/AgentChime.exe hook codex-stop" || warn {
		t.Errorf("short: %s warn=%v", cmdOf(h), warn)
	}
	g, _ = cf.Group(model.Codex, CodexSpecs[1])
	if !IsOurGroup(g) {
		t.Error("short group not ours")
	}

	// Short path fails: quoted, warn.
	cf.ShortPath = func(string) (string, error) { return "", errors.New("no 8.3 names") }
	h, warn = cf.Handler(model.Codex, hookcmd.CodexStop)
	if cmdOf(h) != `"C:/Users/Nguyễn A/AppData/Local/AgentChime/AgentChime.exe" hook codex-stop` || !warn {
		t.Errorf("quoted: %s warn=%v", cmdOf(h), warn)
	}
	g, w := cf.Group(model.Codex, CodexSpecs[1])
	if !IsOurGroup(g) || !w {
		t.Error("quoted group")
	}

	// Short path still has a space, or no ShortPath at all: quoted, warn.
	cf.ShortPath = func(s string) (string, error) { return s, nil }
	if _, warn = cf.Handler(model.Codex, hookcmd.CodexStop); !warn {
		t.Error("space in short path should warn")
	}
	cf.ShortPath = nil
	if _, warn = cf.Handler(model.Codex, hookcmd.CodexStop); !warn {
		t.Error("nil ShortPath should warn")
	}
}

func group(t *testing.T, handlers string) jsonv.Value {
	return mustParse(t, `{"hooks":[`+handlers+`]}`)
}

func TestNotOursLookalikes(t *testing.T) {
	cases := map[string]string{
		"v1 macOS":       `{"type":"command","command":"\"/Users/example/Library/Application Support/AgentChime/agentchime-hook\" claude-stop"}`,
		"play":           `{"type":"command","command":"/usr/bin/agentchime play"}`,
		"other program":  `{"type":"command","command":"/usr/bin/other hook claude-stop"}`,
		"args wrong":     `{"type":"command","command":"C:\\x\\AgentChime.exe","args":["play"]}`,
		"empty args":     `{"type":"command","command":"C:\\x\\AgentChime.exe","args":[]}`,
		"no first arg":   `{"type":"command","command":"/x/agentchime"}`,
		"unterminated":   `{"type":"command","command":"\"/x/agentchime hook a"}`,
		"no command":     `{"type":"command"}`,
		"similar suffix": `{"type":"command","command":"/x/notagentchime hook a"}`,
	}
	for name, h := range cases {
		if IsOurGroup(group(t, h)) {
			t.Errorf("%s: wrongly ours", name)
		}
	}
	if IsOurGroup(mustParse(t, `{"hooks":[]}`)) {
		t.Error("empty hooks is ours")
	}
	if IsOurGroup(mustParse(t, `{"matcher":"x"}`)) || IsOurGroup(jsonv.Str("x")) {
		t.Error("no hooks is ours")
	}
	ours := `{"type":"command","command":"/x/agentchime hook a"}`
	if IsOurGroup(group(t, ours+","+cases["play"])) {
		t.Error("mixed group is ours")
	}
	if !IsOurGroup(group(t, ours+`,{"command":"C:\\x\\AGENTCHIME.EXE","args":["hook","b"]}`)) {
		t.Error("case-insensitive basename and mixed handlers should be ours")
	}
}

func TestCodexWindowsUnsafeCharacters(t *testing.T) {
	for _, prog := range []string{
		`C:\Users\O'Brien\AppData\Local\AgentChime\AgentChime.exe`,
		`C:\Users\A&B\AppData\Local\AgentChime\AgentChime.exe`,
		`C:\Users\x(1)\AppData\Local\AgentChime\AgentChime.exe`,
		`C:\Users\100%\AppData\Local\AgentChime\AgentChime.exe`,
		`C:\Users\a;b!\AppData\Local\AgentChime\AgentChime.exe`,
	} {
		f := Format{GOOS: "windows", Program: prog}
		// No short form: quoted and warned.
		h, warn := f.Handler(model.Codex, hookcmd.CodexStop)
		if !warn || cmdOf(h)[0] != '"' {
			t.Errorf("%s: %s warn=%v", prog, cmdOf(h), warn)
		}
		// A safe short directory is used.
		f.ShortPath = func(string) (string, error) { return `C:\Users\OBRIEN~1\AppData\Local\AGENTC~1`, nil }
		h, warn = f.Handler(model.Codex, hookcmd.CodexStop)
		if warn || cmdOf(h) != "C:/Users/OBRIEN~1/AppData/Local/AGENTC~1/AgentChime.exe hook codex-stop" {
			t.Errorf("%s short: %s warn=%v", prog, cmdOf(h), warn)
		}
		// 8.3 names may keep ' & ( ): still unsafe, so quoted and warned.
		f.ShortPath = func(string) (string, error) { return `C:\Users\O'BRIE~1\AppData\Local\AGENTC~1`, nil }
		if h, warn = f.Handler(model.Codex, hookcmd.CodexStop); !warn || cmdOf(h)[0] != '"' {
			t.Errorf("%s unsafe short: %s warn=%v", prog, cmdOf(h), warn)
		}
		if g, _ := f.Group(model.Codex, CodexSpecs[1]); !IsOurGroup(g) {
			t.Errorf("%s: quoted group not ours", prog)
		}
	}
	// Unicode letters without spaces stay unquoted.
	f := Format{GOOS: "windows", Program: `C:\Users\Nguyễn\AppData\Local\AgentChime\AgentChime.exe`}
	if h, warn := f.Handler(model.Codex, hookcmd.CodexStop); warn || cmdOf(h) != "C:/Users/Nguyễn/AppData/Local/AgentChime/AgentChime.exe hook codex-stop" {
		t.Errorf("unicode: %s warn=%v", cmdOf(h), warn)
	}
}
