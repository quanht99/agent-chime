//go:build updatetest

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quanht99/agent-chime/go/internal/update"
)

const seedPath = "../../internal/update/testdata/TEST-ONLY-signing-seed.txt"

func testSeed(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	t.Fatal("no seed")
	return ""
}

func envOf(v string) func(string) string {
	return func(k string) string {
		if k == "UPDATE_SIGNING_KEY" {
			return v
		}
		return ""
	}
}

var names = []string{
	"AgentChime-1.2.3-macos.zip",
	"AgentChime-1.2.3-windows-amd64.zip",
	"AgentChime-1.2.3-windows-arm64.zip",
	"AgentChime-1.2.3-linux-amd64.tar.gz",
	"AgentChime-1.2.3-linux-arm64.tar.gz",
}

func writeFiles(t *testing.T, dir string, which ...string) map[string][]byte {
	t.Helper()
	got := map[string][]byte{}
	for i, n := range which {
		data := bytes.Repeat([]byte{byte('a' + i)}, 10+i)
		if err := os.WriteFile(filepath.Join(dir, n), data, 0o644); err != nil {
			t.Fatal(err)
		}
		got[n] = data
	}
	return got
}

func TestSignsWithTestKey(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	data := writeFiles(t, dir, names...)
	var stdout bytes.Buffer
	seed := testSeed(t)
	if err := run([]string{"-version", "1.2.3", "-dir", dir, "-out", out}, envOf(seed), &stdout); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(out, "latest.json"))
	sig, _ := os.ReadFile(filepath.Join(out, "latest.json.sig"))
	m, err := update.ParseVerified(body, sig, update.Key())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 5 || m.ReleasePage != update.ReleaseBase+"tag/v1.2.3" {
		t.Fatalf("manifest = %+v", m)
	}
	for _, f := range m.Files {
		sum := sha256.Sum256(data[f.Name])
		if f.Size != int64(len(data[f.Name])) || f.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("bad entry %+v", f)
		}
	}
	if !bytes.HasSuffix(body, []byte("}\n")) || !bytes.HasSuffix(sig, []byte("\n")) {
		t.Fatal("missing final newline")
	}
	if strings.Contains(stdout.String(), seed) || stdout.String() != "latest.json\nlatest.json.sig\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRequireSubset(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	writeFiles(t, dir, names[3])
	err := run([]string{"-version", "1.2.3", "-dir", dir, "-out", out, "-require", "linux-amd64"}, envOf(testSeed(t)), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMissingFile(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	writeFiles(t, dir, names[0], names[1], names[3], names[4])
	err := run([]string{"-version", "1.2.3", "-dir", dir, "-out", out}, envOf(testSeed(t)), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "windows-arm64") {
		t.Fatalf("err = %v", err)
	}
}

func TestWrongKey(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	writeFiles(t, dir, names...)
	other := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	err := run([]string{"-version", "1.2.3", "-dir", dir, "-out", out}, envOf(other), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), other) {
		t.Fatal("error leaks the secret")
	}
}

func TestEmptySecret(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	writeFiles(t, dir, names...)
	if err := run([]string{"-version", "1.2.3", "-dir", dir, "-out", out}, envOf(""), &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
}

func TestBadBase64DoesNotEchoValue(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	writeFiles(t, dir, names...)
	secret := "not*base64*SECRETVALUE"
	err := run([]string{"-version", "1.2.3", "-dir", dir, "-out", out}, envOf(secret), &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "SECRETVALUE") {
		t.Fatalf("err = %v", err)
	}
}

func TestRefusesVersionThatIsNotPlain(t *testing.T) {
	for _, v := range []string{"v1.2.3", "1.2", "1.2.3-rc1", "dev", "1.2.3 "} {
		dir, out := t.TempDir(), t.TempDir()
		writeFiles(t, dir, names...)
		err := run([]string{"-version", v, "-dir", dir, "-out", out}, envOf(testSeed(t)), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "X.Y.Z") {
			t.Fatalf("%q: err = %v", v, err)
		}
		if _, serr := os.Stat(filepath.Join(out, "latest.json")); serr == nil {
			t.Fatalf("%q: wrote a manifest", v)
		}
	}
}
