package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func build(t *testing.T) string {
	t.Helper()
	name := "agentchime"
	args := []string{"build"}
	if runtime.GOOS == "windows" {
		name += ".exe"
		args = append(args, "-ldflags", "-H=windowsgui")
	}
	bin := filepath.Join(t.TempDir(), name)
	args = append(args, "-o", bin, "./")
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// cleanEnv drops anything that could make the hook look nested or headless.
func cleanEnv(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if strings.HasPrefix(k, "CLAUDE") || strings.HasPrefix(k, "CODEX") || strings.HasPrefix(k, "AGENTCHIME_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "AGENTCHIME_HOME="+home)
}

func runBin(t *testing.T, bin, home, stdin string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = cleanEnv(home)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("%v: unexpected output %q", args, out.String())
	}
}

func TestHookEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	p := testenv.Home(t) // sets AGENTCHIME_HOME for this test process
	home := os.Getenv("AGENTCHIME_HOME")
	// Muted forever, so no step can ever reach a sound player.
	c := config.Default()
	c.Mute(nil, time.Now())
	if err := config.Save(p, c); err != nil {
		t.Fatal(err)
	}
	bin := build(t)

	runBin(t, bin, home, `{"session_id":"s1","prompt":"hi","hook_event_name":"UserPromptSubmit"}`, "hook", "claude-prompt")
	if _, ok := (state.Store{P: p}).Load().TurnStarts["claude|s1"]; !ok {
		t.Fatal("state.json has no claude|s1 turn start")
	}
	runBin(t, bin, home, "not json", "hook", "nope")
	runBin(t, bin, home, "not json", "hook", "claude-stop")
	runBin(t, bin, home, "", "hook")
	runBin(t, bin, home, "", "play", "--volume", "bad")
	if s := (state.Store{P: p}).Load().LastSkip; s == nil || s.Reason.Kind != "muted" {
		t.Fatalf("lastSkip: %+v", s)
	}
}

func TestHookWithOpenStdinStillExits(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	testenv.Home(t)
	home := os.Getenv("AGENTCHIME_HOME")
	bin := build(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "hook", "claude-prompt")
	cmd.Env = cleanEnv(home)
	in, err := cmd.StdinPipe() // never written, never closed
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v", d)
	}
}
