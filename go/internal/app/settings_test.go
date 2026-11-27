package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/settingswin"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("AGENTCHIME_FAKE_HELPER"); mode != "" {
		fakeHelper(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeHelper(mode string) {
	dir := os.Getenv("AGENTCHIME_FAKE_HELPER_DIR")
	switch mode {
	case "ready":
		os.Stdout.WriteString("ready\n")
		time.Sleep(5 * time.Second)
	case "exit0":
	case "exit3":
		os.Exit(3)
	case "readyexit":
		os.Stdout.WriteString("ready\n")
		os.Exit(1)
	case "silent":
		time.Sleep(1500 * time.Millisecond)
		os.WriteFile(filepath.Join(dir, "survived"), nil, 0o600)
	case "args":
		os.WriteFile(filepath.Join(dir, "args"), []byte(strings.Join(os.Args[1:], "\n")), 0o600)
		os.Stdout.WriteString("ready\n")
	case "chatty":
		os.Stdout.WriteString("hello\nreadyish\nready\n")
	case "late":
		os.Stdout.WriteString("ready\n")
		settingswin.DetachStdout()
		time.Sleep(300 * time.Millisecond)
		os.Stdout.WriteString("late\n")
		os.WriteFile(filepath.Join(dir, "alive"), nil, 0o600)
	}
}

func fakeSetup(t *testing.T, mode string) (paths.Paths, string) {
	t.Helper()
	p := testenv.Home(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p.SettingsHelper = exe
	dir := t.TempDir()
	t.Setenv("AGENTCHIME_FAKE_HELPER", mode)
	t.Setenv("AGENTCHIME_FAKE_HELPER_DIR", dir)
	return p, dir
}

var testInfo = instance.Info{Port: 4242, Token: "tok", PID: 1}

func TestShowSettingsMissingHelperOpensBrowser(t *testing.T) {
	p := testenv.Home(t)
	p.SettingsHelper = filepath.Join(t.TempDir(), "missing")
	var opened []string
	showSettingsWith(p, testInfo, 15*time.Second, func(u string) error { opened = append(opened, u); return nil })
	if len(opened) != 1 || opened[0] != instance.URL(testInfo) {
		t.Fatalf("opened = %v", opened)
	}
	b, _ := os.ReadFile(p.Log)
	if !strings.Contains(string(b), "settings window: ") || !strings.Contains(string(b), "; opening the browser") {
		t.Fatalf("log = %q", b)
	}
}

func TestShowSettingsNonZeroExitOpensBrowser(t *testing.T) {
	p, _ := fakeSetup(t, "exit3")
	err := startHelper(p.SettingsHelper, 15*time.Second)
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("err = %v", err)
	}
	n := 0
	showSettingsWith(p, testInfo, 15*time.Second, func(string) error { n++; return nil })
	if n != 1 {
		t.Fatalf("open calls = %d", n)
	}
}

func TestShowSettingsExitZeroBeforeReady(t *testing.T) {
	p, _ := fakeSetup(t, "exit0")
	if err := startHelper(p.SettingsHelper, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	showSettingsWith(p, testInfo, 15*time.Second, func(string) error { t.Error("open called"); return nil })
}

func TestShowSettingsReady(t *testing.T) {
	p, _ := fakeSetup(t, "ready")
	start := time.Now()
	if err := startHelper(p.SettingsHelper, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
}

func TestShowSettingsSilentHelperIsKilled(t *testing.T) {
	p, dir := fakeSetup(t, "silent")
	start := time.Now()
	n := 0
	err := startHelper(p.SettingsHelper, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "no ready within") {
		t.Fatalf("err = %v", err)
	}
	showSettingsWith(p, testInfo, 300*time.Millisecond, func(string) error { n++; return nil })
	if n != 1 {
		t.Fatalf("open calls = %d", n)
	}
	time.Sleep(2500*time.Millisecond - time.Since(start))
	if _, err := os.Stat(filepath.Join(dir, "survived")); err == nil {
		t.Fatal("helper was not killed")
	}
}

func TestHelperGetsNoArguments(t *testing.T) {
	p, dir := fakeSetup(t, "args")
	if err := startHelper(p.SettingsHelper, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil || len(b) != 0 {
		t.Fatalf("args = %q, err = %v", b, err)
	}
}

func TestReadyAfterOtherOutput(t *testing.T) {
	p, _ := fakeSetup(t, "chatty")
	if err := startHelper(p.SettingsHelper, 15*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestHelperSurvivesAfterReady(t *testing.T) {
	p, dir := fakeSetup(t, "late")
	if err := startHelper(p.SettingsHelper, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "alive")); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("helper died after ready")
}

func TestStartWaitReadyPassesArgs(t *testing.T) {
	p, dir := fakeSetup(t, "args")
	if err := startWaitReady(p.SettingsHelper, []string{"--after-update"}, 15*time.Second, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil || string(b) != "--after-update" {
		t.Fatalf("args = %q, err = %v", b, err)
	}
}

func TestStartWaitReadyExit0NotReady(t *testing.T) {
	p, _ := fakeSetup(t, "exit0")
	err := startWaitReady(p.SettingsHelper, nil, 15*time.Second, false)
	if !errors.Is(err, errExitedEarly) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartWaitReadyTimeout(t *testing.T) {
	p, _ := fakeSetup(t, "silent")
	err := startWaitReady(p.SettingsHelper, nil, 300*time.Millisecond, true)
	if !errors.Is(err, errNoReady) || !strings.Contains(err.Error(), "no ready within 300ms") {
		t.Fatalf("err = %v", err)
	}
}

func TestStartWaitReadyReadyThenExitIsReady(t *testing.T) {
	p, _ := fakeSetup(t, "readyexit")
	for i := 0; i < 20; i++ {
		if err := startWaitReady(p.SettingsHelper, nil, 15*time.Second, false); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}
