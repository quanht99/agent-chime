package app

import (
	"errors"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/testenv"
)

func TestLockAppIsExclusive(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := lockApp(p)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := lockApp(p); !errors.Is(err, fsx.ErrLocked) {
		if b != nil {
			b.Close()
		}
		t.Fatalf("second lockApp = %v, want ErrLocked", err)
	}
	a.Close() // closing releases the lock
	b, err := lockApp(p)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	b.Close()
}

func TestWaitForInstance(t *testing.T) {
	p := testenv.Home(t)
	if _, ok := waitForInstance(p, 50*time.Millisecond, 10*time.Millisecond); ok {
		t.Fatal("no instance.json yet")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"app":"agentchime"}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	want := instance.Info{Port: ln.Addr().(*net.TCPAddr).Port, Token: "t", PID: 1}

	// The holder writes instance.json a little later, as when it is still deploying.
	go func() {
		time.Sleep(80 * time.Millisecond)
		_ = instance.Write(p, want)
	}()
	got, ok := waitForInstance(p, 3*time.Second, 10*time.Millisecond)
	if !ok || got != want {
		t.Fatalf("got %+v %v", got, ok)
	}
}

func TestWaitForLockTakesItWhenReleased(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lockApp(p)
	if err != nil {
		t.Fatal(err)
	}
	type res struct {
		f   *os.File
		err error
	}
	done := make(chan res, 1)
	go func() {
		f, err := waitForLock(p, 10*time.Millisecond, 2*time.Second)
		done <- res{f, err}
	}()
	select {
	case <-done:
		t.Fatal("waitForLock returned while the lock was held")
	case <-time.After(100 * time.Millisecond):
	}
	held.Close()
	select {
	case r := <-done:
		if r.err != nil || r.f == nil {
			t.Fatalf("waitForLock = %v, %v", r.f, r.err)
		}
		r.f.Close()
	case <-time.After(500 * time.Millisecond):
		t.Fatal("waitForLock did not take the released lock")
	}
}

func TestWaitForLockGivesUp(t *testing.T) {
	p := testenv.Home(t)
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lockApp(p)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	f, err := waitForLock(p, 10*time.Millisecond, 100*time.Millisecond)
	if f != nil {
		f.Close()
	}
	if !errors.Is(err, fsx.ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
}
