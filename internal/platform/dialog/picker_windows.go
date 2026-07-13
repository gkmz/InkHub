//go:build windows

package dialog

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Pick 打开 Windows 目录选择器。
func (NativePicker) Pick(ctx context.Context, title string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	script := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description=$args[0]; if($d.ShowDialog() -eq 'OK'){Write-Output $d.SelectedPath}else{exit 1}`
	output, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script, title).Output()
	if err != nil {
		return "", fmt.Errorf("选择目录: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
