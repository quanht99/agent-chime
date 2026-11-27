// Package web serves the settings page and its JSON API on 127.0.0.1.
// The server can edit Claude and Codex config files, so every request goes
// through the checks in security.go before it reaches a handler.
package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quanht99/agent-chime/go/internal/hooks"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/sounds"
	"github.com/quanht99/agent-chime/go/internal/update"
)

//go:embed static/index.html static/app.js static/style.css
var staticFS embed.FS

// Autostart is the start-at-login control; tests pass a fake.
type Autostart interface {
	Supported() bool
	Enabled() bool
	Enable() error
	Disable() error
}

// Updates is the self-updater; nil means no Updates section and no update routes.
type Updates interface {
	Status() update.Status
	StartCheck()
	StartInstall()
	OpenReleasePage() error
	Restart() error
}

// Deps is everything the server needs from the rest of the app.
type Deps struct {
	P     paths.Paths
	Lib   sounds.Library
	Hooks hooks.Format
	GOOS  string
	Now   func() time.Time
	// Preview plays path at volume and returns a func that stops it.
	Preview       func(path string, volume float64) (stop func(), err error)
	OpenURL       func(string) error
	Quit          func()
	TrayAvailable func() bool
	ProgramExists func() bool
	Autostart     Autostart
	Updates       Updates
}

type Server struct {
	d     Deps
	token string

	mu       sync.Mutex // serialises every mutating call (config read-modify-write, previews, uploads)
	stopPrev func()

	noPlayer     atomic.Bool
	needsRetrust atomic.Bool

	srv *http.Server
}

func New(d Deps, token string) *Server {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	return &Server{d: d, token: token}
}

// NewToken returns 32 random bytes, hex encoded.
func NewToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("web: no randomness: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Start listens on a random local port and serves in the background.
func (s *Server) Start() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	s.srv = &http.Server{Handler: s.Handler(port), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return port, nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	s.stopPreviewLocked()
	s.mu.Unlock()
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

func (s *Server) stopPreviewLocked() {
	if s.stopPrev != nil {
		s.stopPrev()
		s.stopPrev = nil
	}
}

var staticTypes = map[string]string{
	"app.js":    "text/javascript; charset=utf-8",
	"style.css": "text/css; charset=utf-8",
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	ct, ok := staticTypes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	b, err := staticFS.ReadFile("static/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(b)
}

const csp = "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'"

func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if !tokenEqual(r.URL.Query().Get("t"), s.token) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	b, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	page := strings.Replace(string(b), "{{TOKEN}}", html.EscapeString(s.token), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", csp)
	_, _ = fmt.Fprint(w, page)
}
