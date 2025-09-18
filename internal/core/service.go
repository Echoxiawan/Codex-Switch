package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"codex-backup-tool/internal/util"
)

// Config 包含服务运行所需的配置。
type Config struct {
	TargetPath      string
	DataDir         string
	BackupsDir      string
	IndexPath       string
	ScanInterval    time.Duration
	Port            string
	AutoOpenBrowser bool
}

// Service 管理备份逻辑与定时任务。
type Service struct {
	cfg    Config
	store  *Store
	logger *log.Logger

	scanMu sync.Mutex
	ticker *time.Ticker
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewService 创建服务实例。
func NewService(cfg Config, logger *log.Logger) (*Service, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	if err := util.EnsureDir(cfg.DataDir); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}
	if err := util.EnsureDir(cfg.BackupsDir); err != nil {
		return nil, fmt.Errorf("ensure backups dir: %w", err)
	}
	s := &Service{
		cfg:    cfg,
		store:  NewStore(cfg.IndexPath, cfg.TargetPath),
		logger: logger,
	}
	s.logger.Printf("Service init target=%s data_dir=%s scan_interval=%s %s", cfg.TargetPath, cfg.DataDir, cfg.ScanInterval, PlatformInfo())
	return s, nil
}

// Start 启动定时扫描。
func (s *Service) Start(ctx context.Context) {
	if s.cfg.ScanInterval <= 0 {
		s.logger.Println("Scan interval <=0, auto scan disabled")
		return
	}
	if s.ticker != nil {
		return
	}
	s.ticker = time.NewTicker(s.cfg.ScanInterval)
	s.stopCh = make(chan struct{})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				s.logger.Println("Auto scan stopped: context canceled")
				return
			case <-s.stopCh:
				s.logger.Println("Auto scan stopped: stop signal")
				return
			case <-s.ticker.C:
				if _, err := s.Scan(true, nil); err != nil {
					s.logger.Printf("Auto scan error: %v", err)
				}
			}
		}
	}()
}

// Stop 停止定时任务。
func (s *Service) Stop() {
	if s.ticker == nil {
		return
	}
	s.ticker.Stop()
	close(s.stopCh)
	s.wg.Wait()
	s.ticker = nil
}

// StatusInfo 描述当前目标文件状态。
type StatusInfo struct {
	Exists              bool   `json:"exists"`
	Size                int64  `json:"size"`
	ModTime             string `json:"mod_time"`
	Fingerprint         string `json:"fingerprint"`
	ContentHash         string `json:"content_hash"`
	ContentHashShort    string `json:"content_hash_short"`
	LatestFingerprint   string `json:"latest_fingerprint"`
	TargetPath          string `json:"target_path"`
	ScanIntervalSeconds int    `json:"scan_interval_seconds"`
	AutoOpenBrowser     bool   `json:"auto_open_browser"`
}

// Status 返回目标文件状态。
func (s *Service) Status() (*StatusInfo, error) {
	idx, err := s.store.Snapshot()
	if err != nil {
		return nil, err
	}
	status := &StatusInfo{
		LatestFingerprint:   idx.LatestFingerprint,
		TargetPath:          s.cfg.TargetPath,
		ScanIntervalSeconds: int(s.cfg.ScanInterval / time.Second),
		AutoOpenBrowser:     s.cfg.AutoOpenBrowser,
	}
	fingerprintRes, err := ComputeFingerprint(s.cfg.TargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			status.Exists = false
			return status, nil
		}
		return nil, fmt.Errorf("fingerprint: %w", err)
	}
	status.Exists = true
	status.Size = fingerprintRes.Stat.Size
	status.ModTime = fingerprintRes.Stat.ModTime.Format(time.RFC3339)
	status.Fingerprint = fingerprintRes.Fingerprint
	contentHash, _, err := ComputeContentHash(s.cfg.TargetPath)
	if err != nil {
		return nil, fmt.Errorf("content hash: %w", err)
	}
	status.ContentHash = contentHash
	status.ContentHashShort = ShortHash(contentHash)
	return status, nil
}

// ScanResult 描述一次扫描结果。
type ScanResult struct {
	Created bool        `json:"created"`
	Item    *BackupItem `json:"item,omitempty"`
	Reason  string      `json:"reason,omitempty"`
}

// Scan 执行扫描与备份逻辑。

// CreateBackup 手动创建备份。
func (s *Service) CreateBackup(remark *string) (*ScanResult, error) {
	return s.Scan(false, remark)
}

func (s *Service) Scan(isAuto bool, remark *string) (*ScanResult, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	idx, err := s.store.Snapshot()
	if err != nil {
		return nil, err
	}
	fingerprintRes, err := ComputeFingerprint(s.cfg.TargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ScanResult{Created: false, Reason: "目标文件不存在"}, nil
		}
		return nil, fmt.Errorf("stat target: %w", err)
	}
	fingerprint := fingerprintRes.Fingerprint
	if idx.LatestFingerprint == fingerprint {
		return &ScanResult{Created: false, Reason: "文件未变更"}, nil
	}
	contentHash, data, err := ComputeContentHash(s.cfg.TargetPath)
	if err != nil {
		return nil, fmt.Errorf("读取目标内容: %w", err)
	}
	if existing := findByContentHash(idx.Items, contentHash); existing != nil {
		if _, err := s.store.UpdateLatestFingerprint(fingerprint); err != nil {
			return nil, fmt.Errorf("更新最新指纹: %w", err)
		}
		s.logger.Printf("扫描跳过：指纹不同但内容重复 hash=%s", ShortHash(contentHash))
		return &ScanResult{Created: false, Reason: "内容已存在备份"}, nil
	}
	finalRemark, err := s.prepareRemark(idx, isAuto, remark)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	filename := BuildBackupFilename(now, contentHash)
	filename, err = EnsureUniqueFilename(s.cfg.BackupsDir, filename)
	if err != nil {
		return nil, fmt.Errorf("生成备份文件名: %w", err)
	}
	if _, err := WriteBackupFile(s.cfg.BackupsDir, filename, data); err != nil {
		return nil, fmt.Errorf("写入备份文件: %w", err)
	}
	item := BackupItem{
		ID:              uuid.New().String(),
		Filename:        filename,
		ContentHash:     contentHash,
		FileFingerprint: fingerprint,
		Size:            fingerprintRes.Stat.Size,
		CreatedAt:       now,
		Remark:          finalRemark,
		IsAuto:          isAuto,
		SourcePath:      s.cfg.TargetPath,
		LastModified:    fingerprintRes.Stat.ModTime,
	}
	if err := s.persistBackup(item, fingerprint, isAuto); err != nil {
		os.Remove(filepath.Join(s.cfg.BackupsDir, filename))
		return nil, err
	}
	s.logger.Printf("创建备份 succeed id=%s remark=%q fingerprint=%s hash=%s", item.ID, item.Remark, fingerprint, ShortHash(contentHash))
	return &ScanResult{Created: true, Item: &item}, nil
}

func (s *Service) persistBackup(item BackupItem, fingerprint string, isAuto bool) error {
	baseRemark := item.Remark
	counter := 1
	for {
		_, err := s.store.AddBackup(item, fingerprint)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrRemarkExists) && isAuto {
			item.Remark = fmt.Sprintf("%s-%d", baseRemark, counter)
			counter++
			s.logger.Printf("自动备备注名冲突，尝试 %s", item.Remark)
			continue
		}
		return err
	}
}

func (s *Service) prepareRemark(idx *IndexData, isAuto bool, req *string) (string, error) {
	if req != nil {
		r := strings.TrimSpace(*req)
		if r == "" {
			return "", errors.New("备注不能为空字符串")
		}
		if _, ok := idx.Remarks[r]; ok {
			return "", ErrRemarkExists
		}
		return r, nil
	}
	now := time.Now()
	base := "manual-"
	if isAuto {
		base = "auto-"
	}
	remark := fmt.Sprintf("%s%s", base, now.Format("20060102-150405"))
	if _, ok := idx.Remarks[remark]; !ok {
		return remark, nil
	}
	counter := 1
	for {
		candidate := fmt.Sprintf("%s-%d", remark, counter)
		if _, exists := idx.Remarks[candidate]; !exists {
			return candidate, nil
		}
		counter++
	}
}

func findByContentHash(items []BackupItem, hash string) *BackupItem {
	for i := range items {
		if items[i].ContentHash == hash {
			copy := items[i]
			return &copy
		}
	}
	return nil
}

// ListBackups 返回备份列表。
func (s *Service) ListBackups() ([]BackupItem, error) {
	return s.store.ListBackups()
}

// UpdateRemark 更新备注。
func (s *Service) UpdateRemark(id, remark string) (*BackupItem, error) {
	return s.store.UpdateRemark(id, strings.TrimSpace(remark))
}

// RestoreBackup 将备份还原为目标文件。
func (s *Service) RestoreBackup(id string) error {
	item, err := s.store.FindByID(id)
	if err != nil {
		return err
	}
	path := filepath.Join(s.cfg.BackupsDir, item.Filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取备份文件: %w", err)
	}
	if err := util.EnsureDir(filepath.Dir(s.cfg.TargetPath)); err != nil {
		return fmt.Errorf("确保目标目录: %w", err)
	}
	if err := util.AtomicWriteFile(s.cfg.TargetPath, data, 0o600); err != nil {
		return fmt.Errorf("写入目标文件: %w", err)
	}
	if res, err := ComputeFingerprint(s.cfg.TargetPath); err == nil {
		if _, err := s.store.UpdateLatestFingerprint(res.Fingerprint); err != nil {
			s.logger.Printf("更新指纹失败: %v", err)
		}
	}
	s.logger.Printf("还原完成 id=%s -> %s", id, s.cfg.TargetPath)
	return nil
}

// DeleteBackup 删除备份。
func (s *Service) DeleteBackup(id string) error {
	item, err := s.store.DeleteBackup(id)
	if err != nil {
		return err
	}
	path := filepath.Join(s.cfg.BackupsDir, item.Filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		s.logger.Printf("删除备份文件失败: %v", err)
	}
	s.logger.Printf("删除备份 id=%s remark=%q", id, item.Remark)
	return nil
}

// CodexLogin 执行 codex login 命令。
func (s *Service) CodexLogin(ctx context.Context) (string, string, int, error) {
	return RunCodexLogin(ctx)
}

// Config 返回当前配置。
func (s *Service) Config() Config {
	return s.cfg
}
