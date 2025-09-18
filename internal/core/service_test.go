package core_test

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codex-backup-tool/internal/core"
)

func TestServiceBackupLifecycle(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	target := svc.Config().TargetPath
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte(`{"token":"alpha"}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	res1, err := svc.CreateBackup(nil)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	if !res1.Created {
		t.Fatalf("expected first backup to be created")
	}

	// 再次扫描应判定未变化
	res2, err := svc.Scan(false, nil)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if res2.Created {
		t.Fatalf("expected no backup on unchanged file")
	}

	// 修改 mtime 保持内容不变，应触发内容去重
	now := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(target, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	res3, err := svc.Scan(false, nil)
	if err != nil {
		t.Fatalf("third scan: %v", err)
	}
	if res3.Created {
		t.Fatalf("expected deduplicated scan to skip creation")
	}

	// 修改内容，应该新增备份
	updated := []byte(`{"token":"beta"}`)
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(target, updated, 0o600); err != nil {
		t.Fatalf("rewrite target: %v", err)
	}
	res4, err := svc.Scan(false, nil)
	if err != nil {
		t.Fatalf("fourth scan: %v", err)
	}
	if !res4.Created {
		t.Fatalf("expected new backup after content change")
	}

	items, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(items))
	}

	first := items[len(items)-1] // 最早的备份
	latest := items[0]
	if _, err := svc.UpdateRemark(first.ID, "my-manual"); err != nil {
		t.Fatalf("update remark: %v", err)
	}
	if _, err := svc.UpdateRemark(items[0].ID, "my-manual"); err == nil {
		t.Fatalf("expected remark conflict")
	}

	// 覆盖写入再还原
	if err := os.WriteFile(target, []byte(`{"token":"gamma"}`), 0o600); err != nil {
		t.Fatalf("overwrite target: %v", err)
	}
	if err := svc.RestoreBackup(first.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored: %v", err)
	}
	if string(after) != string(original) {
		t.Fatalf("restore content mismatch: got %s", after)
	}
	// 删除最新备份后 latest_fingerprint 应回退
	svcCfg := svc.Config()
	if err := svc.DeleteBackup(latest.ID); err != nil {
		t.Fatalf("delete latest: %v", err)
	}
	idxBytes, err := os.ReadFile(svcCfg.IndexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx struct {
		LatestFingerprint string `json:"latest_fingerprint"`
		Items             []struct {
			ID string `json:"id"`
		}
	}
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(idx.Items) != 1 {
		t.Fatalf("expected 1 item after delete, got %d", len(idx.Items))
	}
	if idx.LatestFingerprint != first.FileFingerprint {
		t.Fatalf("latest fingerprint mismatch after delete: want %s got %s", first.FileFingerprint, idx.LatestFingerprint)
	}
}

func newTestService(t *testing.T) (*core.Service, func()) {
	t.Helper()
	base := t.TempDir()
	targetDir := filepath.Join(base, "codex")
	dataDir := filepath.Join(base, "data")
	cfg := core.Config{
		TargetPath:   filepath.Join(targetDir, "auth.json"),
		DataDir:      dataDir,
		BackupsDir:   filepath.Join(dataDir, "backups"),
		IndexPath:    filepath.Join(dataDir, "index.json"),
		ScanInterval: time.Second,
		Port:         "0",
	}
	svc, err := core.NewService(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, func() { svc.Stop() }
}
