package httptransport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/domain/xiaohongshu"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
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

type xiaohongshuAIRewriteResult struct {
	Generated       xiaohongshuAIGenerated
	CoveredPointIDs []string
}

type xiaohongshuRewriteModelInput struct {
	SourceHTML      string                      `json:"source_html"`
	KnowledgePoints []XiaohongshuKnowledgePoint `json:"knowledge_points"`
}

// xiaohongshuOutline 提取当前文章知识点，不创建或覆盖草稿。
func (h *runtimeHandler) xiaohongshuOutline(response http.ResponseWriter, request *http.Request, articleID string) {
	var input struct{}
	if decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书知识提取请求无效")
		return
	}
	workspaceID, contentHash, title, rendered, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	source, err := prepareXiaohongshuRewriteSource(rendered)
	if err != nil {
		mapError(response, err)
		return
	}
	provider, err := h.buildXiaohongshuAIProvider(request.Context(), workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	points, _, err := generateXiaohongshuOutline(request.Context(), provider, title, contentHash, source)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, XiaohongshuOutlineView{ContentHash: contentHash, KnowledgePoints: points})
}

// xiaohongshuRewrite 使用已确认的知识清单改写并保存新的小红书草稿版本。
func (h *runtimeHandler) xiaohongshuRewrite(response http.ResponseWriter, request *http.Request, articleID string) {
	var input xiaohongshuRewriteInput
	if decodeJSON(request, &input) != nil || strings.TrimSpace(input.ContentHash) == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书改写请求无效")
		return
	}
	if err := validateXiaohongshuKnowledgePoints(input.KnowledgePoints); err != nil {
		mapError(response, err)
		return
	}
	workspaceID, contentHash, title, rendered, _, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	if input.ContentHash != contentHash {
		mapError(response, ErrStaleContent)
		return
	}
	source, err := prepareXiaohongshuRewriteSource(rendered)
	if err != nil {
		mapError(response, err)
		return
	}
	provider, err := h.buildXiaohongshuAIProvider(request.Context(), workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	result, model, err := generateXiaohongshuRewrite(request.Context(), provider, title, contentHash, source, input.KnowledgePoints)
	if err != nil {
		mapError(response, err)
		return
	}
	draft, err := h.saveXiaohongshuAIDraft(request.Context(), articleID, workspaceID, contentHash, model, result, len(input.KnowledgePoints), len(source.Media))
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, xiaohongshuDraftView(draft, contentHash))
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
	topLevelTokens, err := collectTopLevelXiaohongshuMediaTokens(value)
	if err != nil {
		return "", err
	}
	if len(topLevelTokens) != len(found) {
		return "", newXiaohongshuAIResponseError("AI 未将原文素材标记放在独立内容块中")
	}
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
	if strings.Contains(result, xiaohongshuMediaTokenPrefix) {
		return "", newXiaohongshuAIResponseError("AI 改写残留了无效素材标记")
	}
	return result, nil
}

func collectTopLevelXiaohongshuMediaTokens(value string) ([]string, error) {
	context := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(value), context)
	if err != nil {
		return nil, newXiaohongshuAIResponseError("AI 小红书正文 HTML 无效")
	}
	tokens := make([]string, 0)
	for _, node := range nodes {
		if node.Type != html.TextNode {
			continue
		}
		value := strings.TrimSpace(node.Data)
		if value != "" && xiaohongshuMediaTokenPattern.MatchString(value) && xiaohongshuMediaTokenPattern.FindString(value) == value {
			tokens = append(tokens, value)
		}
	}
	return tokens, nil
}

// buildXiaohongshuAIProvider 从当前工作区配置构建可执行的 AI Provider。
func (h *runtimeHandler) buildXiaohongshuAIProvider(ctx context.Context, workspaceID string) (contracts.AIProvider, error) {
	if h.providerRuntime == nil {
		return nil, &contracts.ProviderError{Code: "ai.not_configured", Category: contracts.ErrorConflict, Message: "尚未配置 AI Provider，无法改写小红书笔记"}
	}
	var providerID, rawConfig string
	err := h.db.QueryRowContext(ctx, `SELECT id,config_json FROM provider_instances WHERE workspace_id=? AND provider_type='openai-compatible' AND enabled=1`, workspaceID).Scan(&providerID, &rawConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &contracts.ProviderError{Code: "ai.not_configured", Category: contracts.ErrorConflict, Message: "尚未配置 AI Provider，无法改写小红书笔记"}
	}
	if err != nil {
		return nil, err
	}
	var stored storedAIConfig
	if json.Unmarshal([]byte(rawConfig), &stored) != nil {
		return nil, &contracts.ProviderError{Code: "ai.config_invalid", Category: contracts.ErrorInternal, Message: "AI Provider 配置损坏"}
	}
	providerConfig, err := json.Marshal(map[string]any{"base_url": stored.BaseURL, "model": stored.Model, "timeout": stored.Timeout})
	if err != nil {
		return nil, fmt.Errorf("编码 AI Provider 配置: %w", err)
	}
	return h.providerRuntime.BuildAI(ctx,
		contracts.ProviderRef{ID: providerID, Type: contracts.ProviderOpenAI},
		contracts.ConfigView{Data: providerConfig, SecretRefs: map[string]string{"api_key": stored.SecretRef}},
	)
}

// generateXiaohongshuOutline 调用 AI 提取经过校验的原文知识清单。
func generateXiaohongshuOutline(ctx context.Context, provider contracts.AIProvider, title, contentHash string, source xiaohongshuRewriteSource) ([]XiaohongshuKnowledgePoint, string, error) {
	response, err := provider.Generate(ctx, contracts.AIRequest{
		Task:             contracts.AITaskXiaohongshuOutline,
		Article:          contracts.ArticleInput{Title: title, Body: source.HTML},
		OutputSchema:     xiaohongshuOutlineOutputSchema,
		InputContentHash: contentHash,
		AllowBody:        true,
	})
	if err != nil {
		return nil, "", err
	}
	if response.InputContentHash != contentHash {
		return nil, "", ErrStaleContent
	}
	points, err := parseXiaohongshuKnowledgePoints(response.Suggestions)
	return points, response.Model, err
}

// generateXiaohongshuRewrite 调用 AI 改写笔记并恢复全部锁定素材。
func generateXiaohongshuRewrite(ctx context.Context, provider contracts.AIProvider, title, contentHash string, source xiaohongshuRewriteSource, points []XiaohongshuKnowledgePoint) (xiaohongshuAIRewriteResult, string, error) {
	if err := validateXiaohongshuKnowledgePoints(points); err != nil {
		return xiaohongshuAIRewriteResult{}, "", err
	}
	modelInput, err := json.Marshal(xiaohongshuRewriteModelInput{SourceHTML: source.HTML, KnowledgePoints: points})
	if err != nil {
		return xiaohongshuAIRewriteResult{}, "", fmt.Errorf("编码小红书改写输入: %w", err)
	}
	response, err := provider.Generate(ctx, contracts.AIRequest{
		Task:             contracts.AITaskXiaohongshuRewrite,
		Article:          contracts.ArticleInput{Title: title, Body: string(modelInput)},
		OutputSchema:     xiaohongshuRewriteOutputSchema,
		InputContentHash: contentHash,
		AllowBody:        true,
	})
	if err != nil {
		return xiaohongshuAIRewriteResult{}, "", err
	}
	if response.InputContentHash != contentHash {
		return xiaohongshuAIRewriteResult{}, "", ErrStaleContent
	}
	result, err := parseXiaohongshuRewriteOutput(response.Suggestions)
	if err != nil {
		return xiaohongshuAIRewriteResult{}, "", err
	}
	if err := validateXiaohongshuCoverage(points, result.CoveredPointIDs); err != nil {
		return xiaohongshuAIRewriteResult{}, "", err
	}
	restored, err := restoreXiaohongshuMedia(result.Generated.BodyHTML, source.Media)
	if err != nil {
		return xiaohongshuAIRewriteResult{}, "", err
	}
	result.Generated.BodyHTML = restored
	return result, response.Model, nil
}

func parseXiaohongshuRewriteOutput(items []contracts.Suggestion) (xiaohongshuAIRewriteResult, error) {
	values := make(map[string]json.RawMessage, len(items))
	for _, item := range items {
		values[item.Field] = item.Value
	}
	generated, err := parseXiaohongshuAIOutput(items)
	if err != nil {
		return xiaohongshuAIRewriteResult{}, newXiaohongshuAIResponseError(err.Error())
	}
	var coveredIDs []string
	if err := json.Unmarshal(values["covered_point_ids"], &coveredIDs); err != nil || len(coveredIDs) == 0 {
		return xiaohongshuAIRewriteResult{}, newXiaohongshuAIResponseError("AI 知识点覆盖格式无效")
	}
	return xiaohongshuAIRewriteResult{Generated: generated, CoveredPointIDs: coveredIDs}, nil
}

// saveXiaohongshuAIDraft 在全部生成校验完成后持久化新的草稿版本。
func (h *runtimeHandler) saveXiaohongshuAIDraft(ctx context.Context, articleID, workspaceID, contentHash, model string, result xiaohongshuAIRewriteResult, knowledgePointCount, mediaCount int) (xiaohongshu.Draft, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	generated := result.Generated
	draft := xiaohongshu.Draft{
		ID:                stableXiaohongshuID("draft", articleID, contentHash, now),
		ArticleID:         articleID,
		WorkspaceID:       workspaceID,
		SourceContentHash: contentHash,
		Mode:              xiaohongshu.DraftModeLongCard,
		Title:             generated.Title,
		BodyHTML:          generated.BodyHTML,
		Topics:            generated.Topics,
		SourceNote:        generated.SourceNote,
		CommentCopy:       generated.CommentCopy,
		AIModel:           model,
		PromptVersion:     "xiaohongshu-v3",
		State:             xiaohongshu.DraftStateDraft,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	repo := repository.NewXiaohongshuRepository(h.db)
	if err := repo.SaveDraft(ctx, draft); err != nil {
		return xiaohongshu.Draft{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"source": "ai", "model": model, "prompt_version": "xiaohongshu-v3",
		"knowledge_point_count": knowledgePointCount, "media_count": mediaCount,
	})
	_ = repo.SaveEvent(ctx, xiaohongshu.Event{
		ID: stableXiaohongshuID("event", draft.ID, "generated"), DraftID: draft.ID,
		EventType: "generated", Payload: string(payload),
	})
	return draft, nil
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
