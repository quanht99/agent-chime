// Package tray shows the system tray icon and menu (Windows and Linux).
package tray

import "time"

// Labels are the translated menu captions.
type Labels struct{ Mute, Mute30, Mute60, MuteForever, Unmute, Settings, Quit string }

// Model is what the menu shows right now.
type Model struct {
	Status, LastAlert string
	UpdateLabel       string // "" hides the update item
	Muted             bool
	Labels            Labels
}

// Actions are the menu callbacks. Mute with nil means until unmuted.
type Actions struct {
	Mute                       func(d *time.Duration)
	Unmute, OpenSettings, Quit func()
	Update                     func()
}
