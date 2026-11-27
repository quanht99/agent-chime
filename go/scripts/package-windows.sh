#!/bin/bash
# Usage: package-windows.sh <version>
# Builds AgentChime-<version>-windows-{amd64,arm64}.zip into $OUT (default go/dist).
set -euo pipefail

if [ $# -lt 1 ]; then echo "usage: package-windows.sh <version>" >&2; exit 1; fi
VERSION=$1

GO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ROOT=$(cd "$GO_DIR/.." && pwd)
OUT=${OUT:-$GO_DIR/dist}
MOD=github.com/quanht99/agent-chime/go/internal/version

WORK=""
cleanup() {
	[ -n "$WORK" ] && rm -rf "$WORK"
	return 0
}
trap cleanup EXIT

LD="-s -w -X $MOD.Version=$VERSION"

mkdir -p "$OUT"
WORK=$(mktemp -d)

zip_dir() { # zip_dir <out.zip> <dir>
	local out=$1 dir=$2
	rm -f "$out"
	if command -v zip >/dev/null 2>&1; then
		zip -q -j "$out" "$dir"/*
	else
		(cd "$dir" && python3 -m zipfile -c "$out" ./*)
	fi
}

for arch in amd64 arm64; do
	st="$WORK/win-$arch"; mkdir -p "$st"
	(cd "$GO_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -trimpath \
		-ldflags "$LD -H windowsgui" -o "$st/AgentChime.exe" ./cmd/agentchime)
	(cd "$GO_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -trimpath -tags production \
		-ldflags "$LD -H windowsgui" -o "$st/AgentChime-Settings.exe" ./cmd/agentchime-settings)
	if ! go version -m "$st/AgentChime-Settings.exe" | grep -q -- '-tags=production'; then
		echo "AgentChime-Settings.exe ($arch) was not built with -tags production" >&2
		exit 1
	fi
	for exe in AgentChime.exe AgentChime-Settings.exe; do
		if go version -m "$st/$exe" | grep -q updatetest; then
			echo "FAIL: test build" >&2
			exit 1
		fi
	done
	cp "$ROOT/LICENSE" "$GO_DIR/scripts/README.txt" "$GO_DIR/THIRD_PARTY_NOTICES.md" "$st/"
	zip_dir "$OUT/AgentChime-$VERSION-windows-$arch.zip" "$st"
done

ls -l "$OUT"/AgentChime-"$VERSION"-windows-*
