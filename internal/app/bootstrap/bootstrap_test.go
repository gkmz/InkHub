package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	t.Setenv("INKHUB_DATA_DIR", "")

	config, err := parseServerConfig([]string{"inkhub"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "127.0.0.1" || config.Port != 8080 || filepath.Base(config.DataDir) != "InkHub" {
		t.Fatalf("默认启动配置不安全: %+v", config)
	}
}

func TestParseServerConfigUsesEnvironmentDataDir(t *testing.T) {
	environmentDir := filepath.Join(t.TempDir(), "environment")
	t.Setenv("INKHUB_DATA_DIR", environmentDir)

	config, err := parseServerConfig([]string{"inkhub"})
	if err != nil {
		t.Fatal(err)
	}
	if config.DataDir != filepath.Clean(environmentDir) {
		t.Fatalf("DataDir = %q, want environment value %q", config.DataDir, environmentDir)
	}
	if config.Origins["data_dir"] != "environment" {
		t.Fatalf("data_dir origin = %q, want environment", config.Origins["data_dir"])
	}
	expectedLogFile := filepath.Join(environmentDir, "logs", "inkhub.log")
	if config.Logging.File != expectedLogFile {
		t.Fatalf("日志文件 = %q, want %q", config.Logging.File, expectedLogFile)
	}
}

func TestParseServerConfigExplicitCLIDataDirOverridesEnvironment(t *testing.T) {
	environmentDir := filepath.Join(t.TempDir(), "environment")
	cliDir := filepath.Join(t.TempDir(), "cli")
	t.Setenv("INKHUB_DATA_DIR", environmentDir)

	config, err := parseServerConfig([]string{"inkhub", "--data-dir", cliDir})
	if err != nil {
		t.Fatal(err)
	}
	if config.DataDir != filepath.Clean(cliDir) {
		t.Fatalf("DataDir = %q, want CLI value %q", config.DataDir, cliDir)
	}
	if config.Origins["data_dir"] != "cli" {
		t.Fatalf("data_dir origin = %q, want cli", config.Origins["data_dir"])
	}
}

func TestParseServerConfigRejectsRelativeDataDir(t *testing.T) {
	t.Run("环境变量", func(t *testing.T) {
		t.Setenv("INKHUB_DATA_DIR", "./runtime")
		_, err := parseServerConfig([]string{"inkhub"})
		if err == nil || !strings.Contains(err.Error(), "必须使用绝对路径") {
			t.Fatalf("相对环境路径应被拒绝: %v", err)
		}
	})

	t.Run("命令行", func(t *testing.T) {
		t.Setenv("INKHUB_DATA_DIR", "")
		_, err := parseServerConfig([]string{"inkhub", "--data-dir", "runtime"})
		if err == nil || !strings.Contains(err.Error(), "必须使用绝对路径") {
			t.Fatalf("相对 CLI 路径应被拒绝: %v", err)
		}
	})
}

func TestStartupConfigLogFieldsIncludeDataDirectoryOrigin(t *testing.T) {
	fields := startupConfigLogFields(Config{DataDir: "/tmp/inkhub"}, "environment")
	if len(fields) != 2 {
		t.Fatalf("启动配置日志字段数量 = %d, want 2", len(fields))
	}
	if fields[0].Key != "data_dir" || fields[0].String != "/tmp/inkhub" {
		t.Fatalf("数据目录日志字段错误: %+v", fields[0])
	}
	if fields[1].Key != "data_dir_source" || fields[1].String != "environment" {
		t.Fatalf("数据目录来源日志字段错误: %+v", fields[1])
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
