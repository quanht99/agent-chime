package settingswin

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestResolveURLNoInstanceFile(t *testing.T) {
	p := testenv.Home(t)
	_, err := ResolveURL(p, time.Second)
	if err == nil || !strings.Contains(err.Error(), "no instance file") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveURLNotRunning(t *testing.T) {
	p := testenv.Home(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if err := instance.Write(p, instance.Info{Port: port, Token: "tok"}); err != nil {
		t.Fatal(err)
	}
	_, err = ResolveURL(p, time.Second)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveURLRunning(t *testing.T) {
	p := testenv.Home(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ping" || r.Header.Get("X-AgentChime-Token") != "tok" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		w.Write([]byte(`{"app":"agentchime"}`))
	}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	want := instance.Info{Port: port, Token: "tok", PID: 7}
	if err := instance.Write(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveURL(p, time.Second)
	if err != nil || got != want {
		t.Fatalf("got %+v err %v", got, err)
	}
	if u := instance.URL(got); u != "http://127.0.0.1:"+strconv.Itoa(port)+"/?t=tok" {
		t.Fatalf("url = %s", u)
	}
}

func TestUniqueIDChangesWithInstance(t *testing.T) {
	a := instance.Info{Port: 4100, Token: "aa"}
	id := UniqueID("/home/example/data", a)
	if id != UniqueID("/home/example/data", a) {
		t.Fatal("not stable")
	}
	const prefix = "io.github.quanht99.agentchime.settings.h"
	if !strings.HasPrefix(id, prefix) {
		t.Fatalf("id = %s", id)
	}
	suffix := strings.TrimPrefix(id, prefix)
	if len(suffix) != 16 || strings.Trim(suffix, "0123456789abcdef") != "" {
		t.Fatalf("suffix = %q", suffix)
	}
	for name, other := range map[string]string{
		"port":  UniqueID("/home/example/data", instance.Info{Port: 4101, Token: "aa"}),
		"token": UniqueID("/home/example/data", instance.Info{Port: 4100, Token: "bb"}),
		"dir":   UniqueID("/home/example/other", a),
	} {
		if other == id {
			t.Errorf("%s change did not change the id", name)
		}
	}
}

func TestTitleFollowsLanguage(t *testing.T) {
	p := testenv.Home(t)
	if got := Title(p); got != "AgentChime Settings" {
		t.Fatalf("title = %q", got)
	}
	c := config.Load(p)
	c.Language = i18n.VI
	if err := config.Save(p, c); err != nil {
		t.Fatal(err)
	}
	if got := Title(p); got != "Cài đặt AgentChime" {
		t.Fatalf("title = %q", got)
	}
}

func watchAsync(f func() string) <-chan string {
	ch := make(chan string, 1)
	go func() { ch <- f() }()
	return ch
}

func expect(t *testing.T, ch <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch did not return within 1s")
	}
}

func TestWatchQuitsAfterTwoFailedPings(t *testing.T) {
	p := testenv.Home(t)
	start := instance.Info{Port: 4100, Token: "aa"}
	instance.Write(p, start)
	ch := watchAsync(func() string {
		return Watch(p, start, 10*time.Millisecond, func(instance.Info) bool { return false })
	})
	expect(t, ch, "AgentChime stopped answering")
}

func TestWatchOneFailureIsNotEnough(t *testing.T) {
	p := testenv.Home(t)
	start := instance.Info{Port: 4100, Token: "aa"}
	instance.Write(p, start)
	var n atomic.Int64
	var down atomic.Bool
	ch := watchAsync(func() string {
		return Watch(p, start, 10*time.Millisecond, func(instance.Info) bool {
			if down.Load() {
				return false
			}
			return n.Add(1)%2 == 0
		})
	})
	select {
	case got := <-ch:
		t.Fatalf("returned early: %q", got)
	case <-time.After(200 * time.Millisecond):
	}
	down.Store(true)
	expect(t, ch, "AgentChime stopped answering")
}

func TestWatchQuitsWhenTokenChanges(t *testing.T) {
	p := testenv.Home(t)
	start := instance.Info{Port: 4100, Token: "aa"}
	instance.Write(p, start)
	ch := watchAsync(func() string {
		return Watch(p, start, 10*time.Millisecond, func(instance.Info) bool { return true })
	})
	time.Sleep(50 * time.Millisecond)
	instance.Write(p, instance.Info{Port: 4100, Token: "new"})
	expect(t, ch, "AgentChime restarted")
}

func TestWatchMissingFileCountsAsFailure(t *testing.T) {
	p := testenv.Home(t)
	start := instance.Info{Port: 4100, Token: "aa"}
	instance.Write(p, start)
	os.Remove(p.Instance)
	ch := watchAsync(func() string {
		return Watch(p, start, 10*time.Millisecond, func(instance.Info) bool { return true })
	})
	expect(t, ch, "AgentChime stopped answering")
}
