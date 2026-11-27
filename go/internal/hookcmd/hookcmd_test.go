package hookcmd

import (
	"reflect"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/model"
)

func started(s string) Action   { return Action{Kind: TurnStarted, SessionID: s} }
func continued(s string) Action { return Action{Kind: TurnContinued, SessionID: s} }
func ignore(r string) Action    { return Action{Kind: Ignore, Reason: r} }

// alert expects a session id to be present when it is non-empty; see TestEmptySessionIDIsPresent.
func alert(a model.Agent, k model.AlertKind, session, detail string) Action {
	return Action{Kind: Alert, Agent: a, SessionID: session, HasSession: session != "",
		Event: model.AgentEvent{Agent: a, Kind: k, SessionID: session, Detail: detail}}
}

func act(t *testing.T, c Command, json string) Action {
	t.Helper()
	return Parse(c, []byte(json))
}

// check fills Agent on turn expectations the way Parse does.
func check(t *testing.T, c Command, json string, want Action) {
	t.Helper()
	if want.Kind != Alert && want.Kind != Ignore {
		if want.Agent == "" {
			want.Agent = c.Agent()
		}
		want.HasSession = true
	}
	if got := Parse(c, []byte(json)); !reflect.DeepEqual(got, want) {
		t.Errorf("%s %q:\n got  %+v\n want %+v", c, json, got, want)
	}
}

func TestParseCommandAndAgent(t *testing.T) {
	all := map[string]model.Agent{
		"claude-prompt": model.Claude, "claude-stop": model.Claude, "claude-stop-failure": model.Claude,
		"claude-permission": model.Claude, "claude-notification": model.Claude, "claude-ask": model.Claude,
		"codex-prompt": model.Codex, "codex-stop": model.Codex, "codex-permission": model.Codex, "codex-interrupt": model.Codex,
	}
	alerts := map[string]bool{"claude-stop": true, "claude-stop-failure": true, "claude-permission": true,
		"claude-notification": true, "claude-ask": true, "codex-stop": true, "codex-permission": true}
	for s, a := range all {
		c, ok := ParseCommand(s)
		if !ok || c.Agent() != a {
			t.Errorf("%s: ok=%v agent=%v", s, ok, c.Agent())
		}
		if c.IsAlert() != alerts[s] {
			t.Errorf("%s: IsAlert=%v", s, c.IsAlert())
		}
	}
	if _, ok := ParseCommand("nope"); ok {
		t.Error("unknown command accepted")
	}
}

func TestPromptStartsTurn(t *testing.T) {
	check(t, ClaudePrompt, `{"session_id":"s1"}`, started("s1"))
	check(t, ClaudePrompt, `{}`, ignore("no session id"))
}

func TestStopAndStopFailureAreDone(t *testing.T) {
	done := alert(model.Claude, model.Done, "s1", "")
	check(t, ClaudeStop, `{"session_id":"s1"}`, done)
	check(t, ClaudeStopFailure, `{"session_id":"s1"}`, done)
}

func TestStopHookActiveIgnored(t *testing.T) {
	check(t, ClaudeStop, `{"session_id":"s1","stop_hook_active":true}`, ignore("stop_hook_active"))
}

func TestPermissionIsAttentionWithToolName(t *testing.T) {
	check(t, ClaudePermission, `{"session_id":"s1","tool_name":"Bash"}`, alert(model.Claude, model.Attention, "s1", "Bash"))
}

func TestElicitationIsAttention(t *testing.T) {
	check(t, ClaudeNotification, `{"session_id":"s1","notification_type":"elicitation_dialog","message":"Pick"}`,
		alert(model.Claude, model.Attention, "s1", "Pick"))
}

func TestIdlePromptIgnored(t *testing.T) {
	check(t, ClaudeNotification, `{"session_id":"s1","notification_type":"idle_prompt"}`, ignore("notification idle_prompt"))
	check(t, ClaudeNotification, `{"session_id":"s1"}`, ignore("notification none"))
}

func TestPermissionPromptNotificationIgnored(t *testing.T) {
	check(t, ClaudeNotification, `{"session_id":"s1","notification_type":"permission_prompt"}`, ignore("notification permission_prompt"))
}

func TestAskIsAttention(t *testing.T) {
	check(t, ClaudeAsk, `{"session_id":"s1"}`, alert(model.Claude, model.Attention, "s1", "AskUserQuestion"))
}

func TestGarbageStopStillAlerts(t *testing.T) {
	done := alert(model.Claude, model.Done, "", "")
	check(t, ClaudeStop, "not json", done)
	check(t, ClaudeStopFailure, "", done)
	check(t, CodexStop, "not json", alert(model.Codex, model.Done, "", ""))
	check(t, ClaudeStop, "[1]", done)
	check(t, ClaudeStop, "null", done)
}

func TestGarbagePermissionIsIgnored(t *testing.T) {
	check(t, ClaudePermission, "not json", ignore("invalid payload"))
	check(t, ClaudePrompt, "", ignore("invalid payload"))
	check(t, ClaudePrompt, "null", ignore("invalid payload"))
}

func TestCodexSessionFallsBackToTurnID(t *testing.T) {
	check(t, CodexPrompt, `{"turn_id":"t1"}`, started("t1"))
	check(t, CodexPrompt, `{"session_id":"s","turn_id":"t1"}`, started("s"))
	// Claude does not fall back.
	check(t, ClaudePrompt, `{"turn_id":"t1"}`, ignore("no session id"))
}

func TestCodexStopPermissionInterrupt(t *testing.T) {
	check(t, CodexStop, `{"session_id":"s1"}`, alert(model.Codex, model.Done, "s1", ""))
	check(t, CodexStop, `{"session_id":"s1","stop_hook_active":true}`, ignore("stop_hook_active"))
	check(t, CodexPermission, `{"session_id":"s1","tool_name":"shell"}`, alert(model.Codex, model.Attention, "s1", "shell"))
	check(t, CodexInterrupt, `{"session_id":"s1"}`, Action{Kind: TurnCancelled, SessionID: "s1"})
	check(t, CodexInterrupt, `{}`, ignore("no session id"))
}

func held(s string, n int) Action {
	return Action{Kind: TurnHeld, SessionID: s, Running: n}
}

func TestStopWithRunningBackgroundWorkHoldsTheTurn(t *testing.T) {
	check(t, ClaudeStop, `{"session_id":"s1","background_tasks":[{"id":"b1","type":"shell","status":"running"}]}`, held("s1", 1))
	check(t, ClaudeStop, `{"session_id":"s1","background_tasks":[{"type":"shell","status":"running"},{"type":"subagent","status":"running"}]}`, held("s1", 2))
}

func TestStopWithoutRunningBackgroundWorkIsDone(t *testing.T) {
	done := alert(model.Claude, model.Done, "s1", "")
	check(t, ClaudeStop, `{"session_id":"s1","background_tasks":[]}`, done)
	check(t, ClaudeStop, `{"session_id":"s1","background_tasks":"x"}`, done)
	check(t, ClaudeStop, `{"session_id":"s1","background_tasks":[42,"y",null]}`, done)
	check(t, ClaudeStop, `{"session_id":"s1","background_tasks":[{"status":"completed"},{"type":"shell"}]}`, done)
}

func TestRunningBackgroundWorkWithoutSessionStillAlerts(t *testing.T) {
	check(t, ClaudeStop, `{"background_tasks":[{"status":"running"}]}`, alert(model.Claude, model.Done, "", ""))
}

func TestStopHookActiveWinsOverBackgroundWork(t *testing.T) {
	check(t, ClaudeStop, `{"session_id":"s1","stop_hook_active":true,"background_tasks":[{"status":"running"}]}`, ignore("stop_hook_active"))
}

func TestBackgroundHandBackPromptsContinueTheTurn(t *testing.T) {
	check(t, ClaudePrompt, `{"session_id":"s1","prompt":"<task-notification>\n<task-id>b1</task-id>"}`, continued("s1"))
	check(t, ClaudePrompt, `{"session_id":"s1","prompt":"<agent-message from=\"a1\">\n[Subagent hand-back]"}`, continued("s1"))
	check(t, ClaudePrompt, `{"session_id":"s1","prompt":"  \n<task-notification>x"}`, continued("s1"))
}

func TestTypedPromptsStartATurn(t *testing.T) {
	check(t, ClaudePrompt, `{"session_id":"s1","prompt":"fix the <task-notification> parser"}`, started("s1"))
	check(t, ClaudePrompt, `{"session_id":"s1","prompt":"<agent-messages are odd"}`, started("s1"))
	check(t, ClaudePrompt, `{"session_id":"s1","prompt":42}`, started("s1"))
	check(t, CodexPrompt, `{"session_id":"s1","prompt":"<task-notification>x"}`, started("s1"))
}

func TestDurationStaysNil(t *testing.T) {
	if a := act(t, ClaudeStop, `{"session_id":"s1"}`); a.Event.Duration != nil {
		t.Error("duration should be nil")
	}
}

func TestEmptySessionIDIsPresent(t *testing.T) {
	for _, c := range []Command{ClaudeStop, CodexStop, ClaudePermission} {
		if a := act(t, c, `{"session_id":""}`); a.Kind != Alert || !a.HasSession || a.SessionID != "" {
			t.Errorf("%s empty id: %+v", c, a)
		}
		if a := act(t, c, `{}`); a.Kind != Alert || a.HasSession {
			t.Errorf("%s no id: %+v", c, a)
		}
	}
	if a := act(t, ClaudePrompt, `{"session_id":""}`); a.Kind != TurnStarted || !a.HasSession {
		t.Errorf("prompt empty id: %+v", a)
	}
	if a := act(t, ClaudeStop, "not json"); a.HasSession {
		t.Errorf("garbage: %+v", a)
	}
}
