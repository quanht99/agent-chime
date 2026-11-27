//go:build linux

package openurl

import "os/exec"

func open(target string) error {
	cmd := exec.Command("xdg-open", target)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
