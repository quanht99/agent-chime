package web

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const (
	maxJSONBody   = 64 << 10
	maxUploadBody = 10 << 20
)

func tokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
}

func methodNotAllowed(w http.ResponseWriter) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// Handler returns the whole server for a listener on 127.0.0.1:port.
// No Access-Control-* header is ever set, so other origins cannot read or call it.
func (s *Server) Handler(port int) http.Handler {
	wantHost := fmt.Sprintf("127.0.0.1:%d", port)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		// DNS-rebinding guard: only the exact loopback host and port are served.
		if r.Host != wantHost {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		path := r.URL.Path
		switch {
		case path == "/":
			s.servePage(w, r)
		case strings.HasPrefix(path, "/static/"):
			s.serveStatic(w, r)
		case strings.HasPrefix(path, "/api/"):
			if !tokenEqual(r.Header.Get("X-AgentChime-Token"), s.token) {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
				return
			}
			s.serveAPI(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func isJSONContentType(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// readBody reads at most limit bytes; on failure it has already answered.
func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "too_large"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_body"})
		}
		return nil, false
	}
	return b, true
}
