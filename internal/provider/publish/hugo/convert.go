package hugo

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"gopkg.in/yaml.v3"
)

type hugoFrontmatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description,omitempty"`
	Date        string   `yaml:"date,omitempty"`
	Updated     string   `yaml:"updated,omitempty"`
	URL         string   `yaml:"url,omitempty"`
	Slug        string   `yaml:"slug,omitempty"`
	Categories  []string `yaml:"categories,omitempty"`
	Series      []string `yaml:"series,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	Keywords    []string `yaml:"keywords,omitempty"`
	Cover       string   `yaml:"cover,omitempty"`
	SourceID    string   `yaml:"source_id"`
	SourcePath  string   `yaml:"source_path,omitempty"`
}

var (
	calloutPattern       = regexp.MustCompile(`(?m)^> \[!([A-Za-z]+)\](?:[ \t]+([^\r\n]+))?[ \t]*$`)
	obsidianImagePattern = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)
)

func convertArticle(input contracts.PublishInput) ([]byte, error) {
	if input.Article.StableID == "" || input.Article.Title == "" {
		return nil, fmt.Errorf("Hugo 文章缺少稳定 ID 或标题")
	}
	frontmatter := hugoFrontmatter{
		Title: input.Article.Title, Description: input.Article.Description, Date: input.Article.PublishDate, Updated: input.Article.PublishDate,
		URL: input.Article.URL, Slug: input.Article.Slug,
		Tags: input.Article.Tags, Keywords: input.Article.Keywords, Cover: input.Article.Cover,
		SourceID: string(input.Article.StableID), SourcePath: input.Article.RelativePath,
	}
	if input.Article.Category != "" {
		frontmatter.Categories = []string{input.Article.Category}
	}
	if input.Article.Series != "" {
		frontmatter.Series = []string{input.Article.Series}
	}
	encoded, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("生成 Hugo frontmatter: %w", err)
	}
	body := convertObsidianSyntax(input.Body)
	var result bytes.Buffer
	result.WriteString("---\n")
	result.Write(encoded)
	result.WriteString("---\n\n")
	result.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		result.WriteByte('\n')
	}
	return result.Bytes(), nil
}

func convertObsidianSyntax(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = obsidianImagePattern.ReplaceAllStringFunc(body, func(value string) string {
		matches := obsidianImagePattern.FindStringSubmatch(value)
		return "![](" + filepath.Base(strings.TrimSpace(matches[1])) + ")"
	})
	body = calloutPattern.ReplaceAllStringFunc(body, func(value string) string {
		matches := calloutPattern.FindStringSubmatch(value)
		label := strings.TrimSpace(matches[2])
		if label == "" {
			label = strings.ToUpper(matches[1])
		}
		return "> **" + label + "**"
	})
	return body
}
