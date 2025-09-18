package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codex-backup-tool/internal/util"
)

type fileConfig struct {
	CodexDir        string `json:"codex_dir"`
	CodexFile       string `json:"codex_file"`
	DataDir         string `json:"data_dir"`
	HTTPPort        string `json:"http_port"`
	ScanInterval    int    `json:"scan_interval"`
	AutoOpenBrowser *bool  `json:"auto_open_browser"`
}

func defaultFileConfig() fileConfig {
	return fileConfig{
		CodexDir:     "~/.codex",
		CodexFile:    "auth.json",
		DataDir:      "./data",
		HTTPPort:     "8080",
		ScanInterval: 60,
	}
}

// LoadConfig 读取本地配置文件，返回配置、是否使用默认值以及可能的错误。
func LoadConfig(path string) (Config, bool, error) {
	raw := defaultFileConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg, err := buildConfig(raw)
			return cfg, true, err
		}
		return Config{}, false, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, false, fmt.Errorf("解析配置文件失败: %w", err)
	}
	cfg, err := buildConfig(raw)
	return cfg, false, err
}

func buildConfig(raw fileConfig) (Config, error) {
	codexDir, err := util.ExpandPath(raw.CodexDir)
	if err != nil {
		return Config{}, fmt.Errorf("解析 codex_dir: %w", err)
	}
	dataDir, err := util.ExpandPath(raw.DataDir)
	if err != nil {
		return Config{}, fmt.Errorf("解析 data_dir: %w", err)
	}
	scanInterval := raw.ScanInterval
	if scanInterval <= 0 {
		scanInterval = 60
	}
	autoOpen := true
	if raw.AutoOpenBrowser != nil {
		autoOpen = *raw.AutoOpenBrowser
	}
	cfg := Config{
		TargetPath:      filepath.Join(codexDir, raw.CodexFile),
		DataDir:         dataDir,
		BackupsDir:      filepath.Join(dataDir, "backups"),
		IndexPath:       filepath.Join(dataDir, "index.json"),
		ScanInterval:    time.Duration(scanInterval) * time.Second,
		Port:            raw.HTTPPort,
		AutoOpenBrowser: autoOpen,
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	return cfg, nil
}
