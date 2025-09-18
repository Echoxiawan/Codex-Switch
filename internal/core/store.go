package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"codex-backup-tool/internal/util"
)

var (
	// ErrRemarkExists 在备注重复时返回。
	ErrRemarkExists = errors.New("remark already exists")
	// ErrBackupNotFound 在指定备份不存在时返回。
	ErrBackupNotFound = errors.New("backup not found")
)

// BackupItem 对应 index.json 的 items 元素。
type BackupItem struct {
	ID              string    `json:"id"`
	Filename        string    `json:"filename"`
	ContentHash     string    `json:"content_hash"`
	FileFingerprint string    `json:"file_fingerprint"`
	Size            int64     `json:"size"`
	CreatedAt       time.Time `json:"created_at"`
	Remark          string    `json:"remark"`
	IsAuto          bool      `json:"is_auto"`
	SourcePath      string    `json:"source_path"`
	LastModified    time.Time `json:"last_modified"`
}

// IndexData 对应 index.json 文件结构。
type IndexData struct {
	TargetPath        string            `json:"target_path"`
	HashAlgo          string            `json:"hash_algo"`
	LatestFingerprint string            `json:"latest_fingerprint"`
	Items             []BackupItem      `json:"items"`
	Remarks           map[string]string `json:"remarks"`
}

// Store 管理 index.json 的读写与并发控制。
type Store struct {
	indexPath  string
	lockPath   string
	targetPath string
	mu         sync.Mutex
}

// NewStore 创建 Store 实例。
func NewStore(indexPath, targetPath string) *Store {
	return &Store{
		indexPath:  indexPath,
		lockPath:   indexPath + ".lock",
		targetPath: targetPath,
	}
}

// Snapshot 加载当前索引数据。
func (s *Store) Snapshot() (*IndexData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndexUnlocked()
	if err != nil {
		return nil, err
	}
	return idx.clone(), nil
}

// AddBackup 新增备份并更新最新指纹。
func (s *Store) AddBackup(item BackupItem, latestFingerprint string) (*IndexData, error) {
	return s.update(func(idx *IndexData) error {
		if item.Remark != "" {
			if existing, ok := idx.Remarks[item.Remark]; ok && existing != item.ID {
				return ErrRemarkExists
			}
			idx.Remarks[item.Remark] = item.ID
		}
		idx.Items = append(idx.Items, item)
		idx.LatestFingerprint = latestFingerprint
		return nil
	})
}

// UpdateLatestFingerprint 仅更新最新指纹。
func (s *Store) UpdateLatestFingerprint(fingerprint string) (*IndexData, error) {
	return s.update(func(idx *IndexData) error {
		idx.LatestFingerprint = fingerprint
		return nil
	})
}

// UpdateRemark 修改备注，保持唯一。
func (s *Store) UpdateRemark(id, newRemark string) (*BackupItem, error) {
	var updatedItem *BackupItem
	_, err := s.update(func(idx *IndexData) error {
		var item *BackupItem
		for i := range idx.Items {
			if idx.Items[i].ID == id {
				item = &idx.Items[i]
				break
			}
		}
		if item == nil {
			return ErrBackupNotFound
		}
		if item.Remark == newRemark {
			updatedItem = item.clone()
			return nil
		}
		if newRemark != "" {
			if existing, ok := idx.Remarks[newRemark]; ok && existing != id {
				return ErrRemarkExists
			}
		}
		if item.Remark != "" {
			delete(idx.Remarks, item.Remark)
		}
		item.Remark = newRemark
		if newRemark != "" {
			idx.Remarks[newRemark] = id
		}
		updatedItem = item.clone()
		return nil
	})
	return updatedItem, err
}

// DeleteBackup 删除备份文件记录。
func (s *Store) DeleteBackup(id string) (*BackupItem, error) {
	var removed BackupItem
	_, err := s.update(func(idx *IndexData) error {
		found := false
		items := make([]BackupItem, 0, len(idx.Items))
		var latest BackupItem
		for _, item := range idx.Items {
			if item.ID == id {
				removed = item
				found = true
				continue
			}
			items = append(items, item)
			if latest.CreatedAt.IsZero() || item.CreatedAt.After(latest.CreatedAt) {
				latest = item
			}
		}
		if !found {
			return ErrBackupNotFound
		}
		idx.Items = items
		if removed.Remark != "" {
			delete(idx.Remarks, removed.Remark)
		}
		if latest.ID != "" {
			idx.LatestFingerprint = latest.FileFingerprint
		} else {
			idx.LatestFingerprint = ""
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &removed, nil
}

// FindByContentHash 查找同内容备份。
func (s *Store) FindByContentHash(hash string) (*BackupItem, error) {
	idx, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	for _, item := range idx.Items {
		if item.ContentHash == hash {
			clone := item
			return &clone, nil
		}
	}
	return nil, nil
}

// FindByID 查找备份。
func (s *Store) FindByID(id string) (*BackupItem, error) {
	idx, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	for _, item := range idx.Items {
		if item.ID == id {
			clone := item
			return &clone, nil
		}
	}
	return nil, ErrBackupNotFound
}

// ListBackups 返回按创建时间倒序排列的备份列表。
func (s *Store) ListBackups() ([]BackupItem, error) {
	idx, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	items := make([]BackupItem, len(idx.Items))
	copy(items, idx.Items)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (s *Store) update(mutator func(*IndexData) error) (*IndexData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated *IndexData
	err := util.WithFileLock(s.lockPath, func() error {
		idx, err := s.loadIndexUnlocked()
		if err != nil {
			return err
		}
		if err := mutator(idx); err != nil {
			return err
		}
		idx.ensureDefaults(s.targetPath)
		if err := util.AtomicWriteJSON(s.indexPath, idx); err != nil {
			return err
		}
		updated = idx.clone()
		return nil
	})
	return updated, err
}

func (s *Store) loadIndexUnlocked() (*IndexData, error) {
	data, exists, err := util.ReadFileIfExists(s.indexPath)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var idx IndexData
	if exists {
		if err := json.Unmarshal(data, &idx); err != nil {
			return nil, fmt.Errorf("unmarshal index: %w", err)
		}
	}
	idx.ensureDefaults(s.targetPath)
	return &idx, nil
}

func (idx *IndexData) ensureDefaults(target string) {
	if idx.Remarks == nil {
		idx.Remarks = make(map[string]string)
	}
	if idx.Items == nil {
		idx.Items = make([]BackupItem, 0)
	}
	if idx.HashAlgo == "" {
		idx.HashAlgo = "sha256"
	}
	if idx.TargetPath == "" {
		idx.TargetPath = target
	}
}

func (idx *IndexData) clone() *IndexData {
	copyIdx := *idx
	if idx.Items != nil {
		copyIdx.Items = make([]BackupItem, len(idx.Items))
		copy(copyIdx.Items, idx.Items)
	}
	if idx.Remarks != nil {
		copyIdx.Remarks = make(map[string]string, len(idx.Remarks))
		for k, v := range idx.Remarks {
			copyIdx.Remarks[k] = v
		}
	}
	return &copyIdx
}

func (item *BackupItem) clone() *BackupItem {
	copyItem := *item
	return &copyItem
}
