package app

import (
	"strings"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/state"
	"github.com/quanht99/agent-chime/go/internal/testenv"
	"github.com/quanht99/agent-chime/go/internal/update"
)

var now = time.Date(2026, 1, 2, 12, 0, 0, 0, time.Local)

func TestTrayModel(t *testing.T) {
	c := config.Default()
	m := TrayModel(c, state.State{}, update.Status{}, now)
	if m.Status != i18n.T("statusOn", i18n.EN) || m.Muted || m.LastAlert != i18n.T("menuNoAlert", i18n.EN) {
		t.Fatalf("unmuted: %+v", m)
	}
	if m.Labels.Mute30 != i18n.T("mute30", i18n.EN) || m.Labels.Quit != "Quit" {
		t.Fatalf("labels: %+v", m.Labels)
	}

	until := time.Date(2026, 1, 2, 15, 30, 0, 0, time.Local)
	c.MutedUntil = &model.Time{Time: until}
	m = TrayModel(c, state.State{}, update.Status{}, now)
	if !m.Muted || m.Status != i18n.T("statusMutedUntil", i18n.EN, "15:30") {
		t.Fatalf("muted until: %+v", m)
	}

	c.MutedUntil = &model.Time{Time: model.Forever}
	m = TrayModel(c, state.State{}, update.Status{}, now)
	if !m.Muted || m.Status != i18n.T("statusMuted", i18n.EN) {
		t.Fatalf("forever: %+v", m)
	}

	c.Language = i18n.VI
	st := state.State{LastAlert: &model.AlertRecord{Agent: model.Claude, Kind: model.Done, Date: model.Time{Time: now}}}
	m = TrayModel(c, st, update.Status{}, now)
	if m.Labels.Quit != "Thoát" || m.Labels.Settings != i18n.T("menuSettings", i18n.VI) {
		t.Fatalf("vi labels: %+v", m.Labels)
	}
	if !strings.Contains(m.LastAlert, "12:00") || !strings.HasPrefix(m.LastAlert, "Gần nhất") {
		t.Fatalf("last alert: %q", m.LastAlert)
	}
}

func TestMuteUnmuteActions(t *testing.T) {
	p := testenv.Home(t)
	d := 30 * time.Minute
	muteConfig(p, &d, now)
	if c := config.Load(p); !c.IsMuted(now) || !c.MutedUntil.Equal(now.Add(d)) {
		t.Fatalf("30m: %+v", c.MutedUntil)
	}
	muteConfig(p, nil, now)
	if c := config.Load(p); !c.MutedUntil.Equal(model.Forever) {
		t.Fatalf("forever: %+v", c.MutedUntil)
	}
	unmuteConfig(p)
	if config.Load(p).MutedUntil != nil {
		t.Fatal("still muted")
	}
}

func TestMarkSettingsShown(t *testing.T) {
	p := testenv.Home(t)
	markSettingsShown(p)
	if !config.Load(p).SettingsShownOnce {
		t.Fatal("not set")
	}
}

func TestTrayModelUpdateLabel(t *testing.T) {
	c := config.Default()
	m := TrayModel(c, state.State{}, update.Status{Phase: update.Available, Current: "0.3.0", Latest: "0.3.1"}, now)
	if m.UpdateLabel != "Update to 0.3.1…" {
		t.Fatalf("label %q", m.UpdateLabel)
	}
	m = TrayModel(c, state.State{}, update.Status{Phase: update.UpToDate, Current: "0.3.1", Latest: "0.3.1"}, now)
	if m.UpdateLabel != "" {
		t.Fatalf("label %q", m.UpdateLabel)
	}
}
