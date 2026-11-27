package i18n

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var swiftEntry = regexp.MustCompile(`case \.(\w+): return \("((?:[^"\\]|\\.)*)", "((?:[^"\\]|\\.)*)"\)`)
var swiftEscape = regexp.MustCompile(`\\(.)`)

func unescape(s string) string { return swiftEscape.ReplaceAllString(s, "$1") }

func TestSharedTextMatchesSwift(t *testing.T) {
	src, err := os.ReadFile("../../../AgentChime/Shared/Text.swift")
	if err != nil {
		t.Fatal(err)
	}
	ms := swiftEntry.FindAllStringSubmatch(string(src), -1)
	if len(ms) != 74 {
		t.Fatalf("extracted %d Swift entries, want 74", len(ms))
	}
	for _, m := range ms {
		e, ok := table[Key(m[1])]
		if !ok {
			t.Errorf("missing Go key %s", m[1])
			continue
		}
		if e.en != unescape(m[2]) {
			t.Errorf("%s EN: got %q want %q", m[1], e.en, unescape(m[2]))
		}
		if e.vi != unescape(m[3]) {
			t.Errorf("%s VI: got %q want %q", m[1], e.vi, unescape(m[3]))
		}
	}
}

func TestEveryKeyHasBothLanguages(t *testing.T) {
	seen := map[Key]bool{}
	for _, k := range Keys() {
		if seen[k] {
			t.Errorf("duplicate key %s", k)
		}
		seen[k] = true
		if T(k, EN) == "" || T(k, VI) == "" {
			t.Errorf("key %s missing a language", k)
		}
		if strings.Count(T(k, EN), "%@") != strings.Count(T(k, VI), "%@") {
			t.Errorf("key %s placeholder count differs", k)
		}
	}
	if len(seen) != 74+3 {
		t.Errorf("got %d keys, want 77", len(seen))
	}
}

func TestPlaceholders(t *testing.T) {
	if got := T("statusLastAlert", EN, "a", "b"); got != "Last alert: a — b" {
		t.Errorf("got %q", got)
	}
	if got := T("statusLastAlert", "fr", "a", "b"); got != "Last alert: a — b" {
		t.Errorf("unknown lang: got %q", got)
	}
	if got := T("summaryDone", VI, "Claude"); got != "Claude xong" {
		t.Errorf("got %q", got)
	}
	for _, k := range []Key{"updateAvailable", "currentVersion", "lastChecked", "updateMenuItem", "updateFailed", "updatedTo"} {
		if n := strings.Count(T(k, EN), "%@"); n != 1 {
			t.Errorf("%s: %d placeholders, want 1", k, n)
		}
	}
	if got := T("menuMute", EN, "extra"); got != "Mute" {
		t.Errorf("extra args: got %q", got)
	}
}

func TestNativeName(t *testing.T) {
	if EN.NativeName() != "English" || VI.NativeName() != "Tiếng Việt" {
		t.Error("native names")
	}
}
