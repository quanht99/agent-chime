#!/bin/sh
# GUI smoke test for the Linux settings window. Runs inside the gui-smoke Docker image (or a CI
# runner) under Xvfb, with a temp HOME. Start it with: dbus-run-session -- sh gui-smoke-linux.sh
# Uses only POSIX sh. Never touches the real home.
set -eu

[ -f /.dockerenv ] || [ "${GITHUB_ACTIONS:-}" = true ] || { echo "refusing to run outside Docker or GitHub Actions" >&2; exit 2; }

SRC=$(cd "$(dirname "$0")/.." && pwd)

# Go's caches must not follow HOME into the temp dir (see e2e-linux.sh).
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

SMOKE_OUT=${SMOKE_OUT:-$(mktemp -d)}
mkdir -p "$SMOKE_OUT"
HOME=$(mktemp -d)
export HOME
export AGENTCHIME_HOME="$HOME"
DATA="$HOME/.local/share/agentchime"
LOG="$DATA/agentchime.log"
APP_PID=""
XVFB_PID=""
WM_PID=""
DATA2=""
HOME2=""

# Test-only WebKit settings: no sandbox (no user namespaces in Docker), no DMABUF, software GL.
# They live only in this script, never in the app.
export WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1
export WEBKIT_DISABLE_DMABUF_RENDERER=1
export LIBGL_ALWAYS_SOFTWARE=1

cleanup() {
	code=$?
	if [ -n "$APP_PID" ]; then kill "$APP_PID" 2>/dev/null || true; fi
	# Only helpers started from this run's own data folders.
	pkill -f "$DATA/agentchime-settings" 2>/dev/null || true
	if [ -n "$HOME2" ]; then pkill -f "$HOME2/.local/share/agentchime/agentchime-settings" 2>/dev/null || true; fi
	if [ -n "$WM_PID" ]; then kill "$WM_PID" 2>/dev/null || true; fi
	if [ -n "$XVFB_PID" ]; then kill "$XVFB_PID" 2>/dev/null || true; fi
	if [ "$code" -ne 0 ]; then
		echo "gui-smoke FAILED" >&2
		for l in "$LOG" "$DATA2/agentchime.log"; do
			if [ -f "$l" ]; then
				cp "$l" "$SMOKE_OUT/failed-$(basename "$(dirname "$l")").log" 2>/dev/null || true
				echo "--- log tail ($l) ---" >&2
				tail -n 40 "$l" >&2
			fi
		done
		DISPLAY=:99 import -window root "$SMOKE_OUT/failure-root.png" 2>/dev/null || true
	fi
	chmod -R u+w "$HOME" 2>/dev/null || true
	rm -rf "$HOME"
	if [ -n "$HOME2" ]; then chmod -R u+w "$HOME2" 2>/dev/null || true; rm -rf "$HOME2"; fi
	if [ -n "$CACHE_TMP" ]; then
		chmod -R u+w "$CACHE_TMP" 2>/dev/null || true
		rm -rf "$CACHE_TMP"
	fi
	exit "$code"
}
trap cleanup EXIT INT TERM

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

# Xvfb and a window manager (wmctrl and window activation need one).
Xvfb :99 -screen 0 1280x1024x24 >"$SMOKE_OUT/xvfb.log" 2>&1 &
XVFB_PID=$!
DISPLAY=:99
export DISPLAY
i=0
until xdpyinfo >/dev/null 2>&1; do
	i=$((i + 1))
	[ "$i" -lt 100 ] || fail "Xvfb did not start"
	sleep 0.1
done
openbox >"$SMOKE_OUT/openbox.log" 2>&1 &
WM_PID=$!
sleep 1

# Fakes first on PATH. xdg-open must never open anything.
mkdir -p "$HOME/fakebin" "$HOME/build" "$DATA"
cat >"$HOME/fakebin/pw-play" <<FAKE
#!/bin/sh
echo "\$@" >"$HOME/pwplay.args"
FAKE
cat >"$HOME/fakebin/xdg-open" <<FAKE
#!/bin/sh
echo "\$@" >"$HOME/xdg-open.args"
FAKE
chmod 755 "$HOME/fakebin/pw-play" "$HOME/fakebin/xdg-open"
PATH="$HOME/fakebin:$PATH"
export PATH

printf '%s' '{"settingsShownOnce":true,"checkForUpdates":false}' >"$DATA/config.json"

(cd "$SRC" && CGO_ENABLED=0 go build -o "$HOME/build/agentchime" ./cmd/agentchime)
(cd "$SRC" && CGO_ENABLED=1 go build -tags production -o "$HOME/build/agentchime-settings" ./cmd/agentchime-settings)

# --- helpers -------------------------------------------------------------------------------

wait_instance() { # waits for a fresh instance.json, sets port and token
	i=0
	while [ ! -f "$DATA/instance.json" ] && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
	[ -f "$DATA/instance.json" ] || fail "instance.json did not appear"
	port=$(sed -n 's/.*"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")
	token=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$DATA/instance.json")
	[ -n "$port" ] && [ -n "$token" ] || fail "could not read port/token"
}

api() { # api <path> [GET]
	if [ "${2:-POST}" = GET ]; then
		curl -fsS -H "Host: 127.0.0.1:$port" -H "X-AgentChime-Token: $token" "http://127.0.0.1:$port$1"
	else
		curl -fsS -X POST -H "Host: 127.0.0.1:$port" -H "X-AgentChime-Token: $token" \
			-H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:$port$1"
	fi
}

find_window() { # prints the window id, or fails
	w=$(timeout 30 xdotool search --sync --onlyvisible --class agentchime 2>/dev/null | head -n 1 || true)
	[ -n "$w" ] || fail "no visible agentchime window within 30 s"
	echo "$w"
}

helper_count() { pgrep -fxc "$DATA/agentchime-settings" 2>/dev/null || true; }

wait_helper_gone() { # wait_helper_gone <tenths>
	i=0
	while [ "$(helper_count)" != 0 ] && [ "$i" -lt "$1" ]; do sleep 0.1; i=$((i + 1)); done
	[ "$(helper_count)" = 0 ]
}

wait_pid_gone() { # wait_pid_gone <pid> <tenths>
	i=0
	while kill -0 "$1" 2>/dev/null && [ "$i" -lt "$2" ]; do sleep 0.1; i=$((i + 1)); done
	! kill -0 "$1" 2>/dev/null
}

quit_app() {
	api /api/quit >/dev/null || fail "/api/quit failed"
	wait_pid_gone "$APP_PID" 80 || fail "app did not exit after /api/quit"
	wait "$APP_PID" 2>/dev/null || true
	APP_PID=""
}

# --- 1. the window opens -------------------------------------------------------------------
"$HOME/build/agentchime" >"$HOME/app.out" 2>&1 &
APP_PID=$!
wait_instance
WID=$(find_window)
title=$(xdotool getwindowname "$WID")
[ "$title" = "AgentChime Settings" ] || fail "window title is '$title'"
[ -x "$DATA/agentchime-settings" ] || fail "helper was not deployed to the data folder"
sleep 5
[ ! -e "$HOME/xdg-open.args" ] || fail "the browser was opened although the window showed"
import -window "$WID" "$SMOKE_OUT/1-window.png"
colors=$(identify -format %k "$SMOKE_OUT/1-window.png")
[ "$colors" -gt 20 ] || fail "window screenshot has only $colors colors (blank page?)"
pass "1 the window opens ($colors colors)"

# --- 1b. the file dialog opens -------------------------------------------------------------
HPID=$(pgrep -fx "$DATA/agentchime-settings" | head -n 1 || true)
[ -n "$HPID" ] || fail "helper pid not found"
count_windows() { xdotool search --onlyvisible --pid "$HPID" 2>/dev/null | wc -l | tr -d ' '; }
before=$(count_windows)
wait_dialog() { # waits up to 10 s for more visible windows than before
	i=0
	while [ "$i" -lt 100 ]; do
		[ "$(count_windows)" -gt "$before" ] && return 0
		sleep 0.1
		i=$((i + 1))
	done
	return 1
}
xdotool windowactivate --sync "$WID" || true
xdotool key Tab Down End Return
if wait_dialog; then
	dialog_via=keyboard
else
	import -window root "$SMOKE_OUT/1b-keyboard-attempt.png"
	# Keyboard focus did not reach the picker: click the Done picker instead.
	geom=$(xdotool getwindowgeometry --shell "$WID")
	echo "window geometry: $(echo "$geom" | tr '\n' ' ')" >"$SMOKE_OUT/1b-click.txt"
	fail "file dialog did not open via the keyboard (see 1b-keyboard-attempt.png)"
fi
sleep 1
import -window root "$SMOKE_OUT/1b-file-dialog.png"
xdotool key Escape
sleep 1
pass "1b the file dialog opens (via $dialog_via; windows $before -> more)"

# --- 2. one helper only --------------------------------------------------------------------
rm -f "$HOME/xdg-open.args"
"$HOME/build/agentchime" >"$HOME/app2.out" 2>&1 &
SECOND=$!
wait_pid_gone "$SECOND" 200 || { kill "$SECOND" 2>/dev/null || true; fail "second launch did not exit within 20 s"; }
wait "$SECOND" || fail "second launch exited with an error"
n=$(helper_count)
[ "$n" = 1 ] || fail "expected 1 helper, found $n"
[ ! -e "$HOME/xdg-open.args" ] || fail "the second launch opened the browser"
pass "2 one helper only"

# --- 3. closing the window keeps the tray --------------------------------------------------
wmctrl -i -c "$WID"
wait_helper_gone 50 || fail "helper still running 5 s after the window was closed"
kill -0 "$APP_PID" 2>/dev/null || fail "main app died when the window closed"
api /api/ping GET >/dev/null || fail "main app does not answer /api/ping"
pass "3 closing the window keeps the tray"

# --- 4. quit closes the window -------------------------------------------------------------
"$HOME/build/agentchime" >"$HOME/app3.out" 2>&1 &
THIRD=$!
wait_pid_gone "$THIRD" 200 || { kill "$THIRD" 2>/dev/null || true; fail "launch did not exit"; }
WID=$(find_window)
api /api/quit >/dev/null || fail "/api/quit failed"
wait_pid_gone "$APP_PID" 80 || fail "app did not exit after /api/quit"
wait "$APP_PID" 2>/dev/null || true
APP_PID=""
wait_helper_gone 80 || fail "helper still running 8 s after quit"
pass "4 quit closes the window"

# --- 5. without a helper, the browser opens ------------------------------------------------
rm -f "$DATA/agentchime-settings" "$HOME/build/agentchime-settings" "$HOME/xdg-open.args"
"$HOME/build/agentchime" >"$HOME/app4.out" 2>&1 &
APP_PID=$!
wait_instance
i=0
while [ ! -s "$HOME/xdg-open.args" ] && [ "$i" -lt 200 ]; do sleep 0.1; i=$((i + 1)); done
[ -s "$HOME/xdg-open.args" ] || fail "xdg-open was not called within 20 s"
got=$(cat "$HOME/xdg-open.args")
[ "$got" = "http://127.0.0.1:$port/?t=$token" ] || fail "xdg-open got an unexpected URL (not printed: contains the token)"
grep -q 'settings window: ' "$LOG" || fail "log lacks 'settings window: '"
quit_app
pass "5 without a helper the browser opens"

# --- 6. the .deb on Ubuntu 24.04 -----------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
	echo "SKIP: 6 the .deb needs root"
else
	ARCH=$(go env GOARCH)
	(cd "$SRC" && OUT="$SMOKE_OUT" bash scripts/package-linux.sh 0.0.0 "$ARCH" >"$SMOKE_OUT/package-linux.log" 2>&1) ||
		{ cat "$SMOKE_OUT/package-linux.log" >&2; fail "package-linux.sh failed"; }
	dpkg -i "$SMOKE_OUT/AgentChime-0.0.0-linux-$ARCH.deb" >"$SMOKE_OUT/dpkg-i.log" 2>&1 || { cat "$SMOKE_OUT/dpkg-i.log" >&2; fail "dpkg -i failed"; }
	HOME2=$(mktemp -d)
	DATA2="$HOME2/.local/share/agentchime"
	mkdir -p "$DATA2"
	printf '%s' '{"settingsShownOnce":true,"checkForUpdates":false}' >"$DATA2/config.json"
	rm -f "$HOME/xdg-open.args"
	(
		HOME="$HOME2"
		AGENTCHIME_HOME="$HOME2"
		export HOME AGENTCHIME_HOME
		/usr/bin/agentchime >"$HOME2/app.out" 2>&1 &
		echo $! >"$HOME2/app.pid"
	)
	APP_PID=$(cat "$HOME2/app.pid")
	DATA_SAVE=$DATA
	DATA=$DATA2
	i=0
	while [ ! -f "$DATA/instance.json" ] && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
	[ -f "$DATA/instance.json" ] || fail "deb: instance.json did not appear"
	port=$(sed -n 's/.*"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")
	token=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$DATA/instance.json")
	WID=$(find_window)
	cmp "$DATA2/agentchime-settings" /usr/lib/agentchime/agentchime-settings || fail "deb: deployed helper differs from the package"
	import -window "$WID" "$SMOKE_OUT/6-deb-window.png"
	api /api/quit >/dev/null || fail "deb: /api/quit failed"
	wait_pid_gone "$APP_PID" 80 || fail "deb: app did not exit"
	APP_PID=""
	DATA=$DATA_SAVE
	dpkg -r agentchime >"$SMOKE_OUT/dpkg-r.log" 2>&1 || fail "dpkg -r failed"
	pass "6 the .deb installs and shows the window"
fi

# --- extra folders WebKit made (for the README) -------------------------------------------
find "$HOME" -mindepth 1 -maxdepth 3 -type d | sed "s#^$HOME#~#" | grep -v -e '^~/\.local/share/agentchime' -e '^~/build' -e '^~/fakebin' | sort >"$SMOKE_OUT/extra-folders.txt" || true

echo "gui-smoke ok"
