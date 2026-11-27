package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/hooks"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

const testToken = "tok-0123456789abcdef"

type fakeAutostart struct {
	supported, enabled bool
	err                error
}

func (f *fakeAutostart) Supported() bool { return f.supported }
func (f *fakeAutostart) Enabled() bool   { return f.enabled }
func (f *fakeAutostart) Enable() error   { f.enabled = f.err == nil; return f.err }
func (f *fakeAutostart) Disable() error  { f.enabled = f.err != nil; return f.err }

type previewCall struct {
	path string
	vol  float64
}

type harness struct {
	t      *testing.T
	p      paths.Paths
	s      *Server
	ts     *httptest.Server
	port   int
	now    time.Time
	auto   *fakeAutostart
	quit   chan struct{}
	mu     sync.Mutex
	calls  []previewCall
	stops  int
	pErr   error
	opens  []string
	progOK bool
}

type opt func(*harness, *Deps)

func newHarness(t *testing.T, opts ...opt) *harness {
	t.Helper()
	p := testenv.Home(t)
	h := &harness{t: t, p: p, now: time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC),
		auto: &fakeAutostart{supported: true}, quit: make(chan struct{}, 1), progOK: true}
	// a system sound
	if err := os.MkdirAll(p.SystemSoundsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wav := testenv.WAV16([]int16{0, 100, -100, 0}, 8000)
	if err := os.WriteFile(filepath.Join(p.SystemSoundsDir, "Glass.wav"), wav, 0o644); err != nil {
		t.Fatal(err)
	}
	d := Deps{
		P:     p,
		Lib:   sounds.Library{P: p, GOOS: "linux"},
		Hooks: hooks.Format{GOOS: "linux", Program: "/home/example/agentchime"},
		GOOS:  "linux",
		Now:   func() time.Time { return h.now },
		Preview: func(path string, vol float64) (func(), error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.calls = append(h.calls, previewCall{path, vol})
			if h.pErr != nil {
				return nil, h.pErr
			}
			return func() { h.mu.Lock(); h.stops++; h.mu.Unlock() }, nil
		},
		OpenURL:       func(s string) error { h.opens = append(h.opens, s); return nil },
		Quit:          func() { h.quit <- struct{}{} },
		TrayAvailable: func() bool { return true },
		ProgramExists: func() bool { return h.progOK },
		Autostart:     h.auto,
	}
	for _, o := range opts {
		o(h, &d)
	}
	h.s = New(d, testToken)
	h.ts = httptest.NewUnstartedServer(nil)
	h.port = h.ts.Listener.Addr().(*net.TCPAddr).Port
	h.ts.Config.Handler = h.s.Handler(h.port)
	h.ts.Start()
	t.Cleanup(h.ts.Close)
	return h
}

type resp struct {
	status int
	hdr    http.Header
	body   []byte
}

func (r resp) json() map[string]any {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	return m
}

// req sends a request with the token and, for POST with a body, JSON content type.
func (h *harness) req(method, path string, body any, mod ...func(*http.Request)) resp {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	case string:
		rd = strings.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	r, err := http.NewRequest(method, h.ts.URL+path, rd)
	if err != nil {
		h.t.Fatal(err)
	}
	r.Header.Set("X-AgentChime-Token", testToken)
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mod {
		m(r)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, res.Header, b}
}

func (h *harness) post(path string, body any) resp { return h.req("POST", path, body) }

func (h *harness) cfg() config.Config { return config.Load(h.p) }

func (h *harness) readConfigFile() string {
	b, _ := os.ReadFile(h.p.Config)
	return string(b)
}

func TestHostCheck(t *testing.T) {
	h := newHarness(t)
	for _, host := range []string{"evil.example:" + itoa(h.port), "localhost:" + itoa(h.port), "127.0.0.1:" + itoa(h.port+1), "127.0.0.1"} {
		for _, path := range []string{"/?t=" + testToken, "/api/state", "/static/app.js"} {
			r := h.req("GET", path, nil, func(r *http.Request) { r.Host = host })
			if r.status != 403 {
				t.Errorf("host %q path %s: status %d, want 403", host, path, r.status)
			}
		}
	}
	if r := h.req("GET", "/api/ping", nil); r.status != 200 || r.json()["app"] != "agentchime" {
		t.Fatalf("ping: %d %s", r.status, r.body)
	}
}

func TestTokenRequired(t *testing.T) {
	h := newHarness(t)
	for _, tok := range []string{"", "wrong"} {
		r := h.req("GET", "/api/state", nil, func(r *http.Request) {
			r.Header.Del("X-AgentChime-Token")
			if tok != "" {
				r.Header.Set("X-AgentChime-Token", tok)
			}
		})
		if r.status != 401 {
			t.Errorf("token %q: %d", tok, r.status)
		}
	}
	r := h.req("GET", "/?t=wrong", nil)
	if r.status != 401 || len(r.body) != 0 {
		t.Fatalf("page with wrong token: %d %q", r.status, r.body)
	}
	if r := h.req("GET", "/", nil); r.status != 401 {
		t.Fatalf("page without token: %d", r.status)
	}
	// static files need only the Host check
	if r := h.req("GET", "/static/app.js", nil, func(r *http.Request) { r.Header.Del("X-AgentChime-Token") }); r.status != 200 {
		t.Fatalf("static: %d", r.status)
	}
}

func TestNoCORSHeaders(t *testing.T) {
	h := newHarness(t)
	endpoints := []struct{ m, p string }{
		{"GET", "/?t=" + testToken}, {"GET", "/static/app.js"}, {"GET", "/static/style.css"},
		{"GET", "/api/ping"}, {"GET", "/api/state"},
		{"POST", "/api/config"}, {"POST", "/api/preview"}, {"POST", "/api/preview/stop"},
		{"POST", "/api/hooks/claude/install"}, {"POST", "/api/mute"}, {"POST", "/api/unmute"},
		{"POST", "/api/autostart"}, {"POST", "/api/upload?kind=done"},
		{"POST", "/api/open-log"},
	}
	check := func(r resp, what string) {
		for k := range r.hdr {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("%s: has %s", what, k)
			}
		}
		if r.hdr.Get("Cache-Control") != "no-store" || r.hdr.Get("Referrer-Policy") != "no-referrer" ||
			r.hdr.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing hardening headers: %v", what, r.hdr)
		}
	}
	for _, e := range endpoints {
		check(h.req(e.m, e.p, "{}"), e.m+" "+e.p)
		check(h.req("OPTIONS", e.p, nil, func(r *http.Request) {
			r.Header.Set("Origin", "https://evil.example")
			r.Header.Set("Access-Control-Request-Method", "POST")
		}), "OPTIONS "+e.p)
		check(h.req(e.m, e.p, "{}", func(r *http.Request) { r.Header.Del("X-AgentChime-Token") }), "untokened "+e.p)
	}
	check(h.req("GET", "/api/state", nil, func(r *http.Request) { r.Host = "evil.example" }), "bad host")
}

func TestBodyLimits(t *testing.T) {
	h := newHarness(t)
	big := `{"x":"` + strings.Repeat("a", maxJSONBody) + `"}`
	if r := h.post("/api/config", big); r.status != 413 {
		t.Fatalf("json over limit: %d", r.status)
	}
	if r := h.post("/api/config", `{"alertHeadless":true}`); r.status != 200 {
		t.Fatalf("small json: %d %s", r.status, r.body)
	}
	up := h.req("POST", "/api/upload?kind=done", make([]byte, maxUploadBody+1), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/octet-stream")
		r.Header.Set("X-File-Name", "a.wav")
	})
	if up.status != 413 {
		t.Fatalf("upload over limit: %d", up.status)
	}
}

func TestContentTypeRequired(t *testing.T) {
	h := newHarness(t)
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data", ""} {
		r := h.req("POST", "/api/mute", `{"minutes":30}`, func(r *http.Request) {
			if ct == "" {
				r.Header.Del("Content-Type")
			} else {
				r.Header.Set("Content-Type", ct)
			}
		})
		if r.status != 415 {
			t.Errorf("content type %q: %d", ct, r.status)
		}
	}
	for _, p := range []string{"/api/unmute", "/api/quit", "/api/open-log", "/api/preview/stop"} {
		r := h.req("POST", p, nil, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") })
		if r.status != 415 {
			t.Errorf("%s: %d", p, r.status)
		}
	}
	if h.cfg().MutedUntil != nil {
		t.Fatal("rejected request changed config")
	}
	if r := h.req("POST", "/api/unmute", nil, func(r *http.Request) { r.Header.Set("Content-Type", "application/json; charset=utf-8") }); r.status != 200 {
		t.Fatalf("json with charset: %d", r.status)
	}
}

func TestWrongMethod(t *testing.T) {
	h := newHarness(t)
	if r := h.req("GET", "/api/config", nil); r.status != 405 {
		t.Fatalf("GET config: %d", r.status)
	}
	if r := h.req("POST", "/api/state", "{}"); r.status != 405 {
		t.Fatalf("POST state: %d", r.status)
	}
	if r := h.req("POST", "/?t="+testToken, "{}"); r.status != 405 {
		t.Fatalf("POST /: %d", r.status)
	}
	if r := h.req("GET", "/api/nope", nil); r.status != 404 {
		t.Fatalf("unknown: %d", r.status)
	}
}

func TestPageEmbedsTokenAndCSP(t *testing.T) {
	h := newHarness(t)
	r := h.req("GET", "/?t="+testToken, nil)
	if r.status != 200 {
		t.Fatalf("status %d", r.status)
	}
	if !strings.Contains(string(r.body), `<meta name="agentchime-token" content="`+testToken+`">`) {
		t.Fatalf("token meta missing:\n%s", r.body)
	}
	want := "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'"
	if got := r.hdr.Get("Content-Security-Policy"); got != want {
		t.Fatalf("CSP %q", got)
	}
	if strings.Contains(string(r.body), "{{TOKEN}}") {
		t.Fatal("placeholder left in page")
	}
	// CSP forbids inline code and styles
	lower := strings.ToLower(string(r.body))
	for _, bad := range []string{" style=", "<style", " onclick=", "javascript:"} {
		if strings.Contains(lower, bad) {
			t.Errorf("page has inline %q", bad)
		}
	}
	for _, m := range []string{"<script>"} {
		if strings.Contains(lower, m) {
			t.Errorf("page has inline script")
		}
	}
	if h.req("GET", "/static/app.js", nil).hdr.Get("Content-Security-Policy") != "" {
		t.Log("static also carries CSP (fine)")
	}
	if ct := h.req("GET", "/static/style.css", nil).hdr.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("css type %q", ct)
	}
}

func TestStartAndClose(t *testing.T) {
	testenv.Home(t)
	s := New(Deps{P: paths.Current(), Now: time.Now}, NewToken())
	port, err := s.Start()
	if err != nil || port == 0 {
		t.Fatalf("start: %d %v", port, err)
	}
	if len(NewToken()) != 64 {
		t.Fatal("token should be 32 bytes hex")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
