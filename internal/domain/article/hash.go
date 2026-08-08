package article

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const hashSchemaVersion = 2

// HashInput 包含所有影响 MVP 渠道输出的文章内容。
type HashInput struct {
	Body        string
	Title       string
	Description string
	Tags        []string
	Keywords    []string
	URL         string
	PublishDate string
	Category    string
	Series      string
	Slug        string
	Cover       string
}

type hashEnvelope struct {
	Version     int      `json:"version"`
	Body        string   `json:"body"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Keywords    []string `json:"keywords"`
	URL         string   `json:"url"`
	PublishDate string   `json:"publish_date"`
	Category    string   `json:"category"`
	Series      string   `json:"series"`
	Slug        string   `json:"slug"`
	Cover       string   `json:"cover"`
}

// NormalizeAndHash 规范化文章并返回稳定的 SHA-256 内容版本。
func NormalizeAndHash(input HashInput) (string, error) {
	envelope := hashEnvelope{
		Version:     hashSchemaVersion,
		Body:        normalizeBody(input.Body),
		Title:       input.Title,
		Description: input.Description,
		Tags:        normalizedStrings(input.Tags),
		Keywords:    normalizedStrings(input.Keywords),
		URL:         input.URL,
		PublishDate: input.PublishDate,
		Category:    input.Category,
		Series:      input.Series,
		Slug:        input.Slug,
		Cover:       input.Cover,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("序列化内容版本: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// BodyContentHash 只计算正文内容，用于区分正文变化和 frontmatter 变化。
func BodyContentHash(body string) (string, error) {
	return NormalizeAndHash(HashInput{Body: body})
}

func normalizeBody(body string) string {
	body = strings.TrimPrefix(body, "\ufeff")
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return strings.ReplaceAll(body, "\r", "\n")
}

func normalizedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
