package logging

import (
	"path/filepath"
	"testing"
)

func TestParseConfigUsesSafeDefaults(t *testing.T) {
	t.Parallel()

	config, err := ParseConfig(t.TempDir(), func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if config.Level != "info" || config.MaxSize != 100 || !config.Console {
		t.Fatalf("ParseConfig() = %+v", config)
	}
	if filepath.Base(config.File) != "inkhub.log" || filepath.Base(filepath.Dir(config.File)) != "logs" {
		t.Fatalf("File = %q, want <data-dir>/logs/inkhub.log", config.File)
	}
}

func TestParseConfigReadsEveryEnvironmentSetting(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"INKHUB_LOG_LEVEL":    "WARN",
		"INKHUB_LOG_FILE":     filepath.Join(t.TempDir(), "custom.log"),
		"INKHUB_LOG_MAX_SIZE": "25",
		"INKHUB_LOG_CONSOLE":  "false",
	}
	config, err := ParseConfig(t.TempDir(), func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if config.Level != "warn" || config.File != values["INKHUB_LOG_FILE"] || config.MaxSize != 25 || config.Console {
		t.Fatalf("ParseConfig() = %+v", config)
	}
}

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "日志级别", key: "INKHUB_LOG_LEVEL", value: "verbose"},
		{name: "轮转大小", key: "INKHUB_LOG_MAX_SIZE", value: "0"},
		{name: "控制台开关", key: "INKHUB_LOG_CONSOLE", value: "sometimes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseConfig(t.TempDir(), func(key string) (string, bool) {
				if key == test.key {
					return test.value, true
				}
				return "", false
			})
			if err == nil {
				t.Fatal("ParseConfig() error = nil, want invalid setting error")
			}
		})
	}
}
