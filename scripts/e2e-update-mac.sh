#!/bin/bash
# End-to-end test of the in-app self-update on macOS. Runs on a Mac and touches only a temp folder:
# it builds two test copies of the app (0.9.0 and 0.9.1, with the TEST-ONLY update key), serves 0.9.1
# from a local signed server, lets a copy of 0.9.0 update itself, and checks the result.
# Never touches /Applications, ~/Library or ~/.claude: AGENTCHIME_HOME is a temp folder, and the app
# copy lives in the temp folder too. Only processes this script started are signalled.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
GO_DIR="$ROOT/go"
LSREGISTER=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
[ "$(uname)" = Darwin ] || { echo "e2e-update-mac.sh runs on macOS only" >&2; exit 1; }

T=$(mktemp -d)
export AGENTCHIME_HOME="$T/home"
APPDIR="$T/apps"
APP="$APPDIR/AgentChime.app"
LOG="$AGENTCHIME_HOME/Library/Logs/AgentChime/agentchime.log"
SRV_PID=""
APP_PID=""
mkdir -p "$AGENTCHIME_HOME" "$APPDIR" "$T/srv"

# The old bundle is moved to the user's Trash after an update. Remember what was there, so only
# the test copy this run added can be removed again.
TRASH="$HOME/.Trash"
TRASH_BEFORE=$(ls -1 "$TRASH" 2>/dev/null || true)

app_running() { pgrep -f "$APP" >/dev/null 2>&1; }

cleanup() {
	code=$?
	set +e
	[ -n "$APP_PID" ] && kill "$APP_PID" 2>/dev/null
	pkill -f "$APP" 2>/dev/null # only copies under our temp folder
	[ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null
	if [ "$code" -ne 0 ]; then
		echo "e2e update mac FAILED" >&2
		if [ -f "$LOG" ]; then echo "--- log tail ---" >&2; tail -n 40 "$LOG" >&2; fi
	fi
	# Forget the temp apps in Launch Services before they are deleted.
	for a in "$APP" "$T"/dd-*/Build/Products/Release/AgentChime.app; do
		[ -d "$a" ] && "$LSREGISTER" -u "$a" >/dev/null 2>&1
	done
	# Remove the 0.9.0 bundle the updater trashed (only a new entry whose version is 0.9.0).
	for e in "$TRASH"/AgentChime*; do
		[ -d "$e" ] || continue
		printf '%s\n' "$TRASH_BEFORE" | grep -qxF "$(basename "$e")" && continue
		if [ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$e/Contents/Info.plist" 2>/dev/null)" = "0.9.0" ]; then
			chmod -R u+w "$e" 2>/dev/null
			rm -rf "$e"
		fi
	done
	chmod -R u+w "$T" 2>/dev/null
	rm -rf "$T"
	exit "$code"
}
trap cleanup EXIT INT TERM

fail() { echo "FAIL: $*" >&2; exit 1; }

# wait_until SECONDS COMMAND...
wait_until() {
	local n=$(($1 * 10)) i=0
	shift
	while [ "$i" -lt "$n" ]; do
		if "$@"; then return 0; fi
		sleep 0.1
		i=$((i + 1))
	done
	return 1
}

# alive PID: running and not a zombie.
alive() {
	local st
	st=$(ps -o stat= -p "$1" 2>/dev/null | tr -d ' ') || return 1
	[ -n "$st" ] && [ "${st#Z}" = "$st" ]
}

# The owner's real app, if it runs, must be unharmed at the end.
REAL=/Applications/AgentChime.app
REAL_PIDS=$(pgrep -f "^$REAL/Contents/MacOS/AgentChime" || true)
real_sum() {
	[ -d "$REAL" ] || { echo none; return; }
	(cd "$REAL" && find . -type f -print0 | sort -z | xargs -0 stat -f '%N %z %m' | shasum | cut -d' ' -f1)
}
REAL_SUM=$(real_sum)

# ---- 1. build 0.9.0 and 0.9.1 (Release, release marker on, TEST-ONLY update key) ----
cd "$ROOT"
xcodegen generate >/dev/null
for v in 0.9.0 0.9.1; do
	set +e
	xcodebuild -project AgentChime.xcodeproj -scheme AgentChime -configuration Release \
		-derivedDataPath "$T/dd-$v" \
		MARKETING_VERSION="$v" AGENTCHIME_RELEASE=YES \
		SWIFT_ACTIVE_COMPILATION_CONDITIONS='$(inherited) AGENTCHIME_UPDATE_TEST' \
		build 2>&1 | grep -E "error:|BUILD (SUCCEEDED|FAILED)"
	status=${PIPESTATUS[0]}
	set -e
	[ "$status" -eq 0 ] || fail "xcodebuild $v failed"
	[ -d "$T/dd-$v/Build/Products/Release/AgentChime.app" ] || fail "no app for $v"
done
APP090="$T/dd-0.9.0/Build/Products/Release/AgentChime.app"
APP091="$T/dd-0.9.1/Build/Products/Release/AgentChime.app"

# ---- 2. sign 0.9.1 and serve it ----
(cd "$GO_DIR" && go build -tags updatetest -o "$T/signmanifest" ./scripts/signmanifest && go build -o "$T/e2eserver" ./scripts/e2eserver)
ditto -c -k --keepParent "$APP091" "$T/srv/AgentChime-0.9.1-macos.zip"
UPDATE_SIGNING_KEY=$(tail -1 "$GO_DIR/internal/update/testdata/TEST-ONLY-signing-seed.txt") \
	"$T/signmanifest" -version 0.9.1 -require macos -dir "$T/srv" -out "$T/srv" >/dev/null || fail "signmanifest failed"
"$T/e2eserver" -dir "$T/srv" -portfile "$T/srv.port" >"$T/srv.out" 2>&1 &
SRV_PID=$!
wait_until 10 test -s "$T/srv.port" || fail "e2eserver did not start"
export AGENTCHIME_TEST_UPDATE_URL="http://127.0.0.1:$(tr -d '\n' <"$T/srv.port")/"

# ---- 3. the old app, a copy in the temp folder, with updates on ----
ditto "$APP090" "$APP"
CONF="$AGENTCHIME_HOME/Library/Application Support/AgentChime"
mkdir -p "$CONF"
printf '%s' '{"checkForUpdates":true,"settingsShownOnce":true}' >"$CONF/config.json"
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")" = "0.9.0" ] || fail "the copy is not 0.9.0"
AGENTCHIME_TEST_UPDATE_DELAY=2 AGENTCHIME_TEST_UPDATE_AUTOINSTALL=1 \
	"$APP/Contents/MacOS/AgentChime" >"$T/app.out" 2>&1 &
APP_PID=$!
sleep 1
alive "$APP_PID" || fail "0.9.0 did not start"

# ---- 4. it finds 0.9.1 and installs it ----
started091() { [ -f "$LOG" ] && grep -q 'started 0.9.1' "$LOG"; }
wait_until 60 started091 || fail "the log never shows 'started 0.9.1'"
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")" = "0.9.1" ] || fail "the app on disk is not 0.9.1"
old_gone() { ! alive "$APP_PID"; }
wait_until 20 old_gone || fail "the 0.9.0 process is still running"
APP_PID=""
if xattr -r "$APP" 2>/dev/null | grep -q 'com.apple.quarantine'; then fail "the updated app has the quarantine flag"; fi
codesign --verify --deep --strict "$APP" || fail "codesign --verify failed on the updated app"
grep -q 'update: swapped in 0.9.1' "$LOG" || fail "the log lacks the swap line"

# ---- 5. a plain launch of the updated app works (no Gatekeeper prompt) ----
pkill -f "$APP" 2>/dev/null || true
none_running() { ! app_running; }
wait_until 10 none_running || fail "the updated app did not stop"
open -n --env "AGENTCHIME_HOME=$AGENTCHIME_HOME" "$APP"
wait_until 10 app_running || fail "the updated app did not start with open"
sleep 2
app_running || fail "the updated app quit right after it started"
pkill -f "$APP" 2>/dev/null || true
wait_until 10 none_running || fail "the updated app did not stop at the end"

# ---- 6. rollback: the new version dies before it says `ready` ----
# A fresh 0.9.0 whose relaunched copy (it inherits the environment) exits 1 at once after the update.
: >"$LOG"
rm -rf "$APP"
ditto "$APP090" "$APP"
AGENTCHIME_TEST_FAIL_AFTER_UPDATE=1 AGENTCHIME_TEST_UPDATE_DELAY=2 AGENTCHIME_TEST_UPDATE_AUTOINSTALL=1 \
	"$APP/Contents/MacOS/AgentChime" >"$T/app2.out" 2>&1 &
APP_PID=$!
sleep 1
alive "$APP_PID" || fail "0.9.0 (rollback case) did not start"
rolled_back() { [ -f "$LOG" ] && grep -q 'rolled back' "$LOG"; }
wait_until 60 rolled_back || fail "the log never shows the rollback"
grep -q 'swapped in 0.9.1' "$LOG" || fail "the rollback case never swapped"
if grep -q 'started 0.9.1' "$LOG"; then fail "0.9.1 reported started in the rollback case"; fi
[ "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")" = "0.9.0" ] || fail "the bundle is not 0.9.0 again after the rollback"
[ "$("$APP/Contents/MacOS/AgentChime" --version)" = "0.9.0" ] || fail "--version of the rolled-back bundle is not 0.9.0"
alive "$APP_PID" || fail "the old app is not running after the rollback"
sleep 1
[ "$(pgrep -f "$APP" | wc -l | tr -d ' ')" = "1" ] || fail "more than the old app is running after the rollback"
codesign --verify --deep --strict "$APP" || fail "codesign --verify failed on the rolled-back app"
kill "$APP_PID" 2>/dev/null || true
APP_PID=""
wait_until 10 none_running || fail "the rolled-back app did not stop"

# ---- 7. the owner's own app was not touched ----
for p in $REAL_PIDS; do alive "$p" || fail "the app in /Applications (pid $p) is no longer running"; done
[ "$(real_sum)" = "$REAL_SUM" ] || fail "/Applications/AgentChime.app changed"

echo "e2e update mac ok"
