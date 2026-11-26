#!/bin/bash
# Builds AgentChime in Release and installs it to /Applications. Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")/.."

APP=/Applications/AgentChime.app
BUNDLE_ID=io.github.quanht99.agentchime

xcodegen generate
xcodebuild -project AgentChime.xcodeproj -scheme AgentChime -configuration Release \
  -derivedDataPath build-release build | grep -E "error:|warning: .*AgentChime|BUILD (SUCCEEDED|FAILED)" || true
BUILT=build-release/Build/Products/Release/AgentChime.app
[ -d "$BUILT" ] || { echo "Build failed: $BUILT not found" >&2; exit 1; }

if [ -d "$APP" ]; then
  existing=$(/usr/libexec/PlistBuddy -c "Print :CFBundleIdentifier" "$APP/Contents/Info.plist" 2>/dev/null || true)
  if [ "$existing" != "$BUNDLE_ID" ]; then
    echo "$APP exists but is a different app ($existing); not replacing it." >&2
    exit 1
  fi
fi

osascript -e 'quit app "AgentChime"' >/dev/null 2>&1 || true
for _ in $(seq 1 50); do
  pgrep -x AgentChime >/dev/null || break
  sleep 0.1
done
if pgrep -x AgentChime >/dev/null; then
  echo "AgentChime is still running; quit it and run this script again." >&2
  exit 1
fi

rm -rf "$APP"
ditto "$BUILT" "$APP"
open "$APP"
echo "Installed $APP"
