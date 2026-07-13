// Package taxonomy 定义分类体系和 Tag 确定性规则。
package taxonomy

import (
	"fmt"
	"strings"
)

// Rules 定义 Tag alias、准入表和数量限制。
type Rules struct {
	Aliases map[string]string
	Allowed map[string]bool
	MinTags int
	MaxTags int
}

// ValidateTags 规范化并校验文章 Tag。
func ValidateTags(tags []string, rules Rules) ([]string, error) {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if canonical, ok := rules.Aliases[normalized]; ok {
			normalized = canonical
		}
		if len(rules.Allowed) > 0 && !rules.Allowed[normalized] {
			return nil, fmt.Errorf("Tag %q 未通过准入", normalized)
		}
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	if len(result) < rules.MinTags {
		return nil, fmt.Errorf("Tag 数量不能少于 %d", rules.MinTags)
	}
	if rules.MaxTags > 0 && len(result) > rules.MaxTags {
		return nil, fmt.Errorf("Tag 数量不能超过 %d", rules.MaxTags)
	}
	return result, nil
}
