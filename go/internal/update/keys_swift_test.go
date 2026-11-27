//go:build !updatetest

package update

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestSwiftKeysMatchGo keeps the two hex literals in UpdateKeys.swift equal to
// Go's release key and to the TEST-ONLY key. They are found by constant name.
func TestSwiftKeysMatchGo(t *testing.T) {
	src, err := os.ReadFile("../../../AgentChime/Update/UpdateKeys.swift")
	if err != nil {
		t.Fatal(err)
	}
	lit := func(name string) string {
		re := regexp.MustCompile(`\blet ` + name + ` = "([0-9a-fA-F]*)"`)
		m := re.FindStringSubmatch(string(src))
		if m == nil {
			t.Fatalf("%s not found in UpdateKeys.swift", name)
		}
		return strings.ToLower(m[1])
	}
	if got := lit("releasePublicKeyHex"); got != releasePublicKeyHex {
		t.Errorf("Swift release key %q != Go %q", got, releasePublicKeyHex)
	}
	raw, err := os.ReadFile("testdata/TEST-ONLY-public-key.hex")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := lit("testPublicKeyHex"), strings.ToLower(strings.TrimSpace(string(raw))); got != want {
		t.Errorf("Swift test key %q != %q", got, want)
	}
}
