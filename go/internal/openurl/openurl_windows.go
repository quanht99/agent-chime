//go:build windows

package openurl

import "os/exec"

func open(target string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
