//go:build !updatetest

package update

import (
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestAllowedURL(t *testing.T) {
	yes := []string{
		"https://github.com/quanht99/agent-chime/releases/latest/download/latest.json",
		"https://objects.githubusercontent.com/x",
		"https://release-assets.githubusercontent.com/x",
	}
	no := []string{
		"http://github.com/x",
		"https://githubusercontent.com.evil.com/x",
		"https://github.com.evil.com/x",
		"https://evilgithubusercontent.com/x",
		"https://user@github.com/x",
		"ftp://github.com/x",
		"http://127.0.0.1:1/x",
	}
	for _, s := range yes {
		u, _ := url.Parse(s)
		if !AllowedURL(u) || !Allowed(u) {
			t.Errorf("should allow %s", s)
		}
	}
	for _, s := range no {
		u, _ := url.Parse(s)
		if AllowedURL(u) || Allowed(u) {
			t.Errorf("should refuse %s", s)
		}
	}
}

func TestReleaseKeyIsNotTheTestKey(t *testing.T) {
	raw, err := os.ReadFile("testdata/TEST-ONLY-public-key.hex")
	if err != nil {
		t.Fatal(err)
	}
	k := Key()
	if k != nil && strings.EqualFold(strings.TrimSpace(string(raw)), hex.EncodeToString(k)) {
		t.Fatal("release key equals test key")
	}
	if TestBuild {
		t.Fatal("TestBuild true in release build")
	}
}
