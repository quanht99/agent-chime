//go:build !windows

package fsx

import (
	"os"
	"syscall"
)

func retryable(error) bool { return false }

// Lock takes an exclusive lock on f, waiting for it.
func Lock(f *os.File) error { return flock(f, syscall.LOCK_EX) }

// TryLock takes an exclusive lock on f, or returns ErrLocked at once if another holder has it.
func TryLock(f *os.File) error {
	err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK {
		return ErrLocked
	}
	return err
}

// Unlock releases a lock taken with Lock or TryLock.
func Unlock(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if err != syscall.EINTR {
			return err
		}
	}
}
