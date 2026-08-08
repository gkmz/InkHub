package hugo

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func previewRenderCandidates(input contracts.PublishInput, section, directory, bundle string) []string {
	values := make([]string, 0, 3)
	if value := previewURLPath(input.Article.URL); value != "" {
		values = append(values, value)
	}
	if slug := strings.Trim(input.Article.Slug, "/ "); slug != "" {
		values = append(values, path.Join(section, filepath.ToSlash(directory), slug, "index.html"))
	}
	values = append(values, path.Join(section, filepath.ToSlash(directory), bundle, "index.html"))
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimPrefix(path.Clean("/"+value), "/")
		if value == "" || value == "." || strings.HasPrefix(value, "../") || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func previewURLPath(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Path != "" {
		value = parsed.Path
	}
	value = strings.Trim(value, "/ ")
	if value == "" {
		return ""
	}
	if strings.EqualFold(path.Ext(value), ".html") {
		return path.Clean(value)
	}
	return path.Join(value, "index.html")
}

func findPreviewRenderPath(publicRoot string, candidates []string) (string, error) {
	for _, candidate := range candidates {
		absolute := filepath.Join(publicRoot, filepath.FromSlash(candidate))
		info, err := os.Lstat(absolute)
		if err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("候选路径均不存在: %s", strings.Join(candidates, ", "))
}
