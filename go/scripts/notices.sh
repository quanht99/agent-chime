#!/bin/bash
# Regenerates go/THIRD_PARTY_NOTICES.md from the modules linked into the app and the settings helper on every target.
set -euo pipefail

GO_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
OUT="$GO_DIR/THIRD_PARTY_NOTICES.md"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

list_deps() { # list_deps <goos> <goarch> <cgo> <package> [go list flags]
	local os=$1 arch=$2 cgo=$3 pkg=$4
	shift 4
	(cd "$GO_DIR" && CGO_ENABLED=$cgo GOOS=$os GOARCH=$arch go list "$@" -deps \
		-f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' "$pkg")
}

{
	for t in linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
		list_deps "${t%/*}" "${t#*/}" 0 ./cmd/agentchime
	done
	for t in windows/amd64 windows/arm64; do
		list_deps "${t%/*}" "${t#*/}" 0 ./cmd/agentchime-settings -tags production
	done
	for t in linux/amd64 linux/arm64; do
		list_deps "${t%/*}" "${t#*/}" 1 ./cmd/agentchime-settings -tags production
	done
} | sort -u >"$WORK/deps"

{
	echo "# Third-party notices"
	echo
	echo "AgentChime for Windows and Linux includes the following Go modules."
	echo "Their licenses are reproduced below."
	while read -r mod ver dir; do
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
