package hugo

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

type resourcePlan struct {
	Source string
	Name   string
	Digest [sha256.Size]byte
}

func planResources(resources []contracts.ResourceRef) ([]resourcePlan, error) {
	result := make([]resourcePlan, 0, len(resources))
	byName := make(map[string]resourcePlan, len(resources))
	for _, resource := range resources {
		if resource.Resolved == "" {
			return nil, fmt.Errorf("Hugo 资源缺少已解析路径: %s", resource.Original)
		}
		content, err := os.ReadFile(resource.Resolved)
		if err != nil {
			return nil, fmt.Errorf("读取 Hugo 资源 %s: %w", resource.Original, err)
		}
		name := filepath.Base(resource.Resolved)
		planned := resourcePlan{Source: resource.Resolved, Name: name, Digest: sha256.Sum256(content)}
		if existing, exists := byName[name]; exists {
			if existing.Digest != planned.Digest {
				return nil, fmt.Errorf("Hugo 资源文件名冲突: %s", name)
			}
			continue
		}
		byName[name] = planned
		result = append(result, planned)
	}
	return result, nil
}

func findBundle(root, section, sourceID string) (string, bool, error) {
	sectionRoot := filepath.Join(root, "content", section)
	if _, err := os.Stat(sectionRoot); os.IsNotExist(err) {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("读取 Hugo section: %w", err)
	}
	var found string
	err := filepath.WalkDir(sectionRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "index.md" {
			return nil
		}
		matched, err := fileHasSourceID(path, sourceID)
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}
		if found != "" {
			return fmt.Errorf("多个 Hugo bundle 使用相同 source_id: %s", sourceID)
		}
		found = filepath.Dir(path)
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return found, found != "", nil
}

// FindBundleBySourceID 扫描 Hugo content 目录，按 frontmatter source_id 反查 bundle 路径。
func FindBundleBySourceID(root, sourceID string) (string, string, bool, error) {
	return findBundleBySourceID(filepath.Join(root, "content"), sourceID)
}

// FindPublishedBundleBySourceID 仅返回会被 Hugo 默认构建的 Bundle，草稿目标不会用于生成公开链接。
func FindPublishedBundleBySourceID(root, sourceID string) (string, string, bool, error) {
	bundlePath, section, found, err := FindBundleBySourceID(root, sourceID)
	if err != nil || !found {
		return bundlePath, section, found, err
	}
	published, err := bundleIsPublished(filepath.Join(root, "content"), bundlePath)
	return bundlePath, section, published, err
}

// findBundleBySourceID 按实际 contentDir 扫描 source_id。
func findBundleBySourceID(contentRoot, sourceID string) (string, string, bool, error) {
	if _, err := os.Stat(contentRoot); os.IsNotExist(err) {
		return "", "", false, nil
	} else if err != nil {
		return "", "", false, fmt.Errorf("读取 Hugo content: %w", err)
	}
	var found, foundSection string
	err := filepath.WalkDir(contentRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Name() != "index.md" {
			return nil
		}
		matched, err := fileHasSourceID(path, sourceID)
		if err != nil || !matched {
			return err
		}
		relative, err := filepath.Rel(contentRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 || !safeSegment(parts[0]) || strings.HasPrefix(parts[0], ".") {
			return fmt.Errorf("Hugo bundle 不在合法 Section 下: %s", relative)
		}
		if found != "" {
			return fmt.Errorf("多个 Hugo bundle 使用相同 source_id: %s", sourceID)
		}
		found, foundSection = filepath.Dir(path), parts[0]
		return nil
	})
	if err != nil {
		return "", "", false, err
	}
	return found, foundSection, found != "", nil
}

func fileHasSourceID(path, sourceID string) (bool, error) {
	metadata, err := readBundleMetadata(path)
	if err != nil {
		return false, err
	}
	return metadata.SourceID == sourceID, nil
}

type bundleMetadata struct {
	SourceID string `yaml:"source_id"`
	Draft    *bool  `yaml:"draft"`
	Cascade  struct {
		Draft *bool `yaml:"draft"`
	} `yaml:"cascade"`
}

func readBundleMetadata(path string) (bundleMetadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return bundleMetadata{}, fmt.Errorf("读取 Hugo bundle: %w", err)
	}
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return bundleMetadata{}, nil
	}
	end := bytes.Index(content[4:], []byte("\n---"))
	if end < 0 {
		return bundleMetadata{}, fmt.Errorf("Hugo frontmatter 未闭合: %s", path)
	}
	var metadata bundleMetadata
	decoder := yaml.NewDecoder(strings.NewReader(string(content[4 : 4+end])))
	if err := decoder.Decode(&metadata); err != nil {
		return bundleMetadata{}, fmt.Errorf("解析 Hugo frontmatter: %w", err)
	}
	return metadata, nil
}

func bundleIsPublished(contentRoot, bundlePath string) (bool, error) {
	relative, err := filepath.Rel(contentRoot, bundlePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false, fmt.Errorf("Hugo bundle 超出 content 目录: %s", bundlePath)
	}
	metadata, err := readBundleMetadata(filepath.Join(bundlePath, "index.md"))
	if err != nil {
		return false, err
	}
	if metadata.Draft != nil {
		return !*metadata.Draft, nil
	}
	// 由近到远读取 Section 级 cascade；页面自身未声明 draft 时继承最近的设置。
	for directory := filepath.Dir(bundlePath); ; directory = filepath.Dir(directory) {
		sectionMetadata, readErr := readBundleMetadata(filepath.Join(directory, "_index.md"))
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return false, readErr
		}
		if readErr == nil && sectionMetadata.Cascade.Draft != nil {
			return !*sectionMetadata.Cascade.Draft, nil
		}
		if directory == contentRoot {
			break
		}
	}
	return true, nil
}
