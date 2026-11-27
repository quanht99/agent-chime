//go:build windows

package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestMain(m *testing.M) {
	if os.Getenv("AGENTCHIME_DEPLOY_SLEEP") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	os.Exit(m.Run())
}

// An open settings window locks the helper exe; Sync must still replace it
// by renaming the running file aside, and a later Sync removes the leftover.
func TestSyncReplacesHelperInUse(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(filepath.Dir(p.SettingsHelper), 0o755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.SettingsHelper, data, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(p.SettingsHelper)
	cmd.Env = append(os.Environ(), "AGENTCHIME_DEPLOY_SLEEP=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	killed := false
	stop := func() {
		if !killed {
			killed = true
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}
	defer stop()

	self := mkSelfWithHelper(t, p, "NEW")
	if _, err := Sync(p, self, mapFS(nil)); err != nil {
		t.Fatal(err)
	}
	if read(t, p.SettingsHelper) != "NEW" {
		t.Fatal("helper not replaced")
	}
	if olds, _ := filepath.Glob(p.SettingsHelper + ".old-*"); len(olds) == 0 {
		t.Fatal("expected the running copy to be renamed to .old-N")
	}

	stop()
	if _, err := Sync(p, self, mapFS(nil)); err != nil {
		t.Fatal(err)
	}
	if olds, _ := filepath.Glob(p.SettingsHelper + ".old-*"); len(olds) != 0 {
		t.Fatalf("leftover %v", olds)
	}
}

// A running program exe is replaced the same way: renamed aside to .old-N.
func TestSyncFilesReplacesProgramInUse(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(filepath.Dir(p.Program), 0o755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Program, data, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(p.Program)
	cmd.Env = append(os.Environ(), "AGENTCHIME_DEPLOY_SLEEP=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	src := t.TempDir()
	newProg := filepath.Join(src, "prog")
	if err := os.WriteFile(newProg, []byte("NEW"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncFiles(p, Files{Program: newProg}, mapFS(nil)); err != nil {
		t.Fatal(err)
	}
	if read(t, p.Program) != "NEW" {
		t.Fatal("program not replaced")
	}
	if olds, _ := filepath.Glob(p.Program + ".old-*"); len(olds) == 0 {
		t.Fatal("expected the running copy to be renamed to .old-N")
	}
}
