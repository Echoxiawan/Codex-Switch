//go:build windows

package core

import (
	"os"
	"syscall"
)

func extractSysMetadata(info os.FileInfo) (uint64, uint64) {
	sys, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || sys == nil {
		return 0, 0
	}
	inode := uint64(sys.FileIndexHigh)<<32 | uint64(sys.FileIndexLow)
	dev := uint64(sys.VolumeSerialNumber)
	return inode, dev
}
