//go:build windows

package util

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	h := windows.Handle(f.Fd())
	return windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}

func unlockFile(f *os.File) error {
	h := windows.Handle(f.Fd())
	if err := windows.UnlockFileEx(h, 0, 1, 0, &windows.Overlapped{}); err != nil {
		return fmt.Errorf("unlock file: %w", err)
	}
	return nil
}
