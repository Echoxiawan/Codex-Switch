//go:build !windows

package core

import (
	"os"
	"syscall"
)

func extractSysMetadata(info os.FileInfo) (uint64, uint64) {
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok || sys == nil {
		return 0, 0
	}
	return uint64(sys.Ino), uint64(sys.Dev)
}
