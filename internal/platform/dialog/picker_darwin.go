//go:build darwin

package dialog

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Pick 打开 macOS 目录选择器。
func (NativePicker) Pick(ctx context.Context, title string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	script := `on run argv
set selectedFolder to choose folder with prompt (item 1 of argv)
return POSIX path of selectedFolder
end run`
	output, err := exec.CommandContext(ctx, "osascript", "-e", script, title).Output()
	if err != nil {
		return "", fmt.Errorf("选择目录: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
