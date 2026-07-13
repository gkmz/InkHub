//go:build linux

package dialog

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Pick 使用 zenity 打开 Linux 目录选择器。
func (NativePicker) Pick(ctx context.Context, title string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	output, err := exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title", title).Output()
	if err != nil {
		return "", fmt.Errorf("选择目录: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
