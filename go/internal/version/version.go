// Package version holds the build-time version.
package version

import (
	"debug/buildinfo"
	"regexp"
)

// Set via -ldflags "-X ...".
var Version = "dev"

var ldflagsVersion = regexp.MustCompile(`-X[ =]'?github\.com/quanht99/agent-chime/go/internal/version\.Version=([^ ']+)`)

// Of reads the version of the Go binary at path from its build settings.
// A Go binary built without the version flag is "dev". A file that is not a
// Go binary, or is missing, is an error.
func Of(path string) (string, error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, s := range info.Settings {
		if s.Key != "-ldflags" {
			continue
		}
		if m := ldflagsVersion.FindStringSubmatch(s.Value); m != nil {
			return m[1], nil
		}
	}
	return "dev", nil
}
