package hugo

import (
	"fmt"
	"os"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

type taxonomyRules struct {
	Categories map[string]bool
	Series     map[string]bool
	Tags       map[string]bool
}

type taxonomyFile struct {
	Version    int      `yaml:"version"`
	Categories []string `yaml:"categories"`
	Series     []string `yaml:"series"`
	Tags       []struct {
		Name              string   `yaml:"name"`
		Aliases           []string `yaml:"aliases"`
		Core              bool     `yaml:"core"`
		AllowLowFrequency bool     `yaml:"allowLowFrequency"`
	} `yaml:"tags"`
}

func loadTaxonomy(path string) (taxonomyRules, error) {
	file, err := os.Open(path)
	if err != nil {
		return taxonomyRules{}, fmt.Errorf("打开 taxonomy: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var raw taxonomyFile
	if err := decoder.Decode(&raw); err != nil {
		return taxonomyRules{}, fmt.Errorf("解析 taxonomy: %w", err)
	}
	if raw.Version != 1 {
		return taxonomyRules{}, fmt.Errorf("不支持 taxonomy version %d", raw.Version)
	}
	result := taxonomyRules{Categories: map[string]bool{}, Series: map[string]bool{}, Tags: map[string]bool{}}
	for _, value := range raw.Categories {
		value = strings.TrimSpace(value)
		if value == "" || result.Categories[value] {
			return taxonomyRules{}, fmt.Errorf("Category 为空或重复: %s", value)
		}
		result.Categories[value] = true
	}
	for _, value := range raw.Series {
		value = strings.TrimSpace(value)
		if value == "" || result.Series[value] {
			return taxonomyRules{}, fmt.Errorf("Series 为空或重复: %s", value)
		}
		result.Series[value] = true
	}
	for _, term := range raw.Tags {
		canonical := strings.ToLower(strings.TrimSpace(term.Name))
		if canonical == "" || result.Tags[canonical] {
			return taxonomyRules{}, fmt.Errorf("Tag 为空或重复: %s", term.Name)
		}
		result.Tags[canonical] = true
		for _, alias := range term.Aliases {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" || result.Tags[alias] {
				return taxonomyRules{}, fmt.Errorf("Tag alias 为空或冲突: %s", alias)
			}
			result.Tags[alias] = true
		}
	}
	return result, nil
}

func (r taxonomyRules) check(input contracts.PublishInput) []contracts.Diagnostic {
	var result []contracts.Diagnostic
	if value := strings.TrimSpace(input.Article.Category); value != "" && !r.Categories[value] {
		result = append(result, contracts.Diagnostic{Code: "hugo.category_unknown", Message: "Category 尚未进入 Hugo taxonomy: " + value, Blocking: true})
	}
	if value := strings.TrimSpace(input.Article.Series); value != "" && !r.Series[value] {
		result = append(result, contracts.Diagnostic{Code: "hugo.series_unknown", Message: "Series 尚未进入 Hugo taxonomy: " + value, Blocking: true})
	}
	for _, tag := range input.Article.Tags {
		if !r.Tags[strings.ToLower(strings.TrimSpace(tag))] {
			result = append(result, contracts.Diagnostic{Code: "hugo.tag_unknown", Message: "Tag 尚未进入 Hugo taxonomy: " + tag, Blocking: true})
		}
	}
	return result
}
