// Package openurl opens a URL in the default browser.
package openurl

// Open starts the OS URL handler without waiting for it.
func Open(target string) error { return open(target) }
