//go:build linux

package scanner

import (
	"os"
	"time"
)

func getCreationTime(info os.FileInfo) time.Time {
	// Linux 不支持创建时间，使用修改时间
	return info.ModTime()
}
