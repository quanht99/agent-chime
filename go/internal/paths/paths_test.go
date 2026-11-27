package paths

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveWindows(t *testing.T) {
	p := Resolve("windows", env(map[string]string{
		"LOCALAPPDATA": `C:\Users\example\AppData\Local`,
		"WINDIR":       `C:\Windows`,
	}), `C:\Users\example`)
	cases := map[string][2]string{
		"DataDir":         {p.DataDir, `C:\Users\example\AppData\Local\AgentChime`},
		"Program":         {p.Program, `C:\Users\example\AppData\Local\AgentChime\AgentChime.exe`},
		"AppLock":         {p.AppLock, `C:\Users\example\AppData\Local\AgentChime\app.lock`},
		"ClaudeSettings":  {p.ClaudeSettings, `C:\Users\example\.claude\settings.json`},
		"CodexHooks":      {p.CodexHooks, `C:\Users\example\.codex\hooks.json`},
		"SystemSoundsDir": {p.SystemSoundsDir, `C:\Windows\Media`},
		"AutostartFile":   {p.AutostartFile, ``},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestResolveLinuxXDG(t *testing.T) {
	p := Resolve("linux", env(map[string]string{"XDG_DATA_HOME": "/home/example/data"}), "/home/example")
	if p.DataDir != "/home/example/data/agentchime" {
		t.Errorf("DataDir = %q", p.DataDir)
	}
	if p.AutostartFile != "/home/example/.config/autostart/agentchime.desktop" {
		t.Errorf("AutostartFile = %q", p.AutostartFile)
	}
	if p.Program != "/home/example/data/agentchime/agentchime" {
		t.Errorf("Program = %q", p.Program)
	}
	if p.SystemSoundsDir != "/usr/share/sounds/freedesktop/stereo" {
		t.Errorf("SystemSoundsDir = %q", p.SystemSoundsDir)
	}
}

func TestResolveAgentChimeHomeWins(t *testing.T) {
	p := Resolve("linux", env(map[string]string{
		"AGENTCHIME_HOME": "/tmp/h", "XDG_DATA_HOME": "/x", "XDG_CONFIG_HOME": "/c", "WINDIR": "/w",
	}), "/home/example")
	if p.DataDir != "/tmp/h/.local/share/agentchime" {
		t.Errorf("DataDir = %q", p.DataDir)
	}
	if p.SystemSoundsDir != "/tmp/h/system-sounds" {
		t.Errorf("SystemSoundsDir = %q", p.SystemSoundsDir)
	}
	if p.AutostartFile != "/tmp/h/.config/autostart/agentchime.desktop" {
		t.Errorf("AutostartFile = %q", p.AutostartFile)
	}
	w := Resolve("windows", env(map[string]string{
		"AGENTCHIME_HOME": `C:\t`, "LOCALAPPDATA": `D:\x`, "WINDIR": `D:\w`,
	}), `C:\Users\example`)
	if w.DataDir != `C:\t\AppData\Local\AgentChime` {
		t.Errorf("windows DataDir = %q", w.DataDir)
	}
	if w.SystemSoundsDir != `C:\t\system-sounds` {
		t.Errorf("windows SystemSoundsDir = %q", w.SystemSoundsDir)
	}
}

func TestResolveFallbacksAndDarwin(t *testing.T) {
	w := Resolve("windows", env(nil), `C:\Users\example`)
	if w.DataDir != `C:\Users\example\AppData\Local\AgentChime` || w.SystemSoundsDir != `C:\Windows\Media` {
		t.Errorf("windows fallback: %q %q", w.DataDir, w.SystemSoundsDir)
	}
	d := Resolve("darwin", env(nil), "/Users/example")
	if d.DataDir != "/Users/example/.local/share/agentchime" || d.SystemSoundsDir != "/System/Library/Sounds" || d.AutostartFile != "" {
		t.Errorf("darwin: %+v", d)
	}
}
