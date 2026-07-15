package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeConfigUsesDocumentedPrecedenceAndTracksOrigin(t *testing.T) {
	t.Parallel()

	got := MergeConfig(
		Layer{Name: "默认值", Values: Config{Host: "127.0.0.1", Port: 8080}},
		Layer{Name: "SQLite", Values: Config{Port: 8081}, Set: Fields("port")},
		Layer{Name: "工作区", Values: Config{Port: 8082}, Set: Fields("port")},
		Layer{Name: "环境变量", Values: Config{Port: 8083}, Set: Fields("port")},
		Layer{Name: "CLI", Values: Config{Port: 8084}, Set: Fields("port")},
	)
	if got.Config.Port != 8084 {
		t.Fatalf("Port = %d, want 8084", got.Config.Port)
	}
	if got.Origins["port"] != "CLI" {
		t.Fatalf("port origin = %q, want CLI", got.Origins["port"])
	}
	if got.Config.Host != "127.0.0.1" {
		t.Fatalf("Host = %q", got.Config.Host)
	}
}

func TestLoadDotEnvDoesNotOverrideProcessEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("INKHUB_LOG_LEVEL=debug\nINKHUB_LOG_MAX_SIZE=25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INKHUB_LOG_LEVEL", "error")
	t.Setenv("INKHUB_LOG_MAX_SIZE", "")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
	if os.Getenv("INKHUB_LOG_LEVEL") != "error" {
		t.Fatalf("INKHUB_LOG_LEVEL = %q, want process value", os.Getenv("INKHUB_LOG_LEVEL"))
	}
	// 已显式存在的空环境变量也有更高优先级，后续配置解析会将其视为默认值。
	if os.Getenv("INKHUB_LOG_MAX_SIZE") != "" {
		t.Fatalf("INKHUB_LOG_MAX_SIZE = %q, want empty process value", os.Getenv("INKHUB_LOG_MAX_SIZE"))
	}
}

func TestLoadDotEnvAllowsMissingFile(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
}
