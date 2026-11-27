// Package applog is the app's tiny rotating file log.
package applog

import (
	"os"
	"time"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

const MaxBytes = 1_000_000

func Write(p paths.Paths, msg string) { WriteAt(p, msg, time.Now()) }

// WriteAt appends "<UTC RFC3339> msg\n", rotating first when the log is full.
// Failures are swallowed: logging must never break the caller.
func WriteAt(p paths.Paths, msg string, now time.Time) {
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		return
	}
	if fi, err := os.Stat(p.Log); err == nil && fi.Size() >= MaxBytes {
		_ = os.Remove(p.LogOld)
		_ = os.Rename(p.Log, p.LogOld)
	}
	f, err := os.OpenFile(p.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(now.UTC().Format("2006-01-02T15:04:05Z") + " " + msg + "\n")
}
