package template

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallRejectsZipSlip(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "bad.zip")
	writeZip(t, archive, map[string][]byte{"../escape.txt": []byte("escape")})
	if _, err := Install(context.Background(), archive, filepath.Join(t.TempDir(), "templates"), &memoryActivator{}); err == nil {
		t.Fatal("Zip Slip 应被拒绝")
	}
}

func TestInstallKeepsVersionsImmutableAndActivatesValidatedTemplate(t *testing.T) {
	t.Parallel()

	archive := makeValidArchive(t, "body-v1")
	root := filepath.Join(t.TempDir(), "templates")
	activator := &memoryActivator{}
	installed, err := Install(context.Background(), archive, root, activator)
	if err != nil {
		t.Fatalf("安装模板: %v", err)
	}
	if activator.activeID != "test-template" || activator.activeVersion != "1.0.0" {
		t.Fatalf("模板未激活: %+v", activator)
	}
	if _, err := os.Stat(filepath.Join(installed.Path, "template.yaml")); err != nil {
		t.Fatalf("版本目录未安装: %v", err)
	}

	changed := makeValidArchive(t, "body-v2")
	if _, err := Install(context.Background(), changed, root, activator); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("相同版本不同内容应冲突: %T %v", err, err)
	}
}

func TestInstallActivationFailureKeepsPreviousActiveVersion(t *testing.T) {
	t.Parallel()

	activator := &memoryActivator{activeID: "old", activeVersion: "0.9.0", err: errors.New("db failed")}
	_, err := Install(context.Background(), makeValidArchive(t, "body"), filepath.Join(t.TempDir(), "templates"), activator)
	if err == nil {
		t.Fatal("激活失败应返回错误")
	}
	if activator.activeID != "old" || activator.activeVersion != "0.9.0" {
		t.Fatalf("激活失败改变了旧版本: %+v", activator)
	}
}

type memoryActivator struct {
	activeID      string
	activeVersion string
	err           error
}

func (a *memoryActivator) Activate(_ context.Context, id, version, _ string, _ string) error {
	if a.err != nil {
		return a.err
	}
	a.activeID, a.activeVersion = id, version
	return nil
}

func makeValidArchive(t *testing.T, body string) string {
	t.Helper()
	directory := t.TempDir()
	imagePath := filepath.Join(directory, "preview.png")
	file, _ := os.Create(imagePath)
	preview := image.NewRGBA(image.Rect(0, 0, 1200, 1600))
	preview.Set(0, 0, color.White)
	_ = png.Encode(file, preview)
	_ = file.Close()
	pngContent, _ := os.ReadFile(imagePath)
	css := []byte(`.inkhub-root p { color: #111111; }`)
	markdown := []byte("# 标题\n\n" + body)
	manifest := []byte(`specVersion: "1.0"
id: test-template
name: Test Template
description: 测试模板
author: {name: InkHub, url: https://example.com}
license: Apache-2.0
version: 1.0.0
inkhubVersion: ">=1.0.0 <2.0.0"
entry: styles.css
preview: {markdown: preview.md, image: preview.png}
elements: [paragraph]
variables: {}
files:
  - {path: styles.css, sha256: ` + hash(css) + `}
  - {path: preview.md, sha256: ` + hash(markdown) + `}
  - {path: preview.png, sha256: ` + hash(pngContent) + `}
`)
	archive := filepath.Join(t.TempDir(), "template.zip")
	writeZip(t, archive, map[string][]byte{
		"test-template/template.yaml": manifest,
		"test-template/styles.css":    css,
		"test-template/preview.md":    markdown,
		"test-template/preview.png":   pngContent,
	})
	return archive
}

func writeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	output, _ := os.Create(path)
	writer := zip.NewWriter(output)
	for name, content := range files {
		entry, _ := writer.Create(name)
		_, _ = entry.Write(content)
	}
	_ = writer.Close()
	_ = output.Close()
}

func hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
