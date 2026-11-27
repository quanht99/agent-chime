#!/bin/sh
# Debian 12 check for the .deb: it installs without WebKitGTK, Settings falls back to the browser
# and sounds still play. Runs inside a debian:12 container. Usage: deb-fallback-smoke.sh <deb>
set -eu

[ -f /.dockerenv ] || [ "${GITHUB_ACTIONS:-}" = true ] || { echo "refusing to run outside Docker or GitHub Actions" >&2; exit 2; }
[ $# -ge 1 ] || { echo "usage: deb-fallback-smoke.sh <deb>" >&2; exit 1; }
DEB=$1
[ -f "$DEB" ] || { echo "no such file: $DEB" >&2; exit 1; }

HOME=$(mktemp -d)
export HOME
export AGENTCHIME_HOME="$HOME"
DATA="$HOME/.local/share/agentchime"
LOG="$DATA/agentchime.log"
APP_PID=""

cleanup() {
	code=$?
	if [ -n "$APP_PID" ]; then kill "$APP_PID" 2>/dev/null || true; fi
	if [ "$code" -ne 0 ]; then
		echo "deb-fallback FAILED" >&2
		if [ -f "$LOG" ]; then echo "--- log tail ---" >&2; tail -n 40 "$LOG" >&2; fi
	fi
	rm -rf "$HOME"
	exit "$code"
}
trap cleanup EXIT INT TERM

fail() { echo "FAIL: $*" >&2; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update >/dev/null
# Dry run with Recommends: apt must not refuse the package when GTK 4.10 cannot be met.
apt-get install -s "$DEB" >/dev/null || fail "apt-get install -s refused the package"
# Real install without Recommends: the machine has no WebKitGTK.
apt-get install -y --no-install-recommends "$DEB" curl >/dev/null || fail "apt-get install failed"
n=$(ldconfig -p | grep -c libwebkitgtk-6.0 || true)
[ "$n" = 0 ] || fail "libwebkitgtk-6.0 is installed ($n), so this is not the no-WebKitGTK case"
echo "PASS: installed on Debian 12 without WebKitGTK"

mkdir -p "$DATA" "$HOME/fakebin"
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
printf '%s' '{"doneThresholdSeconds":0,"volumeDone":0.5,"settingsShownOnce":true,"checkForUpdates":false}' >"$DATA/config.json"

agentchime >"$HOME/app.out" 2>&1 &
APP_PID=$!
i=0
while [ ! -f "$DATA/instance.json" ] && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
[ -f "$DATA/instance.json" ] || fail "instance.json did not appear"
port=$(sed -n 's/.*"port": *\([0-9][0-9]*\).*/\1/p' "$DATA/instance.json")
token=$(sed -n 's/.*"token": *"\([^"]*\)".*/\1/p' "$DATA/instance.json")
[ -n "$port" ] && [ -n "$token" ] || fail "could not read port/token"

i=0
while [ ! -s "$HOME/xdg-open.args" ] && [ "$i" -lt 200 ]; do sleep 0.1; i=$((i + 1)); done
[ -s "$HOME/xdg-open.args" ] || fail "xdg-open was not called within 20 s"
got=$(cat "$HOME/xdg-open.args")
[ "$got" = "http://127.0.0.1:$port/?t=$token" ] || fail "xdg-open got an unexpected URL (not printed: contains the token)"
echo "PASS: Settings opened in the browser"

printf '%s' '{"session_id":"deb","prompt":"hi"}' | agentchime hook claude-prompt
printf '%s' '{"session_id":"deb"}' | agentchime hook claude-stop
i=0
while [ ! -s "$HOME/pwplay.args" ] && [ "$i" -lt 30 ]; do sleep 0.1; i=$((i + 1)); done
[ -s "$HOME/pwplay.args" ] || fail "pw-play was not invoked"
echo "PASS: sounds play"

curl -fsS -X POST -H "Host: 127.0.0.1:$port" -H "X-AgentChime-Token: $token" \
	-H 'Content-Type: application/json' -d '{}' "http://127.0.0.1:$port/api/quit" >/dev/null || fail "/api/quit failed"
i=0
while kill -0 "$APP_PID" 2>/dev/null && [ "$i" -lt 80 ]; do sleep 0.1; i=$((i + 1)); done
if kill -0 "$APP_PID" 2>/dev/null; then fail "app did not exit after /api/quit"; fi
APP_PID=""
echo "deb-fallback ok"
