#!/bin/sh
# End-to-end test of the in-app self-update on Linux. Runs inside the Docker image or on a CI runner.
# Uses only sh, sed, grep, curl, tar. Never touches the real home: HOME is a temp dir.
# Builds are signed with the TEST-ONLY key (go build -tags updatetest) and served by
# scripts/e2eserver on 127.0.0.1.
set -eu

SRC=$(cd "$(dirname "$0")/.." && pwd)
export CGO_ENABLED=0

# Go's caches must not follow HOME into the temp dir (see e2e-linux.sh): keep the caller's caches,
# or use a temp dir of their own when there are none.
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
W="$HOME/w"       # builds and tarballs
SRV="$HOME/srv"   # what e2eserver serves
SRV_PID=""
APP_PID=""
ARCH=$(go env GOARCH)
MODVER=github.com/quanht99/agent-chime/go/internal/version.Version

# Every process of ours that could still run: the instance in instance.json and the ones we started.
inst_pid() { sed -n 's/.*"pid": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json" 2>/dev/null || true; }

cleanup() {
	code=$?
	p=$(inst_pid)
	if [ -n "$p" ]; then kill "$p" 2>/dev/null || true; fi
	if [ -n "$APP_PID" ]; then kill "$APP_PID" 2>/dev/null || true; fi
	if [ -n "$SRV_PID" ]; then kill "$SRV_PID" 2>/dev/null || true; fi
	if [ "$code" -ne 0 ]; then
		echo "e2e update FAILED" >&2
		if [ -f "$LOG" ]; then echo "--- log tail ---" >&2; tail -n 60 "$LOG" >&2; fi
	fi
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

# alive PID: running and not a zombie (a dead child of this shell stays a zombie until reaped).
alive() {
	[ -n "$1" ] && kill -0 "$1" 2>/dev/null || return 1
	st=$(sed -n 's/^[^)]*) \(.\).*/\1/p' "/proc/$1/stat" 2>/dev/null || true)
	[ -n "$st" ] && [ "$st" != "Z" ]
}

# wait_until SECONDS COMMAND...: poll every 0.1 s until the command succeeds.
wait_until() {
	n=$(($1 * 10)); shift
	i=0
	while [ "$i" -lt "$n" ]; do
		if "$@"; then return 0; fi
		sleep 0.1
		i=$((i + 1))
	done
	return 1
}

# ---- talking to the running instance (its port and token change after an update) ----
load() {
	port=$(sed -n 's/.*"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")
	token=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$DATA/instance.json")
	pid=$(inst_pid)
	[ -n "$port" ] && [ -n "$token" ] && [ -n "$pid" ]
}
post() {
	load || return 1
	curl -fsS -X POST -H "Host: 127.0.0.1:$port" -H "X-AgentChime-Token: $token" \
		-H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:$port$1"
}
get() {
	load || return 1
	curl -fsS -H "Host: 127.0.0.1:$port" -H "X-AgentChime-Token: $token" "http://127.0.0.1:$port$1"
}
state() { get /api/state 2>/dev/null || true; }
phase_is() {
	s=$(state)
	case "$s" in *'"busy":false'*) ;; *) return 1 ;; esac
	case "$s" in *"\"phase\":\"$1\""*) return 0 ;; esac
	return 1
}
wait_phase() { wait_until 40 phase_is "$1" || fail "phase did not become $1 (state: $(state | grep -o '"update":{[^}]*}'))"; }
have_instance() { [ -f "$DATA/instance.json" ] && load; }
mode() { printf '%s' "$1" >"$SRV/mode"; }

# req_count: how many requests the e2e server has logged.
req_count() { if [ -f "$SRV/requests" ]; then wc -l <"$SRV/requests" | tr -d ' '; else echo 0; fi; }

# do_check PHASE / do_install PHASE: act, then wait for the phase. The short sleep lets the
# background goroutine leave the old phase first. The server must have seen new requests, so a
# check that never ran can't pass on a stale phase.
do_check() {
	before=$(req_count)
	post /api/update/check >/dev/null || fail "check: no answer"; sleep 1; wait_phase "$1"
	[ "$(req_count)" -gt "$before" ] || fail "check: the server saw no request"
}
do_install() {
	before=$(req_count)
	post /api/update/install >/dev/null || fail "install: no answer"; sleep 1; wait_phase "$1"
	[ "$(req_count)" -gt "$before" ] || fail "install: the server saw no request"
}

# still_old: nothing was deployed and the old app still runs.
still_old() {
	cmp -s "$DATA/agentchime" "$W/old/agentchime" || fail "$1: the installed program is not the old one"
	alive "$OLD_PID" || fail "$1: the old app died"
}

# sign: sign what is in $SRV for 0.9.1.
sign() {
	UPDATE_SIGNING_KEY=$(tail -1 "$SRC/internal/update/testdata/TEST-ONLY-signing-seed.txt") \
		"$W/signmanifest" -version 0.9.1 -require "linux-$ARCH" -dir "$SRV" -out "$SRV" >/dev/null ||
		fail "signmanifest failed"
}
TARBALL="$SRV/AgentChime-0.9.1-linux-$ARCH.tar.gz"
# mktar PROGRAM HELPER: write the release tarball (a top folder holding the two files).
mktar() {
	rm -rf "$W/tar" "$TARBALL"
	mkdir -p "$W/tar/AgentChime-0.9.1-linux-$ARCH"
	cp "$1" "$W/tar/AgentChime-0.9.1-linux-$ARCH/agentchime"
	cp "$2" "$W/tar/AgentChime-0.9.1-linux-$ARCH/agentchime-settings"
	chmod 755 "$W/tar/AgentChime-0.9.1-linux-$ARCH/"*
	tar -C "$W/tar" -czf "$TARBALL" "AgentChime-0.9.1-linux-$ARCH"
}

# ---- 1. build ----
mkdir -p "$W/old" "$W/rel" "$W/deb2" "$SRV" "$DATA" "$HOME/fakebin"
for v in "old 0.9.0" "rel 0.9.1" "deb2 0.9.2"; do
	set -- $v
	(cd "$SRC" && go build -tags updatetest -ldflags "-X $MODVER=$2" -o "$W/$1/agentchime" ./cmd/agentchime)
	# The helper stub exits 2, so "settings" falls back to the browser (the fake xdg-open).
	(cd "$SRC" && go build -tags production -o "$W/$1/agentchime-settings" ./cmd/agentchime-settings)
done
(cd "$SRC" && go build -tags updatetest -o "$W/signmanifest" ./scripts/signmanifest)
(cd "$SRC" && go build -o "$W/e2eserver" ./scripts/e2eserver)

# ---- 2. fakes on PATH ----
cat >"$HOME/fakebin/pw-play" <<FAKE
#!/bin/sh
echo "\$@" >>"$HOME/pwplay.log"
FAKE
cat >"$HOME/fakebin/xdg-open" <<FAKE
#!/bin/sh
echo "\$@" >>"$HOME/xdgopen.log"
FAKE
chmod 755 "$HOME/fakebin/pw-play" "$HOME/fakebin/xdg-open"
: >"$HOME/pwplay.log"
: >"$HOME/xdgopen.log"
PATH="$HOME/fakebin:$PATH"
export PATH

# ---- 3. the release, signed, and the server ----
mktar "$W/rel/agentchime" "$W/rel/agentchime-settings"
sign
"$W/e2eserver" -dir "$SRV" -portfile "$HOME/srv.port" >"$HOME/srv.out" 2>&1 &
SRV_PID=$!
wait_until 10 test -s "$HOME/srv.port" || fail "e2eserver did not start"
AGENTCHIME_TEST_UPDATE_URL="http://127.0.0.1:$(tr -d '\n' <"$HOME/srv.port")/"
export AGENTCHIME_TEST_UPDATE_URL

# ---- 4. config, then start the old app and install the Claude hooks ----
printf '%s' '{"doneThresholdSeconds":0,"volumeDone":0.5,"settingsShownOnce":true,"checkForUpdates":false}' >"$DATA/config.json"
"$W/old/agentchime" --background >"$HOME/old.out" 2>&1 &
APP_PID=$!
wait_until 10 have_instance || fail "instance.json did not appear"
OLD_PID=$pid
OLD_PORT=$port
post /api/hooks/claude/install >/dev/null
SETTINGS="$HOME/.claude/settings.json"
grep -q 'hook claude-stop' "$SETTINGS" || fail "settings.json lacks hook claude-stop"
SETTINGS_SUM=$(sha256sum "$SETTINGS")
still_old "start"
phase_is idle || phase_is upToDate || fail "unexpected first phase: $(state)"

# ---- 5. failure cases: nothing changes, the old app keeps running ----
mode down
do_check checkFailed
still_old "down"

mode badsig
do_check unverified
still_old "badsig"

mode redirect-evil
do_check checkFailed
still_old "redirect-evil"

mode ok
do_check available
mode badsum
do_install unverified
still_old "badsum"
mode ok

# A build that does not run at all.
printf '#!/bin/sh\nexit 1\n' >"$W/bad1"
chmod 755 "$W/bad1"
mktar "$W/bad1" "$W/rel/agentchime-settings"
sign
do_check available
do_install badBuild
still_old "bad build"

# A build that prints its version but dies on every other start: the hand-over fails and the
# old files come back.
printf '#!/bin/sh\nif [ "${1:-}" = "--version" ]; then echo 0.9.1; exit 0; fi\nexit 1\n' >"$W/bad2"
chmod 755 "$W/bad2"
mktar "$W/bad2" "$W/rel/agentchime-settings"
sign
do_check available
do_install badBuild
wait_until 5 test ! -e "$DATA/update" || fail "$DATA/update was left behind"
still_old "build that exits"
cmp -s "$DATA/agentchime-settings" "$W/old/agentchime-settings" || fail "rollback: the helper was not restored"

# Sounds still play with the restored program.
SOH=$(printf '\001')
extract() {
	grep "hook $1\"" "$SETTINGS" | head -n 1 |
		sed -e 's/^[^:]*: *"//' -e 's/",\{0,1\}[[:space:]]*$//' \
			-e "s/\\\\\\\\/$SOH/g" -e 's/\\"/"/g' -e "s/$SOH/\\\\/g"
}
PROMPT_CMD=$(extract claude-prompt)
STOP_CMD=$(extract claude-stop)
[ -n "$PROMPT_CMD" ] && [ -n "$STOP_CMD" ] || fail "could not extract hook commands"
before=$(wc -l <"$HOME/pwplay.log")
printf '%s' '{"session_id":"e2e","prompt":"hi"}' | sh -c "$PROMPT_CMD"
printf '%s' '{"session_id":"e2e"}' | sh -c "$STOP_CMD"
sounded() { [ "$(wc -l <"$HOME/pwplay.log")" -gt "$before" ]; }
wait_until 5 sounded || fail "after the rollback the hook played no sound"

[ ! -e "$DATA/update" ] || fail "$DATA/update was left behind"

# ---- 6. success ----
mktar "$W/rel/agentchime" "$W/rel/agentchime-settings"
sign
do_check available
: >"$HOME/xdgopen.log"
post /api/update/install >/dev/null || fail "install: no answer"
swapped() { have_instance && [ "$pid" != "$OLD_PID" ] && alive "$pid"; }
wait_until 30 swapped || fail "no new instance appeared after the install"
NEW_PID=$pid
NEW_PORT=$port
NEW_TOKEN=$token
old_gone() { ! alive "$OLD_PID"; }
wait_until 30 old_gone || fail "the old app is still running"
[ "$NEW_PORT" != "$OLD_PORT" ] || fail "the new instance has the old port"
get /api/ping | grep -q '"app":"agentchime"' || fail "the new instance does not answer /api/ping"
wait_phase updated
state | grep -q '"version":"0.9.1"' || fail "the state does not show version 0.9.1"
opened() { grep -qF "http://127.0.0.1:$NEW_PORT/?t=$NEW_TOKEN" "$HOME/xdgopen.log"; }
wait_until 20 opened || fail "the settings page was not opened for the new instance: $(cat "$HOME/xdgopen.log")"
[ "$(sha256sum "$SETTINGS")" = "$SETTINGS_SUM" ] || fail "settings.json changed during the update"
[ "$("$DATA/agentchime" --version)" = "0.9.1" ] || fail "installed program is not 0.9.1"
wait_until 5 test ! -e "$DATA/update" || fail "$DATA/update was left behind"
cmp -s "$DATA/agentchime" "$W/rel/agentchime" || fail "installed program differs from the release"
cmp -s "$DATA/agentchime-settings" "$W/rel/agentchime-settings" || fail "installed helper differs from the release"

# ---- 7. the newest copy wins ----
"$W/old/agentchime" --background >"$HOME/old2.out" 2>&1 &
OLD2=$!
APP_PID=$OLD2
gone() { ! alive "$OLD2"; }
wait_until 5 gone || fail "the older copy did not exit"
wait "$OLD2" || fail "the older copy exited with an error"
APP_PID=""
alive "$NEW_PID" || fail "0.9.1 died when an older copy started"
[ "$("$DATA/agentchime" --version)" = "0.9.1" ] || fail "an older copy replaced 0.9.1"

quit_app() {
	p=$(inst_pid)
	post /api/quit >/dev/null || fail "quit: no answer"
	dead() { ! alive "$p"; }
	wait_until 10 dead || fail "the app did not quit"
	wait_until 5 test ! -e "$DATA/instance.json" || fail "instance.json still present after quit"
}
quit_app

DEB="$HOME/usr-lib-agentchime"
mkdir -p "$DEB"
cp "$W/old/agentchime" "$DEB/agentchime"
"$DATA/agentchime" --background >"$HOME/deb1.out" 2>&1 &
APP_PID=$!
wait_until 10 have_instance || fail "0.9.1 did not start again"
sleep 3
[ "$("$DATA/agentchime" --version)" = "0.9.1" ] || fail "an older .deb copy replaced 0.9.1"
state | grep -q '"version":"0.9.1"' || fail "the state does not show 0.9.1 with an older .deb copy"
quit_app
APP_PID=""

cp "$W/deb2/agentchime" "$DEB/agentchime"
"$DATA/agentchime" --background >"$HOME/deb2.out" 2>&1 &
APP_PID=$!
is092() { [ "$("$DATA/agentchime" --version 2>/dev/null)" = "0.9.2" ] && have_instance && state | grep -q '"version":"0.9.2"'; }
wait_until 10 is092 || fail "a newer .deb copy did not take over: $(state | grep -o '"version":"[^"]*"')"
quit_app
APP_PID=""

echo "e2e update ok"
