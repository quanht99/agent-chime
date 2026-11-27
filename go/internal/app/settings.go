package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/openurl"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

// showSettings opens the settings window, or the browser when the window cannot show.
func showSettings(p paths.Paths, info instance.Info) {
	showSettingsWith(p, info, 15*time.Second, openurl.Open)
}

func showSettingsWith(p paths.Paths, info instance.Info, timeout time.Duration, open func(string) error) {
	err := startHelper(p.SettingsHelper, timeout)
	if err == nil {
		return
	}
	applog.Write(p, "settings window: "+err.Error()+"; opening the browser")
	_ = open(instance.URL(info))
}

var (
	errNoReady     = errors.New("no ready")
	errExitedEarly = errors.New("exited before ready")
)

// startHelper runs the settings helper and waits for its "ready" line. It returns nil on ready or
// on exit 0 before ready.
func startHelper(path string, timeout time.Duration) error {
	return startWaitReady(path, nil, timeout, true)
}

// startWaitReady runs path with args and waits for a stdout line "ready". An exit 0 before ready
// counts as ready when exit0IsReady, else it is errExitedEarly; another exit returns its error.
func startWaitReady(path string, args []string, timeout time.Duration, exit0IsReady bool) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Stdout = w
	detach(cmd)
	allowForeground()
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return err
	}
	w.Close()

	ready := make(chan struct{})
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) == "ready" {
				close(ready)
				return
			}
		}
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }() // also reaps the child after ready

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ready:
		r.Close()
		return nil
	case err := <-done:
		// The child is gone; its pipe ends at EOF. Let the scanner finish, so a ready line that
		// is still buffered wins over the exit.
		select {
		case <-scanned:
		case <-time.After(time.Second):
		}
		r.Close()
		select {
		case <-ready:
			return nil
		default:
		}
		if err == nil && !exit0IsReady {
			return errExitedEarly
		}
		return err
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		r.Close()
		return fmt.Errorf("%w within %s", errNoReady, timeout)
	}
}
