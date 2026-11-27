// Package hookcmd turns a hook command plus its stdin payload into an Action.
package hookcmd

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/quanht99/agent-chime/go/internal/model"
)

type Command string

const (
	ClaudePrompt       Command = "claude-prompt"
	ClaudeStop         Command = "claude-stop"
	ClaudeStopFailure  Command = "claude-stop-failure"
	ClaudePermission   Command = "claude-permission"
	ClaudeNotification Command = "claude-notification"
	ClaudeAsk          Command = "claude-ask"
	CodexPrompt        Command = "codex-prompt"
	CodexStop          Command = "codex-stop"
	CodexPermission    Command = "codex-permission"
	CodexInterrupt     Command = "codex-interrupt"
)

var allCommands = []Command{ClaudePrompt, ClaudeStop, ClaudeStopFailure, ClaudePermission, ClaudeNotification,
	ClaudeAsk, CodexPrompt, CodexStop, CodexPermission, CodexInterrupt}

func ParseCommand(s string) (Command, bool) {
	for _, c := range allCommands {
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}

func (c Command) Agent() model.Agent {
	if strings.HasPrefix(string(c), "codex-") {
		return model.Codex
	}
	return model.Claude
}

// IsAlert reports whether the command can raise an alert.
func (c Command) IsAlert() bool {
	switch c {
	case ClaudeStop, ClaudeStopFailure, ClaudePermission, ClaudeNotification, ClaudeAsk, CodexStop, CodexPermission:
		return true
	}
	return false
}

type ActionKind int

const (
	TurnStarted ActionKind = iota
	// TurnContinued is Claude Code's own prompt handing background work back.
	TurnContinued
	// TurnHeld means the turn ended while background tasks still run.
	TurnHeld
	TurnCancelled
	Alert
	Ignore
)

type Action struct {
	Kind      ActionKind
	Agent     model.Agent
	SessionID string
	// HasSession is true when the payload carried a session id, even an empty one (Swift binds any
	// present session_id too).
	HasSession bool
	Event      model.AgentEvent // Alert only; Duration is filled in later from state
	Running    int              // TurnHeld only
	Reason     string           // Ignore only
}

var handBackPrefixes = []string{"<task-notification>", "<agent-message "}

func Parse(cmd Command, payload []byte) Action {
	agent := cmd.Agent()
	isStop := cmd == ClaudeStop || cmd == ClaudeStopFailure || cmd == CodexStop

	var root map[string]any
	var v any
	if err := json.Unmarshal(payload, &v); err == nil {
		root, _ = v.(map[string]any)
	}
	if root == nil {
		if isStop {
			return newAlert(agent, model.Done, "", false, "")
		}
		return Action{Kind: Ignore, Reason: "invalid payload"}
	}

	session, hasSession := str(root["session_id"])
	if !hasSession && agent == model.Codex {
		session, hasSession = str(root["turn_id"])
	}
	turn := func(k ActionKind) Action {
		if !hasSession {
			return Action{Kind: Ignore, Reason: "no session id"}
		}
		return Action{Kind: k, Agent: agent, SessionID: session, HasSession: true}
	}

	switch cmd {
	case ClaudePrompt, CodexPrompt:
		if cmd == ClaudePrompt && hasSession {
			if p, _ := str(root["prompt"]); isHandBack(p) {
				return turn(TurnContinued)
			}
		}
		return turn(TurnStarted)
	case CodexInterrupt:
		return turn(TurnCancelled)
	case ClaudeStop, ClaudeStopFailure, CodexStop:
		if b, _ := root["stop_hook_active"].(bool); b {
			return Action{Kind: Ignore, Reason: "stop_hook_active"}
		}
		if n := runningBackgroundTasks(root); n > 0 && hasSession {
			a := turn(TurnHeld)
			a.Running = n
			return a
		}
		return newAlert(agent, model.Done, session, hasSession, "")
	case ClaudePermission, CodexPermission:
		d, _ := str(root["tool_name"])
		return newAlert(agent, model.Attention, session, hasSession, d)
	case ClaudeNotification:
		t, ok := str(root["notification_type"])
		if !ok || t != "elicitation_dialog" {
			if !ok {
				t = "none"
			}
			return Action{Kind: Ignore, Reason: "notification " + t}
		}
		d, _ := str(root["message"])
		return newAlert(agent, model.Attention, session, hasSession, d)
	case ClaudeAsk:
		return newAlert(agent, model.Attention, session, hasSession, "AskUserQuestion")
	}
	return Action{Kind: Ignore, Reason: "unknown command"}
}

func newAlert(agent model.Agent, kind model.AlertKind, session string, hasSession bool, detail string) Action {
	return Action{Kind: Alert, Agent: agent, SessionID: session, HasSession: hasSession,
		Event: model.AgentEvent{Agent: agent, Kind: kind, SessionID: session, Detail: detail}}
}

func str(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func isHandBack(prompt string) bool {
	text := strings.TrimLeftFunc(prompt, unicode.IsSpace)
	for _, p := range handBackPrefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// runningBackgroundTasks counts entries of background_tasks whose status is
// "running"; a missing or malformed field counts as none.
func runningBackgroundTasks(root map[string]any) int {
	tasks, _ := root["background_tasks"].([]any)
	n := 0
	for _, t := range tasks {
		if m, ok := t.(map[string]any); ok && m["status"] == "running" {
			n++
		}
	}
	return n
}
