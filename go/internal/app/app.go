// Package app wires the tray app together: single instance, deploy, the
// settings server, the tray and quitting.
package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/autostart"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/deploy"
	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/hooks"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/openurl"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/settingswin"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/tray"
	"github.com/quanht99/agent-chime/go/internal/update"
	"github.com/quanht99/agent-chime/go/internal/version"
	"github.com/quanht99/agent-chime/go/internal/web"
)

// TrayModel is the pure menu content for the given config and state.
func TrayModel(c config.Config, st state.State, us update.Status, now time.Time) tray.Model {
	lang := c.Language
	t := func(k i18n.Key, a ...string) string { return i18n.T(k, lang, a...) }
	m := tray.Model{
		Status:    t("statusOn"),
		LastAlert: t("menuNoAlert"),
		Muted:     c.IsMuted(now),
		Labels: tray.Labels{
			Mute: t("menuMute"), Mute30: t("mute30"), Mute60: t("mute60"), MuteForever: t("muteForever"),
			Unmute: t("menuUnmute"), Settings: t("menuSettings"), Quit: t("menuQuit"),
		},
	}
	if m.Muted {
		if c.MutedUntil.Equal(model.Forever) {
			m.Status = t("statusMuted")
		} else {
			m.Status = t("statusMutedUntil", model.HHMM(c.MutedUntil.Time))
		}
	}
	if us.CanInstall() {
		m.UpdateLabel = t("updateMenuItem", us.Latest)
	}
	if a := st.LastAlert; a != nil {
		m.LastAlert = t("menuLast", a.Summary(lang), model.HHMM(a.Date.Time))
	}
	return m
}

func muteConfig(p paths.Paths, d *time.Duration, now time.Time) {
	c := config.Load(p)
	c.Mute(d, now)
	if err := config.Save(p, c); err != nil {
		applog.Write(p, "tray: mute failed: "+err.Error())
	}
}

func unmuteConfig(p paths.Paths) {
	c := config.Load(p)
	c.Unmute()
	if err := config.Save(p, c); err != nil {
		applog.Write(p, "tray: unmute failed: "+err.Error())
	}
}

func markSettingsShown(p paths.Paths) {
	c := config.Load(p)
	if c.SettingsShownOnce {
		return
	}
	c.SettingsShownOnce = true
	if err := config.Save(p, c); err != nil {
		applog.Write(p, "settingsShownOnce save failed: "+err.Error())
	}
}

// heldLock is app.lock, kept open (and so locked) for the whole life of the app process.
var heldLock *os.File

func deploySelf(p paths.Paths) {
	if self, err := os.Executable(); err == nil {
		if _, err := deploy.Sync(p, self, sounds.BuiltinFS); err != nil {
			applog.Write(p, "deploy failed: "+err.Error())
		}
	}
}

// handOff deploys this copy (so starting a newer version updates the installed one even while the
// old one runs), then opens the running instance's page. When this copy is newer than the
// installed one it asks the running instance to restart into it instead.
func handOff(p paths.Paths, background bool, act startAction) {
	if act != startAsIs {
		deploySelf(p)
	}
	info, ok := waitForInstance(p, handOffWait, 100*time.Millisecond)
	if !ok {
		applog.Write(p, "another AgentChime holds app.lock but did not answer")
		return
	}
	if act == startDeployNewer {
		if err := postRestart(info); err == nil {
			return
		} else {
			applog.Write(p, "restart of the running AgentChime failed: "+err.Error())
		}
	}
	if !background {
		showSettings(p, info)
	}
}

const handOffWait = 10 * time.Second

// Run starts the app. With background it never opens the settings page.
func Run(background bool) {
	p := paths.Current()
	_ = os.MkdirAll(p.DataDir, 0o755)

	selfIsData := selfIsInstalled(p)
	if selfIsData {
		// A newer .deb: deploy it, then run it. If the deploy fails, never exec the data copy,
		// which is still this one: start normally.
		if f, ok := debNewer(p); ok {
			if _, err := deploy.SyncFiles(p, f, sounds.BuiltinFS); err != nil {
				applog.Write(p, "deb: deploy failed: "+err.Error())
			} else {
				v, _ := version.Of(f.Program)
				applog.Write(p, "deb: deployed "+v)
				if err := runDataCopy(p, os.Args[1:]); err == nil {
					return
				} else {
					applog.Write(p, "deb: cannot start the deployed copy: "+err.Error())
				}
			}
		}
	}
	dataVer, _ := version.Of(p.Program)
	_, statErr := os.Stat(p.Program)
	act := chooseStart(version.Version, dataVer, selfIsData, statErr == nil)
	if act == startRunData {
		if err := runDataCopy(p, os.Args[1:]); err == nil {
			return
		} else {
			applog.Write(p, "cannot start the installed newer copy: "+err.Error())
			act = startAsIs // never downgrade it
		}
	}

	f, err := lockApp(p)
	if errors.Is(err, fsx.ErrLocked) {
		handOff(p, background, act)
		return
	}
	if err != nil {
		applog.Write(p, "cannot lock app.lock, relying on instance.json only: "+err.Error())
	}
	heldLock = f
	// An instance that does not take the lock (an older build) still answers the ping.
	if info, ok := instance.Read(p); ok && instance.Ping(info, time.Second) {
		if act != startAsIs {
			deploySelf(p)
		}
		if act == startDeployNewer && postRestart(info) == nil {
			return
		}
		if !background {
			showSettings(p, info)
		}
		return
	}

	cleanUpdateDir(p) // we hold the lock, so any leftover is from a dead attempt
	if act != startAsIs {
		deploySelf(p)
	}
	serve(p, serveOpts{background: background})
}

// RunAfterUpdate is the new process started by the updater: it serves and says "ready" at once,
// then waits for the old app to release app.lock before it takes over.
func RunAfterUpdate() {
	p := paths.Current()
	_ = os.MkdirAll(p.DataDir, 0o755)
	deploySelf(p)
	serve(p, serveOpts{afterUpdate: true})
}

type serveOpts struct{ background, afterUpdate bool }

// serve runs the web server, the updater and the tray until quit. With afterUpdate it prints
// "ready" first and writes instance.json, cleans the update folder and starts the tray and the
// updater only after it holds app.lock.
func serve(p paths.Paths, opts serveOpts) {
	lib := sounds.Library{P: p, GOOS: runtime.GOOS, Players: audio.FindPlayers(exec.LookPath)}

	var (
		srv      *web.Server
		quitOnce sync.Once
		owner    atomic.Bool // this process owns instance.json
	)
	owner.Store(!opts.afterUpdate)
	quit := func() {
		quitOnce.Do(func() {
			if owner.Load() {
				instance.Remove(p)
			}
			if srv != nil {
				_ = srv.Close()
			}
			// On Windows fyne removes the notify icon only in its own quit path, which os.Exit skips.
			tray.Quit(time.Second)
			if heldLock != nil {
				_ = heldLock.Close()
			}
			os.Exit(0)
		})
	}
	token := web.NewToken()
	u := newUpdater(p, quit, opts.afterUpdate)
	srv = web.New(web.Deps{
		P: p, Lib: lib, GOOS: runtime.GOOS, Now: time.Now,
		Hooks: hooks.Format{GOOS: runtime.GOOS, Program: p.Program, ShortPath: shortPath},
		Preview: func(path string, volume float64) (func(), error) {
			return preview(p, lib.Players, path, volume)
		},
		OpenURL:       openurl.Open,
		Quit:          quit,
		TrayAvailable: tray.Available,
		ProgramExists: func() bool { _, err := os.Stat(p.Program); return err == nil },
		Autostart:     autostart.Control{P: p},
		Updates:       u,
	}, token)
	port, err := srv.Start()
	if err != nil {
		applog.Write(p, "server start failed: "+err.Error())
		os.Exit(1)
	}
	if opts.afterUpdate {
		fmt.Println("ready")
		_ = settingswin.DetachStdout()
		f, err := waitForLock(p, 100*time.Millisecond, 30*time.Second)
		if err != nil {
			applog.Write(p, "after update: the old AgentChime did not quit")
			_ = srv.Close()
			os.Exit(1)
		}
		heldLock = f
		cleanUpdateDir(p) // the old app has quit, so the folder is no one's
		owner.Store(true)
	}
	info := instance.Info{Port: port, Token: token, PID: os.Getpid()}
	if err := instance.Write(p, info); err != nil {
		applog.Write(p, "instance write failed: "+err.Error())
		_ = srv.Close()
		os.Exit(1)
	}

	go u.loop(make(chan struct{}), time.After)

	if !opts.background {
		go showSettings(p, info)
	}
	markSettingsShown(p)

	// On Linux the tray runs whenever there is a session bus: fyne registers the icon again when a
	// StatusNotifierWatcher appears later (a panel that starts after us at login). Without a bus at
	// all (containers, CI) the app keeps serving its page until Quit.
	if tray.CanRun() {
		tray.Run(func() tray.Model {
			return TrayModel(config.Load(p), state.Store{P: p}.Load(), u.Status(), time.Now())
		}, tray.Actions{
			Mute:         func(d *time.Duration) { muteConfig(p, d, time.Now()) },
			Unmute:       func() { unmuteConfig(p) },
			OpenSettings: func() { go showSettings(p, info) },
			Update:       u.StartInstall,
			Quit:         quit,
		}, nil)
		quit() // the tray loop ended on its own
		return
	}
	select {}
}
