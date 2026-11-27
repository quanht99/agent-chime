package runctx

import (
	"strings"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/model"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestNestedDetection(t *testing.T) {
	if !IsNested(model.Codex, env(map[string]string{"CLAUDECODE": "1"})) {
		t.Error("codex with CLAUDECODE should be nested")
	}
	if !IsNested(model.Codex, env(map[string]string{"CLAUDE_CODE_SESSION_ID": "x"})) {
		t.Error("codex with CLAUDE_CODE_SESSION_ID should be nested")
	}
	if !IsNested(model.Claude, env(map[string]string{"CODEX_THREAD_ID": "x"})) {
		t.Error("claude with CODEX_THREAD_ID should be nested")
	}
	if !IsNested(model.Claude, env(map[string]string{"CODEX_SANDBOX": "seatbelt"})) {
		t.Error("claude with CODEX_SANDBOX should be nested")
	}
	if IsNested(model.Claude, env(map[string]string{"CLAUDECODE": "1"})) {
		t.Error("claude with own marker is not nested")
	}
	if IsNested(model.Codex, env(map[string]string{"CLAUDECODE": ""})) {
		t.Error("empty marker is not set")
	}
}

func TestClaudeHeadlessByEntrypoint(t *testing.T) {
	for _, e := range []string{"sdk-cli", "sdk-ts", "sdk-py"} {
		if !IsHeadless(model.Claude, env(map[string]string{"CLAUDE_CODE_ENTRYPOINT": e}), nil) {
			t.Errorf("%s should be headless", e)
		}
	}
	for _, e := range []string{"cli", "claude-desktop", ""} {
		if IsHeadless(model.Claude, env(map[string]string{"CLAUDE_CODE_ENTRYPOINT": e}), nil) {
			t.Errorf("%q should not be headless", e)
		}
	}
}

func TestCodexHeadlessByExecAncestor(t *testing.T) {
	e := env(nil)
	yes := [][][]string{
		{{"/bin/zsh", "-lc", "x"}, {"/home/example/.local/bin/codex", "exec", "hi"}},
		{{"node", "/a/bin/codex.js", "e", "hi"}},
	}
	for _, a := range yes {
		if !IsHeadless(model.Codex, e, a) {
			t.Errorf("%v should be headless", a)
		}
	}
	no := [][][]string{{{"codex"}}, {{"codex", "resume"}}, {{"notcodex", "exec"}}}
	for _, a := range no {
		if IsHeadless(model.Codex, e, a) {
			t.Errorf("%v should not be headless", a)
		}
	}
}

func TestCodexHeadlessWindowsNames(t *testing.T) {
	cases := [][][]string{
		{{`C:\Users\example\AppData\Roaming\npm\node_modules\@openai\codex\bin\codex.js`, "exec"}},
		{{"CODEX.EXE", "e"}},
		{{`C:\x\codex.cmd`, "exec"}},
		{{"node", `C:\x\Codex.JS`, "e"}},
	}
	for _, a := range cases {
		if !IsHeadless(model.Codex, env(nil), a) {
			t.Errorf("%v should be headless", a)
		}
	}
}

func TestProbeLineRedactsAndOmitsPrompt(t *testing.T) {
	environ := []string{"CLAUDE_CODE_OAUTH_TOKEN=tok123", "CLAUDE_CODE_ENTRYPOINT=cli", "HOME=/home/example"}
	payload := []byte(`{"prompt":"secret plan","hook_event_name":"Stop"}`)
	line := ProbeLine(environ, payload)
	for _, bad := range []string{"CLAUDE_CODE_OAUTH_TOKEN", "tok123", "secret plan", "/home/example"} {
		if strings.Contains(line, bad) {
			t.Errorf("line leaks %q: %s", bad, line)
		}
	}
	for _, want := range []string{"CLAUDE_CODE_ENTRYPOINT=cli", "prompt", "hook_event_name=Stop"} {
		if !strings.Contains(line, want) {
			t.Errorf("line missing %q: %s", want, line)
		}
	}
}

func TestProbeLineExactFormat(t *testing.T) {
	environ := []string{"TERM_PROGRAM=vscode", "CLAUDE_X=1", "CODEX_Y=2", "AI_AGENT=a", "PATH=/bin"}
	payload := []byte(`{"tool_name":"Bash","stop_hook_active":true,"agent_id":42,"zeta":1}`)
	got := ProbeLine(environ, payload)
	want := "env{AI_AGENT=a CLAUDE_X=1 CODEX_Y=2 TERM_PROGRAM=vscode} " +
		"payload{keys=[agent_id,stop_hook_active,tool_name,zeta] tool_name=Bash stop_hook_active=true agent_id=42}"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if g := ProbeLine(nil, []byte("not json")); g != "env{} payload{invalid}" {
		t.Errorf("got %s", g)
	}
}

func TestProbeLineFullMatchesSwiftFormat(t *testing.T) {
	got := ProbeLineFull("claude-stop", []string{"CLAUDE_A=b"},
		[][]string{{"/usr/bin/claude", "--resume-long-argument-value-xyz"}, {"zsh"}}, []byte(`{}`))
	want := "cmd=claude-stop env{CLAUDE_A=b} ancestors[claude --resume-long-argume | zsh] payload{keys=[]}"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestProbeLineNeverIncludesSecretNames(t *testing.T) {
	environ := []string{
		"CLAUDE_API_KEY=example", "CODEX_Token=example2", "CLAUDE_MY_SECRET=example3",
		"CLAUDE_AUTHORIZATION=example4", "CLAUDE_CODE_ENTRYPOINT=cli",
	}
	line := ProbeLine(environ, []byte(`{}`))
	for _, bad := range []string{"example", "API_KEY", "Token", "SECRET", "AUTHORIZATION"} {
		if strings.Contains(line, bad) {
			t.Errorf("line leaks %q: %s", bad, line)
		}
	}
	if !strings.Contains(line, "CLAUDE_CODE_ENTRYPOINT=cli") {
		t.Errorf("safe var missing: %s", line)
	}
}

func TestProbeLineClipsValues(t *testing.T) {
	long := strings.Repeat("a", 300)
	line := ProbeLine([]string{"CLAUDE_L=" + long}, []byte(`{"tool_name":"`+long+`"}`))
	if strings.Contains(line, strings.Repeat("a", 121)) {
		t.Errorf("values not clipped to 120")
	}
	if !strings.Contains(line, "CLAUDE_L="+strings.Repeat("a", 120)+"}") {
		t.Errorf("env clip wrong")
	}
}

func TestAncestorsLive(t *testing.T) {
	// Best effort everywhere; must not panic and must respect MaxDepth.
	if a := Ancestors(); len(a) > MaxDepth {
		t.Errorf("too many ancestors: %d", len(a))
	}
}
