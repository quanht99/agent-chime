// Package hooks installs and removes AgentChime's hook groups in the Claude Code and Codex hook files.
package hooks

import (
	"github.com/quanht99/agent-chime/go/internal/hookcmd"
	"github.com/quanht99/agent-chime/go/internal/model"
)

// Spec is one hook group we register: the event, an optional matcher and the subcommand it runs.
type Spec struct {
	Event, Matcher string
	Command        hookcmd.Command
}

// ClaudeSpecs and CodexSpecs mirror AgentChime/Shared/AgentHooks.swift, in the same order.
var ClaudeSpecs = []Spec{
	{"UserPromptSubmit", "", hookcmd.ClaudePrompt},
	{"Stop", "", hookcmd.ClaudeStop},
	{"StopFailure", "", hookcmd.ClaudeStopFailure},
	{"PermissionRequest", "", hookcmd.ClaudePermission},
	{"Notification", "elicitation_dialog", hookcmd.ClaudeNotification},
	{"PreToolUse", "AskUserQuestion", hookcmd.ClaudeAsk},
}

var CodexSpecs = []Spec{
	{"UserPromptSubmit", "", hookcmd.CodexPrompt},
	{"Stop", "", hookcmd.CodexStop},
	{"PermissionRequest", "", hookcmd.CodexPermission},
	{"Interrupt", "", hookcmd.CodexInterrupt},
}

func SpecsFor(a model.Agent) []Spec {
	if a == model.Codex {
		return CodexSpecs
	}
	return ClaudeSpecs
}
