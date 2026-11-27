# AgentChime

A small app for macOS, Windows and Linux that plays a sound when your coding agent needs you.
On macOS it lives in the menu bar. On Windows and Linux it lives in the system tray.

- **Done** — a Claude Code or Codex turn finished, and it ran for at least the threshold
  (60 seconds by default; set it to 0 to hear every turn).
- **Needs you** — the agent is waiting for an approval or an answer. This one plays right away.

Each sound has its own choice, volume and Preview button. Pick from the system sounds, the 14
sounds that ship with the app (Aurora, Bloom, Breeze, Chime, Crystal, Ding, Drift, Ember, Knock,
Lumen, Pebble, Ripple, Rise and Tide), or any audio file of your own.
In the sound list, rest the pointer on a sound for a second to hear it. The tray icon can also mute
alerts for 30 minutes, an hour, or until you unmute. The interface is in English or Vietnamese.

It works with:

| Where | How |
|---|---|
| Claude Code CLI | hooks in `~/.claude/settings.json` |
| Claude desktop app (Code tab) | the same hooks |
| Codex CLI | hooks in `~/.codex/hooks.json` |
| Codex app | the same hooks |

In the Claude desktop app, only the Code tab runs hooks. Ordinary chats there make no sound.

AgentChime only plays sounds. It does not read your prompts. There is no telemetry.
Once a day AgentChime asks GitHub whether a new version exists. It sends nothing about you or
your prompts; GitHub sees your IP address, as with any download. You can turn this off in
Settings → Updates.

## What runs where

| Host | macOS | Windows | Linux |
|---|---|---|---|
| Claude Code CLI | yes | yes | yes |
| Claude desktop, Code tab | expected (same settings file); verify | expected (same settings file); verify | Debian/Ubuntu beta only; expected; verify |
| Codex CLI | yes, after trust | yes, after trust | yes, after trust |
| Codex app | yes, after trust | hooks may not launch ([openai/codex#35863](https://github.com/openai/codex/issues/35863)) | ChatGPT desktop preview; unverified |

On Windows, use a current version of Claude Code. AgentChime registers its Claude hook in the
exec form (a program plus `args`), and older versions may not support `args`.

## Install

Download the file for your system from the
[latest release](https://github.com/quanht99/agent-chime/releases/latest):

| System | File |
|---|---|
| macOS 13 or later, Apple silicon or Intel | `AgentChime-<version>-macos.zip` |
| Windows on Intel or AMD (most PCs) | `AgentChime-<version>-windows-amd64.zip` |
| Windows on ARM | `AgentChime-<version>-windows-arm64.zip` |
| Linux on Intel or AMD (most PCs) | `AgentChime-<version>-linux-amd64.tar.gz` |
| Linux on ARM | `AgentChime-<version>-linux-arm64.tar.gz` |
| Ubuntu 24.04+ or Debian 13+ on Intel or AMD | `AgentChime-<version>-linux-amd64.deb` |
| Ubuntu 24.04+ or Debian 13+ on ARM | `AgentChime-<version>-linux-arm64.deb` |

GitHub Actions builds these files from the tagged source code. `SHA256SUMS.txt` in the release
lists a checksum for each file. To build from source instead, follow the steps for your system
below.

### macOS

macOS 13 or later.

From a release: unzip `AgentChime-<version>-macos.zip` and move `AgentChime.app` to
`/Applications`. The app is ad-hoc signed and not notarized, so the first launch is blocked. Open
it once, then go to System Settings > Privacy & Security, find the AgentChime message and click
**Open Anyway**.

From source (needs Xcode 15 or later and [XcodeGen](https://github.com/yonaskolb/XcodeGen),
`brew install xcodegen`):

```bash
scripts/install-local.sh
```

This builds a Release copy, installs it to `/Applications/AgentChime.app` and opens it.
Run it again to update; your settings are kept.

### Windows

1. Unzip `AgentChime-<version>-windows-<arch>.zip`.
2. Double-click `AgentChime.exe`. The build is unsigned, so SmartScreen may warn you. Click
   **More info**, then **Run anyway**.

The zip has two programs, `AgentChime.exe` and `AgentChime-Settings.exe`. Keep them together. The
Settings window needs the Microsoft Edge WebView2 Runtime. It is built into Windows 11 and present
on up-to-date Windows 10. Without it, Settings opens in your browser instead.

On every start the program copies itself and its sounds to `%LOCALAPPDATA%\AgentChime`. The hooks
point at that copy, so you can move or delete the downloaded folder afterwards. A tray icon appears.
If AgentChime is already running, starting a new version still updates that copy and opens the
running app's settings page and replaces the running app (see Updating).
Use a current Claude Code version (see the table above).

From source (needs [Go](https://go.dev/dl/) 1.26 or later and Git), in PowerShell:

```powershell
git clone https://github.com/quanht99/agent-chime.git
cd agent-chime\go
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-H windowsgui" -o AgentChime.exe .\cmd\agentchime
.\AgentChime.exe
```

This `AgentChime.exe` works like the release build: on start it copies itself to
`%LOCALAPPDATA%\AgentChime`. To update, pull the latest code, build again and start the new file.

### Linux

#### From the .deb (Ubuntu 24.04+, Debian 13+)

```bash
sudo apt install ./AgentChime-<version>-linux-amd64.deb
```

Use the `arm64` file on ARM. Then open AgentChime from your app menu once.

#### From the tarball

1. Unpack `AgentChime-<version>-linux-<arch>.tar.gz`.
2. Run the installer:

```bash
tar xzf AgentChime-<version>-linux-<arch>.tar.gz
cd AgentChime-<version>-linux-<arch>
./install.sh
```

`install.sh` copies the program to `~/.local/bin`, the launcher to `~/.local/share/applications`
and the icon to `~/.local/share/icons`. It also installs the Settings window program,
`agentchime-settings`, next to `agentchime`. Start AgentChime from your application menu or run
`agentchime`.

From source (needs [Go](https://go.dev/dl/) 1.26 or later, Git, and the GTK 4 and WebKitGTK 6.0
development packages, `libgtk-4-dev libwebkitgtk-6.0-dev`): build the release files yourself, then
install from the one for your machine (`amd64`, or `arm64` on ARM):

```bash
git clone https://github.com/quanht99/agent-chime.git
cd agent-chime
go/scripts/package-linux.sh 0.2.0 "$(go env GOARCH)"
cd go/dist
tar xzf AgentChime-0.2.0-linux-amd64.tar.gz
cd AgentChime-0.2.0-linux-amd64
./install.sh
```

The Settings window needs GTK 4.10 or later and WebKitGTK 6.0 (Ubuntu 24.04+, Debian 13+, current
Fedora). On older systems, Settings opens in your browser instead.

On GNOME the tray icon needs the AppIndicator extension (for example "AppIndicator and
KStatusNotifierItem Support"). Without it you will not see an icon; open the settings page instead
(run `agentchime` again, or use the launcher) and use it from there.

## Updating

AgentChime tells you in the menu and in Settings when a new version is out. **Update** downloads
it, checks its signature, installs it and restarts. Your settings, sounds and hooks are kept.
**Check now** works even when the daily check is turned off.

- **Windows and Linux:** starting a newer downloaded copy also replaces the running one. Starting
  an older copy runs the installed newer one instead. To go back to an older version, uninstall
  first (see Uninstall), then install the older one.
- **.deb:** AgentChime updates your own copy. `apt` still shows the version you installed. An
  `apt upgrade` to a newer version is picked up at the next start.
- **macOS:** keep AgentChime in Applications. If you run it from Downloads, or your account can't
  write to Applications, use the release page instead. After an update the previous version is
  moved to the Trash.

## First run

The settings page opens by itself the first time the app starts. On macOS, Windows and Linux it
is a window. On Windows and Linux, if the window can't start, Settings opens in your browser
instead, at a page served only on this computer (127.0.0.1) behind a random token. Click
**Install** next to Claude Code and next to Codex. AgentChime adds its own hook group at the end
of each file and leaves everything else alone. Before each change it copies the
file to its `Backups` folder (the last 10 copies of each file are kept). The first install may also
rewrite the file's JSON formatting (key order and spacing); the content stays the same.

The hooks call the installed copy of the program (`hook <name>`). Start at login is off until you
turn it on in Settings.

### Trusting the Codex hooks

Codex runs a hook only after you trust it. After installing, open Codex and trust the
AgentChime hooks:

- Codex CLI: run `/hooks` and trust them.
- Codex app: Settings > Hooks.

If you use Codex profiles (`~/.codex/<name>.config.toml`), trust the hooks in each profile you
use. Settings shows whether Codex trusts the hooks.

On Windows, Codex may not find a hook whose program path contains a space. AgentChime uses the
short (8.3) form of the path when it can. If it cannot, Settings shows a warning that Codex hooks
may not run. Installing from a folder without spaces in the user name avoids this.

## How alerts are decided

- **Done** is measured from the prompt you sent to the end of the turn. Time spent waiting for
  your approval counts too.
- If Claude Code ends a turn while background shell tasks or subagents are still running, the
  turn is held open. The Done sound plays when the work they hand back has finished.
- **Needs you** plays for approval prompts and questions. The same alert is not repeated within
  3 seconds; Done is not repeated for the same session within 60 seconds.
- Runs without a person at the keyboard (`claude -p`, the SDKs, `codex exec`) stay silent unless
  you turn on "Alert for non-interactive runs" in Settings.
- An agent started by another agent (for example Codex run from inside Claude Code) stays
  silent, so you hear the outer agent only.
- A pressed Esc or an interrupted turn plays nothing.

## Privacy

AgentChime only plays sounds. It does not read your prompts and has no
telemetry. The hook never uses the network.
Once a day AgentChime asks GitHub whether a new version exists. It sends nothing about you or
your prompts; GitHub sees your IP address, as with any download. You can turn this off in
Settings → Updates.

## Files and logs

| | macOS | Windows | Linux |
|---|---|---|---|
| App data | `~/Library/Application Support/AgentChime` | `%LOCALAPPDATA%\AgentChime` | `${XDG_DATA_HOME:-~/.local/share}/agentchime` |
| Backups | `Backups` in the app data folder | `Backups` in the data folder | `Backups` in the data folder |
| Log | `~/Library/Logs/AgentChime/agentchime.log` | `%LOCALAPPDATA%\AgentChime\agentchime.log` | `${XDG_DATA_HOME:-~/.local/share}/agentchime/agentchime.log` |
| Start at login | the Settings switch | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value `AgentChime` | `${XDG_CONFIG_HOME:-~/.config}/autostart/agentchime.desktop` |
| Claude settings | `~/.claude/settings.json` | `%USERPROFILE%\.claude\settings.json` | `~/.claude/settings.json` |
| Codex hooks | `~/.codex/hooks.json` | `%USERPROFILE%\.codex\hooks.json` | `~/.codex/hooks.json` |

On Windows and Linux the data folder holds `config.json`, `state.json`,
`agentchime.log`, `Sounds`, `CustomSounds`, `Backups` and the installed programs
(`AgentChime.exe` and `AgentChime-Settings.exe`, or `agentchime` and `agentchime-settings`).

The Settings window keeps web view data too. On Windows it is `%LOCALAPPDATA%\AgentChime\webview`,
inside the data folder. On Linux the data folder may also hold web view data, and the WebKit
cache follows `XDG_CACHE_HOME` (default `~/.cache/agentchime`).

AgentChime writes only `~/.claude/settings.json` and `~/.codex/hooks.json`. It never writes
`~/.codex/config.toml` or the Codex `notify` setting.

## Built-in sounds

The 14 bundled sounds live in `AgentChime/Resources/Sounds` (the Go app embeds identical copies
from `go/internal/sounds/builtin`). The script `scripts/make-sounds.swift` regenerates all 14:

```bash
swift scripts/make-sounds.swift AgentChime/Resources/Sounds
```

To add or replace one, put the file in that folder, add a row to
`AgentChime/Resources/Sounds/CREDITS.md` with its source and license, copy the identical file to
`go/internal/sounds/builtin/`, and rebuild. Add the new name to the sound lists and counts in the
tests (`builtinWavNames` in `go/internal/sounds/guard_test.go`, and the 14-count tests in Go and
Swift). Only add sounds you are allowed to redistribute.

## Uninstall

### macOS

1. In Settings, click **Remove** for Claude Code and for Codex.
2. Quit AgentChime and delete `/Applications/AgentChime.app`.
3. Delete `~/Library/Application Support/AgentChime` and `~/Library/Logs/AgentChime`.

### Windows

1. In Settings, click **Remove** for Claude Code and for Codex. If you turned on start at login,
   turn it off first.
2. Quit AgentChime from the tray icon.
3. Delete `%LOCALAPPDATA%\AgentChime`. `AgentChime-Settings.exe` and the `webview` folder go with
   it.

### Linux

1. In Settings, click **Remove** for Claude Code and for Codex. If you turned on start at login,
   turn it off first.
2. Quit AgentChime from the tray icon.
3. If you installed the `.deb`, run `sudo apt remove agentchime`.
4. If you used the tarball, delete `~/.local/bin/agentchime`, `~/.local/bin/agentchime-settings`,
   `~/.local/share/applications/agentchime.desktop` and the AgentChime icon in
   `~/.local/share/icons`.
5. Delete the data folder `${XDG_DATA_HOME:-~/.local/share}/agentchime` and the web view folder
   `~/.cache/agentchime`.

## Development

macOS (Swift):

```bash
bash scripts/test.sh                                   # all tests
bash scripts/test.sh -only-testing:AgentChimeTests/HookParserTests
```

Windows and Linux (Go 1.26, no cgo):

```bash
cd go && CGO_ENABLED=0 go test ./...
go/scripts/docker-test.sh      # runs the Go tests on Linux in Docker (golang:1.26)
```

The Settings window has a GUI smoke test. It runs only in Docker:

```bash
bash go/scripts/docker-gui-smoke.sh
```

Tests never touch your real `~/.claude`, `~/.codex`, AppData, `~/.local` or Library folders. Each
one runs against a temporary home directory set through `AGENTCHIME_HOME`. You can set it yourself
to try the app in isolation:

```bash
AGENTCHIME_HOME=/tmp/agentchime-home ./agentchime
```

The Go program has four modes: the tray app, `--background` (tray only, no settings page on start),
`hook <name>` (called by Claude Code and Codex) and `play` (plays one file).

For a Debug macOS build:

```bash
xcodegen generate
xcodebuild -project AgentChime.xcodeproj -scheme AgentChime -configuration Debug -derivedDataPath build build
open build/Build/Products/Debug/AgentChime.app
```

### Update signing

Updates are signed with an Ed25519 key. The owner sets it up once:

1. Run `cd go && go run ./scripts/updatekey -out <a file outside the repo>`. It prints only the
   public key.
2. Put the public key in `go/internal/update/keys.go` and `AgentChime/Update/UpdateKeys.swift`.
   `TestSwiftKeysMatchGo` checks that they match.
3. Add the content of that file as the Actions secret `UPDATE_SIGNING_KEY`.
4. Keep a backup of the file.
5. If the key is lost, users update by hand once.

Test builds use `-tags updatetest` (Go) or `AGENTCHIME_UPDATE_TEST` (Swift). They use a test key
and a local server, and the packaging scripts refuse to ship them. The end-to-end scripts are
`go/scripts/e2e-update-linux.sh` (run through `go/scripts/docker-test.sh`) and
`scripts/e2e-update-mac.sh`.

### Release packages

```bash
go/scripts/package-windows.sh <version>          # Windows zips, amd64 and arm64, into go/dist/
go/scripts/package-linux.sh <version> <arch>     # Linux tar.gz and .deb, into go/dist/
scripts/package-mac.sh <version>                 # macOS zip, into dist/
```

Outputs in `go/dist/`: `AgentChime-<version>-windows-<arch>.zip`,
`AgentChime-<version>-linux-<arch>.tar.gz` and `AgentChime-<version>-linux-<arch>.deb`. The Windows
zips are cross-built with `CGO_ENABLED=0` and `-H windowsgui`. The Linux package builds only on
Linux, and only for the machine's own architecture, because the settings window helper links GTK 4
and WebKitGTK 6.0 (`apt install libgtk-4-dev libwebkitgtk-6.0-dev`). Release files are made by CI
from a tag. The macOS app is built for both Apple silicon and Intel.

To publish a release, push a version tag (`vX.Y.Z`). The `release` workflow builds the 7 archives
on GitHub, adds `SHA256SUMS.txt`, `latest.json` and `latest.json.sig`, and creates the release:

```bash
git tag v0.2.0
git push origin v0.2.0
```

## Third-party notices

See [go/THIRD_PARTY_NOTICES.md](go/THIRD_PARTY_NOTICES.md) for the Go dependencies.

## License

MIT. See [LICENSE](LICENSE).
