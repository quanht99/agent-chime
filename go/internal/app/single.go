package app

import (
	"os"
	"time"

	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

// lockApp opens app.lock in the data dir and takes an exclusive lock on it without waiting. The file
// must stay open for the life of the process. fsx.ErrLocked means another app process holds it.
func lockApp(p paths.Paths) (*os.File, error) {
	f, err := os.OpenFile(p.AppLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := fsx.TryLock(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// waitForInstance polls instance.json until the instance it names answers, for up to timeout: the
// lock holder may still be deploying or starting its server.
func waitForInstance(p paths.Paths, timeout, every time.Duration) (instance.Info, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if info, ok := instance.Read(p); ok && instance.Ping(info, time.Second) {
			return info, true
		}
		if !time.Now().Before(deadline) {
			return instance.Info{}, false
		}
		time.Sleep(every)
	}
}
