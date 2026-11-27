#!/bin/bash
# Builds a Release AgentChime.app and zips it to dist/AgentChime-<version>-macos.zip.
# Usage: scripts/package-mac.sh <version>
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-}"
[ -n "$VERSION" ] || { echo "usage: $0 <version>" >&2; exit 2; }
xcodegen generate
rm -rf build-release
set +e
# The generic destination builds for both Apple silicon and Intel Macs.
xcodebuild -project AgentChime.xcodeproj -scheme AgentChime -configuration Release \
  -destination 'generic/platform=macOS' \
  -derivedDataPath build-release \
  MARKETING_VERSION="$VERSION" \
  AGENTCHIME_RELEASE=YES \
  build | grep -E "error:|BUILD (SUCCEEDED|FAILED)"
status=${PIPESTATUS[0]}
set -e
[ "$status" -eq 0 ] || { echo "xcodebuild failed (exit $status)" >&2; exit 1; }

APP=build-release/Build/Products/Release/AgentChime.app
[ -d "$APP" ] || { echo "Build failed: $APP not found" >&2; exit 1; }

# A release must not contain the test key, the test URL override, or a source-build marker.
BIN="$APP/Contents/MacOS/AgentChime"
if grep -aq "$(tr -d '[:space:]' < go/internal/update/testdata/TEST-ONLY-public-key.hex)" "$BIN"; then
  echo "Release binary contains the TEST-ONLY update key" >&2; exit 1
fi
if grep -aq "AGENTCHIME_TEST_UPDATE_URL" "$BIN"; then
  echo "Release binary contains the test update URL override" >&2; exit 1
fi
[ "$(/usr/libexec/PlistBuddy -c 'Print :AgentChimeRelease' "$APP/Contents/Info.plist")" = "YES" ] \
  || { echo "AgentChimeRelease is not YES in Info.plist" >&2; exit 1; }

mkdir -p dist
OUT="dist/AgentChime-$VERSION-macos.zip"
rm -f "$OUT"
ditto -c -k --keepParent "$APP" "$OUT"
echo "Wrote $OUT"
