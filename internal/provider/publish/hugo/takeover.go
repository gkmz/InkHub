package hugo

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"gopkg.in/yaml.v3"
)

// TakeoverBundle 是历史 Hugo Page Bundle 的可匹配元数据快照。
type TakeoverBundle struct {
	IndexPath  string `json:"-"`
	BundlePath string `json:"bundle_path"`
	Section    string `json:"section"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	URL        string `json:"url"`
	SourceID   string `json:"source_id"`
	SourcePath string `json:"source_path"`
}

// ScanTakeoverBundles 扫描 Hugo content 下的 Page Bundle，不读取正文内容。
func ScanTakeoverBundles(root string) ([]TakeoverBundle, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || !hasHugoConfig(absolute) {
		return nil, fmt.Errorf("目录不是有效 Hugo 站点: %s", root)
	}
	contentRoot := filepath.Join(absolute, "content")
	if _, err := os.Stat(contentRoot); os.IsNotExist(err) {
		return []TakeoverBundle{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("读取 Hugo content: %w", err)
	}
	result := make([]TakeoverBundle, 0)
	err = filepath.WalkDir(contentRoot, func(path string, entry fs.DirEntry, walkErr error) error {
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
		metadata, err := readTakeoverFrontmatter(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(contentRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		section := ""
		if len(parts) > 0 {
			section = parts[0]
		}
		metadata.IndexPath = path
		metadata.BundlePath = filepath.ToSlash(relative)
		metadata.Section = section
		result = append(result, metadata)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描 Hugo Bundle: %w", err)
	}
	return result, nil
}

// WriteTakeoverIdentity 原子补齐历史 Bundle 的源文章身份，不改动正文。
func WriteTakeoverIdentity(bundle TakeoverBundle, sourceID, sourcePath string) error {
	content, err := os.ReadFile(bundle.IndexPath)
	if err != nil {
		return fmt.Errorf("读取 Hugo Bundle: %w", err)
	}
	frontmatter, body, err := splitTakeoverDocument(content)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(frontmatter, &root); err != nil {
		return fmt.Errorf("解析 Hugo frontmatter: %w", err)
	}
	mapping, err := takeoverMapping(&root)
	if err != nil {
		return err
	}
	setTakeoverScalar(mapping, "source_id", sourceID)
	setTakeoverScalar(mapping, "source_path", filepath.ToSlash(sourcePath))
	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := encoder.Encode(mapping); err != nil {
		return fmt.Errorf("编码 Hugo frontmatter: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("关闭 Hugo frontmatter 编码器: %w", err)
	}
	updated := append([]byte("---\n"), encoded.Bytes()...)
	updated = append(updated, []byte("---\n")...)
	updated = append(updated, body...)
	if err := filesystem.AtomicWrite(bundle.IndexPath, updated, nil); err != nil {
		return fmt.Errorf("写入 Hugo Bundle 身份: %w", err)
	}
	return nil
}

func readTakeoverFrontmatter(path string) (TakeoverBundle, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return TakeoverBundle{}, fmt.Errorf("读取 Hugo Bundle: %w", err)
	}
	frontmatter, _, err := splitTakeoverDocument(content)
	if err != nil {
		return TakeoverBundle{}, fmt.Errorf("%s: %w", path, err)
	}
	var metadata struct {
		Title      string `yaml:"title"`
		Date       string `yaml:"date"`
		URL        string `yaml:"url"`
		SourceID   string `yaml:"source_id"`
		SourcePath string `yaml:"source_path"`
	}
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return TakeoverBundle{}, fmt.Errorf("解析 Hugo frontmatter %s: %w", path, err)
	}
	return TakeoverBundle{Title: metadata.Title, Date: metadata.Date, URL: metadata.URL, SourceID: metadata.SourceID, SourcePath: filepath.ToSlash(metadata.SourcePath)}, nil
}

func splitTakeoverDocument(content []byte) ([]byte, []byte, error) {
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, nil, fmt.Errorf("Hugo frontmatter 缺失")
	}
	end := bytes.Index(normalized[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, nil, fmt.Errorf("Hugo frontmatter 未闭合")
	}
	frontmatterEnd := 4 + end
	return normalized[4:frontmatterEnd], normalized[frontmatterEnd+5:], nil
}

func takeoverMapping(root *yaml.Node) (*yaml.Node, error) {
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Hugo frontmatter 必须是对象")
	}
	return root, nil
}

func setTakeoverScalar(mapping *yaml.Node, key, value string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
