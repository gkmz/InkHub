// Package taxonomy 定义分类体系和 Tag 确定性规则。
package taxonomy

import (
	"fmt"
	"strings"
)

// Rules 定义 Tag 标准名称映射和数量限制。
type Rules struct {
	Aliases map[string]string
	MinTags int
	MaxTags int
}

// NormalizeTags 清理空值和重复值，并优先使用 taxonomy 快照中的标准名称。
func NormalizeTags(tags []string, canonical map[string]string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		key := strings.ToLower(value)
		if key == "" || seen[key] {
			continue
		}
		if standard, ok := canonical[key]; ok {
			value = standard
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

// ValidateTags 规范化并校验文章 Tag 的数量。
func ValidateTags(tags []string, rules Rules) ([]string, error) {
	result := NormalizeTags(tags, rules.Aliases)
	if len(result) < rules.MinTags {
		return nil, fmt.Errorf("Tag 数量不能少于 %d", rules.MinTags)
	}
	if rules.MaxTags > 0 && len(result) > rules.MaxTags {
		return nil, fmt.Errorf("Tag 数量不能超过 %d", rules.MaxTags)
	}
	return result, nil
}
