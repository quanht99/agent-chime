//go:build windows

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// onReady calls f once WebView2 has finished loading the page. WindowShow is
// not used: on Windows it fires before WebView2 is set up.
func onReady(w *application.WebviewWindow, f func()) {
	w.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(*application.WindowEvent) { f() })
}
