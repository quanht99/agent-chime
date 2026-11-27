AgentChime - sound alerts for Claude Code and Codex

LINUX
  1. Extract the archive.
  2. Run: sh install.sh
  3. Start AgentChime from your app menu, or run: agentchime
  The installer copies files under ~/.local and never turns on autostart.
  You need pw-play (PipeWire), paplay (PulseAudio) or aplay (ALSA) for sound.
  A tray needs a desktop with StatusNotifier support.
  Settings opens in its own window (agentchime-settings). That needs GTK 4.10+
  and WebKitGTK 6.0 (Ubuntu 24.04+, Debian 13+, current Fedora). If the window
  can't start, Settings opens in your browser, on a local address only.

WINDOWS
  1. Extract the zip and double-click AgentChime.exe.
  2. The app is not code-signed. If SmartScreen says "Windows protected your
     PC", click "More info", then "Run anyway".
  3. AgentChime lives in the tray (notification area) and opens Settings in
     its own window. If the window can't start, it opens in your browser
     instead.
  Keep AgentChime.exe and AgentChime-Settings.exe in the same folder. The
  window needs the Microsoft Edge WebView2 Runtime (built into Windows 11).

Settings and sounds are stored in your user profile.

Once a day AgentChime asks GitHub whether a new version exists. It sends
nothing about you or your prompts; GitHub sees your IP address, as with any
download. You can turn this off in Settings > Updates.

UPDATING
  AgentChime tells you in the menu and in Settings when a new version is out.
  Update downloads it, checks its signature, installs it and restarts. Your
  settings, sounds and hooks are kept. Check now works even when the daily
  check is turned off.
  Starting a newer downloaded copy also replaces the running one. Starting an
  older copy runs the installed newer one instead. To go back to an older
  version, uninstall first, then install the older one.
  On Linux, if you installed the .deb, AgentChime updates your own copy. apt
  still shows the version you installed, and an apt upgrade to a newer version
  is picked up at the next start.

See LICENSE and THIRD_PARTY_NOTICES.md.
