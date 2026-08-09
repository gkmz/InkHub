// Package taxonomy 定义分类体系和 Tag 确定性规则。
package taxonomy

import (
	"fmt"
	"strings"
	"unicode"
)

// Rules 定义 Tag 标准名称映射和数量限制。
type Rules struct {
	Aliases map[string]string
	MinTags int
	MaxTags int
}

// NormalizeTag 将标签转换为跨渠道稳定的 lowercase kebab-case 名称。
func NormalizeTag(tag string) string {
	value := strings.TrimSpace(tag)
	if value == "" {
		return ""
	}

	var result strings.Builder
	pendingSeparator := false
	for _, character := range strings.ToLower(value) {
		if unicode.IsSpace(character) || character == '/' || character == '_' || character == '-' {
			pendingSeparator = result.Len() > 0
			continue
		}
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '.' && character != '+' && character != '#' {
			pendingSeparator = result.Len() > 0
			continue
		}
		if pendingSeparator {
			result.WriteByte('-')
			pendingSeparator = false
		}
		result.WriteRune(character)
	}
	return strings.Trim(result.String(), "-")
}

// NormalizeTags 清理空值和规范化后的重复值，并优先应用 taxonomy 别名。
func NormalizeTags(tags []string, canonical map[string]string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		value := NormalizeTag(tag)
		if value == "" {
			continue
		}
		if standard, ok := canonical[value]; ok {
			value = NormalizeTag(standard)
		}
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// NormalizeTagsStrict 规范化标签，并拒绝不包含任何可用字符的非空标签。
func NormalizeTagsStrict(tags []string, canonical map[string]string) ([]string, error) {
	for _, tag := range tags {
		if strings.TrimSpace(tag) != "" && NormalizeTag(tag) == "" {
			return nil, fmt.Errorf("Tag %q 不包含可用的文字或数字", tag)
		}
	}
	return NormalizeTags(tags, canonical), nil
}

// ValidateTags 规范化并校验文章 Tag 的数量。
func ValidateTags(tags []string, rules Rules) ([]string, error) {
	result, err := NormalizeTagsStrict(tags, rules.Aliases)
	if err != nil {
		return nil, err
	}
	if len(result) < rules.MinTags {
		return nil, fmt.Errorf("Tag 数量不能少于 %d", rules.MinTags)
	}
	if rules.MaxTags > 0 && len(result) > rules.MaxTags {
		return nil, fmt.Errorf("Tag 数量不能超过 %d", rules.MaxTags)
	}
	return result, nil
}
