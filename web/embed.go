// Package web 嵌入 InkHub React 生产资源。
package web

import "embed"

// Assets 包含已跟踪的 Vite 生产构建，发布二进制不依赖 Node.js。
//
//go:embed dist/*
var Assets embed.FS
