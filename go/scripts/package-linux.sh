#!/bin/bash
# Usage: package-linux.sh <version> <arch>
# Builds AgentChime-<version>-linux-<arch>.tar.gz and .deb into $OUT (default go/dist).
# Linux only: the settings helper is a cgo binary (needs libgtk-4-dev libwebkitgtk-6.0-dev),
# so <arch> must equal the machine's own architecture.
set -euo pipefail

if [ $# -lt 2 ]; then echo "usage: package-linux.sh <version> <arch>" >&2; exit 1; fi
VERSION=$1
ARCH=$2

GO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ROOT=$(cd "$GO_DIR/.." && pwd)
OUT=${OUT:-$GO_DIR/dist}
MOD=github.com/quanht99/agent-chime/go/internal/version

if [ "$(go env GOOS)" != linux ]; then
	echo "package-linux.sh runs on Linux only (the settings helper needs cgo)" >&2
	exit 1
fi
if [ "$ARCH" != "$(go env GOARCH)" ]; then
	echo "arch $ARCH does not match this machine ($(go env GOARCH)); build on a matching runner" >&2
	exit 1
fi

WORK=""
cleanup() {
	[ -n "$WORK" ] && rm -rf "$WORK"
	return 0
}
trap cleanup EXIT

LD="-s -w -X $MOD.Version=$VERSION"

mkdir -p "$OUT"
WORK=$(mktemp -d)

# The tarball stores root as owner, not the name of whoever built it.
if tar --version 2>/dev/null | grep -q 'GNU tar'; then
	TAR_OWNER=(--owner=0 --group=0 --numeric-owner)
else
	TAR_OWNER=(--uid 0 --gid 0 --uname root --gname root)
fi

BIN="$WORK/bin"; mkdir -p "$BIN"
(cd "$GO_DIR" && CGO_ENABLED=0 go build -trimpath -ldflags "$LD" -o "$BIN/agentchime" ./cmd/agentchime)
(cd "$GO_DIR" && CGO_ENABLED=1 go build -trimpath -tags production -ldflags "$LD" \
	-o "$BIN/agentchime-settings" ./cmd/agentchime-settings)

main_info=$(go version -m "$BIN/agentchime")
helper_info=$(go version -m "$BIN/agentchime-settings")
case "$main_info$helper_info" in *updatetest*) echo "FAIL: test build" >&2; exit 1 ;; esac
case "$main_info" in *CGO_ENABLED=0*) ;; *) echo "agentchime was not built with CGO_ENABLED=0" >&2; exit 1 ;; esac
case "$helper_info" in *CGO_ENABLED=1*) ;; *) echo "agentchime-settings was not built with CGO_ENABLED=1" >&2; exit 1 ;; esac
case "$helper_info" in *-tags=production*) ;; *) echo "agentchime-settings was not built with -tags production" >&2; exit 1 ;; esac

# Tarball: one top-level folder, so "tar xzf" then "cd AgentChime-<version>-linux-<arch>" works
name="AgentChime-$VERSION-linux-$ARCH"
st="$WORK/tar/$name"; mkdir -p "$st"
cp "$BIN/agentchime" "$BIN/agentchime-settings" "$st/"
cp "$GO_DIR/scripts/agentchime.desktop" "$GO_DIR/scripts/install.sh" "$GO_DIR/scripts/README.txt" \
	"$GO_DIR/THIRD_PARTY_NOTICES.md" "$ROOT/LICENSE" "$st/"
cp "$GO_DIR/internal/tray/icon.png" "$st/agentchime.png"
chmod 755 "$st/agentchime" "$st/agentchime-settings" "$st/install.sh"
tar "${TAR_OWNER[@]}" -czf "$OUT/$name.tar.gz" -C "$WORK/tar" "$name"

# .deb
deb="$WORK/deb"
install -d -m 755 "$deb/DEBIAN" "$deb/usr/lib/agentchime" "$deb/usr/bin" \
	"$deb/usr/share/applications" "$deb/usr/share/icons/hicolor/64x64/apps" \
	"$deb/usr/share/icons/hicolor/256x256/apps" "$deb/usr/share/doc/agentchime"
install -m 755 "$BIN/agentchime" "$BIN/agentchime-settings" "$deb/usr/lib/agentchime/"
ln -s ../lib/agentchime/agentchime "$deb/usr/bin/agentchime"
install -m 644 "$GO_DIR/scripts/agentchime.desktop" "$deb/usr/share/applications/agentchime.desktop"
install -m 644 "$GO_DIR/internal/tray/icon.png" "$deb/usr/share/icons/hicolor/64x64/apps/agentchime.png"
install -m 644 "$GO_DIR/scripts/icons/agentchime-256.png" "$deb/usr/share/icons/hicolor/256x256/apps/agentchime.png"
{ cat "$ROOT/LICENSE"; echo; cat "$GO_DIR/THIRD_PARTY_NOTICES.md"; } >"$deb/usr/share/doc/agentchime/copyright"
chmod 644 "$deb/usr/share/doc/agentchime/copyright"
cat >"$deb/DEBIAN/control" <<CTL
Package: agentchime
Version: $VERSION
Architecture: $ARCH
Maintainer: quanht99 <quanht99@users.noreply.github.com>
Homepage: https://github.com/quanht99/agent-chime
Section: utils
Priority: optional
Recommends: libgtk-4-1 (>= 4.10), libwebkitgtk-6.0-4, pipewire-bin | pulseaudio-utils | alsa-utils
Description: Sound alerts for Claude Code and Codex
 Plays a sound when a Claude Code or Codex turn finishes or needs you.
CTL
chmod 644 "$deb/DEBIAN/control"
dpkg-deb --root-owner-group --build "$deb" "$OUT/$name.deb" >/dev/null

ls -l "$OUT/$name.tar.gz" "$OUT/$name.deb"
