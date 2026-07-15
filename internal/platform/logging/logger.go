package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// New 构造文件 JSON 日志，并按配置附加适合本地阅读的控制台日志。
func New(config Config, console io.Writer) (*zap.Logger, func() error, error) {
	if config.File == "" {
		return nil, nil, fmt.Errorf("日志文件路径不能为空")
	}
	if config.MaxSize <= 0 {
		return nil, nil, fmt.Errorf("日志文件分割大小必须大于 0")
	}
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		return nil, nil, fmt.Errorf("解析日志级别: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(config.File), 0o700); err != nil {
		return nil, nil, fmt.Errorf("创建日志目录: %w", err)
	}
	// lumberjack 会延迟打开文件；启动时主动探测，避免进程运行后才发现日志不可写。
	probe, err := os.OpenFile(config.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("打开日志文件: %w", err)
	}
	if err := probe.Close(); err != nil {
		return nil, nil, fmt.Errorf("关闭日志文件探测句柄: %w", err)
	}

	rotator := &lumberjack.Logger{
		Filename:   config.File,
		MaxSize:    config.MaxSize,
		MaxBackups: config.Backups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
		LocalTime:  true,
	}
	fileSink := zapcore.AddSync(rotator)
	cores := []zapcore.Core{zapcore.NewCore(jsonEncoder(), fileSink, level)}
	if config.Console && console != nil {
		cores = append(cores, zapcore.NewCore(consoleEncoder(), zapcore.AddSync(console), level))
	}
	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// lumberjack 的 Close 会关闭当前文件句柄，确保测试和进程退出时内容完整落盘。
	closeLogger := func() error {
		_ = logger.Sync()
		if err := rotator.Close(); err != nil {
			return fmt.Errorf("关闭日志文件: %w", err)
		}
		return nil
	}
	return logger, closeLogger, nil
}

func jsonEncoder() zapcore.Encoder {
	config := commonEncoderConfig()
	config.EncodeTime = zapcore.EpochNanosTimeEncoder
	return zapcore.NewJSONEncoder(config)
}

func consoleEncoder() zapcore.Encoder {
	config := commonEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return zapcore.NewConsoleEncoder(config)
}

func commonEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}
}

// Duration 将耗时统一编码为毫秒，供关键路径日志复用。
func Duration(start time.Time) zap.Field {
	return zap.Int64("duration_ms", time.Since(start).Milliseconds())
}
