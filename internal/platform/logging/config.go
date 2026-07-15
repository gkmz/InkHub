// Package logging 提供 InkHub 的结构化日志配置与构造能力。
package logging

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultLevel   = "info"
	defaultMaxSize = 100
)

// Config 描述日志级别、输出目标和文件轮转策略。
type Config struct {
	Level    string
	File     string
	MaxSize  int
	Console  bool
	MaxAge   int
	Backups  int
	Compress bool
}

// LookupEnv 是读取环境变量的最小接口，便于独立验证配置合并逻辑。
type LookupEnv func(key string) (value string, found bool)

// ParseConfig 从环境变量解析日志配置，未设置项使用安全默认值。
func ParseConfig(dataDir string, lookup LookupEnv) (Config, error) {
	config := Config{
		Level:    defaultLevel,
		File:     filepath.Join(dataDir, "logs", "inkhub.log"),
		MaxSize:  defaultMaxSize,
		Console:  true,
		MaxAge:   30,
		Backups:  5,
		Compress: true,
	}
	if value, ok := lookup("INKHUB_LOG_LEVEL"); ok && strings.TrimSpace(value) != "" {
		config.Level = strings.ToLower(strings.TrimSpace(value))
	}
	if !validLevel(config.Level) {
		return Config{}, fmt.Errorf("INKHUB_LOG_LEVEL 仅支持 debug、info、warn 或 error")
	}
	if value, ok := lookup("INKHUB_LOG_FILE"); ok && strings.TrimSpace(value) != "" {
		config.File = strings.TrimSpace(value)
	}
	if value, ok := lookup("INKHUB_LOG_MAX_SIZE"); ok && strings.TrimSpace(value) != "" {
		maxSize, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || maxSize <= 0 {
			return Config{}, fmt.Errorf("INKHUB_LOG_MAX_SIZE 必须是大于 0 的整数")
		}
		config.MaxSize = maxSize
	}
	if value, ok := lookup("INKHUB_LOG_CONSOLE"); ok && strings.TrimSpace(value) != "" {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "true" && normalized != "false" {
			return Config{}, fmt.Errorf("INKHUB_LOG_CONSOLE 必须是 true 或 false")
		}
		config.Console = normalized == "true"
	}
	return config, nil
}

func validLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
