// Package hugo 实现基于 Hugo 标准资源的 Taxonomy Provider。
package hugo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

// Config 定义 Hugo taxonomy 发现所需的站点配置。
type Config struct {
	ProviderID   string
	Root         string
	ContentDir   string
	PollInterval time.Duration
}

// Provider 从 Hugo 配置、内容 frontmatter 和 term 页面发现 taxonomy。
type Provider struct {
	config Config
}

var _ contracts.TaxonomyProvider = (*Provider)(nil)

// New 创建并校验 Hugo Taxonomy Provider。
func New(config Config) (*Provider, error) {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("解析 Hugo 根目录: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("Hugo 根目录不可用: %s", root)
	}
	if config.ContentDir == "" {
		config.ContentDir = "content"
	}
	if filepath.IsAbs(config.ContentDir) || strings.HasPrefix(filepath.Clean(config.ContentDir), "..") {
		return nil, fmt.Errorf("Hugo contentDir 必须是站点内相对路径")
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 2 * time.Second
	}
	config.Root = root
	return &Provider{config: config}, nil
}

// Descriptor 返回 Hugo 标准 taxonomy 的能力描述。
func (*Provider) Descriptor() contracts.TaxonomyDescriptor {
	return contracts.TaxonomyDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderHugo, DisplayName: "Hugo Taxonomy", Version: "1",
		Capabilities: []contracts.Capability{contracts.CapabilityTaxonomy, contracts.CapabilityWatch},
	}, Writable: true}
}

// Validate 检查 Hugo 配置和标准 taxonomy 资源是否可解析。
func (p *Provider) Validate(ctx context.Context) error {
	_, err := p.Discover(ctx, contracts.TaxonomyCursor{})
	return err
}

// Discover 返回当前 Hugo taxonomy 的确定性完整快照。
func (p *Provider) Discover(ctx context.Context, cursor contracts.TaxonomyCursor) (contracts.TaxonomySnapshot, error) {
	mappings, contentDir, configPath, configContent, err := p.loadMappings()
	if err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	terms := make(map[string]contracts.TaxonomyTerm)
	hash := sha256.New()
	_, _ = hash.Write([]byte(filepath.ToSlash(configPath)))
	_, _ = hash.Write(configContent)
	contentRoot := filepath.Join(p.config.Root, contentDir)
	paths, err := markdownPaths(ctx, contentRoot)
	if err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return contracts.TaxonomySnapshot{}, fmt.Errorf("读取 Hugo 内容: %w", readErr)
		}
		relative, _ := filepath.Rel(p.config.Root, path)
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hash.Write(content)
		frontmatter, parseErr := parseFrontmatter(content)
		if parseErr != nil {
			return contracts.TaxonomySnapshot{}, fmt.Errorf("解析 Hugo frontmatter %s: %w", relative, parseErr)
		}
		for singular, plural := range mappings {
			for _, value := range stringValues(frontmatter[plural]) {
				addUsage(terms, singular, value)
			}
		}
	}
	applyTermPages(terms, mappings, contentRoot, paths)
	revision := hex.EncodeToString(hash.Sum(nil))
	ref := contracts.ProviderRef{ID: p.config.ProviderID, Type: contracts.ProviderHugo}
	if cursor.Revision != "" && cursor.Revision == revision {
		return contracts.TaxonomySnapshot{ProviderRef: ref, Revision: revision, Complete: true, Unchanged: true}, nil
	}
	values := make([]contracts.TaxonomyTerm, 0, len(terms))
	for _, term := range terms {
		values = append(values, term)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Kind == values[j].Kind {
			return values[i].Key < values[j].Key
		}
		return values[i].Kind < values[j].Kind
	})
	return contracts.TaxonomySnapshot{ProviderRef: ref, Revision: revision, Terms: values, Complete: true}, nil
}

// PlanChange 为新 term 生成标准 Hugo branch bundle 变更，不写文件。
func (p *Provider) PlanChange(ctx context.Context, command contracts.TaxonomyCommand) (contracts.TaxonomyChangeSet, error) {
	if command.Kind != contracts.TaxonomyCreateTerm {
		return contracts.TaxonomyChangeSet{}, fmt.Errorf("Hugo MVP 只支持创建 taxonomy term")
	}
	current, err := p.Discover(ctx, contracts.TaxonomyCursor{})
	if err != nil {
		return contracts.TaxonomyChangeSet{}, err
	}
	if command.ExpectedRevision == "" || command.ExpectedRevision != current.Revision {
		return contracts.TaxonomyChangeSet{}, fmt.Errorf("Hugo taxonomy revision 已变化")
	}
	mappings, contentDir, _, _, err := p.loadMappings()
	if err != nil {
		return contracts.TaxonomyChangeSet{}, err
	}
	plural, exists := mappings[command.Term.Kind]
	if !exists || !safeTermKey(command.Term.Key) || strings.TrimSpace(command.Term.Name) == "" {
		return contracts.TaxonomyChangeSet{}, fmt.Errorf("Hugo taxonomy term 无效")
	}
	for _, term := range current.Terms {
		if term.Kind == command.Term.Kind && term.Key == strings.ToLower(command.Term.Key) {
			return contracts.TaxonomyChangeSet{}, fmt.Errorf("Hugo taxonomy term 已存在")
		}
	}
	frontmatter := struct {
		Title       string   `yaml:"title"`
		Description string   `yaml:"description,omitempty"`
		Aliases     []string `yaml:"aliases,omitempty"`
	}{Title: strings.TrimSpace(command.Term.Name), Description: command.Term.Metadata["description"]}
	if aliases := command.Term.Metadata["aliases"]; aliases != "" {
		frontmatter.Aliases = strings.Split(aliases, "\n")
	}
	encoded, err := yaml.Marshal(frontmatter)
	if err != nil {
		return contracts.TaxonomyChangeSet{}, fmt.Errorf("编码 Hugo term page: %w", err)
	}
	relative := filepath.ToSlash(filepath.Join(contentDir, plural, strings.ToLower(command.Term.Key), "_index.md"))
	return contracts.TaxonomyChangeSet{
		ProviderRef: contracts.ProviderRef{ID: p.config.ProviderID, Type: contracts.ProviderHugo}, ExpectedRevision: current.Revision,
		Files: []contracts.TaxonomyFileChange{{RelativePath: relative, After: "---\n" + string(encoded) + "---\n"}},
	}, nil
}

// ApplyChange 校验 revision 和文件前态后原子创建标准 Hugo term page。
func (p *Provider) ApplyChange(ctx context.Context, change contracts.TaxonomyChangeSet) (contracts.TaxonomySnapshot, error) {
	if change.ProviderRef.ID != p.config.ProviderID || change.ProviderRef.Type != contracts.ProviderHugo || len(change.Files) != 1 {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("Hugo taxonomy 变更集无效")
	}
	current, err := p.Discover(ctx, contracts.TaxonomyCursor{})
	if err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	if change.ExpectedRevision == "" || current.Revision != change.ExpectedRevision {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("Hugo taxonomy 已被外部修改")
	}
	fileChange := change.Files[0]
	target, err := p.resolveChangePath(fileChange.RelativePath)
	if err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	if _, err := os.Stat(target); err == nil || !os.IsNotExist(err) || fileChange.Before != "" {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("Hugo taxonomy 目标文件前态不匹配")
	}
	termDir := filepath.Dir(target)
	if err := os.MkdirAll(filepath.Dir(termDir), 0o755); err != nil {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("创建 Hugo taxonomy 目录: %w", err)
	}
	// term 目录使用独占创建取得所有权，避免检查后被外部进程并发创建并覆盖。
	if err := os.Mkdir(termDir, 0o755); err != nil {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("Hugo term 目录已存在或不可创建: %w", err)
	}
	if err := filesystem.AtomicWrite(target, []byte(fileChange.After), func(path string) error {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		_, parseErr := parseFrontmatter(content)
		return parseErr
	}); err != nil {
		_ = os.Remove(termDir)
		return contracts.TaxonomySnapshot{}, err
	}
	return p.Discover(ctx, contracts.TaxonomyCursor{})
}

// Watch 轮询权威 revision，并在变化时通知 Application 重新发现。
func (p *Provider) Watch(ctx context.Context, changes chan<- contracts.TaxonomyChange) error {
	current, err := p.Discover(ctx, contracts.TaxonomyCursor{})
	if err != nil {
		return err
	}
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			next, discoverErr := p.Discover(ctx, contracts.TaxonomyCursor{Revision: current.Revision})
			if discoverErr != nil {
				return discoverErr
			}
			if next.Unchanged {
				continue
			}
			current = next
			select {
			case changes <- contracts.TaxonomyChange{Revision: next.Revision}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (p *Provider) loadMappings() (map[string]string, string, string, []byte, error) {
	path := ""
	for _, name := range []string{"hugo.toml", "hugo.yaml", "hugo.yml", "hugo.json", "config.toml", "config.yaml", "config.yml", "config.json"} {
		candidate := filepath.Join(p.config.Root, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			path = candidate
			break
		}
	}
	if path == "" {
		return nil, "", "", nil, fmt.Errorf("未找到 Hugo 配置文件")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("读取 Hugo 配置: %w", err)
	}
	var raw struct {
		Taxonomies map[string]string `json:"taxonomies" yaml:"taxonomies" toml:"taxonomies"`
		ContentDir string            `json:"contentDir" yaml:"contentDir" toml:"contentDir"`
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		err = toml.Unmarshal(content, &raw)
	case ".json":
		err = json.Unmarshal(content, &raw)
	default:
		err = yaml.Unmarshal(content, &raw)
	}
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("解析 Hugo 配置: %w", err)
	}
	contentDir := p.config.ContentDir
	if raw.ContentDir != "" && contentDir == "content" {
		contentDir = raw.ContentDir
	}
	if !safeRelativeDir(contentDir) {
		return nil, "", "", nil, fmt.Errorf("Hugo contentDir 必须位于站点内")
	}
	if len(raw.Taxonomies) == 0 {
		raw.Taxonomies = map[string]string{"category": "categories", "tag": "tags"}
	}
	for singular, plural := range raw.Taxonomies {
		if !safeTermKey(singular) || !safeTermKey(plural) {
			return nil, "", "", nil, fmt.Errorf("Hugo taxonomies 包含空名称")
		}
	}
	return raw.Taxonomies, contentDir, path, content, nil
}

func safeTermKey(key string) bool {
	clean := filepath.Clean(strings.TrimSpace(key))
	return clean != "" && clean != "." && clean != ".." && filepath.Base(clean) == clean && !strings.ContainsAny(clean, `/\\`)
}

func safeRelativeDir(value string) bool {
	if filepath.IsAbs(value) {
		return false
	}
	clean := filepath.Clean(strings.TrimSpace(value))
	return clean != "" && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func (p *Provider) resolveChangePath(relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("Hugo taxonomy 变更路径必须是相对路径")
	}
	target := filepath.Join(p.config.Root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(p.config.Root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Hugo taxonomy 变更路径越界")
	}
	return target, nil
}

func markdownPaths(ctx context.Context, root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func parseFrontmatter(content []byte) (map[string]any, error) {
	text := string(content)
	var raw string
	switch {
	case strings.HasPrefix(text, "---\n"):
		end := strings.Index(text[4:], "\n---")
		if end < 0 {
			return nil, fmt.Errorf("YAML frontmatter 未闭合")
		}
		raw = text[4 : 4+end]
		var result map[string]any
		if err := yaml.Unmarshal([]byte(raw), &result); err != nil {
			return nil, err
		}
		return result, nil
	case strings.HasPrefix(text, "+++\n"):
		end := strings.Index(text[4:], "\n+++")
		if end < 0 {
			return nil, fmt.Errorf("TOML frontmatter 未闭合")
		}
		raw = text[4 : 4+end]
		var result map[string]any
		if err := toml.Unmarshal([]byte(raw), &result); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return map[string]any{}, nil
	}
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func addUsage(terms map[string]contracts.TaxonomyTerm, kind, name string) {
	name = strings.TrimSpace(name)
	key := strings.ToLower(name)
	if key == "" {
		return
	}
	mapKey := kind + "\x00" + key
	term := terms[mapKey]
	if term.Key == "" {
		term = contracts.TaxonomyTerm{Kind: kind, Key: key, Name: name, CanonicalName: name, Metadata: map[string]string{}}
	}
	term.UsageCount++
	terms[mapKey] = term
}

func applyTermPages(terms map[string]contracts.TaxonomyTerm, mappings map[string]string, contentRoot string, paths []string) {
	for singular, plural := range mappings {
		base := filepath.Join(contentRoot, plural)
		for _, path := range paths {
			relative, err := filepath.Rel(base, path)
			if err != nil || filepath.Base(relative) != "_index.md" || strings.HasPrefix(relative, "..") {
				continue
			}
			key := strings.ToLower(filepath.ToSlash(filepath.Dir(relative)))
			if key == "." || key == "" {
				continue
			}
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			frontmatter, err := parseFrontmatter(content)
			if err != nil {
				continue
			}
			mapKey := singular + "\x00" + key
			term := terms[mapKey]
			if term.Key == "" {
				term = contracts.TaxonomyTerm{Kind: singular, Key: key, Name: key, CanonicalName: key, Metadata: map[string]string{}}
			}
			if title, ok := frontmatter["title"].(string); ok && strings.TrimSpace(title) != "" {
				term.Name, term.CanonicalName = title, title
			}
			for _, field := range []string{"description", "aliases"} {
				values := stringValues(frontmatter[field])
				if len(values) > 0 {
					term.Metadata[field] = strings.Join(values, "\n")
				}
			}
			terms[mapKey] = term
		}
	}
}
