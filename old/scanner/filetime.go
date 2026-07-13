package scanner

import (
	"os"
	"time"
)

// getFileCreationTime 获取文件创建时间（跨平台）
func getFileCreationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}
	return getCreationTime(info)
}
