// Package codextrust reports whether Codex trusts our hooks. It reads ~/.codex/config.toml and the
// profile files ~/.codex/<profile>.config.toml and never writes any of them.
package codextrust

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/quanht99/agent-chime/go/internal/hooks"
	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

type Kind int

const (
	NotInstalled Kind = iota
	Untrusted
	Trusted
	Disabled
)

// State is the trust state; Pending counts our handlers without a trusted hash (Untrusted only).
type State struct {
	Kind    Kind
	Pending int
}

// SnakeCase turns "UserPromptSubmit" into "user_prompt_submit".
func SnakeCase(event string) string {
	var sb strings.Builder
	for i, r := range []rune(event) {
		if unicode.IsUpper(r) {
			if i > 0 {
				sb.WriteByte('_')
			}
			sb.WriteRune(unicode.ToLower(r))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// norm makes a hooks path comparable across separators, the Windows extended prefix and case.
func norm(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	p = strings.TrimPrefix(p, "//?/")
	return strings.ToLower(p)
}

type handlerKey struct {
	path, event string
	group, idx  int
}

// parseKey splits "<path>:<event>:<group>:<handler>" from the right.
func parseKey(key string) (handlerKey, bool) {
	var parts [3]string
	rest := key
	for i := 2; i >= 0; i-- {
		j := strings.LastIndexByte(rest, ':')
		if j < 0 {
			return handlerKey{}, false
		}
		parts[i] = rest[j+1:]
		rest = rest[:j]
	}
	g, err1 := strconv.Atoi(parts[1])
	h, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || g < 0 || h < 0 {
		return handlerKey{}, false
	}
	return handlerKey{path: norm(rest), event: parts[0], group: g, idx: h}, true
}

func ourKeys(hooksPath string, root jsonv.Value) []handlerKey {
	var keys []handlerKey
	evs, ok := root.Get("hooks")
	if !ok {
		return nil
	}
	p := norm(hooksPath)
	for _, m := range evs.O {
		if m.Val.Kind != jsonv.KindArray {
			continue
		}
		for g, group := range m.Val.A {
			if !hooks.IsOurGroup(group) {
				continue
			}
			hs, _ := group.Get("hooks")
			for h := range hs.A {
				keys = append(keys, handlerKey{p, SnakeCase(m.Key), g, h})
			}
		}
	}
	return keys
}

// sectionKey extracts the key from a `[hooks.state."<key>"]` or `[hooks.state.'<key>']` header.
func sectionKey(line string) (string, bool) {
	const prefix = "[hooks.state."
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	rest := line[len(prefix):]
	switch {
	case strings.HasPrefix(rest, `"`) && strings.HasSuffix(rest, `"]`) && len(rest) >= 3:
		return unescapeBasic(rest[1 : len(rest)-2]), true
	case strings.HasPrefix(rest, `'`) && strings.HasSuffix(rest, `']`) && len(rest) >= 3:
		return rest[1 : len(rest)-2], true
	}
	return "", false
}

func unescapeBasic(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '\\' || s[i+1] == '"') {
			i++
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// StateFor computes the state of one config file. A nil hooks value means no hooks file.
func StateFor(hooksPath string, hooksRoot *jsonv.Value, toml string) State {
	if hooksRoot == nil {
		return State{Kind: NotInstalled}
	}
	keys := ourKeys(hooksPath, *hooksRoot)
	if len(keys) == 0 {
		return State{Kind: NotInstalled}
	}

	type entry struct{ hash, disabled bool }
	entries := map[handlerKey]*entry{}
	var cur *entry
	for _, raw := range strings.FieldsFunc(toml, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") {
			cur = nil
			if k, ok := sectionKey(line); ok {
				if hk, ok := parseKey(k); ok {
					if entries[hk] == nil {
						entries[hk] = &entry{}
					}
					cur = entries[hk]
				}
			}
			continue
		}
		if cur == nil {
			continue
		}
		compact := strings.ReplaceAll(line, " ", "")
		if strings.HasPrefix(compact, `trusted_hash="`) {
			cur.hash = true
		}
		if strings.HasPrefix(compact, "enabled=false") {
			cur.disabled = true
		}
	}

	pending := 0
	for _, k := range keys {
		if e := entries[k]; e != nil && e.disabled {
			return State{Kind: Disabled}
		}
	}
	for _, k := range keys {
		if e := entries[k]; e == nil || !e.hash {
			pending++
		}
	}
	if pending == 0 {
		return State{Kind: Trusted}
	}
	return State{Kind: Untrusted, Pending: pending}
}

// ConfigFiles lists config.toml first, then every *.config.toml beside it sorted by name.
func ConfigFiles(p paths.Paths) []string {
	dir := filepath.Dir(p.CodexConfig)
	var profiles []string
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".config.toml") && !e.IsDir() {
				profiles = append(profiles, filepath.Join(dir, e.Name()))
			}
		}
	}
	sort.Strings(profiles)
	return append([]string{p.CodexConfig}, profiles...)
}

// Combined picks the best state across files: trusted, then disabled, then the fewest pending.
func Combined(states []State) State {
	best, haveDisabled := -1, false
	for _, s := range states {
		switch s.Kind {
		case Trusted:
			return State{Kind: Trusted}
		case Disabled:
			haveDisabled = true
		case Untrusted:
			if best < 0 || s.Pending < best {
				best = s.Pending
			}
		}
	}
	if haveDisabled {
		return State{Kind: Disabled}
	}
	if best >= 0 {
		return State{Kind: Untrusted, Pending: best}
	}
	return State{Kind: NotInstalled}
}

// Current reads the Codex hooks file and every config file and combines the results.
func Current(p paths.Paths) State {
	root, _ := hooks.ReadJSONFile(p.CodexHooks)
	var states []State
	for _, f := range ConfigFiles(p) {
		data, _ := os.ReadFile(f)
		states = append(states, StateFor(p.CodexHooks, root, string(data)))
	}
	return Combined(states)
}
