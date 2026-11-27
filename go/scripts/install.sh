#!/bin/sh
# Installs AgentChime for the current user. Does not turn on autostart.
set -eu

DIR=$(cd "$(dirname "$0")" && pwd)
BIN="$HOME/.local/bin"
APPS="$HOME/.local/share/applications"
ICONS="$HOME/.local/share/icons/hicolor/64x64/apps"

mkdir -p "$BIN" "$APPS" "$ICONS"
install -m 0755 "$DIR/agentchime" "$BIN/agentchime"
install -m 0644 "$DIR/agentchime.png" "$ICONS/agentchime.png"

cat >"$APPS/agentchime.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=AgentChime
Comment=Sound alerts for Claude Code and Codex
Exec="$HOME/.local/bin/agentchime"
Icon=agentchime
Terminal=false
Categories=Utility;
DESKTOP

echo "Installed. Start AgentChime from your app menu or run: agentchime"
