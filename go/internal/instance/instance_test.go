package instance

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func portOf(t *testing.T, s *httptest.Server) int {
	t.Helper()
	_, ps, _ := net.SplitHostPort(s.Listener.Addr().String())
	n, _ := strconv.Atoi(ps)
	return n
}

func TestRoundTripAndGarbage(t *testing.T) {
	p := testenv.Home(t)
	if _, ok := Read(p); ok {
		t.Fatal("read of missing file")
	}
	in := Info{Port: 1234, Token: "tok", PID: 42}
	if err := Write(p, in); err != nil {
		t.Fatal(err)
	}
	if got, ok := Read(p); !ok || got != in {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
	os.WriteFile(p.Instance, []byte("not json"), 0o600)
	if _, ok := Read(p); ok {
		t.Fatal("garbage must read as missing")
	}
	Remove(p)
	Remove(p)
}

func TestInstanceFilePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no posix modes")
	}
	p := testenv.Home(t)
	if err := Write(p, Info{Port: 1}); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p.Instance)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

func TestURL(t *testing.T) {
	if got := URL(Info{Port: 80, Token: "abc"}); got != "http://127.0.0.1:80/?t=abc" {
		t.Fatal(got)
	}
}

func TestPingRejectsStaleInstance(t *testing.T) {
	// Closed port.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if Ping(Info{Port: port, Token: "t"}, time.Second) {
		t.Fatal("closed port answered")
	}

	// Something else answers 200.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"app":"other"}`))
	}))
	defer other.Close()
	if Ping(Info{Port: portOf(t, other), Token: "t"}, time.Second) {
		t.Fatal("foreign app accepted")
	}

	// Redirects are not followed.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ping" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.Write([]byte(`{"app":"agentchime"}`))
	}))
	defer redir.Close()
	if Ping(Info{Port: portOf(t, redir), Token: "t"}, time.Second) {
		t.Fatal("redirect followed")
	}

	// Non-200 with the right body.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"app":"agentchime"}`))
	}))
	defer bad.Close()
	if Ping(Info{Port: portOf(t, bad), Token: "t"}, time.Second) {
		t.Fatal("500 accepted")
	}

	// Correct server, token is sent.
	var gotTok string
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTok = r.Header.Get("X-AgentChime-Token")
		w.Write([]byte(`{"app":"agentchime"}`))
	}))
	defer good.Close()
	if !Ping(Info{Port: portOf(t, good), Token: "secret"}, time.Second) {
		t.Fatal("good server rejected")
	}
	if gotTok != "secret" {
		t.Fatalf("token = %q", gotTok)
	}
}
