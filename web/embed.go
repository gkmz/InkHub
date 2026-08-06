// Package web 嵌入 InkHub React 生产资源。
package web

import "embed"

// Assets 包含已跟踪的 Vite 生产构建，发布二进制不依赖 Node.js。
//
// Vite 的公共依赖文件可能以下划线开头，all: 模式确保发布二进制完整嵌入构建产物。
//
//go:embed all:dist
var Assets embed.FS
