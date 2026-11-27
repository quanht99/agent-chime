package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWritesSeedAndPrintsPublicKey(t *testing.T) {
	out := filepath.Join(t.TempDir(), "key.txt")
	var stdout bytes.Buffer
	if err := run([]string{"-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(out)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v", st.Mode(), err)
		}
	}
	raw, err := os.ReadFile(out)
	if err != nil || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("read: %v", err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatalf("seed: %v len %d", err, len(seed))
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	want := "public key: " + hex.EncodeToString(pub) + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRefusesExisting(t *testing.T) {
	out := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(out, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"-out", out}, &stdout); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(out); string(b) != "keep" || stdout.Len() != 0 {
		t.Fatal("existing file changed or output printed")
	}
}

func TestRefusesMissingOut(t *testing.T) {
	if err := run(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
}

func TestRefusesInsideRepo(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(sub, "key.txt")
	var stdout bytes.Buffer
	if err := run([]string{"-out", out}, &stdout); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(out); err == nil || stdout.Len() != 0 {
		t.Fatal("file written or output printed")
	}
}

func TestRefusesSymlinkIntoRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(link, "key.txt")
	var stdout bytes.Buffer
	if err := run([]string{"-out", out}, &stdout); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Join(repo, "key.txt")); err == nil || stdout.Len() != 0 {
		t.Fatal("file written or output printed")
	}
}
