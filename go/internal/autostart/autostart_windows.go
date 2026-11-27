//go:build windows

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

type registryRunKey struct{}

func (registryRunKey) Get(name string) (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	return v, err == nil
}

func (registryRunKey) Set(name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (registryRunKey) Delete(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// WindowsRunKey is the real HKCU Run key; tests replace it with a fake.
var WindowsRunKey RunKey = registryRunKey{}

func enabled(p paths.Paths) bool  { return runKeyEnabled(WindowsRunKey, p.Program) }
func enable(p paths.Paths) error  { return enableRunKey(WindowsRunKey, p.Program) }
func disable(p paths.Paths) error { return disableRunKey(WindowsRunKey) }
