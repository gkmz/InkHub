//go:build windows

package scanner

import (
	"os"
	"syscall"
	"time"
)

func getCreationTime(info os.FileInfo) time.Time {
	stat := info.Sys().(*syscall.Win32FileAttributeData)
	return time.Unix(0, stat.CreationTime.Nanoseconds())
}
