package wechat

import (
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

const onePixelWebP = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"

func TestInspectImageUsesSignatureAndReturnsCanonicalMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "伪装成-jpeg.jpg")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := InspectImage(path)
	if err != nil {
		t.Fatalf("检查有效图片: %v", err)
	}
	if info.MediaType != "image/png" || info.Extension != ".png" || info.Width != 3 || info.Height != 2 || len(info.Digest) != 64 || info.Size <= 0 {
		t.Fatalf("图片规范信息不正确: %+v", info)
	}
}

func TestInspectImageRejectsAnimatedGIFWithoutLeakingPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "private-animation.gif")
	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{
		Image: []*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 2, 2), palette),
			image.NewPaletted(image.Rect(0, 0, 2, 2), palette),
		},
		Delay: []int{1, 1},
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(file, animation); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = InspectImage(path)
	assertImageError(t, err, "wechat.image_invalid", path)
}

func TestInspectImageRejectsMissingAndEmptyFiles(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.png")
	_, err := InspectImage(missing)
	assertImageError(t, err, "wechat.image_missing", missing)

	empty := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = InspectImage(empty)
	assertImageError(t, err, "wechat.image_invalid", empty)
}

func TestInspectImageAcceptsStaticWebP(t *testing.T) {
	t.Parallel()

	content, err := base64.StdEncoding.DecodeString(onePixelWebP)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cover.bin")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := InspectImage(path)
	if err != nil {
		t.Fatalf("检查 WebP: %v", err)
	}
	if info.MediaType != "image/webp" || info.Extension != ".webp" || info.Width != 1 || info.Height != 1 {
		t.Fatalf("WebP 规范信息不正确: %+v", info)
	}
}

func assertImageError(t *testing.T, err error, code, secretPath string) {
	t.Helper()
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != code {
		t.Fatalf("错误=%v，期望 code=%s", err, code)
	}
	if strings.Contains(err.Error(), secretPath) {
		t.Fatalf("错误泄露绝对路径: %v", err)
	}
}
