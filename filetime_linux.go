//go:build linux

package main

import (
	"os"
	"time"
)

// getFileCreationTime 获取文件创建时间 (Linux)
// 注意：Linux 文件系统通常不存储创建时间，这里返回修改时间
func getFileCreationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}

	// Linux 没有创建时间，使用修改时间
	return info.ModTime()
}
