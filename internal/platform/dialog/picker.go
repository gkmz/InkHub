// Package dialog 封装操作系统原生目录选择器。
package dialog

import "context"

// DirectoryPicker 打开系统目录选择器并返回绝对路径。
type DirectoryPicker interface {
	Pick(ctx context.Context, title string) (string, error)
}

// NativePicker 使用当前操作系统的目录选择器。
type NativePicker struct{}
