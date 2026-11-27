package jsonv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) Value {
	t.Helper()
	v, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return v
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	d, err := os.ReadFile(filepath.Join("..", "..", "..", "AgentChime", "Tests", "Fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	if len(d) == 0 {
		t.Fatalf("%s empty", name)
	}
	return d
}

func TestRoundTripFixtures(t *testing.T) {
	for _, name := range []string{"claude-settings.json", "codex-hooks.json"} {
		t.Run(name, func(t *testing.T) {
			d := fixture(t, name)
			got := mustParse(t, string(d)).Serialize() + "\n"
			if got != string(d) {
				t.Errorf("not byte-identical to fixture")
			}
			golden, err := os.ReadFile(filepath.Join("testdata", name+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(golden) {
				t.Errorf("differs from JSON.stringify golden")
			}
		})
	}
}

func TestParseAcceptsLeadingBOM(t *testing.T) {
	v := mustParse(t, "\xEF\xBB\xBF{\"a\":1}")
	if got := v.Serialize(); got != "{\n  \"a\": 1\n}" {
		t.Errorf("got %q", got)
	}
	if _, err := Parse([]byte("\xEF\xBB\xBF\xEF\xBB\xBF{}")); err == nil {
		t.Error("double BOM accepted")
	}
}

func TestNumbersKeepTheirSpelling(t *testing.T) {
	out := mustParse(t, `{"a": 1.50, "b": 1e3, "c": -2.5e3, "d": 1.0}`).Serialize()
	for _, w := range []string{`"a": 1.50`, `"b": 1e3`, `"c": -2.5e3`, `"d": 1.0`} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %s in %s", w, out)
		}
	}
}

func TestStringEscapes(t *testing.T) {
	v := mustParse(t, `{"s":"a\"b\\c\n\u0001/é😀\b\f\r\t\u001f"}`)
	want := "{\n  \"s\": " + `"a\"b\\c\n\u0001/é😀\b\f\r\t\u001f"` + "\n}"
	if got := v.Serialize(); got != want {
		t.Errorf("got %q want %q", got, want)
	}
	v = mustParse(t, `{"s":"\/"}`)
	if s, _ := v.Get("s"); s.S != "/" {
		t.Errorf("got %q", s.S)
	}
}

func TestSurrogatePairDecodes(t *testing.T) {
	v := mustParse(t, `{"s":"😀"}`)
	if s, _ := v.Get("s"); s.S != "😀" {
		t.Errorf("got %q", s.S)
	}
}

func TestEscapedNulDoesNotTruncate(t *testing.T) {
	v := mustParse(t, `{"s":"a\u0000b"}`)
	if s, _ := v.Get("s"); s.S != "a\x00b" {
		t.Errorf("got %q", s.S)
	}
	if got, want := v.Serialize(), "{\n  \"s\": \"a\\u0000b\"\n}"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestEmptyContainers(t *testing.T) {
	got := mustParse(t, `{"a":[],"b":{}}`).Serialize()
	if want := "{\n  \"a\": [],\n  \"b\": {}\n}"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestKeyOrderKept(t *testing.T) {
	got := mustParse(t, `{"z":1,"a":[true,false,null],"m":{"y":"s","b":2}}`).Serialize()
	want := "{\n  \"z\": 1,\n  \"a\": [\n    true,\n    false,\n    null\n  ],\n  \"m\": {\n    \"y\": \"s\",\n    \"b\": 2\n  }\n}"
	if got != want {
		t.Errorf("got %q", got)
	}
}

func keys(v Value) []string {
	var k []string
	for _, m := range v.O {
		k = append(k, m.Key)
	}
	return k
}

func TestSetKeepsPositionAndAppendsNewKeys(t *testing.T) {
	v := mustParse(t, `{"x":1,"y":2}`)
	v.Set("x", Value{Kind: KindNumber, Num: "3"})
	v.Set("z", Value{Kind: KindNumber, Num: "4"})
	if got := strings.Join(keys(v), ","); got != "x,y,z" {
		t.Errorf("keys %s", got)
	}
	if x, _ := v.Get("x"); x.Num != "3" {
		t.Errorf("x=%q", x.Num)
	}
}

func TestDeleteAndAccessors(t *testing.T) {
	v := mustParse(t, `{"a":true,"b":"s","c":[1]}`)
	if a, ok := v.Get("a"); !ok || !a.B {
		t.Error("a")
	}
	if b, _ := v.Get("b"); b.S != "s" {
		t.Error("b")
	}
	if c, _ := v.Get("c"); len(c.A) != 1 || c.A[0].Num != "1" {
		t.Error("c")
	}
	if _, ok := v.Get("missing"); ok {
		t.Error("missing found")
	}
	v.Delete("b")
	v.Delete("nope")
	if got := strings.Join(keys(v), ","); got != "a,c" {
		t.Errorf("keys %s", got)
	}
}

func TestNonObjectGetSetDelete(t *testing.T) {
	v := Value{}
	if _, ok := v.Get("a"); ok {
		t.Error("Get on null")
	}
	v.Set("a", Bool(true))
	v.Delete("a")
	if v.Kind != KindNull || v.O != nil {
		t.Errorf("mutated: %+v", v)
	}
	arr := Arr(Str("x"))
	arr.Set("a", Bool(true))
	if len(arr.A) != 1 || arr.O != nil {
		t.Error("array mutated")
	}
}

func TestDuplicateKeysKeptInOrder(t *testing.T) {
	v := mustParse(t, `{"a":1,"a":2}`)
	if got := strings.Join(keys(v), ","); got != "a,a" {
		t.Errorf("keys %s", got)
	}
	if a, _ := v.Get("a"); a.Num != "1" {
		t.Errorf("Get returns first, got %s", a.Num)
	}
}

func TestRejectsInvalidJSON(t *testing.T) {
	bad := []string{
		`{"a": 1,}`, `{"a": 1} x`, "// c\n{}", "/* c */{}", "", " ", `[1,]`, `{"a": 01}`,
		"\"\x01\"", `{"a" 1}`, `{'a':1}`, `['a']`, `{"a":'b'}`, `[1 2]`, `{"a":1 "b":2}`,
		`-`, `1.`, `.5`, `1e`, `+1`, `"\x"`, `"\ud800"`, `"\udc00"`, `"\ud800A"`, `"\u12"`,
		`"abc`, `tru`, `nul`, `[`, `{`, `{"a"`, `{1:2}`, "\"\xff\"", "\xEF\xBB",
		"\xEF\xBB\xBF\xEF\xBB\xBF{}", "{}\x00",
	}
	for _, s := range bad {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestAcceptsWhitespaceAndScalarsAtTopLevel(t *testing.T) {
	if got := mustParse(t, " \n null \t").Serialize(); got != "null" {
		t.Errorf("got %q", got)
	}
	if got := mustParse(t, "[1, 2]").Serialize(); got != "[\n  1,\n  2\n]" {
		t.Errorf("got %q", got)
	}
	if got := mustParse(t, `"x"`).Serialize(); got != `"x"` {
		t.Errorf("got %q", got)
	}
}

func TestDepthLimit(t *testing.T) {
	nest := func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) }
	if _, err := Parse([]byte(nest(512))); err != nil {
		t.Errorf("depth 512 rejected: %v", err)
	}
	if _, err := Parse([]byte(nest(513))); err == nil {
		t.Error("depth 513 accepted")
	}
	obj := strings.Repeat(`{"a":`, 513) + "1" + strings.Repeat("}", 513)
	if _, err := Parse([]byte(obj)); err == nil {
		t.Error("object depth 513 accepted")
	}
}

func TestConstructors(t *testing.T) {
	v := Obj(Member{"s", Str("x")}, Member{"b", Bool(false)}, Member{"a", Arr()}, Member{"o", Obj()})
	want := "{\n  \"s\": \"x\",\n  \"b\": false,\n  \"a\": [],\n  \"o\": {}\n}"
	if got := v.Serialize(); got != want {
		t.Errorf("got %q", got)
	}
}
