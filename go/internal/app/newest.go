package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/quanht99/agent-chime/go/internal/deploy"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/update"
	"github.com/quanht99/agent-chime/go/internal/version"
)

type startAction int

const (
	startAsIs        startAction = iota // this copy is the installed one: run it
	startDeploy                         // deploy this copy over the installed one
	startDeployNewer                    // as startDeploy, and a running older instance should restart
	startRunData                        // the installed copy is newer: run that instead
)

// chooseStart decides what a copy that was started by hand does: the newest copy wins.
func chooseStart(selfVer, dataVer string, selfIsData, dataExists bool) startAction {
	if selfIsData {
		return startAsIs
	}
	if !dataExists {
		return startDeploy
	}
	c, ok := update.Compare(selfVer, dataVer)
	switch {
	case !ok:
		return startDeploy
	case c > 0:
		return startDeployNewer
	case c < 0:
		return startRunData
	}
	return startDeploy
}

// sameFile reports whether a and b are the same existing file.
func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// selfIsInstalled reports whether the running executable is the installed copy.
func selfIsInstalled(p paths.Paths) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	return sameFile(self, p.Program)
}

func debNewer(p paths.Paths) (deploy.Files, bool) { return debNewerOS(p, runtime.GOOS) }

// debNewerOS is true only on Linux, when the .deb's program is strictly newer than the installed one.
func debNewerOS(p paths.Paths, goos string) (deploy.Files, bool) {
	if goos != "linux" || p.DebDir == "" {
		return deploy.Files{}, false
	}
	prog := filepath.Join(p.DebDir, filepath.Base(p.Program))
	if fi, err := os.Stat(prog); err != nil || !fi.Mode().IsRegular() {
		return deploy.Files{}, false
	}
	dv, err := version.Of(prog)
	if err != nil {
		return deploy.Files{}, false
	}
	iv, err := version.Of(p.Program)
	if err != nil {
		return deploy.Files{}, false
	}
	if c, ok := update.Compare(dv, iv); !ok || c != 1 {
		return deploy.Files{}, false
	}
	f := deploy.Files{Program: prog}
	helper := filepath.Join(p.DebDir, filepath.Base(p.SettingsHelper))
	if fi, err := os.Stat(helper); err == nil && fi.Mode().IsRegular() {
		f.Helper = helper
	}
	return f, true
}

// postRestart asks the running instance to restart into the deployed program. Anything but 200
// (an older instance answers 404) is an error.
func postRestart(info instance.Info) error {
	c := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	host := "127.0.0.1:" + strconv.Itoa(info.Port)
	req, err := http.NewRequest(http.MethodPost, "http://"+host+"/api/update/restart", nil)
	if err != nil {
		return err
	}
	req.Host = host
	req.Header.Set("X-AgentChime-Token", info.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("restart: status %d", resp.StatusCode)
	}
	return nil
}
