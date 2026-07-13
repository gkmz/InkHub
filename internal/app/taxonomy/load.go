// Package taxonomy 编排权威 taxonomy 文件的加载和受控修改。
package taxonomy

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Term 是一个规范 Tag 及其治理属性。
type Term struct {
	Name              string
	Aliases           []string
	Core              bool
	AllowLowFrequency bool
}

// Authoritative 是解析并校验后的权威 taxonomy。
type Authoritative struct {
	Version    int
	Categories []string
	Series     []string
	Tags       map[string]Term
	Aliases    map[string]string
}

type taxonomyFile struct {
	Version    int        `yaml:"version"`
	Categories []string   `yaml:"categories"`
	Series     []string   `yaml:"series"`
	Tags       []termFile `yaml:"tags"`
}

type termFile struct {
	Name              string   `yaml:"name"`
	Aliases           []string `yaml:"aliases"`
	Core              bool     `yaml:"core"`
	AllowLowFrequency bool     `yaml:"allowLowFrequency"`
}

// LoadAuthoritative 严格读取并校验 Hugo taxonomy 权威文件。
func LoadAuthoritative(ctx context.Context, path string) (Authoritative, error) {
	if err := ctx.Err(); err != nil {
		return Authoritative{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Authoritative{}, fmt.Errorf("打开 taxonomy: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var raw taxonomyFile
	if err := decoder.Decode(&raw); err != nil {
		return Authoritative{}, fmt.Errorf("解析 taxonomy: %w", err)
	}
	if raw.Version != 1 {
		return Authoritative{}, fmt.Errorf("不支持 taxonomy version %d", raw.Version)
	}
	result := Authoritative{Version: raw.Version, Categories: raw.Categories, Series: raw.Series, Tags: map[string]Term{}, Aliases: map[string]string{}}
	if err := validateUnique("Category", raw.Categories); err != nil {
		return Authoritative{}, err
	}
	if err := validateUnique("Series", raw.Series); err != nil {
		return Authoritative{}, err
	}
	for _, item := range raw.Tags {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if name == "" {
			return Authoritative{}, fmt.Errorf("Tag 名称不能为空")
		}
		if _, exists := result.Tags[name]; exists {
			return Authoritative{}, fmt.Errorf("Tag 重复: %s", name)
		}
		result.Tags[name] = Term{Name: name, Aliases: item.Aliases, Core: item.Core, AllowLowFrequency: item.AllowLowFrequency}
	}
	seenAliases := make(map[string]bool)
	for canonical, item := range result.Tags {
		for _, rawAlias := range item.Aliases {
			alias := strings.ToLower(strings.TrimSpace(rawAlias))
			if alias == "" || result.Tags[alias].Name != "" {
				return Authoritative{}, fmt.Errorf("Alias 与规范 Tag 冲突: %s", alias)
			}
			if seenAliases[alias] {
				return Authoritative{}, fmt.Errorf("Alias 重复: %s", alias)
			}
			seenAliases[alias] = true
			result.Aliases[alias] = canonical
		}
	}
	return result, nil
}

func validateUnique(kind string, values []string) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			return fmt.Errorf("%s 不能为空", kind)
		}
		if seen[normalized] {
			return fmt.Errorf("%s 重复: %s", kind, normalized)
		}
		seen[normalized] = true
	}
	return nil
}
