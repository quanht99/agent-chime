// Package settingswin is the cgo-free core of the settings helper: it finds
// the running instance, names the window, and decides when the window quits.
package settingswin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/i18n"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

// ResolveURL reads the instance file and pings it.
func ResolveURL(p paths.Paths, timeout time.Duration) (instance.Info, error) {
	info, ok := instance.Read(p)
	if !ok {
		return instance.Info{}, errors.New("no instance file")
	}
	if !instance.Ping(info, timeout) {
		return instance.Info{}, errors.New("AgentChime is not running")
	}
	return info, nil
}

// UniqueID names one window per data folder and instance. The "h" keeps the
// last D-Bus name element from starting with a digit.
func UniqueID(dataDir string, i instance.Info) string {
	sum := sha256.Sum256([]byte(dataDir + "\x00" + strconv.Itoa(i.Port) + "\x00" + i.Token))
	return "io.github.quanht99.agentchime.settings.h" + hex.EncodeToString(sum[:])[:16]
}

// Title is the window title in the saved language.
func Title(p paths.Paths) string {
	return i18n.T("settingsTitle", config.Load(p).Language)
}

// Watch blocks until the instance restarted or stopped answering.
func Watch(p paths.Paths, start instance.Info, every time.Duration, ping func(instance.Info) bool) string {
	failures := 0
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		cur, ok := instance.Read(p)
		if ok && (cur.Port != start.Port || cur.Token != start.Token) {
			return "AgentChime restarted"
		}
		if ok && ping(start) {
			failures = 0
			continue
		}
		failures++
		if failures >= 2 {
			return "AgentChime stopped answering"
		}
	}
	return ""
}
