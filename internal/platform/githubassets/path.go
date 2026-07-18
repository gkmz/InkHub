// Package githubassets 通过公开 GitHub 仓库托管微信文章图片。
package githubassets

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	githubNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	digestPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Config 定义公开 GitHub 图片仓库位置和写入凭据。
type Config struct {
	Owner      string
	Repository string
	Branch     string
	Prefix     string
	Token      string
}

// AssetPath 生成由图片内容摘要决定的仓库相对路径。
func AssetPath(prefix, digest, extension string) (string, error) {
	if !digestPattern.MatchString(digest) || !allowedExtension(extension) {
		return "", fmt.Errorf("GitHub 图片摘要或类型无效")
	}
	parts, err := safePathParts(prefix, true)
	if err != nil {
		return "", err
	}
	parts = append(parts, digest[:2], digest+extension)
	return strings.Join(parts, "/"), nil
}

// RawURL 生成只指向 GitHub 官方匿名内容域名的公开地址。
func RawURL(config Config, assetPath string) (string, error) {
	if err := validateConfig(config, false); err != nil {
		return "", err
	}
	pathParts, err := safePathParts(assetPath, false)
	if err != nil {
		return "", err
	}
	branchParts, err := safePathParts(config.Branch, false)
	if err != nil {
		return "", err
	}
	segments := []string{config.Owner, config.Repository}
	segments = append(segments, branchParts...)
	segments = append(segments, pathParts...)
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return "https://raw.githubusercontent.com/" + strings.Join(segments, "/"), nil
}

func validateConfig(config Config, requireToken bool) error {
	if !githubNamePattern.MatchString(config.Owner) || !githubNamePattern.MatchString(config.Repository) {
		return fmt.Errorf("GitHub 仓库配置无效")
	}
	if _, err := safePathParts(config.Branch, false); err != nil {
		return fmt.Errorf("GitHub 分支配置无效")
	}
	if _, err := safePathParts(config.Prefix, true); err != nil {
		return fmt.Errorf("GitHub 图片前缀无效")
	}
	if requireToken && strings.TrimSpace(config.Token) == "" {
		return fmt.Errorf("GitHub Token 未配置")
	}
	return nil
}

func safePathParts(value string, allowEmpty bool) ([]string, error) {
	if value == "" && allowEmpty {
		return nil, nil
	}
	if value == "" || len(value) > 512 || strings.Contains(value, `\`) {
		return nil, fmt.Errorf("GitHub 路径无效")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.IndexFunc(part, func(value rune) bool { return value < 0x20 || value == 0x7f }) >= 0 {
			return nil, fmt.Errorf("GitHub 路径无效")
		}
	}
	return parts, nil
}

func allowedExtension(value string) bool {
	switch value {
	case ".png", ".jpg", ".gif", ".webp":
		return true
	default:
		return false
	}
}
