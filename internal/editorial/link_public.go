package editorial

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func publishedArticleURL(cfg hugoProviderConfig, bundlePath string) (string, error) {
	publicPath, err := publishedArticlePath(filepath.Join(bundlePath, "index.md"))
	if err != nil {
		return "", err
	}
	if publicPath == "" {
		relative, relErr := filepath.Rel(filepath.Join(cfg.Root, "content"), bundlePath)
		if relErr != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
			return "", fmt.Errorf("Hugo Bundle 公开路径无效")
		}
		publicPath = filepath.ToSlash(relative)
	}
	base, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/") + "/")
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("Hugo Base URL 无效")
	}
	if absolute, parseErr := url.Parse(publicPath); parseErr == nil && absolute.IsAbs() {
		return absolute.String(), nil
	}
	referencePath := strings.Trim(publicPath, "/") + "/"
	if strings.HasPrefix(publicPath, "/") {
		referencePath = "/" + referencePath
	}
	reference := &url.URL{Path: referencePath}
	base.Path = strings.TrimRight(base.Path, "/") + "/"
	return base.ResolveReference(reference).String(), nil
}

func publishedArticlePath(indexPath string) (string, error) {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(string(content), "---\n") {
		return "", nil
	}
	end := strings.Index(string(content[4:]), "\n---")
	if end < 0 {
		return "", fmt.Errorf("Hugo frontmatter 未闭合")
	}
	var metadata struct {
		URL  string `yaml:"url"`
		Slug string `yaml:"slug"`
	}
	if yaml.Unmarshal(content[4:4+end], &metadata) != nil {
		return "", fmt.Errorf("Hugo frontmatter 无效")
	}
	if strings.TrimSpace(metadata.URL) != "" {
		return strings.TrimSpace(metadata.URL), nil
	}
	if strings.TrimSpace(metadata.Slug) != "" {
		return path.Clean(strings.TrimSpace(metadata.Slug)), nil
	}
	return "", nil
}

func loadHugoLinkConfig(ctx context.Context, db *sql.DB, workspaceID string) hugoProviderConfig {
	if db == nil || workspaceID == "" {
		return hugoProviderConfig{}
	}
	var configJSON string
	err := db.QueryRowContext(ctx, `SELECT config_json FROM provider_instances WHERE workspace_id=? AND provider_type='hugo' AND enabled=1`, workspaceID).Scan(&configJSON)
	if err != nil {
		return hugoProviderConfig{}
	}
	var cfg hugoProviderConfig
	if json.Unmarshal([]byte(configJSON), &cfg) != nil {
		return hugoProviderConfig{}
	}
	return cfg
}
