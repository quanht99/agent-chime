package update

import "time"

// Phase is where the updater is, for the UI.
type Phase string

const (
	Idle          Phase = "idle"
	Checking      Phase = "checking"
	UpToDate      Phase = "upToDate"
	Available     Phase = "available"
	Downloading   Phase = "downloading"
	CheckFailed   Phase = "checkFailed"
	Unverified    Phase = "unverified"
	Failed        Phase = "failed"
	BadBuild      Phase = "badBuild"
	RestartNeeded Phase = "restartNeeded"
	Updated       Phase = "updated"
	SourceBuild   Phase = "sourceBuild"
)

// Status is a snapshot of the updater for the settings window and menu.
type Status struct {
	Phase       Phase
	Current     string
	Latest      string // "" until a check succeeds
	ReleasePage string
	LastSuccess *time.Time
	Detail      string // error text for Failed
}

// CanInstall reports whether a strictly newer version is known and nothing
// that blocks a new install is in progress.
func (s Status) CanInstall() bool {
	if c, ok := Compare(s.Latest, s.Current); !ok || c != 1 {
		return false
	}
	switch s.Phase {
	case Downloading, RestartNeeded, Updated:
		return false
	}
	return true
}
