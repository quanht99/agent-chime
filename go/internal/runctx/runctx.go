// Package runctx decides whether a hook runs nested inside another agent or
// headless, and builds the privacy-safe debug probe line.
package runctx

import (
	"sort"
	"strings"

	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/model"
)

// MaxDepth is how many ancestors Ancestors inspects.
const MaxDepth = 8

var (
	claudeMarkers = []string{"CLAUDECODE", "CLAUDE_CODE_SESSION_ID"}
	codexMarkers  = []string{"CODEX_THREAD_ID", "CODEX_SANDBOX"}

	probeEnvPrefixes   = []string{"CLAUDE", "CODEX", "TERM_PROGRAM", "__CFBundleIdentifier", "AI_AGENT"}
	probeSecretWords   = []string{"TOKEN", "KEY", "SECRET", "AUTH"}
	probePayloadFields = []string{"hook_event_name", "permission_mode", "tool_name", "notification_type",
		"stop_hook_active", "agent_id", "agent_type"}
)

func anySet(names []string, getenv func(string) string) bool {
	for _, n := range names {
		if getenv(n) != "" {
			return true
		}
	}
	return false
}

// IsNested: a Claude hook that sees Codex markers, or a Codex hook that sees
// Claude markers, runs inside the other agent.
func IsNested(a model.Agent, getenv func(string) string) bool {
	switch a {
	case model.Claude:
		return anySet(codexMarkers, getenv)
	case model.Codex:
		return anySet(claudeMarkers, getenv)
	}
	return false
}

// IsHeadless: `claude -p` / SDK for Claude, `codex exec` for Codex.
func IsHeadless(a model.Agent, getenv func(string) string, ancestors [][]string) bool {
	switch a {
	case model.Claude:
		switch getenv("CLAUDE_CODE_ENTRYPOINT") {
		case "sdk-cli", "sdk-ts", "sdk-py":
			return true
		}
	case model.Codex:
		for _, argv := range ancestors {
			for i := 0; i+1 < len(argv); i++ {
				if isCodexName(baseName(argv[i])) && (argv[i+1] == "exec" || argv[i+1] == "e") {
					return true
				}
			}
		}
	}
	return false
}

// baseName splits on both / and \ so Windows paths work on every OS.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func isCodexName(n string) bool {
	switch strings.ToLower(n) {
	case "codex", "codex.js", "codex.exe", "codex.cmd":
		return true
	}
	return false
}

// Ancestors returns the argv of up to MaxDepth ancestors, nearest first. Best effort.
func Ancestors() [][]string { return ancestors() }

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func hasAnySubstr(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func envPart(environ []string) string {
	vals := map[string]string{}
	var names []string
	for _, kv := range environ {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if !hasAnyPrefix(name, probeEnvPrefixes) || hasAnySubstr(strings.ToUpper(name), probeSecretWords) {
			continue
		}
		if _, dup := vals[name]; !dup {
			names = append(names, name)
		}
		vals[name] = val
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = n + "=" + clip(vals[n], 120)
	}
	return strings.Join(parts, " ")
}

func payloadPart(payload []byte) string {
	root, err := jsonv.Parse(payload)
	if err != nil || root.Kind != jsonv.KindObject {
		return "invalid"
	}
	keys := make([]string, len(root.O))
	for i, m := range root.O {
		keys[i] = m.Key
	}
	sort.Strings(keys)
	parts := []string{"keys=[" + strings.Join(keys, ",") + "]"}
	for _, f := range probePayloadFields {
		v, ok := root.Get(f)
		if !ok {
			continue
		}
		switch v.Kind {
		case jsonv.KindString:
			parts = append(parts, f+"="+clip(v.S, 120))
		case jsonv.KindBool:
			if v.B {
				parts = append(parts, f+"=true")
			} else {
				parts = append(parts, f+"=false")
			}
		case jsonv.KindNumber:
			parts = append(parts, f+"="+clip(v.Num, 120))
		}
	}
	return strings.Join(parts, " ")
}

// ProbeLine is the privacy-safe debug line: whitelisted env names (never ones
// containing TOKEN, KEY, SECRET or AUTH) and whitelisted payload fields only.
func ProbeLine(environ []string, payload []byte) string {
	return "env{" + envPart(environ) + "} payload{" + payloadPart(payload) + "}"
}

// ProbeLineFull is the Swift-identical line, with the hook command and ancestors.
func ProbeLineFull(cmd string, environ []string, ancestors [][]string, payload []byte) string {
	parts := make([]string, len(ancestors))
	for i, argv := range ancestors {
		name := ""
		if len(argv) > 0 {
			name = baseName(argv[0])
		}
		if len(argv) > 1 {
			name += " " + clip(argv[1], 20)
		}
		parts[i] = name
	}
	return "cmd=" + cmd + " env{" + envPart(environ) + "} ancestors[" + strings.Join(parts, " | ") +
		"] payload{" + payloadPart(payload) + "}"
}
