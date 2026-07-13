//go:build darwin

package scanner

import (
	"os"
	"syscall"
	"time"
)

func getCreationTime(info os.FileInfo) time.Time {
	stat := info.Sys().(*syscall.Stat_t)
	return time.Unix(stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec)
}
