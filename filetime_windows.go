//go:build windows

package main

import (
	"os"
	"syscall"
	"time"
)

// getFileCreationTime 获取文件创建时间 (Windows)
func getFileCreationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}

	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return info.ModTime()
	}

	// Windows 有 CreationTime 字段
	return time.Unix(0, stat.CreationTime.Nanoseconds())
}
