package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/testenv"
	"github.com/quanht99/agent-chime/go/internal/update"
	"github.com/quanht99/agent-chime/go/internal/version"
)

func testPriv(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	raw, err := os.ReadFile("../update/testdata/TEST-ONLY-signing-seed.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		seed, err := base64.StdEncoding.DecodeString(line)
		if err != nil {
			t.Fatal(err)
		}
		return ed25519.NewKeyFromSeed(seed)
	}
	t.Fatal("no seed")
	return nil
}

func allowLocal(u *url.URL) bool { return u.Scheme == "http" && u.Hostname() == "127.0.0.1" }

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

type fakeServer struct {
	srv      *httptest.Server
	requests atomic.Int32
	mu       sync.Mutex
	status   int  // non-zero: every request gets this status
	corrupt  bool // flip a byte of the archive
	body     []byte
	sig      []byte
	// resign serves a signed manifest for another version, with the same archive entry.
	resign func(version string)
}

// newFake serves a signed manifest for serverVersion and a linux/amd64 archive whose program
// prints progVersion for --version.
func newFake(t *testing.T, p paths.Paths, serverVersion, progVersion string) *fakeServer {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script archive")
	}
	priv := testPriv(t)
	f := &fakeServer{}
	arch := tarGz(t, map[string]string{
		filepath.Base(p.Program):        "#!/bin/sh\n[ \"$1\" = --version ] && echo " + progVersion + "\n",
		filepath.Base(p.SettingsHelper): "#!/bin/sh\nexit 2\n",
	})
	sum := sha256.Sum256(arch)
	name := "AgentChime-" + serverVersion + "-linux-amd64.tar.gz"
	sign := func(version string) {
		b, _ := json.Marshal(update.Manifest{
			Format: 1, Version: version, ReleasePage: update.ReleaseBase + "tag/v" + version,
			Files: []update.File{{OS: "linux", Arch: "amd64", Name: name, Size: int64(len(arch)), SHA256: hex.EncodeToString(sum[:])}},
		})
		f.mu.Lock()
		f.body = b
		f.sig = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, b)))
		f.mu.Unlock()
	}
	sign(serverVersion)
	f.resign = sign
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		f.mu.Lock()
		status, corrupt, body, sig := f.status, f.corrupt, f.body, f.sig
		f.mu.Unlock()
		if status != 0 {
			http.Error(w, "x", status)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "latest.json"):
			w.Write(body)
		case strings.HasSuffix(r.URL.Path, "latest.json.sig"):
			w.Write(sig)
		case strings.HasSuffix(r.URL.Path, name):
			b := append([]byte(nil), arch...)
			if corrupt {
				b[len(b)/2] ^= 0xff
			}
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type harness struct {
	p       paths.Paths
	u       *updater
	fake    *fakeServer
	starts  [][]string
	startFn func() error
	quits   int
}

// newHarness builds an updater for current version cur against a server offering serverVersion.
func newHarness(t *testing.T, cur, serverVersion, progVersion string, afterUpdate bool) *harness {
	t.Helper()
	p := testenv.Home(t)
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := version.Version
	version.Version = cur
	t.Cleanup(func() { version.Version = old })
	h := &harness{p: p}
	h.fake = newFake(t, p, serverVersion, progVersion)
	h.u = newUpdater(p, func() { h.quits++ }, afterUpdate)
	if h.u.client != nil {
		h.u.client = &update.Client{
			Base: h.fake.srv.URL + "/", Key: testPriv(t).Public().(ed25519.PublicKey), Allow: allowLocal,
			HTTP: &http.Client{Timeout: 10 * time.Second}, Sleep: func(time.Duration) {},
		}
	}
	h.u.goos, h.u.goarch = "linux", "amd64"
	h.u.start = func(path string, args []string, d time.Duration, e0 bool) error {
		h.starts = append(h.starts, append([]string{path}, args...))
		if h.startFn != nil {
			return h.startFn()
		}
		return nil
	}
	return h
}

func (h *harness) writeOld(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(h.p.Program, []byte("OLD-P"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.p.SettingsHelper, []byte("OLD-H"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (h *harness) writeConfig(t *testing.T, on bool) {
	t.Helper()
	b := `{"checkForUpdates":false}`
	if on {
		b = `{"checkForUpdates":true}`
	}
	if err := os.WriteFile(h.p.Config, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runLoop runs three ticks and returns the durations asked for.
func runLoop(h *harness) []time.Duration {
	stop := make(chan struct{})
	var mu sync.Mutex
	var ds []time.Duration
	done := make(chan struct{})
	after := func(d time.Duration) <-chan time.Time {
		mu.Lock()
		ds = append(ds, d)
		n := len(ds)
		mu.Unlock()
		if n > 3 {
			close(stop)
			return nil
		}
		c := make(chan time.Time, 1)
		c <- time.Now()
		return c
	}
	go func() { h.u.loop(stop, after); close(done) }()
	<-done
	mu.Lock()
	defer mu.Unlock()
	return ds[:3]
}

func wantDurations(t *testing.T, got []time.Duration) {
	t.Helper()
	want := []time.Duration{time.Minute, 24 * time.Hour, 24 * time.Hour}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("durations = %v, want %v", got, want)
		}
	}
}

func TestLoopSwitchOffMakesNoRequests(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeConfig(t, false)
	wantDurations(t, runLoop(h))
	if n := h.fake.requests.Load(); n != 0 {
		t.Fatalf("requests = %d, want 0", n)
	}
}

func TestLoopSwitchOnChecks(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeConfig(t, true)
	wantDurations(t, runLoop(h))
	if n := h.fake.requests.Load(); n != 6 {
		t.Fatalf("requests = %d, want 6", n)
	}
	if ph := h.u.Status().Phase; ph != update.Available {
		t.Fatalf("phase = %s", ph)
	}
}

func TestSourceBuildNeverChecks(t *testing.T) {
	h := newHarness(t, "dev", "0.9.1", "0.9.1", false)
	h.writeConfig(t, true)
	if h.u.client != nil {
		t.Fatal("client should be nil")
	}
	runLoop(h)
	h.u.CheckNow()
	if n := h.fake.requests.Load(); n != 0 {
		t.Fatalf("requests = %d", n)
	}
	if ph := h.u.Status().Phase; ph != update.SourceBuild {
		t.Fatalf("phase = %s", ph)
	}
}

func TestCheckIgnoresOlderOrEqual(t *testing.T) {
	for _, sv := range []string{"0.9.0", "0.8.0"} {
		h := newHarness(t, "0.9.0", sv, sv, false)
		h.writeOld(t)
		h.u.CheckNow()
		if ph := h.u.Status().Phase; ph != update.UpToDate {
			t.Fatalf("%s: phase = %s", sv, ph)
		}
		n := h.fake.requests.Load()
		h.u.Install()
		if h.fake.requests.Load() != n || len(h.starts) != 0 || h.readFile(t, h.p.Program) != "OLD-P" {
			t.Fatalf("%s: Install did something", sv)
		}
	}
}

func TestCheckFailedKeepsLastSuccess(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.u.CheckNow()
	first := h.u.Status().LastSuccess
	if first == nil {
		t.Fatal("no LastSuccess")
	}
	h.fake.mu.Lock()
	h.fake.status = 500
	h.fake.mu.Unlock()
	h.u.CheckNow()
	st := h.u.Status()
	if st.Phase != update.CheckFailed || st.LastSuccess == nil || !st.LastSuccess.Equal(*first) {
		t.Fatalf("status = %+v", st)
	}
}

func TestUpdatedStaysAfterUpToDate(t *testing.T) {
	h := newHarness(t, "0.9.1", "0.9.1", "0.9.1", true)
	if ph := h.u.Status().Phase; ph != update.Updated {
		t.Fatalf("initial phase = %s", ph)
	}
	h.u.CheckNow()
	if ph := h.u.Status().Phase; ph != update.Updated {
		t.Fatalf("phase = %s, want Updated", ph)
	}
}

func TestUpdatedPlusNewerManifestBecomesAvailable(t *testing.T) {
	h := newHarness(t, "0.9.1", "0.9.2", "0.9.2", true)
	h.u.CheckNow()
	st := h.u.Status()
	if st.Phase != update.Available || !st.CanInstall() {
		t.Fatalf("status = %+v, want Available and installable", st)
	}
}

func TestRestartNeededStaysForTheDeployedVersion(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.u.st.Phase = update.RestartNeeded
	h.u.deployed = "0.9.1"
	h.u.CheckNow()
	if ph := h.u.Status().Phase; ph != update.RestartNeeded {
		t.Fatalf("phase = %s, want RestartNeeded", ph)
	}
}

func TestRestartNeededPlusNewerThanDeployedBecomesAvailable(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.2", "0.9.2", false)
	h.u.st.Phase = update.RestartNeeded
	h.u.deployed = "0.9.1"
	h.u.CheckNow()
	if ph := h.u.Status().Phase; ph != update.Available {
		t.Fatalf("phase = %s, want Available", ph)
	}
}

func available(t *testing.T, h *harness) {
	t.Helper()
	h.u.CheckNow()
	if ph := h.u.Status().Phase; ph != update.Available {
		t.Fatalf("phase = %s, want Available", ph)
	}
}

func TestInstallHappyPath(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeOld(t)
	available(t, h)
	h.u.Install()
	if got := h.readFile(t, h.p.Program); !strings.Contains(got, "echo 0.9.1") {
		t.Fatalf("program = %q", got)
	}
	if got := h.readFile(t, h.p.SettingsHelper); !strings.Contains(got, "exit 2") {
		t.Fatalf("helper = %q", got)
	}
	if len(h.starts) != 1 || strings.Join(h.starts[0], " ") != h.p.Program+" --after-update" {
		t.Fatalf("starts = %v", h.starts)
	}
	if h.quits != 1 {
		t.Fatalf("quits = %d", h.quits)
	}
	if _, err := os.Stat(updateDir(h.p)); !os.IsNotExist(err) {
		t.Fatalf("update dir remains: %v", err)
	}
}

func TestInstallPassesTheReadyTimeout(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeOld(t)
	var gotD time.Duration
	var gotE0 bool
	h.u.start = func(path string, args []string, d time.Duration, e0 bool) error {
		gotD, gotE0 = d, e0
		return nil
	}
	available(t, h)
	h.u.Install()
	if gotD != 20*time.Second || gotE0 {
		t.Fatalf("start got %v, %v", gotD, gotE0)
	}
}

func TestInstallRollsBackWhenNewVersionExits(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeOld(t)
	exitErr := exec.Command("false").Run()
	if exitErr == nil {
		t.Skip("no false binary")
	}
	h.startFn = func() error { return exitErr }
	available(t, h)
	h.u.Install()
	if got := h.readFile(t, h.p.Program); got != "OLD-P" {
		t.Fatalf("program = %q", got)
	}
	if got := h.readFile(t, h.p.SettingsHelper); got != "OLD-H" {
		t.Fatalf("helper = %q", got)
	}
	if ph := h.u.Status().Phase; ph != update.BadBuild {
		t.Fatalf("phase = %s", ph)
	}
	if h.quits != 0 {
		t.Fatal("quit was called")
	}
	if _, err := os.Stat(updateDir(h.p)); !os.IsNotExist(err) {
		t.Fatalf("update dir remains: %v", err)
	}
}

func TestInstallTimeoutKeepsNewFiles(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeOld(t)
	h.startFn = func() error { return errNoReady }
	available(t, h)
	h.u.Install()
	if got := h.readFile(t, h.p.Program); !strings.Contains(got, "echo 0.9.1") {
		t.Fatalf("program = %q", got)
	}
	if ph := h.u.Status().Phase; ph != update.RestartNeeded {
		t.Fatalf("phase = %s", ph)
	}
	if h.quits != 0 {
		t.Fatal("quit was called")
	}
}

func TestInstallBadVersionChangesNothing(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.0", false)
	h.writeOld(t)
	available(t, h)
	h.u.Install()
	if ph := h.u.Status().Phase; ph != update.BadBuild {
		t.Fatalf("phase = %s", ph)
	}
	if h.readFile(t, h.p.Program) != "OLD-P" || h.readFile(t, h.p.SettingsHelper) != "OLD-H" {
		t.Fatal("files changed")
	}
	if len(h.starts) != 0 {
		t.Fatal("start was called")
	}
}

func TestInstallBadSum(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeOld(t)
	available(t, h)
	h.fake.mu.Lock()
	h.fake.corrupt = true
	h.fake.mu.Unlock()
	h.u.Install()
	if ph := h.u.Status().Phase; ph != update.Unverified {
		t.Fatalf("phase = %s", ph)
	}
	if h.readFile(t, h.p.Program) != "OLD-P" || len(h.starts) != 0 {
		t.Fatal("something changed")
	}
}

func TestStartRemovesLeftoverUpdateDir(t *testing.T) {
	p := testenv.Home(t)
	x := filepath.Join(updateDir(p), "previous", "x")
	if err := os.MkdirAll(filepath.Dir(x), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(x, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanUpdateDir(p)
	if _, err := os.Stat(updateDir(p)); !os.IsNotExist(err) {
		t.Fatalf("update dir remains: %v", err)
	}
}

func TestBeginCheckAndInstallSetThePhaseSynchronously(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	if _, ok := h.u.beginCheck(); !ok {
		t.Fatal("beginCheck refused")
	}
	if ph := h.u.Status().Phase; ph != update.Checking {
		t.Fatalf("phase = %s, want Checking", ph)
	}
	if _, ok := h.u.beginCheck(); ok {
		t.Fatal("a second click started a second check")
	}
	h = newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	available(t, h)
	if _, ok := h.u.beginInstall(); !ok {
		t.Fatal("beginInstall refused")
	}
	if ph := h.u.Status().Phase; ph != update.Downloading {
		t.Fatalf("phase = %s, want Downloading", ph)
	}
	if _, ok := h.u.beginInstall(); ok {
		t.Fatal("a second click started a second install")
	}
}

func TestInstallRefusesAnOlderRefetchedManifest(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.writeOld(t)
	available(t, h)
	// The first archive is corrupt; before the refetch the release is pulled back to 0.8.0.
	h.fake.mu.Lock()
	h.fake.corrupt = true
	h.fake.mu.Unlock()
	h.u.client.Sleep = func(time.Duration) {
		h.fake.mu.Lock()
		h.fake.corrupt = false
		h.fake.mu.Unlock()
		h.fake.resign("0.8.0")
	}
	h.u.Install()
	if ph := h.u.Status().Phase; ph != update.Unverified {
		t.Fatalf("phase = %s, want Unverified", ph)
	}
	if h.readFile(t, h.p.Program) != "OLD-P" || len(h.starts) != 0 || h.quits != 0 {
		t.Fatal("a downgrade was installed")
	}
}

func TestNoKeyMakesNoRequest(t *testing.T) {
	h := newHarness(t, "0.9.0", "0.9.1", "0.9.1", false)
	h.u.client.Key = nil
	slept := false
	h.u.client.Sleep = func(time.Duration) { slept = true }
	h.u.CheckNow()
	h.u.CheckNow()
	if n := h.fake.requests.Load(); n != 0 || slept {
		t.Fatalf("requests = %d, slept = %v", n, slept)
	}
	if ph := h.u.Status().Phase; ph != update.Unverified {
		t.Fatalf("phase = %s", ph)
	}
	b, _ := os.ReadFile(h.p.Log)
	if strings.Count(string(b), "no release key") != 1 {
		t.Fatalf("log = %q", b)
	}
}
