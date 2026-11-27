package audio

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Players holds the resolved paths of the Linux players that were found.
type Players struct{ PwPlay, Paplay, Aplay string }

// FindPlayers looks up pw-play, paplay and aplay with lookPath.
func FindPlayers(lookPath func(string) (string, error)) Players {
	find := func(n string) string {
		p, err := lookPath(n)
		if err != nil {
			return ""
		}
		return p
	}
	return Players{PwPlay: find("pw-play"), Paplay: find("paplay"), Aplay: find("aplay")}
}

// AllowedExtensions lists the lowercase extensions (with dot) playable on goos.
func AllowedExtensions(goos string, pl Players) []string {
	switch goos {
	case "windows":
		return []string{".wav"}
	case "linux":
		if pl.PwPlay != "" || pl.Paplay != "" {
			return []string{".wav", ".oga", ".ogg", ".flac", ".mp3"}
		}
		return []string{".wav"}
	case "darwin":
		return []string{".wav", ".aiff", ".aif"}
	}
	return []string{".wav"}
}

// LinuxCommand picks the player command: pw-play, paplay, then aplay. For aplay
// the scaled WAV is returned as stdin; when it cannot be scaled the file path is
// passed instead.
func LinuxCommand(pl Players, path string, volume float64) (name string, args []string, stdin []byte, err error) {
	if math.IsNaN(volume) {
		volume = 1
	}
	volume = math.Min(1, math.Max(0, volume))
	switch {
	case pl.PwPlay != "":
		return pl.PwPlay, []string{fmt.Sprintf("--volume=%.2f", volume), path}, nil, nil
	case pl.Paplay != "":
		return pl.Paplay, []string{fmt.Sprintf("--volume=%d", int(math.Round(volume*65536))), path}, nil, nil
	case pl.Aplay != "":
		if strings.ToLower(filepath.Ext(path)) != ".wav" {
			return "", nil, nil, ErrNoPlayer
		}
		if b, rerr := os.ReadFile(path); rerr == nil {
			if scaled, ok := ScaleWAV(b, volume); ok {
				return pl.Aplay, []string{"-q", "-"}, scaled, nil
			}
		}
		return pl.Aplay, []string{"-q", path}, nil, nil
	}
	return "", nil, nil, ErrNoPlayer
}
