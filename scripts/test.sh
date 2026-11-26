#!/bin/bash
set -o pipefail
cd "$(dirname "$0")/.." && xcodegen generate >/dev/null && \
xcodebuild -project AgentChime.xcodeproj -scheme AgentChime -configuration Debug -derivedDataPath build test "$@" 2>&1 \
  | grep -E "error:|Test Case .* failed|Executed [0-9]+ test|TEST (SUCCEEDED|FAILED)" | tail -40
