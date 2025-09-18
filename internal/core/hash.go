package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

// FileStat 捕获文件指纹相关的元数据。
type FileStat struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Inode   uint64    `json:"inode"`
	Dev     uint64    `json:"dev"`
}

// FingerprintResult 包含快速指纹与文件元数据。
type FingerprintResult struct {
	Stat        *FileStat
	Fingerprint string
}

// ComputeFingerprint 基于文件元信息生成快速指纹。
func ComputeFingerprint(path string) (*FingerprintResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	stat := &FileStat{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	inode, dev := extractSysMetadata(info)
	stat.Inode = inode
	stat.Dev = dev
	seed := fmt.Sprintf("%d|%d|%d|%d", stat.Size, stat.ModTime.UnixNano(), stat.Inode, stat.Dev)
	sum := sha256.Sum256([]byte(seed))
	fingerprint := hex.EncodeToString(sum[:8])
	return &FingerprintResult{Stat: stat, Fingerprint: fingerprint}, nil
}

// ComputeContentHash 计算文件全量内容 SHA-256，同时返回文件字节。
func ComputeContentHash(path string) (string, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	sum := sha256.New()
	data, err := io.ReadAll(io.TeeReader(f, sum))
	if err != nil {
		return "", nil, fmt.Errorf("read file: %w", err)
	}
	hash := hex.EncodeToString(sum.Sum(nil))
	return hash, data, nil
}

// ShortHash 返回 content hash 截断字符串。
func ShortHash(contentHash string) string {
	if len(contentHash) <= 12 {
		return contentHash
	}
	return contentHash[:12]
}

// PlatformInfo 提供调试时的运行时信息。
func PlatformInfo() string {
	return fmt.Sprintf("go=%s os=%s arch=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
