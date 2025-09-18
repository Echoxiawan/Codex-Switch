package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codex-backup-tool/internal/util"
)

// BuildBackupFilename 根据时间戳与内容哈希生成文件名。
func BuildBackupFilename(ts time.Time, contentHash string) string {
	short := ShortHash(contentHash)
	return fmt.Sprintf("%s_%s.json", ts.Format("20060102-150405"), short)
}

// EnsureUniqueFilename 确保文件名在目录下唯一。
func EnsureUniqueFilename(backupsDir, base string) (string, error) {
	if err := util.EnsureDir(backupsDir); err != nil {
		return "", err
	}
	candidate := base
	path := filepath.Join(backupsDir, candidate)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return candidate, nil
	}
	prefix := strings.TrimSuffix(base, ".json")
	counter := 1
	for {
		candidate = fmt.Sprintf("%s-%d.json", prefix, counter)
		path = filepath.Join(backupsDir, candidate)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return candidate, nil
		}
		counter++
	}
}

// WriteBackupFile 将备份内容写入指定目录，返回文件相对路径。
func WriteBackupFile(backupsDir, filename string, data []byte) (string, error) {
	if err := util.EnsureDir(backupsDir); err != nil {
		return "", err
	}
	path := filepath.Join(backupsDir, filename)
	if err := util.AtomicWriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return filename, nil
}
