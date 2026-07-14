package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunVersionDoesNotOpenWorkspace(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), []string{"inkhub", "--version"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestAcquireInstanceLockReclaimsStaleFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inkhub.lock"), []byte("99999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireInstanceLock(dir)
	if err != nil {
		t.Fatalf("陈旧锁不应阻止启动: %v", err)
	}
	unlock()
}

func TestRunStartsOnLoopbackAndStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := Run(ctx, []string{"inkhub", "--host", "127.0.0.1", "--port", "0", "--data-dir", t.TempDir()}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestParseServerConfigDefaultsToLoopback(t *testing.T) {
	config, err := parseServerConfig([]string{"inkhub"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "127.0.0.1" || config.Port != 8080 || filepath.Base(config.DataDir) != "InkHub" {
		t.Fatalf("默认启动配置不安全: %+v", config)
	}
}

func TestRunKeepsRecoveryServerAvailableForInvalidDatabase(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "inkhub.db")
	original := []byte("not-a-sqlite-database")
	if err := os.WriteFile(databasePath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := Run(ctx, []string{"inkhub", "--host", "127.0.0.1", "--port", "0", "--data-dir", dataDir}); err != nil {
		t.Fatalf("恢复 Server 启动失败: %v", err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil || string(after) != string(original) {
		t.Fatalf("恢复模式修改了损坏数据库: content=%q err=%v", after, err)
	}
}
