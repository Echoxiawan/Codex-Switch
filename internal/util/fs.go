package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath 将 ~ 展开并返回绝对路径。
func ExpandPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("path is empty")
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home: %w", err)
		}
		if p == "~" {
			p = home
		} else if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		} else {
			return "", fmt.Errorf("unsupported path expansion: %s", p)
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}
	return abs, nil
}

// EnsureDir 确保目录存在。
func EnsureDir(dir string) error {
	if dir == "" {
		return errors.New("dir is empty")
	}
	return os.MkdirAll(dir, 0o755)
}

// AtomicWriteJSON 以原子方式写入 JSON 文件。
func AtomicWriteJSON(path string, data any) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// ReadFileIfExists 读取文件，若不存在返回 (nil, false, nil)。
func ReadFileIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// WithFileLock 对 lockPath 加锁，执行 fn 后释放。

// AtomicWriteFile 以原子方式写入原始字节。
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

func WithFileLock(lockPath string, fn func() error) error {
	if err := EnsureDir(filepath.Dir(lockPath)); err != nil {
		return fmt.Errorf("ensure lock dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return fmt.Errorf("lock file: %w", err)
	}
	defer unlockFile(f)
	return fn()
}
