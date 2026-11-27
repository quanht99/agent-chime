package web

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/update"
)

type fakeUpdates struct {
	mu    sync.Mutex
	st    update.Status
	calls chan string
	err   error
}

func newFakeUpdates(st update.Status) *fakeUpdates {
	return &fakeUpdates{st: st, calls: make(chan string, 8)}
}

func (f *fakeUpdates) Status() update.Status { f.mu.Lock(); defer f.mu.Unlock(); return f.st }
func (f *fakeUpdates) StartCheck()           { f.calls <- "check" }
func (f *fakeUpdates) StartInstall()         { f.calls <- "install" }
func (f *fakeUpdates) OpenReleasePage() error {
	f.calls <- "release"
	return f.err
}
func (f *fakeUpdates) Restart() error { f.calls <- "restart"; return nil }

func (f *fakeUpdates) wait(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-f.calls:
		if got != want {
			t.Fatalf("call %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("%q was not called", want)
	}
}

func withUpdates(f *fakeUpdates) opt {
	return func(_ *harness, d *Deps) { d.Updates = f }
}

var updateRoutes = []string{"/api/update/check", "/api/update/install", "/api/update/release-page", "/api/update/restart"}

func TestUpdateRoutesNeedToken(t *testing.T) {
	f := newFakeUpdates(update.Status{})
	h := newHarness(t, withUpdates(f))
	for _, route := range updateRoutes {
		r := h.req("POST", route, map[string]any{}, func(r *http.Request) { r.Header.Del("X-AgentChime-Token") })
		if r.status != 401 {
			t.Errorf("%s without token: %d", route, r.status)
		}
	}
	select {
	case c := <-f.calls:
		t.Fatalf("called %s without token", c)
	default:
	}
}

func TestUpdateCheckCallsCheckNow(t *testing.T) {
	f := newFakeUpdates(update.Status{Phase: update.Checking, Current: "0.3.0"})
	h := newHarness(t, withUpdates(f))
	r := h.post("/api/update/check", map[string]any{})
	if r.status != 200 || r.json()["update"] == nil {
		t.Fatalf("%d %s", r.status, r.body)
	}
	f.wait(t, "check")
}

func TestUpdateInstallCallsInstall(t *testing.T) {
	f := newFakeUpdates(update.Status{Phase: update.Available, Current: "0.3.0", Latest: "0.3.1"})
	h := newHarness(t, withUpdates(f))
	if r := h.post("/api/update/install", map[string]any{}); r.status != 200 || r.json()["update"] == nil {
		t.Fatalf("%d %s", r.status, r.body)
	}
	f.wait(t, "install")
}

func TestReleasePageCalls(t *testing.T) {
	f := newFakeUpdates(update.Status{})
	h := newHarness(t, withUpdates(f))
	if r := h.post("/api/update/release-page", map[string]any{}); r.status != 200 {
		t.Fatalf("%d %s", r.status, r.body)
	}
	f.wait(t, "release")
}

func TestRestartAnswersThenRestarts(t *testing.T) {
	f := newFakeUpdates(update.Status{})
	h := newHarness(t, withUpdates(f))
	r := h.post("/api/update/restart", map[string]any{})
	if r.status != 200 || r.json()["ok"] != true {
		t.Fatalf("%d %s", r.status, r.body)
	}
	f.wait(t, "restart")
}

func TestStateHasUpdate(t *testing.T) {
	f := newFakeUpdates(update.Status{Phase: update.Available, Current: "0.3.0", Latest: "0.3.1", ReleasePage: "https://example.com/r"})
	h := newHarness(t, withUpdates(f))
	u := h.req("GET", "/api/state", nil).json()["update"].(map[string]any)
	if u["text"] != "Version 0.3.1 is available" || u["canInstall"] != true || u["checkAuto"] != true {
		t.Fatalf("%v", u)
	}
	if u["phase"] != "available" || u["current"] != "0.3.0" || u["latest"] != "0.3.1" || u["busy"] != false || u["lastChecked"] != "" {
		t.Fatalf("%v", u)
	}
	f.mu.Lock()
	f.st.Phase = update.Downloading
	f.mu.Unlock()
	if u := h.req("GET", "/api/state", nil).json()["update"].(map[string]any); u["busy"] != true || u["canInstall"] != false {
		t.Fatalf("%v", u)
	}
}

func TestStateUpdateLastChecked(t *testing.T) {
	ts := time.Date(2026, 11, 27, 9, 5, 0, 0, time.Local)
	f := newFakeUpdates(update.Status{Phase: update.UpToDate, Current: "0.3.0", LastSuccess: &ts})
	h := newHarness(t, withUpdates(f))
	if u := h.req("GET", "/api/state", nil).json()["update"].(map[string]any); u["lastChecked"] != "Last checked 2026-11-27 09:05" {
		t.Fatalf("%v", u)
	}
	c := config.Load(h.p)
	c.CheckForUpdates = false
	if err := config.Save(h.p, c); err != nil {
		t.Fatal(err)
	}
	if u := h.req("GET", "/api/state", nil).json()["update"].(map[string]any); u["checkAuto"] != false {
		t.Fatalf("%v", u)
	}
}

func TestUpdateTextEveryPhase(t *testing.T) {
	en := func(k i18n.Key, a ...string) string { return i18n.T(k, i18n.EN, a...) }
	cases := []struct {
		st   update.Status
		want string
	}{
		{update.Status{Phase: update.Idle}, ""},
		{update.Status{Phase: update.Checking}, "Checking…"},
		{update.Status{Phase: update.UpToDate}, "You're up to date"},
		{update.Status{Phase: update.Available, Latest: "0.3.1"}, "Version 0.3.1 is available"},
		{update.Status{Phase: update.Downloading}, "Downloading…"},
		{update.Status{Phase: update.CheckFailed}, "Couldn't check for updates"},
		{update.Status{Phase: update.Unverified}, "The update couldn't be verified, so it was skipped"},
		{update.Status{Phase: update.Failed, Detail: "disk full"}, "Update failed: disk full"},
		{update.Status{Phase: update.BadBuild}, "The new version didn't start, so this one was kept"},
		{update.Status{Phase: update.RestartNeeded}, "Update installed. Restart AgentChime to finish"},
		{update.Status{Phase: update.Updated, Current: "0.3.1"}, "Updated to 0.3.1"},
		{update.Status{Phase: update.SourceBuild}, "Built from source: updates are off"},
	}
	if len(cases) != 12 {
		t.Fatal("need all 12 phases")
	}
	for _, c := range cases {
		if got := updateText(c.st, en); got != c.want {
			t.Errorf("%s: %q, want %q", c.st.Phase, got, c.want)
		}
	}
	vi := func(k i18n.Key, a ...string) string { return i18n.T(k, i18n.VI, a...) }
	if got := updateText(update.Status{Phase: update.Updated, Current: "0.3.1"}, vi); got != "Đã cập nhật lên 0.3.1" {
		t.Errorf("vi: %q", got)
	}
}

func TestNoUpdatesDep(t *testing.T) {
	h := newHarness(t)
	if _, ok := h.req("GET", "/api/state", nil).json()["update"]; ok {
		t.Fatal("state.update present without Updates")
	}
	for _, route := range updateRoutes {
		if r := h.post(route, map[string]any{}); r.status != 404 {
			t.Errorf("%s: %d", route, r.status)
		}
	}
}
