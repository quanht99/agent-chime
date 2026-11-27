package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/deploy"
	"github.com/quanht99/agent-chime/go/internal/openurl"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/update"
	"github.com/quanht99/agent-chime/go/internal/version"
)

const (
	readyTimeout   = 20 * time.Second
	versionTimeout = 10 * time.Second
	checkTimeout   = 2 * time.Minute
)

// updater checks for, installs and hands over to new releases. Only the long-running app owns one.
type updater struct {
	p            paths.Paths
	client       *update.Client // nil: source build
	goos, goarch string
	now          func() time.Time
	quit         func() // the app's quit
	start        func(path string, args []string, timeout time.Duration, exit0IsReady bool) error
	mu           sync.Mutex
	st           update.Status
	latest       *update.Manifest
	deployed     string // version of the copy deployed while in RestartNeeded
	busy         bool
	loggedNoKey  bool
}

func newUpdater(p paths.Paths, quit func(), afterUpdate bool) *updater {
	u := &updater{
		p: p, goos: runtime.GOOS, goarch: runtime.GOARCH, now: time.Now,
		quit: quit, start: startWaitReady,
		st: update.Status{Phase: update.Idle, Current: version.Version},
	}
	switch {
	case version.Version == "dev":
		u.st.Phase = update.SourceBuild
	case afterUpdate:
		u.st.Phase = update.Updated
		u.client = update.NewClient()
	default:
		u.client = update.NewClient()
	}
	return u
}

// cleanUpdateDir removes <DataDir>/update. Call it only while holding app.lock: before that the
// folder may belong to a running instance's download.
func cleanUpdateDir(p paths.Paths) { _ = os.RemoveAll(updateDir(p)) }

func updateDir(p paths.Paths) string { return filepath.Join(p.DataDir, "update") }

// debCheck (Linux) deploys a newer .deb pair and restarts into it, or restarts into a newer copy
// that someone else deployed while this one ran. It holds no lock while it deploys or restarts.
func (u *updater) debCheck() {
	if u.goos != "linux" {
		return
	}
	if f, ok := debNewer(u.p); ok {
		if _, err := deploy.SyncFiles(u.p, f, sounds.BuiltinFS); err != nil {
			u.log("deb: deploy failed: " + err.Error())
			return
		}
		v, _ := version.Of(f.Program)
		u.log("deb: deployed " + v)
		_ = u.Restart()
		return
	}
	if dv, err := version.Of(u.p.Program); err == nil {
		if c, ok := update.Compare(dv, version.Version); ok && c == 1 {
			_ = u.Restart()
		}
	}
}

func (u *updater) Status() update.Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

func (u *updater) setPhase(ph update.Phase, detail string) {
	u.mu.Lock()
	u.st.Phase, u.st.Detail = ph, detail
	u.mu.Unlock()
}

func (u *updater) log(msg string) { applog.Write(u.p, msg) }

// CheckNow fetches the manifest and records the result. It does nothing in a source build or while
// another check or install runs.
func (u *updater) CheckNow() {
	if prev, ok := u.beginCheck(); ok {
		u.runCheck(prev)
	}
}

// StartCheck is CheckNow in the background: the Checking phase is set before it returns, so the
// caller's next Status already shows it.
func (u *updater) StartCheck() {
	if prev, ok := u.beginCheck(); ok {
		go u.runCheck(prev)
	}
}

// beginCheck claims the updater and sets Checking. It reports false when nothing should run.
func (u *updater) beginCheck() (update.Phase, bool) {
	if u.client == nil {
		return "", false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy {
		return "", false
	}
	u.busy = true
	prev := u.st.Phase
	u.st.Phase, u.st.Detail = update.Checking, ""
	return prev, true
}

func (u *updater) runCheck(prev update.Phase) {
	if len(u.client.Key) == 0 {
		// No release key compiled in: nothing could be verified, so make no request at all.
		u.mu.Lock()
		u.busy = false
		u.st.Phase = update.Unverified
		first := !u.loggedNoKey
		u.loggedNoKey = true
		u.mu.Unlock()
		if first {
			u.log("update check skipped: no release key in this build")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	m, err := u.client.Latest(ctx)

	u.mu.Lock()
	defer u.mu.Unlock()
	u.busy = false
	sticky := prev == update.Updated || prev == update.RestartNeeded
	if err != nil {
		switch {
		case sticky:
			u.st.Phase = prev
		case errors.Is(err, update.ErrUnverified):
			u.st.Phase = update.Unverified
		default:
			u.st.Phase = update.CheckFailed
		}
		u.log("update check: " + err.Error())
		return
	}
	t := u.now()
	u.st.LastSuccess = &t
	u.st.Latest = m.Version
	u.st.ReleasePage = m.ReleasePage
	u.latest = &m
	// A sticky phase survives only while nothing newer than what it already covers is known.
	covered := u.st.Current
	if prev == update.RestartNeeded && u.deployed != "" {
		covered = u.deployed
	}
	newerThanCovered := false
	if c, ok := update.Compare(m.Version, covered); ok && c == 1 {
		newerThanCovered = true
	}
	switch {
	case sticky && !newerThanCovered:
		u.st.Phase = prev
	default:
		if c, ok := update.Compare(m.Version, u.st.Current); ok && c == 1 {
			u.st.Phase = update.Available
		} else {
			u.st.Phase = update.UpToDate
		}
	}
}

func copyTo(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Install downloads, verifies and deploys the known newer version, then hands over to it.
func (u *updater) Install() {
	if m, ok := u.beginInstall(); ok {
		u.runInstall(m)
	}
}

// StartInstall is Install in the background: the Downloading phase is set before it returns.
func (u *updater) StartInstall() {
	if m, ok := u.beginInstall(); ok {
		go u.runInstall(m)
	}
}

// beginInstall claims the updater and sets Downloading. It reports false when nothing should run.
func (u *updater) beginInstall() (update.Manifest, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.client == nil || u.busy || u.latest == nil || !u.st.CanInstall() {
		return update.Manifest{}, false
	}
	u.busy = true
	u.st.Phase, u.st.Detail = update.Downloading, ""
	return *u.latest, true
}

func (u *updater) runInstall(m update.Manifest) {
	p := u.p
	dir := updateDir(p)
	defer func() {
		_ = os.RemoveAll(dir)
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()

	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		u.log("update: " + err.Error())
		u.setPhase(update.Failed, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	archive, used, err := u.client.Download(ctx, m, u.goos, u.goarch, dir)
	if err != nil {
		u.log("update download: " + err.Error())
		if errors.Is(err, update.ErrUnverified) {
			u.setPhase(update.Unverified, "")
		} else {
			u.setPhase(update.Failed, err.Error())
		}
		return
	}
	// A refetch inside Download may have returned another manifest: never install anything that
	// is not strictly newer than the running version.
	if c, ok := update.Compare(used.Version, u.Status().Current); !ok || c != 1 {
		u.log("update: refetched manifest " + used.Version + " is not newer; nothing installed")
		u.setPhase(update.Unverified, "")
		return
	}
	prog, helper := filepath.Base(p.Program), filepath.Base(p.SettingsHelper)
	newDir := filepath.Join(dir, "new")
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		u.setPhase(update.Failed, err.Error())
		return
	}
	files, err := update.Extract(archive, newDir, []string{prog, helper})
	if err != nil {
		u.log("update extract: " + err.Error())
		u.setPhase(update.Unverified, "")
		return
	}
	newProg, newHelper := files[prog], files[helper]

	vctx, vcancel := context.WithTimeout(ctx, versionTimeout)
	out, err := exec.CommandContext(vctx, newProg, "--version").Output()
	vcancel()
	if err != nil || strings.TrimSpace(string(out)) != used.Version {
		got := strings.TrimSpace(string(out))
		u.log(fmt.Sprintf("update: new build failed its --version check (got %q, want %q, err %v)", got, used.Version, err))
		u.setPhase(update.BadBuild, "")
		return
	}

	prev := filepath.Join(dir, "previous")
	if err := os.MkdirAll(prev, 0o700); err != nil {
		u.setPhase(update.Failed, err.Error())
		return
	}
	var old deploy.Files
	if err := copyTo(filepath.Join(prev, prog), p.Program); err != nil {
		u.log("update backup: " + err.Error())
		u.setPhase(update.Failed, err.Error())
		return
	}
	old.Program = filepath.Join(prev, prog)
	if _, err := os.Stat(p.SettingsHelper); err == nil {
		if err := copyTo(filepath.Join(prev, helper), p.SettingsHelper); err != nil {
			u.log("update backup: " + err.Error())
			u.setPhase(update.Failed, err.Error())
			return
		}
		old.Helper = filepath.Join(prev, helper)
	}
	restore := func() error {
		_, err := deploy.SyncFiles(p, old, sounds.BuiltinFS)
		return err
	}

	if _, err := deploy.SyncFiles(p, deploy.Files{Program: newProg, Helper: newHelper}, sounds.BuiltinFS); err != nil {
		u.log("update deploy: " + err.Error())
		if rerr := restore(); rerr != nil {
			u.log("update restore: " + rerr.Error())
		}
		u.setPhase(update.Failed, err.Error())
		return
	}

	err = u.start(p.Program, []string{"--after-update"}, readyTimeout, false)
	switch {
	case err == nil:
		u.log("update: handing over to " + used.Version)
		_ = os.RemoveAll(dir)
		u.quit()
	case errors.Is(err, errNoReady):
		u.log("update: " + err.Error())
		u.mu.Lock()
		u.deployed = used.Version
		u.mu.Unlock()
		u.setPhase(update.RestartNeeded, "")
	default:
		u.log("update: new version did not start: " + err.Error())
		if rerr := restore(); rerr != nil {
			u.log("update restore: " + rerr.Error())
		}
		u.setPhase(update.BadBuild, "")
	}
}

// Restart hands over to the deployed program without downloading anything.
func (u *updater) Restart() error {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return errors.New("update: busy")
	}
	u.busy = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()
	err := u.start(u.p.Program, []string{"--after-update"}, readyTimeout, false)
	if err == nil {
		u.log("update: restarting into the deployed version")
		u.quit()
		return nil
	}
	u.log("update restart: " + err.Error())
	if errors.Is(err, errNoReady) {
		if dv, verr := version.Of(u.p.Program); verr == nil {
			u.mu.Lock()
			u.deployed = dv
			u.mu.Unlock()
		}
		u.setPhase(update.RestartNeeded, "")
	}
	return err
}

// OpenReleasePage opens the release page, or the releases page when the known one is not ours.
func (u *updater) OpenReleasePage() error {
	page := u.Status().ReleasePage
	if !strings.HasPrefix(page, update.ReleaseBase) {
		page = update.ReleaseBase + "latest"
	}
	return openurl.Open(page)
}

func (u *updater) loop(stop <-chan struct{}, after func(time.Duration) <-chan time.Time) {
	d := time.Minute
	for {
		select {
		case <-stop:
			return
		case <-after(d):
		}
		d = 24 * time.Hour
		u.debCheck()
		if u.client != nil && config.Load(u.p).CheckForUpdates {
			u.CheckNow()
		}
	}
}
