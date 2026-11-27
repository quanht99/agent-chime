package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteSync(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.json")
	if err := WriteSync(p, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSync(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Errorf("content %q", b)
	}
}

func TestRenameRetriesOnlyRetryableErrors(t *testing.T) {
	denied := errors.New("access denied")
	calls := 0
	osRename = func(string, string) error {
		calls++
		if calls < 3 {
			return denied
		}
		return nil
	}
	t.Cleanup(func() { osRename = os.Rename })
	isDenied := func(err error) bool { return errors.Is(err, denied) }

	if err := renameRetry("a", "b", isDenied, 5, time.Millisecond); err != nil || calls != 3 {
		t.Errorf("err=%v calls=%d, want nil after 3", err, calls)
	}

	calls = 0
	osRename = func(string, string) error { calls++; return denied }
	if err := renameRetry("a", "b", isDenied, 5, time.Millisecond); !errors.Is(err, denied) || calls != 5 {
		t.Errorf("err=%v calls=%d, want denied after 5", err, calls)
	}

	calls = 0
	other := errors.New("no such file")
	osRename = func(string, string) error { calls++; return other }
	if err := renameRetry("a", "b", isDenied, 5, time.Millisecond); !errors.Is(err, other) || calls != 1 {
		t.Errorf("err=%v calls=%d, want one try for a non-retryable error", err, calls)
	}
}

func TestRenameReplaces(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	_ = os.WriteFile(a, []byte("new"), 0o600)
	_ = os.WriteFile(b, []byte("old"), 0o600)
	if err := Rename(a, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(b); string(got) != "new" {
		t.Errorf("got %q", got)
	}
}

func TestTryLockIsExclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "app.lock")
	open := func() *os.File {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	a, b := open(), open()
	if err := TryLock(a); err != nil {
		t.Fatal(err)
	}
	if err := TryLock(b); !errors.Is(err, ErrLocked) {
		t.Fatalf("second TryLock = %v, want ErrLocked", err)
	}
	Unlock(a)
	if err := TryLock(b); err != nil {
		t.Errorf("after unlock: %v", err)
	}
}
