//go:build windows || (linux && cgo)

// Command agentchime-settings shows the AgentChime settings page in a window.
// It takes no arguments: it reads the running instance from the data folder.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/quanht99/agent-chime/go/internal/applog"
	"github.com/quanht99/agent-chime/go/internal/instance"
	"github.com/quanht99/agent-chime/go/internal/paths"
	"github.com/quanht99/agent-chime/go/internal/settingswin"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	p := paths.Current()
	info, err := settingswin.ResolveURL(p, time.Second)
	if err != nil {
		applog.Write(p, "settings window: "+err.Error())
		os.Exit(4)
	}

	var win *application.WebviewWindow
	app := application.New(application.Options{
		Name:         "AgentChime",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ErrorHandler: func(err error) { applog.Write(p, "settings window: "+err.Error()) },
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: settingswin.UniqueID(p.DataDir, info),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if win != nil {
					win.Show()
					win.UnMinimise()
					win.Focus()
				}
			},
		},
		Windows: application.WindowsOptions{WebviewUserDataPath: filepath.Join(p.DataDir, "webview")},
		Linux:   application.LinuxOptions{ProgramName: "agentchime", ApplicationID: "io.github.quanht99.agentchime"},
	})

	win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           settingswin.Title(p),
		Width:           640,
		Height:          760,
		MinWidth:        520,
		MinHeight:       480,
		URL:             instance.URL(info),
		InitialPosition: application.WindowCentered,
		EnableFileDrop:  false,
		DevToolsEnabled: false,
	})

	var once sync.Once
	onReady(win, func() {
		once.Do(func() {
			fmt.Println("ready")
			_ = settingswin.DetachStdout()
		})
	})

	go func() {
		why := settingswin.Watch(p, info, 2*time.Second, func(i instance.Info) bool { return instance.Ping(i, time.Second) })
		applog.Write(p, "settings window closed: "+why)
		app.Quit()
	}()

	if err := app.Run(); err != nil {
		applog.Write(p, "settings window: "+err.Error())
		os.Exit(1)
	}
}
