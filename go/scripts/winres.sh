#!/usr/bin/env bash
# Regenerates the Windows resource objects (.syso) for both executables.
# Run from the go/ directory: bash scripts/winres.sh
set -euo pipefail
go run github.com/tc-hib/go-winres@v0.3.3 make --in scripts/winres/main.json --out cmd/agentchime/rsrc --arch amd64,arm64
go run github.com/tc-hib/go-winres@v0.3.3 make --in scripts/winres/settings.json --out cmd/agentchime-settings/rsrc --arch amd64,arm64
