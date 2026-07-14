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
