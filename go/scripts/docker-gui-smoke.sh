#!/bin/bash
# Runs the Linux GUI smoke test (Ubuntu 24.04, Xvfb) and the Debian 12 .deb fallback test.
# Usage: bash go/scripts/docker-gui-smoke.sh [amd64|arm64]   (default: this machine's arch)
# Screenshots and logs go to $SMOKE_OUT_DIR, or a new temp folder. Nothing is written to the repo.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
case "${1:-$(uname -m)}" in
	amd64 | x86_64) ARCH=amd64 ;;
	arm64 | aarch64) ARCH=arm64 ;;
	*) echo "usage: docker-gui-smoke.sh [amd64|arm64]" >&2; exit 1 ;;
esac
OUT=${SMOKE_OUT_DIR:-$(mktemp -d)}
mkdir -p "$OUT"
GIT_TRUST=(-e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0=/src)

docker build --platform "linux/$ARCH" -t agentchime-gui-smoke "$ROOT/go/scripts/gui-smoke"

docker run --rm --platform "linux/$ARCH" \
	-v "$ROOT":/src:ro -w /src/go \
	-v agentchime-gomod:/root/go/pkg/mod \
	-v agentchime-gobuild-gui:/root/.cache/go-build \
	-v "$OUT":/out -e SMOKE_OUT=/out "${GIT_TRUST[@]}" \
	agentchime-gui-smoke dbus-run-session -- sh scripts/gui-smoke-linux.sh

docker run --rm --platform "linux/$ARCH" \
	-v "$ROOT":/src:ro -v "$OUT":/out \
	debian:12 sh /src/go/scripts/deb-fallback-smoke.sh "/out/AgentChime-0.0.0-linux-$ARCH.deb"

echo "gui smoke ok. Screenshots and logs: $OUT"
