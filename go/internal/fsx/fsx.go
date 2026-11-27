// Package fsx holds the small file helpers shared by every package that writes files: an fsynced
// write, a rename that retries on Windows while another process has the target open, and an
// exclusive whole-process file lock (flock, or LockFileEx on Windows).
package fsx

import (
	"errors"
	"os"
	"time"
)

// ErrLocked is returned by TryLock when another process holds the lock.
var ErrLocked = errors.New("locked by another process")

// WriteSync creates or truncates name with perm, writes data, fsyncs and closes it, so a crash
// right after the following rename cannot leave an empty file behind.
func WriteSync(name string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	return finish(f, data)
}

// WriteSyncFile writes data to an already open file, fsyncs and closes it.
func WriteSyncFile(f *os.File, data []byte) error { return finish(f, data) }

func finish(f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

const (
	renameTries = 5
	renameWait  = 10 * time.Millisecond
)

var osRename = os.Rename

// Rename is os.Rename. On Windows a rename over a file that another process has open without
// FILE_SHARE_DELETE (Go's os.Open does that) fails with access denied for as long as the reader
// holds it, so it is retried a few times.
func Rename(oldpath, newpath string) error {
	return renameRetry(oldpath, newpath, retryable, renameTries, renameWait)
}

func renameRetry(oldpath, newpath string, retry func(error) bool, tries int, wait time.Duration) error {
	var err error
	for i := 0; i < tries; i++ {
		if i > 0 {
			time.Sleep(wait)
		}
		if err = osRename(oldpath, newpath); err == nil || !retry(err) {
			return err
		}
	}
	return err
}
