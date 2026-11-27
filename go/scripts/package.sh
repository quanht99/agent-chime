#!/bin/bash
# Usage: package.sh <version>
# Builds the four release archives into go/dist/.
set -euo pipefail

if [ $# -lt 1 ]; then echo "usage: package.sh <version>" >&2; exit 1; fi
VERSION=$1

GO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ROOT=$(cd "$GO_DIR/.." && pwd)
DIST="$GO_DIR/dist"
MOD=github.com/quanht99/agent-chime/go/internal/version

WORK=""
cleanup() {
	[ -n "$WORK" ] && rm -rf "$WORK"
	return 0
}
trap cleanup EXIT

LD="-s -w -X $MOD.Version=$VERSION"

mkdir -p "$DIST"
WORK=$(mktemp -d)

# The tarball stores root as owner, not the name of whoever built it.
if tar --version 2>/dev/null | grep -q 'GNU tar'; then
	TAR_OWNER=(--owner=0 --group=0 --numeric-owner)
else
	TAR_OWNER=(--uid 0 --gid 0 --uname root --gname root)
fi

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
	# Windows
	st="$WORK/win-$arch"; mkdir -p "$st"
	(cd "$GO_DIR" && CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -trimpath \
		-ldflags "$LD -H windowsgui" -o "$st/AgentChime.exe" ./cmd/agentchime)
	cp "$ROOT/LICENSE" "$GO_DIR/scripts/README.txt" "$GO_DIR/THIRD_PARTY_NOTICES.md" "$st/"
	zip_dir "$DIST/AgentChime-$VERSION-windows-$arch.zip" "$st"

	# Linux: one top-level folder, so "tar xzf" then "cd AgentChime-<version>-linux-<arch>" works
	name="AgentChime-$VERSION-linux-$arch"
	st="$WORK/linux-$arch/$name"; mkdir -p "$st"
	(cd "$GO_DIR" && CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
		-ldflags "$LD" -o "$st/agentchime" ./cmd/agentchime)
	cp "$GO_DIR/scripts/agentchime.desktop" "$GO_DIR/scripts/install.sh" "$GO_DIR/scripts/README.txt" \
		"$GO_DIR/THIRD_PARTY_NOTICES.md" "$ROOT/LICENSE" "$st/"
	cp "$GO_DIR/internal/tray/icon.png" "$st/agentchime.png"
	chmod 755 "$st/agentchime" "$st/install.sh"
	tar "${TAR_OWNER[@]}" -czf "$DIST/$name.tar.gz" -C "$WORK/linux-$arch" "$name"
done

ls -l "$DIST"/AgentChime-"$VERSION"-*
