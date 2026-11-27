package hooks

import (
	"time"

	"github.com/quanht99/agent-chime/go/internal/jsonv"
	"github.com/quanht99/agent-chime/go/internal/model"
)

type StatusKind int

const (
	NotInstalled StatusKind = iota
	Installed
	Partial
	Unreadable
)

type Status struct {
	Kind             StatusKind
	Installed, Total int
	Path             string
	Warn             bool // a handler we would write may not run (Codex on Windows, path with a space)
}

func clone(v jsonv.Value) jsonv.Value {
	c := v
	if v.A != nil {
		c.A = make([]jsonv.Value, len(v.A))
		for i := range v.A {
			c.A[i] = clone(v.A[i])
		}
	}
	if v.O != nil {
		c.O = make([]jsonv.Member, len(v.O))
		for i := range v.O {
			c.O[i] = jsonv.Member{Key: v.O[i].Key, Val: clone(v.O[i].Val)}
		}
	}
	return c
}

func same(a, b jsonv.Value) bool { return a.Serialize() == b.Serialize() }

func matcherOf(g jsonv.Value) string {
	if m, ok := g.Get("matcher"); ok && m.Kind == jsonv.KindString {
		return m.S
	}
	return ""
}

func hasMatcher(g jsonv.Value) bool {
	m, ok := g.Get("matcher")
	return ok && m.Kind == jsonv.KindString
}

// merged returns a copy of root with our groups added or replaced in place.
func merged(root *jsonv.Value, a model.Agent, specs []Spec, f Format) (out jsonv.Value, replaced bool) {
	if root == nil {
		out = jsonv.Obj()
	} else {
		out = clone(*root)
	}
	hooks, ok := out.Get("hooks")
	if !ok {
		hooks = jsonv.Obj()
	}
	for _, s := range specs {
		wanted, _ := f.Group(a, s)
		var groups []jsonv.Value
		if cur, ok := hooks.Get(s.Event); ok {
			groups = cur.A
		}
		idx := -1
		for i, g := range groups {
			if IsOurGroup(g) && matcherOf(g) == s.Matcher && hasMatcher(g) == (s.Matcher != "") {
				idx = i
				break
			}
		}
		if idx >= 0 {
			if !same(groups[idx], wanted) {
				groups[idx] = wanted
				replaced = true
			}
		} else {
			groups = append(groups, wanted)
		}
		hooks.Set(s.Event, jsonv.Arr(groups...))
	}
	out.Set("hooks", hooks)
	return out, replaced
}

// removed returns root without our groups; events and `hooks` left empty are dropped.
func removed(root jsonv.Value) jsonv.Value {
	hooks, ok := root.Get("hooks")
	if !ok || hooks.Kind != jsonv.KindObject {
		return root
	}
	out := clone(root)
	hooks = clone(hooks)
	any := false
	for _, m := range append([]jsonv.Member{}, hooks.O...) {
		if m.Val.Kind != jsonv.KindArray {
			continue
		}
		var kept []jsonv.Value
		for _, g := range m.Val.A {
			if !IsOurGroup(g) {
				kept = append(kept, g)
			}
		}
		if len(kept) == len(m.Val.A) {
			continue
		}
		any = true
		if len(kept) == 0 {
			hooks.Delete(m.Key)
		} else {
			hooks.Set(m.Key, jsonv.Arr(kept...))
		}
	}
	if !any {
		return root
	}
	if len(hooks.O) == 0 {
		out.Delete("hooks")
	} else {
		out.Set("hooks", hooks)
	}
	return out
}

// validateShape refuses what we cannot edit without damaging other data: a non-object root, a non-object
// `hooks`, or a non-array value for an event we touch.
func validateShape(root *jsonv.Value, specs []Spec, path string) error {
	if root == nil {
		return nil
	}
	if root.Kind != jsonv.KindObject {
		return &UnreadableError{Path: path}
	}
	hooks, ok := root.Get("hooks")
	if !ok {
		return nil
	}
	if hooks.Kind != jsonv.KindObject {
		return &UnreadableError{Path: path}
	}
	for _, s := range specs {
		if v, ok := hooks.Get(s.Event); ok && v.Kind != jsonv.KindArray {
			return &UnreadableError{Path: path}
		}
	}
	return nil
}

func statusOf(root *jsonv.Value, a model.Agent, specs []Spec, f Format) (installed int, anyOurs bool) {
	if root == nil {
		return 0, false
	}
	hooks, ok := root.Get("hooks")
	if !ok {
		return 0, false
	}
	for _, m := range hooks.O {
		for _, g := range m.Val.A {
			if IsOurGroup(g) {
				anyOurs = true
			}
		}
	}
	for _, s := range specs {
		wanted, _ := f.Group(a, s)
		cur, _ := hooks.Get(s.Event)
		for _, g := range cur.A {
			if same(g, wanted) {
				installed++
				break
			}
		}
	}
	return installed, anyOurs
}

// StatusOf reports whether every spec has our group with exactly the handler Format would write.
func StatusOf(path string, a model.Agent, f Format) Status {
	specs := SpecsFor(a)
	st := Status{Path: path, Total: len(specs)}
	for _, s := range specs {
		if _, w := f.Group(a, s); w {
			st.Warn = true
		}
	}
	root, err := ReadJSONFile(path)
	if err == nil {
		err = validateShape(root, specs, path)
	}
	if err != nil {
		st.Kind = Unreadable
		return st
	}
	n, anyOurs := statusOf(root, a, specs, f)
	st.Installed = n
	switch {
	case n == len(specs) && len(specs) > 0:
		st.Kind = Installed
	case n == 0 && !anyOurs:
		st.Kind = NotInstalled
	default:
		st.Kind = Partial
	}
	return st
}

// Install adds or replaces our groups. It reports whether an existing group of ours was replaced.
func Install(path string, a model.Agent, f Format, backupsDir string, now time.Time) (replacedExisting bool, err error) {
	specs := SpecsFor(a)
	existing, err := ReadJSONFile(path)
	if err != nil {
		return false, err
	}
	if err := validateShape(existing, specs, path); err != nil {
		return false, err
	}
	out, replaced := merged(existing, a, specs, f)
	if existing == nil || !same(out, *existing) {
		if err := WriteJSONFile(out, path, backupsDir, now); err != nil {
			return false, err
		}
	}
	return replaced, nil
}

// Uninstall removes every group of ours from the file, whichever agent it was written for.
func Uninstall(path string, a model.Agent, backupsDir string, now time.Time) error {
	existing, err := ReadJSONFile(path)
	if err != nil || existing == nil {
		return err
	}
	if err := validateShape(existing, append(append([]Spec{}, ClaudeSpecs...), CodexSpecs...), path); err != nil {
		return err
	}
	out := removed(*existing)
	if !same(out, *existing) {
		return WriteJSONFile(out, path, backupsDir, now)
	}
	return nil
}
