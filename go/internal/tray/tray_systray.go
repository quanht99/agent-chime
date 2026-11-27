//go:build windows || linux

package tray

import (
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

const refreshEvery = 5 * time.Second

var (
	running  atomic.Bool
	exited   = make(chan struct{})
	exitOnce sync.Once
)

// Run shows the tray icon and blocks until the menu's Quit runs (which exits
// the process). ready is called once the menu exists.
func Run(model func() Model, act Actions, ready func()) {
	running.Store(true)
	onReady := func() {
		systray.SetIcon(iconBytes)
		systray.SetTooltip("AgentChime")
		m := model()
		upd := systray.AddMenuItem(m.UpdateLabel, "")
		status := systray.AddMenuItem(m.Status, "")
		status.Disable()
		last := systray.AddMenuItem(m.LastAlert, "")
		last.Disable()
		systray.AddSeparator()
		mute := systray.AddMenuItem(m.Labels.Mute, "")
		m30 := mute.AddSubMenuItem(m.Labels.Mute30, "")
		m60 := mute.AddSubMenuItem(m.Labels.Mute60, "")
		mForever := mute.AddSubMenuItem(m.Labels.MuteForever, "")
		unmute := systray.AddMenuItem(m.Labels.Unmute, "")
		systray.AddSeparator()
		settings := systray.AddMenuItem(m.Labels.Settings, "")
		quit := systray.AddMenuItem(m.Labels.Quit, "")

		refresh := func() {
			m := model()
			if m.UpdateLabel == "" {
				upd.Hide()
			} else {
				upd.SetTitle(m.UpdateLabel)
				upd.Show()
			}
			status.SetTitle(m.Status)
			last.SetTitle(m.LastAlert)
			mute.SetTitle(m.Labels.Mute)
			m30.SetTitle(m.Labels.Mute30)
			m60.SetTitle(m.Labels.Mute60)
			mForever.SetTitle(m.Labels.MuteForever)
			unmute.SetTitle(m.Labels.Unmute)
			settings.SetTitle(m.Labels.Settings)
			quit.SetTitle(m.Labels.Quit)
			if m.Muted {
				mute.Hide()
				unmute.Show()
			} else {
				unmute.Hide()
				mute.Show()
			}
		}
		refresh()

		go func() {
			tick := time.NewTicker(refreshEvery)
			defer tick.Stop()
			dur := func(min int) *time.Duration { d := time.Duration(min) * time.Minute; return &d }
			for {
				select {
				case <-tick.C:
					refresh()
					continue
				case <-m30.ClickedCh:
					act.Mute(dur(30))
				case <-m60.ClickedCh:
					act.Mute(dur(60))
				case <-mForever.ClickedCh:
					act.Mute(nil)
				case <-unmute.ClickedCh:
					act.Unmute()
				case <-upd.ClickedCh:
					if act.Update != nil {
						act.Update()
					}
				case <-settings.ClickedCh:
					act.OpenSettings()
				case <-quit.ClickedCh:
					act.Quit()
				}
				refresh()
			}
		}()
		if ready != nil {
			ready()
		}
	}
	systray.Run(onReady, func() { exitOnce.Do(func() { close(exited) }) })
}
