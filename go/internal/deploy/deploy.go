// Package deploy copies the running binary and the embedded sounds to their
// fixed install locations, which the agent hooks point at.
package deploy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/quanht99/agent-chime/go/internal/paths"
)

// Files names the two executables to deploy. Helper may be empty.
type Files struct{ Program, Helper string }

// copyFile is a seam for tests that record the copy order.
var copyFile = copySelf

// Sync copies self (and the settings helper next to it, when there is one)
// to their install locations and syncs sounds. changed lists the destination
// paths written or removed.
func Sync(p paths.Paths, self string, builtin fs.FS) (changed []string, err error) {
	return syncWithOS(p, self, builtin, runtime.GOOS)
}

func syncWithOS(p paths.Paths, self string, builtin fs.FS, goos string) ([]string, error) {
	return syncFilesWithOS(p, Files{Program: self, Helper: helperSource(self, p.SettingsHelper)}, builtin, goos)
}

// SyncFiles copies f.Helper to p.SettingsHelper, then f.Program to p.Program,
// then syncs sounds. A non-empty f.Helper that is not a regular file is an
// error before anything is copied.
func SyncFiles(p paths.Paths, f Files, builtin fs.FS) ([]string, error) {
	return syncFilesWithOS(p, f, builtin, runtime.GOOS)
}

func syncFilesWithOS(p paths.Paths, f Files, builtin fs.FS, goos string) ([]string, error) {
	if f.Helper != "" {
		if fi, err := os.Stat(f.Helper); err != nil {
			return nil, err
		} else if !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", f.Helper)
		}
	}
	var changed []string
	if err := os.MkdirAll(filepath.Dir(p.Program), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p.SettingsHelper), 0o755); err != nil {
		return nil, err
	}
	if goos == "windows" {
		// Leftovers from earlier upgrades; they fail to delete while running.
		for _, target := range []string{p.Program, p.SettingsHelper} {
			olds, _ := filepath.Glob(target + ".old-*")
			for _, o := range olds {
				_ = os.Remove(o)
			}
		}
	}
	// The helper is copied BEFORE the main program. If an update is cut short
	// after one copy, the data folder still holds a working pair: the program
	// is the file that starts everything, so it must be the last one replaced.
	if f.Helper != "" {
		helperCopied, err := copyFile(p.SettingsHelper, f.Helper, goos)
		if err != nil {
			return changed, err
		}
		if helperCopied {
			changed = append(changed, p.SettingsHelper)
		}
	}
	copied, err := copyFile(p.Program, f.Program, goos)
	if err != nil {
		return changed, err
	}
	if copied {
		changed = append(changed, p.Program)
	}
	c, err := syncDir(p.SoundsDir, builtin)
	changed = append(changed, c...)
	return changed, err
}

// helperSource returns the settings helper that sits next to the real file
// behind self (symlinks resolved), or "" when there is none.
func helperSource(self, helper string) string {
	resolved, err := filepath.EvalSymlinks(self)
	if err != nil {
		resolved = self
	}
	src := filepath.Join(filepath.Dir(resolved), filepath.Base(helper))
	if fi, err := os.Stat(src); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	return src
}

func copySelf(program, self, goos string) (bool, error) {
	si, err := os.Stat(self)
	if err != nil {
		return false, err
	}
	if pi, err := os.Stat(program); err == nil {
		if os.SameFile(si, pi) {
			return false, nil
		}
		if si.Size() == pi.Size() && sameBytes(self, program) {
			return false, nil
		}
	}
	src, err := os.Open(self)
	if err != nil {
		return false, err
	}
	defer src.Close()

	tmp, err := tempName(program)
	if err != nil {
		return false, err
	}
	// Never open program for writing: a running hook makes that fail with
	// ETXTBSY on Linux, and a half-written copy would break every hook.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return false, err
	}
	fail := func(e error) (bool, error) {
		f.Close()
		os.Remove(tmp)
		return false, e
	}
	if _, err := io.Copy(f, src); err != nil {
		return fail(err)
	}
	if err := f.Chmod(0o755); err != nil && goos != "windows" {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return false, err
	}

	oldName := ""
	if goos == "windows" {
		if _, err := os.Lstat(program); err == nil {
			for n := 1; ; n++ {
				cand := program + ".old-" + strconv.Itoa(n)
				if _, err := os.Lstat(cand); os.IsNotExist(err) {
					oldName = cand
					break
				}
			}
			if err := os.Rename(program, oldName); err != nil {
				os.Remove(tmp)
				return false, err
			}
		}
	}
	if err := os.Rename(tmp, program); err != nil {
		os.Remove(tmp)
		if oldName != "" {
			_ = os.Rename(oldName, program)
		}
		return false, err
	}
	if oldName != "" {
		_ = os.Remove(oldName) // fails while still running; cleaned next start
	}
	return true, nil
}

func sameBytes(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

func tempName(target string) (string, error) {
	var r [6]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".agentchime-"+hex.EncodeToString(r[:])+".tmp"), nil
}

// syncDir mirrors the top level regular files of src into dir.
// A nil src means empty. Subdirectories are never touched.
func syncDir(dir string, src fs.FS) ([]string, error) {
	var changed []string
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	if src != nil {
		entries, err := fs.ReadDir(src, ".")
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}
			want[e.Name()] = true
			data, err := fs.ReadFile(src, e.Name())
			if err != nil {
				return changed, err
			}
			dst := filepath.Join(dir, e.Name())
			if cur, err := os.ReadFile(dst); err == nil && bytes.Equal(cur, data) {
				continue
			}
			if err := writeAtomic(dst, data, 0o644); err != nil {
				return changed, err
			}
			changed = append(changed, dst)
		}
	}
	existing, err := os.ReadDir(dir)
	if err != nil {
		return changed, err
	}
	for _, e := range existing {
		if want[e.Name()] || !e.Type().IsRegular() {
			continue
		}
		dst := filepath.Join(dir, e.Name())
		if err := os.Remove(dst); err != nil {
			return changed, err
		}
		changed = append(changed, dst)
	}
	return changed, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := tempName(path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
