package template

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
)

// DefaultIndexURL 是内置的官方模板索引地址。
const DefaultIndexURL = "https://raw.githubusercontent.com/gkmz/InkHub/main/templates/index.json"

// IndexFetcher 通过受控 HTTP 客户端读取模板索引。
type IndexFetcher interface {
	Fetch(ctx context.Context, url string) ([]byte, error)
}

// Index 是经过验证的模板仓库索引。
type Index struct {
	SpecVersion string       `json:"specVersion"`
	GeneratedAt time.Time    `json:"generatedAt"`
	Templates   []IndexEntry `json:"templates"`
}

// IndexEntry 描述一个可下载的不可变模板包。
type IndexEntry struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	DownloadURL   string `json:"downloadURL"`
	PackageSHA256 string `json:"packageSHA256"`
}

// LoadIndex 优先使用更新的远程索引，失败或回退时读取已验证缓存。
func LoadIndex(ctx context.Context, fetcher IndexFetcher, cachePath, indexURL string) (Index, error) {
	cached, cacheErr := readIndex(cachePath)
	if fetcher != nil {
		content, err := fetcher.Fetch(ctx, indexURL)
		if err == nil {
			remote, validateErr := decodeIndex(content)
			if validateErr == nil && (cacheErr != nil || !remote.GeneratedAt.Before(cached.GeneratedAt)) {
				if err := filesystem.AtomicWrite(cachePath, content, nil); err != nil {
					return Index{}, fmt.Errorf("保存模板索引缓存: %w", err)
				}
				return remote, nil
			}
		}
	}
	if cacheErr == nil {
		return cached, nil
	}
	return Index{}, fmt.Errorf("模板索引和缓存均不可用: %w", cacheErr)
}

func readIndex(path string) (Index, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Index{}, err
	}
	return decodeIndex(content)
}

func decodeIndex(content []byte) (Index, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var index Index
	if err := decoder.Decode(&index); err != nil {
		return Index{}, fmt.Errorf("解析模板索引: %w", err)
	}
	if index.SpecVersion != "1.0" || index.GeneratedAt.IsZero() {
		return Index{}, fmt.Errorf("模板索引版本或时间无效")
	}
	digestPattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
	seen := map[string]bool{}
	for _, entry := range index.Templates {
		parsed, err := url.Parse(entry.DownloadURL)
		key := entry.ID + "\x00" + entry.Version
		if entry.ID == "" || entry.Version == "" || err != nil || parsed.Scheme != "https" || parsed.Host == "" || !digestPattern.MatchString(entry.PackageSHA256) || seen[key] {
			return Index{}, fmt.Errorf("模板索引条目无效: %s", entry.ID)
		}
		seen[key] = true
	}
	return index, nil
}
