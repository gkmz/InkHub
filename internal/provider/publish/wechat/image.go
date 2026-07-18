package wechat

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	_ "golang.org/x/image/webp"
)

const (
	maxWeChatImageSize   = 10 << 20
	maxWeChatImagePixels = 40_000_000
)

// ImageInfo 是经过签名、尺寸和静态内容校验的图片信息。
type ImageInfo struct {
	Digest    string
	MediaType string
	Extension string
	Size      int64
	Width     int
	Height    int
}

// InspectImage 校验微信本地图片并返回由真实内容派生的规范信息。
func InspectImage(path string) (ImageInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ImageInfo{}, providerError("wechat.image_missing", "微信图片不存在", contracts.ErrorValidation, nil)
		}
		return ImageInfo{}, providerError("wechat.image_invalid", "微信图片无法读取", contracts.ErrorValidation, nil)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > maxWeChatImageSize {
		return ImageInfo{}, providerError("wechat.image_invalid", "微信图片大小或文件类型不符合要求", contracts.ErrorValidation, nil)
	}
	reader := bufio.NewReader(file)
	config, format, err := image.DecodeConfig(reader)
	if err != nil {
		return ImageInfo{}, providerError("wechat.image_invalid", "微信图片格式无效", contracts.ErrorValidation, nil)
	}
	mediaType, extension, ok := canonicalImageFormat(format)
	if !ok || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxWeChatImagePixels {
		return ImageInfo{}, providerError("wechat.image_invalid", "微信图片格式或尺寸不符合要求", contracts.ErrorValidation, nil)
	}
	if format == "gif" {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return ImageInfo{}, providerError("wechat.image_invalid", "微信图片无法读取", contracts.ErrorValidation, nil)
		}
		decoded, err := gif.DecodeAll(file)
		if err != nil || len(decoded.Image) != 1 {
			return ImageInfo{}, providerError("wechat.image_invalid", "微信图片不支持动画格式", contracts.ErrorValidation, nil)
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ImageInfo{}, providerError("wechat.image_invalid", "微信图片无法读取", contracts.ErrorValidation, nil)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ImageInfo{}, providerError("wechat.image_invalid", "微信图片无法读取", contracts.ErrorValidation, nil)
	}
	return ImageInfo{
		Digest: hex.EncodeToString(hash.Sum(nil)), MediaType: mediaType, Extension: extension,
		Size: stat.Size(), Width: config.Width, Height: config.Height,
	}, nil
}

func canonicalImageFormat(format string) (string, string, bool) {
	switch format {
	case "png":
		return "image/png", ".png", true
	case "jpeg":
		return "image/jpeg", ".jpg", true
	case "gif":
		return "image/gif", ".gif", true
	case "webp":
		return "image/webp", ".webp", true
	default:
		return "", "", false
	}
}
