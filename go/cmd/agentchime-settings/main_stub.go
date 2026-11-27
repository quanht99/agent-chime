//go:build !windows && !(linux && cgo)

package main

import "os"

// This platform has no settings window; the main app opens the browser.
func main() { os.Exit(2) }
