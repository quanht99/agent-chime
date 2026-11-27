#!/bin/bash
# Runs vet, unit tests and the Linux end-to-end test in a golang container, then the
# end-to-end test once more as a non-root user.
# Nothing is written into the repo: outputs go to a temp dir inside the container.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
# The mounted repo can show a different owner inside the container, and git then refuses it
# ("dubious ownership"), which breaks go build's VCS stamp and the git ls-files guard test.
# This trusts /src for these runs only, without writing any git config.
GIT_TRUST=(-e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0=/src)

docker run --rm --platform "${DOCKER_PLATFORM:-linux/arm64}" \
	-v "$ROOT":/src -w /src/go \
	-v agentchime-gomod:/go/pkg/mod \
	-v agentchime-gobuild:/root/.cache/go-build \
	-e CGO_ENABLED=0 "${GIT_TRUST[@]}" \
	golang:1.26 bash -c 'export TMPDIR=$(mktemp -d) && go vet ./... && go test ./... && bash scripts/e2e-linux.sh && bash scripts/e2e-update-linux.sh'

# The e2e test again as a plain user with no HOME and no GOPATH, the repo mounted read-only:
# the script must find its own Go caches and clean up everything it wrote.
docker run --rm --platform "${DOCKER_PLATFORM:-linux/arm64}" --user 1000:1000 \
	-v "$ROOT":/src:ro -w /src/go \
	-e CGO_ENABLED=0 "${GIT_TRUST[@]}" \
	golang:1.26 sh -c 'env -u HOME -u GOPATH sh scripts/e2e-linux.sh && env -u HOME -u GOPATH bash scripts/e2e-update-linux.sh'
