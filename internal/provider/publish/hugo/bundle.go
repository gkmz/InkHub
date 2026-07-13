package hugo

import (
	"bytes"
	"crypto/sha256"
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

func fileHasSourceID(path, sourceID string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("读取 Hugo bundle: %w", err)
	}
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return false, nil
	}
	end := bytes.Index(content[4:], []byte("\n---"))
	if end < 0 {
		return false, fmt.Errorf("Hugo frontmatter 未闭合: %s", path)
	}
	var metadata struct {
		SourceID string `yaml:"source_id"`
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(content[4 : 4+end])))
	if err := decoder.Decode(&metadata); err != nil {
		return false, fmt.Errorf("解析 Hugo frontmatter: %w", err)
	}
	return metadata.SourceID == sourceID, nil
}
