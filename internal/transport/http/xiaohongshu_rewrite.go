package httptransport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const xiaohongshuMediaTokenPrefix = "{{INKHUB_MEDIA:"

var xiaohongshuMediaTokenPattern = regexp.MustCompile(`\{\{INKHUB_MEDIA:[^}]+\}\}`)

const xiaohongshuOutlineOutputSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["knowledge_points"],
  "properties":{
    "knowledge_points":{
      "type":"array",
      "minItems":1,
      "items":{
        "type":"object",
        "additionalProperties":false,
        "required":["id","kind","summary","source_evidence"],
        "properties":{
          "id":{"type":"string","pattern":"^kp-[1-9][0-9]*$"},
          "kind":{"type":"string","enum":["claim","fact","step","warning","example","conclusion"]},
          "summary":{"type":"string","minLength":1},
          "source_evidence":{"type":"string","minLength":1}
        }
      }
    }
  }
}`

const xiaohongshuRewriteOutputSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["title","body_html","covered_point_ids","topics","source_note","comment_copy"],
  "properties":{
    "title":{"type":"string","description":"忠于原文且适合小红书的标题，20字以内"},
	    "body_html":{"type":"string","description":"结构清晰的中文笔记 HTML，可使用 h2、h3、p、strong、em、code、blockquote、ul、ol、li、a；全部 INKHUB_MEDIA 标记必须作为标签之外的独立内容块原样保留，不要 h1"},
    "covered_point_ids":{"type":"array","minItems":1,"items":{"type":"string","pattern":"^kp-[1-9][0-9]*$"}},
    "topics":{"type":"string","description":"可直接复制的小红书话题，例如 #AI编程 #效率工具，话题内部不能有空格"},
    "source_note":{"type":"string"},
    "comment_copy":{"type":"string"}
  }
}`

// XiaohongshuKnowledgePoint 是从原文中提取且必须在最终笔记覆盖的知识项。
type XiaohongshuKnowledgePoint struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Summary        string `json:"summary"`
	SourceEvidence string `json:"source_evidence"`
}

// XiaohongshuOutlineView 返回当前文章版本对应的知识清单。
type XiaohongshuOutlineView struct {
	ContentHash     string                      `json:"content_hash"`
	KnowledgePoints []XiaohongshuKnowledgePoint `json:"knowledge_points"`
}

type xiaohongshuRewriteInput struct {
	ContentHash     string                      `json:"content_hash"`
	KnowledgePoints []XiaohongshuKnowledgePoint `json:"knowledge_points"`
}

type xiaohongshuLockedMedia struct {
	Token string
	HTML  string
}

type xiaohongshuRewriteSource struct {
	HTML  string
	Media []xiaohongshuLockedMedia
}

// prepareXiaohongshuRewriteSource 将不可由 AI 修改的素材块替换为稳定标记。
func prepareXiaohongshuRewriteSource(value string) (xiaohongshuRewriteSource, error) {
	if strings.Contains(value, xiaohongshuMediaTokenPrefix) {
		return xiaohongshuRewriteSource{}, newXiaohongshuAIResponseError("原文包含保留的素材标记格式")
	}
	context := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(value), context)
	if err != nil {
		return xiaohongshuRewriteSource{}, fmt.Errorf("解析小红书原文: %w", err)
	}
	var output bytes.Buffer
	media := make([]xiaohongshuLockedMedia, 0)
	for _, node := range nodes {
		if !containsXiaohongshuLockedMedia(node) {
			if err := html.Render(&output, node); err != nil {
				return xiaohongshuRewriteSource{}, fmt.Errorf("序列化小红书原文: %w", err)
			}
			continue
		}
		var locked bytes.Buffer
		if err := html.Render(&locked, node); err != nil {
			return xiaohongshuRewriteSource{}, fmt.Errorf("序列化小红书素材: %w", err)
		}
		token := fmt.Sprintf("{{INKHUB_MEDIA:media-%d}}", len(media)+1)
		media = append(media, xiaohongshuLockedMedia{Token: token, HTML: locked.String()})
		output.WriteString(token)
	}
	return xiaohongshuRewriteSource{HTML: output.String(), Media: media}, nil
}

func containsXiaohongshuLockedMedia(node *html.Node) bool {
	locked := false
	walkXiaohongshuNodes(node, func(current *html.Node) {
		if locked || current.Type != html.ElementNode {
			return
		}
		switch current.Data {
		case "img", "table":
			locked = true
		case "pre":
			locked = containsXiaohongshuMermaidCode(current)
		}
	})
	return locked
}

func containsXiaohongshuMermaidCode(node *html.Node) bool {
	contains := false
	walkXiaohongshuNodes(node, func(current *html.Node) {
		if contains || current.Type != html.ElementNode || current.Data != "code" {
			return
		}
		for _, className := range strings.Fields(htmlAttribute(current, "class")) {
			if className == "language-mermaid" || className == "lang-mermaid" {
				contains = true
				return
			}
		}
	})
	return contains
}

// validateXiaohongshuKnowledgePoints 校验知识清单顺序、类型和必需内容。
func validateXiaohongshuKnowledgePoints(points []XiaohongshuKnowledgePoint) error {
	if len(points) == 0 {
		return newXiaohongshuAIResponseError("AI 未返回原文知识点")
	}
	validKinds := map[string]struct{}{
		"claim": {}, "fact": {}, "step": {}, "warning": {}, "example": {}, "conclusion": {},
	}
	for index, point := range points {
		expectedID := fmt.Sprintf("kp-%d", index+1)
		if strings.TrimSpace(point.ID) != expectedID {
			return newXiaohongshuAIResponseError("AI 知识点编号不连续")
		}
		if _, ok := validKinds[strings.TrimSpace(point.Kind)]; !ok {
			return newXiaohongshuAIResponseError("AI 知识点类型无效")
		}
		if strings.TrimSpace(point.Summary) == "" || strings.TrimSpace(point.SourceEvidence) == "" {
			return newXiaohongshuAIResponseError("AI 知识点内容不完整")
		}
	}
	return nil
}

// validateXiaohongshuCoverage 要求最终笔记声明覆盖全部且仅覆盖已提取知识点。
func validateXiaohongshuCoverage(points []XiaohongshuKnowledgePoint, coveredIDs []string) error {
	if err := validateXiaohongshuKnowledgePoints(points); err != nil {
		return err
	}
	if len(coveredIDs) != len(points) {
		return newXiaohongshuAIResponseError("AI 改写遗漏了原文知识点")
	}
	covered := make(map[string]struct{}, len(coveredIDs))
	for _, id := range coveredIDs {
		id = strings.TrimSpace(id)
		if _, exists := covered[id]; exists {
			return newXiaohongshuAIResponseError("AI 重复声明知识点覆盖")
		}
		covered[id] = struct{}{}
	}
	for _, point := range points {
		if _, exists := covered[point.ID]; !exists {
			return newXiaohongshuAIResponseError("AI 改写遗漏了原文知识点 " + point.ID)
		}
	}
	return nil
}

// restoreXiaohongshuMedia 校验素材标记后恢复模型不可修改的原始 HTML。
func restoreXiaohongshuMedia(value string, media []xiaohongshuLockedMedia) (string, error) {
	found := xiaohongshuMediaTokenPattern.FindAllString(value, -1)
	counts := make(map[string]int, len(found))
	for _, token := range found {
		counts[token]++
	}
	if len(found) != len(media) {
		return "", newXiaohongshuAIResponseError("AI 改写遗漏或重复了原文素材")
	}
	expected := make(map[string]xiaohongshuLockedMedia, len(media))
	for _, item := range media {
		expected[item.Token] = item
		if counts[item.Token] != 1 {
			return "", newXiaohongshuAIResponseError("AI 改写遗漏或重复了原文素材 " + item.Token)
		}
	}
	for token := range counts {
		if _, exists := expected[token]; !exists {
			return "", newXiaohongshuAIResponseError("AI 改写返回了未知素材标记")
		}
	}
	result := value
	for _, item := range media {
		result = strings.ReplaceAll(result, item.Token, item.HTML)
	}
	return result, nil
}

func parseXiaohongshuKnowledgePoints(items []contracts.Suggestion) ([]XiaohongshuKnowledgePoint, error) {
	for _, item := range items {
		if item.Field != "knowledge_points" {
			continue
		}
		var points []XiaohongshuKnowledgePoint
		if err := json.Unmarshal(item.Value, &points); err != nil {
			return nil, newXiaohongshuAIResponseError("AI 知识清单格式无效")
		}
		if err := validateXiaohongshuKnowledgePoints(points); err != nil {
			return nil, err
		}
		return points, nil
	}
	return nil, newXiaohongshuAIResponseError("AI 未返回知识清单")
}

func newXiaohongshuAIResponseError(message string) *contracts.ProviderError {
	return &contracts.ProviderError{
		Code:     "openai.response_invalid",
		Category: contracts.ErrorPermanent,
		Message:  message,
	}
}
