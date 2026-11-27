// Package instance detects an already running app through instance.json.
package instance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

type Info struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// Write stores i atomically with mode 0600.
func Write(p paths.Paths, i Info) error {
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.Instance), 0o755); err != nil {
		return err
	}
	var r [6]byte
	_, _ = rand.Read(r[:])
	tmp := p.Instance + "." + hex.EncodeToString(r[:]) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	err = fsx.WriteSyncFile(f, b)
	if err == nil {
		err = fsx.Rename(tmp, p.Instance)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// Read returns false when the file is missing or unparseable.
func Read(p paths.Paths) (Info, bool) {
	b, err := os.ReadFile(p.Instance)
	if err != nil {
		return Info{}, false
	}
	var i Info
	if json.Unmarshal(b, &i) != nil {
		return Info{}, false
	}
	return i, true
}

func Remove(p paths.Paths) { _ = os.Remove(p.Instance) }

// Ping is true only when the port answers 200 with {"app":"agentchime"}.
func Ping(i Info, timeout time.Duration) bool {
	if i.Port <= 0 || i.Port > 65535 {
		return false
	}
	c := &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(i.Port)+"/api/ping", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-AgentChime-Token", i.Token)
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}
	var v struct {
		App string `json:"app"`
	}
	return json.Unmarshal(body, &v) == nil && v.App == "agentchime"
}

// URL is the settings page address, carrying the token.
func URL(i Info) string {
	return "http://127.0.0.1:" + strconv.Itoa(i.Port) + "/?t=" + i.Token
}
