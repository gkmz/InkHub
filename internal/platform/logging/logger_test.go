package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestNewWritesFilteredJSONToFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "inkhub.log")
	logger, closeLogger, err := New(Config{
		Level: "info", File: path, MaxSize: 1, MaxAge: 30, Backups: 5, Compress: true,
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Debug("不可见")
	logger.Info("应用启动", zap.String("component", "bootstrap"))
	if err := closeLogger(); err != nil {
		t.Fatalf("closeLogger() error = %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var records []map[string]any
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("日志不是 JSON: %v", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	if len(records) != 1 || records[0]["message"] != "应用启动" || records[0]["component"] != "bootstrap" {
		t.Fatalf("records = %#v", records)
	}
}

func TestNewHonorsConsoleSetting(t *testing.T) {
	t.Parallel()

	var console bytes.Buffer
	logger, closeLogger, err := New(Config{
		Level: "info", File: filepath.Join(t.TempDir(), "inkhub.log"), MaxSize: 1, Console: true,
	}, &console)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Warn("需要关注")
	if err := closeLogger(); err != nil {
		t.Fatalf("closeLogger() error = %v", err)
	}
	if !bytes.Contains(console.Bytes(), []byte("需要关注")) {
		t.Fatalf("console = %q", console.String())
	}
}

func TestNewRejectsUnwritableLogTargetImmediately(t *testing.T) {
	t.Parallel()

	_, _, err := New(Config{
		Level: "info", File: t.TempDir(), MaxSize: 1,
	}, nil)
	if err == nil {
		t.Fatal("New() error = nil, want directory target error")
	}
}
