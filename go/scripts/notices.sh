#!/bin/bash
# Regenerates go/THIRD_PARTY_NOTICES.md from the modules linked into the four targets.
set -euo pipefail

GO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT="$GO_DIR/THIRD_PARTY_NOTICES.md"
MODCACHE=$(cd "$GO_DIR" && go env GOMODCACHE)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

for t in windows/amd64 windows/arm64 linux/amd64 linux/arm64; do
	os=${t%/*}; arch=${t#*/}
	(cd "$GO_DIR" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -o "$WORK/$os-$arch" ./cmd/agentchime)
	go version -m "$WORK/$os-$arch" | awk '$1=="dep"{print $2" "$3}'
done | sort -u >"$WORK/deps"

{
	echo "# Third-party notices"
	echo
	echo "AgentChime for Windows and Linux includes the following Go modules."
	echo "Their licenses are reproduced below."
	while read -r mod ver; do
		dir="$MODCACHE/$(echo "$mod" | sed 's/[A-Z]/!\L&/g')@$ver"
		echo
		echo "## $mod $ver"
		lic=$(ls "$dir" 2>/dev/null | grep -iE '^(LICENSE|LICENCE|COPYING)' | head -n 1 || true)
		if [ -z "$lic" ]; then echo; echo "No license file found in the module."; continue; fi
		kind="unknown"
		if grep -q "Apache License" "$dir/$lic"; then kind="Apache-2.0"
		elif grep -q "Permission is hereby granted" "$dir/$lic"; then kind="MIT"
		elif grep -q "Redistributions of source code must retain" "$dir/$lic"; then kind="BSD"; fi
		echo
		echo "License: $kind"
		echo
		echo '```text'
		cat "$dir/$lic"
		echo '```'
		for n in $(ls "$dir" | grep -iE '^NOTICE' || true); do
			echo
			echo "NOTICE:"
			echo
			echo '```text'
			cat "$dir/$n"
			echo '```'
		done
	done <"$WORK/deps"
} >"$OUT"
echo "wrote $OUT"
