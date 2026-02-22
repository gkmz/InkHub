//go:build darwin

package main

import (
	"os"
	"syscall"
	"time"
)

// getFileCreationTime 获取文件创建时间 (macOS)
func getFileCreationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime()
	}

	// macOS 有 Birthtimespec 字段
	return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
}
