package applog

import (
	"bytes"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestLogLineFormat(t *testing.T) {
	p := testenv.Home(t)
	WriteAt(p, "hello", time.Date(2026, 11, 27, 3, 4, 5, 0, time.FixedZone("x", 7*3600)))
	b, err := os.ReadFile(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ hello\n$`).Match(b) {
		t.Fatalf("bad line %q", b)
	}
	if !strings.HasPrefix(string(b), "2026-11-26T20:04:05Z") {
		t.Fatalf("not UTC: %q", b)
	}
}

func TestLogAppends(t *testing.T) {
	p := testenv.Home(t)
	Write(p, "a")
	Write(p, "b")
	b, _ := os.ReadFile(p.Log)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], " a") || !strings.HasSuffix(lines[1], " b") {
		t.Fatalf("lines %q", lines)
	}
}

func TestLogRotatesAtMaxBytes(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := bytes.Repeat([]byte("a"), MaxBytes)
	if err := os.WriteFile(p.Log, old, 0o644); err != nil {
		t.Fatal(err)
	}
	Write(p, "x")
	rotated, err := os.ReadFile(p.LogOld)
	if err != nil || !bytes.Equal(rotated, old) {
		t.Fatalf("rotated file missing or wrong: %v", err)
	}
	b, _ := os.ReadFile(p.Log)
	if lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"); len(lines) != 1 || !strings.HasSuffix(lines[0], " x") {
		t.Fatalf("new log %q", b)
	}
}

func TestLogCreatesDirAndMode(t *testing.T) {
	p := testenv.Home(t)
	if _, err := os.Stat(p.DataDir); !os.IsNotExist(err) {
		t.Fatal("dir should not exist yet")
	}
	Write(p, "hi")
	fi, err := os.Stat(p.Log)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}
