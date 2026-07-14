package obsidian

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

func parseDocument(content []byte) (contracts.SourceDocument, error) {
	frontmatter, body, err := splitDocument(string(content))
	if err != nil {
		return contracts.SourceDocument{}, err
	}
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	if frontmatter != "" {
		var root yaml.Node
		if err := yaml.Unmarshal([]byte(frontmatter), &root); err != nil {
			return contracts.SourceDocument{}, fmt.Errorf("解析 frontmatter: %w", err)
		}
		mapping, err = mappingRoot(&root)
		if err != nil {
			return contracts.SourceDocument{}, err
		}
	}
	if err := validateFrontmatter(mapping); err != nil {
		return contracts.SourceDocument{}, err
	}
	publish := mappingValue(mapping, "publish")
	value := article.Article{
		StableID:    article.StableID(scalarValue(mapping, "id")),
		Title:       scalarValue(mapping, "title"),
		Description: scalarValue(mapping, "description"),
		Tags:        stringSequence(mapping, "tags"),
		Keywords:    stringSequence(mapping, "keywords"),
		Category:    scalarValue(publish, "category"),
		Series:      scalarValue(publish, "series"),
		Slug:        scalarValue(publish, "slug"),
		Cover:       scalarValue(publish, "cover"),
	}
	sum := sha256.Sum256(content)
	return contracts.SourceDocument{
		Article:        value,
		Body:           body,
		RawFrontmatter: frontmatter,
		Fingerprint:    hex.EncodeToString(sum[:]),
	}, nil
}

func validateFrontmatter(mapping *yaml.Node) error {
	for _, key := range []string{"id", "title", "description"} {
		if err := validateStringField(mapping, key); err != nil {
			return err
		}
	}
	for _, key := range []string{"tags", "keywords"} {
		value := mappingValue(mapping, key)
		if value == nil {
			continue
		}
		if value.Kind == yaml.ScalarNode && value.Tag == "!!str" {
			continue
		}
		if value.Kind != yaml.SequenceNode {
			return fmt.Errorf("字段 %s 必须是字符串数组", key)
		}
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
				return fmt.Errorf("字段 %s 必须是字符串数组", key)
			}
		}
	}
	publish := mappingValue(mapping, "publish")
	if publish != nil {
		if publish.Kind != yaml.MappingNode {
			return fmt.Errorf("字段 publish 必须是对象")
		}
		for _, key := range []string{"category", "series", "slug", "cover"} {
			if err := validateStringField(publish, key); err != nil {
				return fmt.Errorf("publish.%w", err)
			}
		}
	}
	return nil
}

func validateStringField(mapping *yaml.Node, key string) error {
	value := mappingValue(mapping, key)
	if value != nil && (value.Kind != yaml.ScalarNode || (value.Tag != "!!str" && value.Tag != "!!null")) {
		return fmt.Errorf("字段 %s 必须是字符串", key)
	}
	return nil
}

func splitDocument(content string) (string, string, error) {
	normalized := strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(normalized, "---\n") && !strings.HasPrefix(normalized, "---\r\n") {
		return "", normalized, nil
	}
	start := strings.Index(normalized, "\n") + 1
	remaining := normalized[start:]
	end, delimiterLength := closingDelimiter(remaining)
	if end < 0 {
		return "", "", fmt.Errorf("frontmatter 未闭合")
	}
	return remaining[:end], remaining[end+delimiterLength:], nil
}

func closingDelimiter(content string) (int, int) {
	candidates := []string{"\n---\r\n", "\n---\n"}
	bestIndex, bestLength := -1, 0
	for _, candidate := range candidates {
		if index := strings.Index(content, candidate); index >= 0 && (bestIndex < 0 || index < bestIndex) {
			bestIndex, bestLength = index, len(candidate)
		}
	}
	if strings.HasSuffix(content, "\n---") {
		index := len(content) - len("\n---")
		if bestIndex < 0 || index < bestIndex {
			bestIndex, bestLength = index, len("\n---")
		}
	}
	return bestIndex, bestLength
}

func mappingRoot(root *yaml.Node) (*yaml.Node, error) {
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter 必须是对象")
	}
	return root.Content[0], nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func scalarValue(mapping *yaml.Node, key string) string {
	value := mappingValue(mapping, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

func stringSequence(mapping *yaml.Node, key string) []string {
	value := mappingValue(mapping, key)
	if value != nil && value.Kind == yaml.ScalarNode && value.Tag == "!!str" {
		return []string{value.Value}
	}
	if value == nil || value.Kind != yaml.SequenceNode {
		return nil
	}
	result := make([]string, 0, len(value.Content))
	for _, item := range value.Content {
		if item.Kind == yaml.ScalarNode {
			result = append(result, item.Value)
		}
	}
	return result
}
