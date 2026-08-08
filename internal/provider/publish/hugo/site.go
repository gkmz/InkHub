package hugo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// SiteInfo 描述从 Hugo 根配置中确定的站点路径。
type SiteInfo struct {
	Root       string `json:"root"`
	ContentDir string `json:"content_dir"`
}

// InspectSite 校验 Hugo 根目录并解析 contentDir；未配置时使用 Hugo 默认值 content。
func InspectSite(root string) (SiteInfo, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return SiteInfo{}, fmt.Errorf("Hugo 根目录无效")
	}
	configPath := hugoConfigPath(absolute)
	if configPath == "" {
		return SiteInfo{}, fmt.Errorf("目录不是有效 Hugo 站点: %s", root)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return SiteInfo{}, fmt.Errorf("读取 Hugo 配置: %w", err)
	}
	var raw struct {
		ContentDir string `json:"contentDir" yaml:"contentDir" toml:"contentDir"`
		Module     struct {
			Mounts []struct {
				Source string `json:"source" yaml:"source" toml:"source"`
				Target string `json:"target" yaml:"target" toml:"target"`
			} `json:"mounts" yaml:"mounts" toml:"mounts"`
		} `json:"module" yaml:"module" toml:"module"`
	}
	switch strings.ToLower(filepath.Ext(configPath)) {
	case ".toml":
		err = toml.Unmarshal(content, &raw)
	case ".json":
		err = json.Unmarshal(content, &raw)
	default:
		err = yaml.Unmarshal(content, &raw)
	}
	if err != nil {
		return SiteInfo{}, fmt.Errorf("解析 Hugo 配置: %w", err)
	}
	for _, mount := range raw.Module.Mounts {
		source := filepath.ToSlash(filepath.Clean(strings.TrimSpace(mount.Source)))
		target := filepath.ToSlash(filepath.Clean(strings.TrimSpace(mount.Target)))
		if (target == "content" || strings.HasPrefix(target, "content/")) && source != target {
			return SiteInfo{}, fmt.Errorf("暂不支持通过 Hugo module mounts 重定向 content，请改用 contentDir")
		}
	}
	contentDir := strings.TrimSpace(raw.ContentDir)
	if contentDir == "" {
		contentDir = "content"
	}
	if !safeRelativeDirectory(contentDir) {
		return SiteInfo{}, fmt.Errorf("Hugo contentDir 必须是站点内的相对目录")
	}
	return SiteInfo{Root: filepath.Clean(absolute), ContentDir: filepath.ToSlash(filepath.Clean(contentDir))}, nil
}

func hugoConfigPath(root string) string {
	for _, name := range []string{"hugo.yaml", "hugo.yml", "hugo.toml", "hugo.json", "config.yaml", "config.yml", "config.toml", "config.json"} {
		candidate := filepath.Join(root, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func safeRelativeDirectory(value string) bool {
	if filepath.IsAbs(value) {
		return false
	}
	clean := filepath.Clean(strings.TrimSpace(value))
	return clean != "" && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func contentRoot(root, contentDir string) string {
	if strings.TrimSpace(contentDir) == "" {
		contentDir = "content"
	}
	return filepath.Join(root, filepath.FromSlash(contentDir))
}
