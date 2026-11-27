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
  build | grep -E "error:|BUILD (SUCCEEDED|FAILED)"
status=${PIPESTATUS[0]}
set -e
[ "$status" -eq 0 ] || { echo "xcodebuild failed (exit $status)" >&2; exit 1; }

APP=build-release/Build/Products/Release/AgentChime.app
[ -d "$APP" ] || { echo "Build failed: $APP not found" >&2; exit 1; }

mkdir -p dist
OUT="dist/AgentChime-$VERSION-macos.zip"
rm -f "$OUT"
ditto -c -k --keepParent "$APP" "$OUT"
echo "Wrote $OUT"
