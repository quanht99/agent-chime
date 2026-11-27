// Package paths resolves every file location the app uses.
package paths

import (
	"os"
	"runtime"
	"strings"
)

type Paths struct {
	OS, Home, DataDir                           string
	Config, State, StateLock, AppLock, Instance string
	Log, LogOld, DebugEnvFlag                   string
	SoundsDir, CustomSoundsDir, BackupsDir      string
	Program                                     string // installed copy
	SettingsHelper                              string // installed settings window helper
	ClaudeSettings, CodexHooks, CodexConfig     string
	SystemSoundsDir, AutostartFile              string
	DebDir                                      string // where the .deb installs the pair; "" off Linux
}

// Resolve is pure: it never touches the OS.
func Resolve(goos string, getenv func(string) string, userHome string) Paths {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	join := func(parts ...string) string { return strings.Join(parts, sep) }

	home := userHome
	override := false
	if h := getenv("AGENTCHIME_HOME"); h != "" {
		home, override = h, true
	}

	var dataDir string
	if goos == "windows" {
		if la := getenv("LOCALAPPDATA"); !override && la != "" {
			dataDir = join(la, "AgentChime")
		} else {
			dataDir = join(home, "AppData", "Local", "AgentChime")
		}
	} else {
		if x := getenv("XDG_DATA_HOME"); !override && x != "" {
			dataDir = join(x, "agentchime")
		} else {
			dataDir = join(home, ".local", "share", "agentchime")
		}
	}

	p := Paths{
		OS:              goos,
		Home:            home,
		DataDir:         dataDir,
		Config:          join(dataDir, "config.json"),
		State:           join(dataDir, "state.json"),
		StateLock:       join(dataDir, "state.lock"),
		AppLock:         join(dataDir, "app.lock"),
		Instance:        join(dataDir, "instance.json"),
		Log:             join(dataDir, "agentchime.log"),
		LogOld:          join(dataDir, "agentchime.log.1"),
		DebugEnvFlag:    join(dataDir, "debug-env"),
		SoundsDir:       join(dataDir, "Sounds"),
		CustomSoundsDir: join(dataDir, "CustomSounds"),
		BackupsDir:      join(dataDir, "Backups"),
		Program:         join(dataDir, "agentchime"),
		SettingsHelper:  join(dataDir, "agentchime-settings"),
		ClaudeSettings:  join(home, ".claude", "settings.json"),
		CodexHooks:      join(home, ".codex", "hooks.json"),
		CodexConfig:     join(home, ".codex", "config.toml"),
	}
	if goos == "windows" {
		p.Program = join(dataDir, "AgentChime.exe")
		p.SettingsHelper = join(dataDir, "AgentChime-Settings.exe")
	}

	switch {
	case override:
		p.SystemSoundsDir = join(home, "system-sounds")
	case goos == "windows":
		win := getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		p.SystemSoundsDir = join(win, "Media")
	case goos == "darwin":
		p.SystemSoundsDir = "/System/Library/Sounds"
	default:
		p.SystemSoundsDir = "/usr/share/sounds/freedesktop/stereo"
	}

	switch {
	case goos != "linux":
	case override:
		p.DebDir = join(home, "usr-lib-agentchime")
	default:
		p.DebDir = "/usr/lib/agentchime"
	}

	if goos == "linux" {
		cfg := join(home, ".config")
		if x := getenv("XDG_CONFIG_HOME"); !override && x != "" {
			cfg = x
		}
		p.AutostartFile = join(cfg, "autostart", "agentchime.desktop")
	}
	return p
}

// Current resolves against the real environment; never cached.
func Current() Paths {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return Resolve(runtime.GOOS, os.Getenv, home)
}
