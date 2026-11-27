// Package sounds lists and resolves the sounds a user can pick.
package sounds

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/quanht99/agent-chime/go/internal/audio"
	"github.com/quanht99/agent-chime/go/internal/config"
	"github.com/quanht99/agent-chime/go/internal/model"
	"github.com/quanht99/agent-chime/go/internal/paths"
)

type Item struct {
	Choice config.SoundChoice `json:"choice"`
	Name   string             `json:"name"`
}

type Group struct {
	ID    string `json:"id"` // system, builtin, file
	Items []Item `json:"items"`
}

type Library struct {
	P       paths.Paths
	GOOS    string
	Players audio.Players
}

func itemFor(src config.Source, file string) Item {
	return Item{
		Choice: config.SoundChoice{Source: src, Value: file},
		Name:   strings.TrimSuffix(file, filepath.Ext(file)),
	}
}

func sortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].Choice.Value), strings.ToLower(items[j].Choice.Value)
		if a != b {
			return a < b
		}
		return items[i].Choice.Value < items[j].Choice.Value
	})
}

// System lists playable files at the top level of the system sounds dir.
func (l Library) System() []Item {
	items := []Item{}
	entries, err := os.ReadDir(l.P.SystemSoundsDir)
	if err != nil {
		return items
	}
	allowed := audio.AllowedExtensions(l.GOOS, l.Players)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			// Follow symlinks to regular files, skip directories.
			st, err := os.Stat(filepath.Join(l.P.SystemSoundsDir, e.Name()))
			if err != nil || !st.Mode().IsRegular() {
				continue
			}
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, a := range allowed {
			if a == ext {
				items = append(items, itemFor(config.System, e.Name()))
				break
			}
		}
	}
	sortItems(items)
	return items
}

func listFS(fsys fs.FS, src config.Source) []Item {
	items := []Item{}
	if fsys == nil {
		return items
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return items
	}
	for _, e := range entries {
		if e.IsDir() || strings.ToLower(filepath.Ext(e.Name())) != ".wav" {
			continue
		}
		items = append(items, itemFor(src, e.Name()))
	}
	sortItems(items)
	return items
}

func (l Library) Builtin() []Item { return listFS(BuiltinFS, config.Bundled) }

// Groups returns system, builtin and file groups, each always present.
func (l Library) Groups(c config.Config) []Group {
	files := []Item{}
	seen := map[string]bool{}
	for _, k := range []model.AlertKind{model.Done, model.Attention} {
		s := c.Sound(k)
		if s.Source != config.File || seen[s.Value] {
			continue
		}
		seen[s.Value] = true
		files = append(files, Item{Choice: s, Name: s.DisplayName()})
	}
	return []Group{
		{ID: "system", Items: l.System()},
		{ID: "builtin", Items: l.Builtin()},
		{ID: "file", Items: files},
	}
}

// Path maps a choice to the file it plays.
func (l Library) Path(c config.SoundChoice) string {
	switch c.Source {
	case config.System:
		return filepath.Join(l.P.SystemSoundsDir, c.Value)
	case config.Bundled:
		return filepath.Join(l.P.SoundsDir, c.Value)
	}
	return c.Value
}

func listed(items []Item, v string) bool {
	for _, it := range items {
		if it.Choice.Value == v {
			return true
		}
	}
	return false
}

// ValidName rejects traversal and unlisted names for system and bundled.
func (l Library) ValidName(c config.SoundChoice) bool {
	if c.Source == config.File {
		return true
	}
	v := c.Value
	if v == "" || v == "." || v == ".." || v != filepath.Base(v) || strings.ContainsAny(v, `/\`) {
		return false
	}
	switch c.Source {
	case config.System:
		return listed(l.System(), v)
	case config.Bundled:
		return listed(l.Builtin(), v)
	}
	return false
}

// ResolvePlayable returns the path to play, or the default sound plus a
// warning when the choice is invalid or unplayable.
func (l Library) ResolvePlayable(c config.SoundChoice, k model.AlertKind) (string, *model.SoundWarning) {
	wanted := l.Path(c)
	if l.ValidName(c) && audio.IsPlayable(wanted, l.GOOS, l.Players) {
		return wanted, nil
	}
	// Like v1, the default is returned even if it is itself not playable.
	return l.Path(config.DefaultChoice(k)), &model.SoundWarning{Kind: k, Path: wanted}
}
