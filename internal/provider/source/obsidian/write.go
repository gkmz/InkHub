package obsidian

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

func applyMetadataPatch(frontmatter, body string, patch contracts.MetadataPatch) ([]byte, error) {
	var root yaml.Node
	if strings.TrimSpace(frontmatter) == "" {
		// 普通 Obsidian 笔记允许没有 frontmatter，首次受控写回时创建标准对象。
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	} else if err := yaml.Unmarshal([]byte(frontmatter), &root); err != nil {
		return nil, fmt.Errorf("解析待写回 frontmatter: %w", err)
	}
	mapping, err := mappingRoot(&root)
	if err != nil {
		return nil, err
	}
	setString(mapping, "id", patch.StableID)
	setString(mapping, "title", patch.Title)
	setString(mapping, "description", patch.Description)
	setStrings(mapping, "tags", patch.Tags)
	setStrings(mapping, "keywords", patch.Keywords)
	if patch.Category != nil || patch.Series != nil || patch.Slug != nil || patch.Cover != nil {
		publish := ensureMapping(mapping, "publish")
		setString(publish, "category", patch.Category)
		setString(publish, "series", patch.Series)
		setString(publish, "slug", patch.Slug)
		setString(publish, "cover", patch.Cover)
	}

	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := encoder.Encode(mapping); err != nil {
		return nil, fmt.Errorf("编码 frontmatter: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("关闭 YAML encoder: %w", err)
	}
	result := []byte("---\n" + encoded.String() + "---\n" + body)
	return result, nil
}

func applyTakeoverIdentity(frontmatter, body, stableID string) ([]byte, error) {
	var root yaml.Node
	if strings.TrimSpace(frontmatter) == "" {
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	} else if err := yaml.Unmarshal([]byte(normalizeLegacyFrontmatterText(frontmatter)), &root); err != nil {
		return nil, fmt.Errorf("解析待接管 frontmatter: %w", err)
	}
	mapping, err := mappingRoot(&root)
	if err != nil {
		return nil, err
	}
	setString(mapping, "id", &stableID)
	// 历史库中曾出现数字 URL；接管时一次性规范化，常规解析继续保持严格契约。
	normalizeTakeoverStrings(mapping, []string{"title", "description", "url"})
	if publish := mappingValue(mapping, "publish"); publish != nil && publish.Kind == yaml.MappingNode {
		normalizeTakeoverStrings(publish, []string{"category", "series", "slug", "cover", "url"})
	}
	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := encoder.Encode(mapping); err != nil {
		return nil, fmt.Errorf("编码接管 frontmatter: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("关闭接管 frontmatter 编码器: %w", err)
	}
	return []byte("---\n" + encoded.String() + "---\n" + body), nil
}

func normalizeLegacyFrontmatterText(frontmatter string) string {
	scalarKeys := map[string]bool{"id": true, "url": true, "title": true, "description": true, "date": true, "updated": true}
	lines := strings.Split(strings.ReplaceAll(frontmatter, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) == "" || len(line) != len(strings.TrimLeft(line, " \t")) {
			continue
		}
		separator := strings.IndexByte(line, ':')
		if separator <= 0 || !scalarKeys[strings.TrimSpace(line[:separator])] {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		value := strings.TrimSpace(line[separator+1:])
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		lines[index] = key + ": " + strconv.Quote(value)
	}
	return strings.Join(lines, "\n")
}

func insertTakeoverStableID(content []byte, stableID string) []byte {
	offset := 0
	if bytes.HasPrefix(content, []byte("\xef\xbb\xbf")) {
		offset = 3
	}
	prefix := []byte("---\n")
	if bytes.HasPrefix(content[offset:], []byte("---\r\n")) {
		prefix = []byte("---\r\n")
	}
	if bytes.HasPrefix(content[offset:], prefix) {
		result := make([]byte, 0, len(content)+len(stableID)+5)
		result = append(result, content[:offset+len(prefix)]...)
		result = append(result, []byte("id: "+stableID+string(prefix[3:]))...)
		result = append(result, content[offset+len(prefix):]...)
		return result
	}
	result := []byte("---\nid: " + stableID + "\n---\n")
	return append(result, content[offset:]...)
}

func normalizeTakeoverStrings(mapping *yaml.Node, keys []string) {
	for _, key := range keys {
		value := mappingValue(mapping, key)
		if value == nil || value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
			continue
		}
		value.Tag = "!!str"
	}
}

func setString(mapping *yaml.Node, key string, value *string) {
	if value == nil {
		return
	}
	node := ensureValue(mapping, key)
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	node.Value = *value
	node.Content = nil
}

func setStrings(mapping *yaml.Node, key string, values *[]string) {
	if values == nil {
		return
	}
	node := ensureValue(mapping, key)
	node.Kind = yaml.SequenceNode
	node.Tag = "!!seq"
	node.Value = ""
	node.Content = nil
	for _, value := range *values {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
}

func ensureMapping(mapping *yaml.Node, key string) *yaml.Node {
	node := ensureValue(mapping, key)
	if node.Kind != yaml.MappingNode {
		node.Kind = yaml.MappingNode
		node.Tag = "!!map"
		node.Value = ""
		node.Content = nil
	}
	return node
}

func ensureValue(mapping *yaml.Node, key string) *yaml.Node {
	if value := mappingValue(mapping, key); value != nil {
		return value
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}
