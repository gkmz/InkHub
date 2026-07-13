package obsidian

import (
	"bytes"
	"fmt"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

func applyMetadataPatch(frontmatter, body string, patch contracts.MetadataPatch) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatter), &root); err != nil {
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
