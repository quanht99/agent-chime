// Package app wires the tray app together: single instance, deploy, the
// settings server, the tray and quitting.
package app

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync"
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
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/tray"
	"github.com/quanht99/agent-chime/go/internal/web"
)

// TrayModel is the pure menu content for the given config and state.
func TrayModel(c config.Config, st state.State, now time.Time) tray.Model {
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
// old one runs), then opens the running instance's page.
func handOff(p paths.Paths, background bool) {
	deploySelf(p)
	info, ok := waitForInstance(p, handOffWait, 100*time.Millisecond)
	if !ok {
		applog.Write(p, "another AgentChime holds app.lock but did not answer")
		return
	}
	if !background {
		_ = openurl.Open(instance.URL(info))
	}
}

const handOffWait = 10 * time.Second

// Run starts the app. With background it never opens the settings page.
func Run(background bool) {
	p := paths.Current()
	_ = os.MkdirAll(p.DataDir, 0o755)

	f, err := lockApp(p)
	if errors.Is(err, fsx.ErrLocked) {
		handOff(p, background)
		return
	}
	if err != nil {
		applog.Write(p, "cannot lock app.lock, relying on instance.json only: "+err.Error())
	}
	heldLock = f
	// An instance that does not take the lock (an older build) still answers the ping.
	if info, ok := instance.Read(p); ok && instance.Ping(info, time.Second) {
		deploySelf(p)
		if !background {
			_ = openurl.Open(instance.URL(info))
		}
		return
	}

	deploySelf(p)

	lib := sounds.Library{P: p, GOOS: runtime.GOOS, Players: audio.FindPlayers(exec.LookPath)}

	var (
		srv      *web.Server
		quitOnce sync.Once
	)
	quit := func() {
		quitOnce.Do(func() {
			instance.Remove(p)
			if srv != nil {
				_ = srv.Close()
			}
			// On Windows fyne removes the notify icon only in its own quit path, which os.Exit skips.
			tray.Quit(time.Second)
			os.Exit(0)
		})
	}

	token := web.NewToken()
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
	}, token)
	port, err := srv.Start()
	if err != nil {
		applog.Write(p, "server start failed: "+err.Error())
		os.Exit(1)
	}
	info := instance.Info{Port: port, Token: token, PID: os.Getpid()}
	if err := instance.Write(p, info); err != nil {
		applog.Write(p, "instance write failed: "+err.Error())
		_ = srv.Close()
		os.Exit(1)
	}

	if !background {
		_ = openurl.Open(instance.URL(info))
	}
	markSettingsShown(p)

	// On Linux the tray runs whenever there is a session bus: fyne registers the icon again when a
	// StatusNotifierWatcher appears later (a panel that starts after us at login). Without a bus at
	// all (containers, CI) the app keeps serving its page until Quit.
	if tray.CanRun() {
		tray.Run(func() tray.Model {
			return TrayModel(config.Load(p), state.Store{P: p}.Load(), time.Now())
		}, tray.Actions{
			Mute:         func(d *time.Duration) { muteConfig(p, d, time.Now()) },
			Unmute:       func() { unmuteConfig(p) },
			OpenSettings: func() { _ = openurl.Open(instance.URL(info)) },
			Quit:         quit,
		}, nil)
		quit() // the tray loop ended on its own
		return
	}
	select {}
}
