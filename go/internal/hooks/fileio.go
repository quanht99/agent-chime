package hooks

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/quanht99/agent-chime/go/internal/fsx"
	"github.com/quanht99/agent-chime/go/internal/jsonv"
)

const maxBackups = 10

// UnreadableError means the file holds invalid JSON or an unexpected shape; nothing was written.
type UnreadableError struct{ Path string }

func (e *UnreadableError) Error() string { return "cannot read or edit " + e.Path }

// ReadJSONFile returns nil, nil when the file is missing. A leading BOM is accepted.
func ReadJSONFile(path string) (*jsonv.Value, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &UnreadableError{Path: path}
	}
	v, err := jsonv.Parse(data)
	if err != nil {
		return nil, &UnreadableError{Path: path}
	}
	return &v, nil
}

// resolve follows symlinks, including a dangling one, so the link itself is never replaced.
func resolve(path string) string {
	for i := 0; i < 16; i++ {
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		dest, err := os.Readlink(path)
		if err != nil {
			break
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(path), dest)
		}
		path = dest
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// WriteJSONFile backs up the existing file, then writes v (UTF-8, no BOM) through a tmp file and a rename.
// It writes nothing when the content is unchanged. A failed backup aborts the write.
func WriteJSONFile(v jsonv.Value, path, backupsDir string, now time.Time) error {
	target := resolve(path)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out := []byte(v.Serialize() + "\n")
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(target); err == nil {
		old, rerr := os.ReadFile(target)
		if rerr == nil && bytes.Equal(old, out) {
			return nil
		}
		perm = fi.Mode().Perm()
		if err := backUp(target, old, rerr, backupsDir, now); err != nil {
			return fmt.Errorf("backup of %s failed: %w", filepath.Base(target), err)
		}
	}

	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+filepath.Base(target)+".agentchime-"+hex.EncodeToString(rnd[:])+".tmp")
	if err := fsx.WriteSync(tmp, out, perm); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := fsx.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func backUp(file string, content []byte, readErr error, backupsDir string, now time.Time) error {
	if readErr != nil {
		return readErr
	}
	if err := os.MkdirAll(backupsDir, 0o700); err != nil {
		return err
	}
	name := filepath.Base(file)
	base := fmt.Sprintf("%s.%s-%03d", name, now.Format("20060102-150405"), now.Nanosecond()/1e6)
	dest := filepath.Join(backupsDir, base)
	for n := 2; ; n++ {
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, werr := f.Write(content)
			cerr := f.Close()
			if werr != nil {
				return werr
			}
			if cerr != nil {
				return cerr
			}
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		dest = filepath.Join(backupsDir, fmt.Sprintf("%s-%d", base, n))
	}
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return nil
	}
	var mine []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), name+".") {
			mine = append(mine, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(mine)))
	if len(mine) > maxBackups {
		for _, old := range mine[maxBackups:] {
			os.Remove(filepath.Join(backupsDir, old))
		}
	}
	return nil
}
