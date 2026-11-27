package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func build(t *testing.T, ldflags string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a binary")
	}
	bin := filepath.Join(t.TempDir(), "a")
	args := []string{"build"}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "-o", bin, "./cmd/agentchime")
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestOfReadsLdflags(t *testing.T) {
	bin := build(t, "-s -w -X github.com/quanht99/agent-chime/go/internal/version.Version=1.2.3")
	got, err := Of(bin)
	if err != nil || got != "1.2.3" {
		t.Fatalf("Of = %q, %v", got, err)
	}
}

func TestOfDevBuild(t *testing.T) {
	got, err := Of(build(t, ""))
	if err != nil || got != "dev" {
		t.Fatalf("Of = %q, %v", got, err)
	}
}

func TestOfNotGo(t *testing.T) {
	txt := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Of(txt); err == nil {
		t.Fatal("text file: want error")
	}
	if _, err := Of(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing: want error")
	}
}
