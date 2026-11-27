#!/bin/sh
# End-to-end test for the Linux build. Runs inside the Docker image or on a CI runner.
# Uses only sh, sed, grep, curl. Never touches the real home: HOME is a temp dir.
set -eu

SRC=$(cd "$(dirname "$0")/.." && pwd)
export CGO_ENABLED=0

# Go's caches must not follow HOME into the temp dir: the module cache is read-only, so the
# cleanup could not delete it, and a cold cache would download everything again. Keep the
# caller's caches; when there are none (no HOME and no GOPATH, e.g. docker --user), use a
# temp dir of their own.
CACHE_TMP=""
gocache=$(go env GOCACHE 2>/dev/null || true)
gomodcache=$(go env GOMODCACHE 2>/dev/null || true)
if [ -z "$gocache" ] || [ "$gocache" = "off" ] || [ -z "$gomodcache" ]; then
	CACHE_TMP=$(mktemp -d)
	[ -n "$gocache" ] && [ "$gocache" != "off" ] || gocache="$CACHE_TMP/gocache"
	[ -n "$gomodcache" ] || gomodcache="$CACHE_TMP/gomod"
fi
GOCACHE=$gocache
GOMODCACHE=$gomodcache
export GOCACHE GOMODCACHE

HOME=$(mktemp -d)
export HOME
export AGENTCHIME_HOME="$HOME"
DATA="$HOME/.local/share/agentchime"
LOG="$DATA/agentchime.log"
APP_PID=""
NEW_PID=""

cleanup() {
	code=$?
	if [ -n "$APP_PID" ]; then kill "$APP_PID" 2>/dev/null || true; fi
	if [ -n "$NEW_PID" ]; then kill "$NEW_PID" 2>/dev/null || true; fi
	if [ "$code" -ne 0 ]; then
		echo "e2e FAILED" >&2
		if [ -f "$LOG" ]; then echo "--- log tail ---" >&2; tail -n 50 "$LOG" >&2; fi
	fi
	# Module cache files are read-only; make everything writable so rm can delete it.
	chmod -R u+w "$HOME" 2>/dev/null || true
	rm -rf "$HOME"
	if [ -n "$CACHE_TMP" ]; then
		chmod -R u+w "$CACHE_TMP" 2>/dev/null || true
		rm -rf "$CACHE_TMP"
	fi
	exit "$code"
}
trap cleanup EXIT INT TERM

fail() { echo "FAIL: $*" >&2; exit 1; }

# 1. build: the app, and a second copy with another version for the upgrade hand-off
mkdir -p "$HOME/build" "$DATA"
(cd "$SRC" && go build -o "$HOME/build/agentchime" ./cmd/agentchime)
(cd "$SRC" && go build -ldflags "-X github.com/quanht99/agent-chime/go/internal/version.Version=e2e-new" \
	-o "$HOME/build/agentchime-new" ./cmd/agentchime)

# 2. config
printf '%s' '{"doneThresholdSeconds":0,"volumeDone":0.5,"settingsShownOnce":true}' >"$DATA/config.json"

# 3. fake pw-play first on PATH
mkdir -p "$HOME/fakebin"
cat >"$HOME/fakebin/pw-play" <<FAKE
#!/bin/sh
echo "\$@" >"$HOME/pwplay.args"
FAKE
chmod 755 "$HOME/fakebin/pw-play"
PATH="$HOME/fakebin:$PATH"
export PATH

# 4. start the app
"$HOME/build/agentchime" --background >"$HOME/app.out" 2>&1 &
APP_PID=$!
i=0
while [ ! -f "$DATA/instance.json" ] && [ "$i" -lt 50 ]; do sleep 0.1; i=$((i + 1)); done
[ -f "$DATA/instance.json" ] || fail "instance.json did not appear"
port=$(sed -n 's/.*"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")
token=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$DATA/instance.json")
[ -n "$port" ] && [ -n "$token" ] || fail "could not read port/token"

api() {
	curl -fsS -X POST -H "Host: 127.0.0.1:$port" -H "X-AgentChime-Token: $token" \
		-H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:$port$1"
}

# 5. install Claude hooks
api /api/hooks/claude/install >/dev/null

# 6. assertions
SETTINGS="$HOME/.claude/settings.json"
grep -q 'hook claude-stop' "$SETTINGS" || fail "settings.json lacks hook claude-stop"
[ -x "$DATA/agentchime" ] || fail "deployed agentchime missing or not executable"
[ -f "$DATA/Sounds/Chime.wav" ] || fail "Sounds/Chime.wav missing"

# 6b. a second launch of a newer build hands off to the running app and deploys itself
"$HOME/build/agentchime-new" --background >"$HOME/new.out" 2>&1 &
NEW_PID=$!
i=0
while kill -0 "$NEW_PID" 2>/dev/null && [ "$i" -lt 50 ]; do sleep 0.1; i=$((i + 1)); done
if kill -0 "$NEW_PID" 2>/dev/null; then fail "second launch did not exit (single instance)"; fi
wait "$NEW_PID" || fail "second launch exited with an error"
NEW_PID=""
cmp -s "$HOME/build/agentchime-new" "$DATA/agentchime" || fail "second launch did not deploy the new build"
kill -0 "$APP_PID" 2>/dev/null || fail "first app died after the second launch"
port2=$(sed -n 's/.*"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")
[ "$port2" = "$port" ] || fail "instance.json port changed: $port -> $port2"
api /api/preview/stop >/dev/null || fail "first app no longer answers"

# 7. extract and run the hook commands
SOH=$(printf '\001')
extract() {
	grep "hook $1\"" "$SETTINGS" | head -n 1 |
		sed -e 's/^[^:]*: *"//' -e 's/",\{0,1\}[[:space:]]*$//' \
			-e "s/\\\\\\\\/$SOH/g" -e 's/\\"/"/g' -e "s/$SOH/\\\\/g"
}
PROMPT_CMD=$(extract claude-prompt)
STOP_CMD=$(extract claude-stop)
[ -n "$PROMPT_CMD" ] && [ -n "$STOP_CMD" ] || fail "could not extract hook commands"
printf '%s' '{"session_id":"e2e","prompt":"hi"}' | sh -c "$PROMPT_CMD"
printf '%s' '{"session_id":"e2e"}' | sh -c "$STOP_CMD"

# 8. wait for the fake player
i=0
while [ ! -s "$HOME/pwplay.args" ] && [ "$i" -lt 30 ]; do sleep 0.1; i=$((i + 1)); done
[ -s "$HOME/pwplay.args" ] || fail "pw-play was not invoked"
ARGS=$(cat "$HOME/pwplay.args")
case "$ARGS" in *--volume=0.50*) ;; *) fail "args lack --volume=0.50: $ARGS" ;; esac
case "$ARGS" in */.local/share/agentchime/Sounds/Chime.wav) ;; *) fail "args do not end with Chime.wav path: $ARGS" ;; esac
grep -q '"agent": *"claude"' "$DATA/state.json" || fail "state.json lastAlert agent"
grep -q '"kind": *"done"' "$DATA/state.json" || fail "state.json lastAlert kind"
grep -q 'alert played: Claude Code done' "$LOG" || fail "log lacks alert played line"

# 9. quit
api /api/quit >/dev/null
i=0
while kill -0 "$APP_PID" 2>/dev/null && [ "$i" -lt 30 ]; do sleep 0.1; i=$((i + 1)); done
if kill -0 "$APP_PID" 2>/dev/null; then fail "app did not exit after /api/quit"; fi
wait "$APP_PID" 2>/dev/null || true
APP_PID=""
[ ! -f "$DATA/instance.json" ] || fail "instance.json still present after quit"

# 10. install.sh
mkdir -p "$HOME/stage"
cp "$HOME/build/agentchime" "$HOME/stage/agentchime"
cp "$SRC/internal/tray/icon.png" "$HOME/stage/agentchime.png"
cp "$SRC/scripts/install.sh" "$HOME/stage/install.sh"
sh "$HOME/stage/install.sh" >/dev/null
[ -x "$HOME/.local/bin/agentchime" ] || fail "install.sh: binary missing"
[ -f "$HOME/.local/share/applications/agentchime.desktop" ] || fail "install.sh: desktop file missing"
[ -f "$HOME/.local/share/icons/hicolor/64x64/apps/agentchime.png" ] || fail "install.sh: icon missing"

echo "e2e ok"
