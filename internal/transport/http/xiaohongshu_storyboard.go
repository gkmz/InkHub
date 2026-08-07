package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/domain/xiaohongshu"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

const xiaohongshuStoryboardOutputSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["title","body","topics","script_pages"],
  "properties":{
    "title":{"type":"string","description":"适合图片短文发布的标题，20字以内"},
    "body":{"type":"string","description":"补充整组图片的短正文，不逐页复述"},
    "topics":{"type":"string","description":"可直接复制的话题标签，例如 #AI编程 #效率工具"},
    "script_pages":{
      "type":"array","minItems":5,"maxItems":8,
      "items":{
        "type":"object","additionalProperties":false,
        "required":["title","prompt"],
        "properties":{
          "title":{"type":"string","description":"该页分镜名称"},
          "prompt":{"type":"string","description":"可独立复制给图片生成模型的完整中文提示词，包含画面中必须准确出现的文字"}
        }
      }
    }
  }
}`

type xiaohongshuStoryboardGenerated struct {
	Title       string
	Body        string
	Topics      []string
	ScriptPages []xiaohongshu.ScriptPage
}

// xiaohongshuStoryboard 一次生成完整分镜脚本和配套发布文案。
func (h *runtimeHandler) xiaohongshuStoryboard(response http.ResponseWriter, request *http.Request, articleID string) {
	var input struct{}
	if decodeJSON(request, &input) != nil {
		writeError(response, http.StatusBadRequest, "request.invalid", "小红书分镜生成请求无效")
		return
	}
	workspaceID, contentHash, title, rendered, err := h.xiaohongshuArticle(request.Context(), articleID)
	if err != nil {
		mapError(response, err)
		return
	}
	provider, err := h.buildXiaohongshuAIProvider(request.Context(), workspaceID)
	if err != nil {
		mapError(response, err)
		return
	}
	generated, model, err := generateXiaohongshuStoryboard(request.Context(), provider, title, rendered, contentHash)
	if err != nil {
		mapError(response, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	draft := xiaohongshu.Draft{
		ID:        stableXiaohongshuID("draft", articleID, contentHash, string(xiaohongshu.DraftModeVisualScript), now),
		ArticleID: articleID, WorkspaceID: workspaceID, SourceContentHash: contentHash,
		Mode: xiaohongshu.DraftModeVisualScript, Title: generated.Title, BodyHTML: generated.Body,
		ScriptPages: generated.ScriptPages, Topics: generated.Topics, AIModel: model,
		PromptVersion: "xiaohongshu-storyboard-v1", State: xiaohongshu.DraftStateDraft,
		CreatedAt: now, UpdatedAt: now,
	}
	repo := repository.NewXiaohongshuRepository(h.db)
	if err := repo.SaveDraft(request.Context(), draft); err != nil {
		mapError(response, err)
		return
	}
	payload, _ := json.Marshal(map[string]any{"source": "ai", "model": model, "prompt_version": draft.PromptVersion, "page_count": len(draft.ScriptPages), "mode": draft.Mode})
	_ = repo.SaveEvent(request.Context(), xiaohongshu.Event{ID: stableXiaohongshuID("event", draft.ID, "generated"), DraftID: draft.ID, EventType: "generated", Payload: string(payload)})
	writeJSON(response, http.StatusCreated, xiaohongshuDraftView(draft, contentHash))
}

func generateXiaohongshuStoryboard(ctx context.Context, provider contracts.AIProvider, title, body, contentHash string) (xiaohongshuStoryboardGenerated, string, error) {
	result, err := provider.Generate(ctx, contracts.AIRequest{
		Task:             contracts.AITaskXiaohongshuStoryboard,
		Article:          contracts.ArticleInput{Title: title, Body: body},
		OutputSchema:     xiaohongshuStoryboardOutputSchema,
		InputContentHash: contentHash,
		AllowBody:        true,
	})
	if err != nil {
		return xiaohongshuStoryboardGenerated{}, "", err
	}
	if result.InputContentHash != contentHash {
		return xiaohongshuStoryboardGenerated{}, "", ErrStaleContent
	}
	generated, err := parseXiaohongshuStoryboardOutput(result.Suggestions)
	return generated, result.Model, err
}

func parseXiaohongshuStoryboardOutput(items []contracts.Suggestion) (xiaohongshuStoryboardGenerated, error) {
	values := make(map[string]json.RawMessage, len(items))
	for _, item := range items {
		values[item.Field] = item.Value
	}
	var generated xiaohongshuStoryboardGenerated
	var topics string
	if json.Unmarshal(values["title"], &generated.Title) != nil || strings.TrimSpace(generated.Title) == "" {
		return xiaohongshuStoryboardGenerated{}, errors.New("AI 分镜标题无效")
	}
	if json.Unmarshal(values["body"], &generated.Body) != nil || strings.TrimSpace(generated.Body) == "" {
		return xiaohongshuStoryboardGenerated{}, errors.New("AI 分镜发布正文无效")
	}
	if json.Unmarshal(values["topics"], &topics) != nil || json.Unmarshal(values["script_pages"], &generated.ScriptPages) != nil {
		return xiaohongshuStoryboardGenerated{}, errors.New("AI 分镜结构无效")
	}
	if len(generated.ScriptPages) < 5 || len(generated.ScriptPages) > 8 {
		return xiaohongshuStoryboardGenerated{}, errors.New("AI 分镜必须包含 5 至 8 页")
	}
	for index := range generated.ScriptPages {
		page := &generated.ScriptPages[index]
		page.ID = stableXiaohongshuID("script", strconv.Itoa(index+1), page.Title, page.Prompt)
		page.Title, page.Prompt = strings.TrimSpace(page.Title), strings.TrimSpace(page.Prompt)
		if page.Title == "" || page.Prompt == "" {
			return xiaohongshuStoryboardGenerated{}, errors.New("AI 分镜页面不完整")
		}
	}
	generated.Title, generated.Body, generated.Topics = strings.TrimSpace(generated.Title), strings.TrimSpace(generated.Body), parseXiaohongshuTopics(topics)
	return generated, nil
}
