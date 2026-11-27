//go:build linux && cgo

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// onReady calls f once WebKitGTK has finished loading the page.
func onReady(w *application.WebviewWindow, f func()) {
	w.OnWindowEvent(events.Linux.WindowLoadFinished, func(*application.WindowEvent) { f() })
}
