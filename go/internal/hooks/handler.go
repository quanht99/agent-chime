package hooks

import (
	"strings"
	"unicode"

	"github.com/quanht99/agent-chime/go/internal/hookcmd"
	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/model"
)

// Format says how a hook handler is written on one system.
type Format struct {
	GOOS    string
	Program string
	// ShortPath returns the 8.3 form of a path. Only used for Codex on windows.
	ShortPath func(string) (string, error)
}

func member(k string, v jsonv.Value) jsonv.Member { return jsonv.Member{Key: k, Val: v} }

// shellQuoted returns `"<program>" hook <sub>` with \ " $ ` escaped in the program.
func shellQuoted(program string, c hookcmd.Command) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range program {
		if r == '\\' || r == '"' || r == '$' || r == '`' {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	sb.WriteString(`" hook `)
	sb.WriteString(string(c))
	return sb.String()
}

// shellSafe reports whether s can be written unquoted in a shell command: ASCII letters, digits,
// `._~:/-` and non-ASCII letters only. Anything else (space, quote, &, (, ), ;, %, `, !, $ ...) is unsafe.
func shellSafe(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("._~:/-", r):
		case r > unicode.MaxASCII && unicode.IsLetter(r):
		default:
			return false
		}
	}
	return true
}

// splitDir splits a Windows or unix path at its last separator.
func splitDir(p string) (dir, base string) {
	i := strings.LastIndexAny(p, `/\`)
	if i < 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

// Handler builds the handler object. warn is true when the handler may not run (Codex on Windows with
// a program path that is not shell-safe and has no shell-safe short form).
func (f Format) Handler(a model.Agent, c hookcmd.Command) (h jsonv.Value, warn bool) {
	if f.GOOS == "windows" {
		if a == model.Claude {
			return jsonv.Obj(
				member("type", jsonv.Str("command")),
				member("command", jsonv.Str(strings.ReplaceAll(f.Program, "/", `\`))),
				member("args", jsonv.Arr(jsonv.Str("hook"), jsonv.Str(string(c)))),
				member("async", jsonv.Bool(true)),
			), false
		}
		prog := strings.ReplaceAll(f.Program, `\`, "/")
		if shellSafe(prog) {
			return commandHandler(prog + " hook " + string(c)), false
		}
		// Only the directory is shortened: GetShortPathName would also turn AgentChime.exe into
		// AGENTC~1.EXE, which is no longer recognised as ours.
		if dir, base := splitDir(f.Program); f.ShortPath != nil && dir != "" {
			if sp, err := f.ShortPath(dir); err == nil && sp != "" {
				cand := strings.TrimRight(strings.ReplaceAll(sp, `\`, "/"), "/") + "/" + base
				if shellSafe(cand) {
					return commandHandler(cand + " hook " + string(c)), false
				}
			}
		}
		return commandHandler(shellQuoted(prog, c)), true
	}
	return commandHandler(shellQuoted(f.Program, c)), false
}

func commandHandler(cmd string) jsonv.Value {
	return jsonv.Obj(
		member("type", jsonv.Str("command")),
		member("command", jsonv.Str(cmd)),
		member("async", jsonv.Bool(true)),
	)
}

// Group builds {"matcher"?, "hooks":[handler]}.
func (f Format) Group(a model.Agent, s Spec) (g jsonv.Value, warn bool) {
	h, warn := f.Handler(a, s.Command)
	var ms []jsonv.Member
	if s.Matcher != "" {
		ms = append(ms, member("matcher", jsonv.Str(s.Matcher)))
	}
	ms = append(ms, member("hooks", jsonv.Arr(h)))
	return jsonv.Obj(ms...), warn
}

// IsOurGroup reports whether every handler of the group runs `agentchime[.exe] hook ...`.
func IsOurGroup(g jsonv.Value) bool {
	hs, ok := g.Get("hooks")
	if !ok || hs.Kind != jsonv.KindArray || len(hs.A) == 0 {
		return false
	}
	for _, h := range hs.A {
		if !isOurHandler(h) {
			return false
		}
	}
	return true
}

func isOurHandler(h jsonv.Value) bool {
	cmdV, ok := h.Get("command")
	if !ok || cmdV.Kind != jsonv.KindString {
		return false
	}
	var program, first string
	if args, has := h.Get("args"); has {
		if args.Kind != jsonv.KindArray || len(args.A) == 0 || args.A[0].Kind != jsonv.KindString {
			return false
		}
		program, first = cmdV.S, args.A[0].S
	} else {
		program, first = splitCommand(cmdV.S)
	}
	if program == "" || first == "" {
		return false
	}
	base := program
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	return (base == "agentchime" || base == "agentchime.exe") && first == "hook"
}

// splitCommand returns the program and the first argument of a shell-form command string.
func splitCommand(s string) (program, first string) {
	s = strings.TrimLeft(s, " \t")
	rest := ""
	if strings.HasPrefix(s, `"`) {
		var sb strings.Builder
		i := 1
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				sb.WriteByte(s[i+1])
				i += 2
				continue
			}
			i++
			if c == '"' {
				closed = true
				break
			}
			sb.WriteByte(c)
		}
		if !closed {
			return "", ""
		}
		program, rest = sb.String(), s[i:]
	} else {
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			return s, ""
		}
		program, rest = s[:i], s[i:]
	}
	rest = strings.TrimLeft(rest, " \t")
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[:i]
	}
	return program, rest
}
