package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func readLimited(reader io.Reader, limit int64) ([]byte, bool, error) {
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, err
	}
	return content, int64(len(content)) > limit, nil
}

func mapNetworkError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &contracts.ProviderError{Code: "openai.timeout", Category: contracts.ErrorTemporary, Message: "AI 请求超时", Retryable: true, Cause: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &contracts.ProviderError{Code: "openai.timeout", Category: contracts.ErrorTemporary, Message: "AI 请求超时", Retryable: true, Cause: err}
	}
	return &contracts.ProviderError{Code: "openai.unavailable", Category: contracts.ErrorDependency, Message: "AI 服务不可用", Retryable: true, Cause: err}
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type metadataResponse struct {
	Description       string            `json:"description"`
	Category          string            `json:"category"`
	Series            string            `json:"series"`
	RecommendedTags   []string          `json:"recommended_tags"`
	NewTagCandidates  []string          `json:"new_tag_candidates"`
	Slug              string            `json:"slug"`
	PrimaryKeyword    string            `json:"primary_keyword"`
	SecondaryKeywords []string          `json:"secondary_keywords"`
	Reasons           map[string]string `json:"reasons"`
}

type decodedResponse struct {
	Model       string
	Suggestions []contracts.Suggestion
}

func decodeResponse(content []byte, task contracts.AITask) (decodedResponse, error) {
	var response chatResponse
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	if err := decoder.Decode(&response); err != nil || len(response.Choices) == 0 {
		return decodedResponse{}, invalidResponseError(err)
	}
	switch task {
	case contracts.AITaskXiaohongshu:
		return decodeXiaohongshuResponse(response.Choices[0].Message.Content, response.Model)
	case contracts.AITaskXiaohongshuOutline:
		return decodeXiaohongshuOutlineResponse(response.Choices[0].Message.Content, response.Model)
	case contracts.AITaskXiaohongshuRewrite:
		return decodeXiaohongshuRewriteResponse(response.Choices[0].Message.Content, response.Model)
	case contracts.AITaskXiaohongshuStoryboard:
		return decodeXiaohongshuStoryboardResponse(response.Choices[0].Message.Content, response.Model)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &fields); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	for _, required := range []string{
		"description", "category", "series", "recommended_tags", "new_tag_candidates",
		"slug", "primary_keyword", "secondary_keywords", "reasons",
	} {
		if _, exists := fields[required]; !exists {
			return decodedResponse{}, invalidResponseError(errors.New("结构化响应缺少字段: " + required))
		}
	}
	var metadata metadataResponse
	metadataDecoder := json.NewDecoder(strings.NewReader(response.Choices[0].Message.Content))
	metadataDecoder.DisallowUnknownFields()
	if err := metadataDecoder.Decode(&metadata); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	suggestions := make([]contracts.Suggestion, 0, 8)
	appendStringSuggestion := func(field, value string) {
		if value == "" {
			return
		}
		suggestions = append(suggestions, contracts.Suggestion{Field: field, Value: mustJSON(value), Rationale: metadata.Reasons[field]})
	}
	appendStringSuggestion("description", metadata.Description)
	appendStringSuggestion("category", metadata.Category)
	appendStringSuggestion("series", metadata.Series)
	appendStringSuggestion("slug", metadata.Slug)
	if len(metadata.RecommendedTags) > 0 {
		suggestions = append(suggestions, contracts.Suggestion{Field: "tags", Value: mustJSON(metadata.RecommendedTags), Rationale: metadata.Reasons["tags"]})
	}
	for _, tag := range metadata.NewTagCandidates {
		if strings.TrimSpace(tag) != "" {
			suggestions = append(suggestions, contracts.Suggestion{Field: "tags", Value: mustJSON([]string{tag}), Rationale: metadata.Reasons["tags"], NewTerm: true})
		}
	}
	keywords := append([]string(nil), metadata.SecondaryKeywords...)
	if metadata.PrimaryKeyword != "" {
		keywords = append([]string{metadata.PrimaryKeyword}, keywords...)
	}
	if len(keywords) > 0 {
		suggestions = append(suggestions, contracts.Suggestion{Field: "keywords", Value: mustJSON(keywords), Rationale: metadata.Reasons["keywords"]})
	}
	return decodedResponse{Model: response.Model, Suggestions: suggestions}, nil
}

type xiaohongshuStoryboardPageResponse struct {
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
}

// decodeXiaohongshuStoryboardResponse 校验逐页分镜和配套发布文案。
func decodeXiaohongshuStoryboardResponse(content, model string) (decodedResponse, error) {
	var output struct {
		Title       string                              `json:"title"`
		Body        string                              `json:"body"`
		Topics      string                              `json:"topics"`
		ScriptPages []xiaohongshuStoryboardPageResponse `json:"script_pages"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	if strings.TrimSpace(output.Title) == "" || strings.TrimSpace(output.Body) == "" || len(output.ScriptPages) < 1 {
		return decodedResponse{}, invalidResponseError(errors.New("小红书分镜标题、发布正文或页面不能为空"))
	}
	for _, page := range output.ScriptPages {
		if strings.TrimSpace(page.Title) == "" || strings.TrimSpace(page.Prompt) == "" {
			return decodedResponse{}, invalidResponseError(errors.New("小红书分镜页面标题或提示词不能为空"))
		}
	}
	return decodedResponse{Model: model, Suggestions: []contracts.Suggestion{
		{Field: "title", Value: mustJSON(strings.TrimSpace(output.Title))},
		{Field: "body", Value: mustJSON(strings.TrimSpace(output.Body))},
		{Field: "topics", Value: mustJSON(strings.TrimSpace(output.Topics))},
		{Field: "script_pages", Value: mustJSON(output.ScriptPages)},
	}}, nil
}

type xiaohongshuKnowledgePointResponse struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Summary        string `json:"summary"`
	SourceEvidence string `json:"source_evidence"`
}

// decodeXiaohongshuOutlineResponse 校验知识清单并保留结构化数组供传输层复核。
func decodeXiaohongshuOutlineResponse(content, model string) (decodedResponse, error) {
	var output struct {
		KnowledgePoints []xiaohongshuKnowledgePointResponse `json:"knowledge_points"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	if len(output.KnowledgePoints) == 0 {
		return decodedResponse{}, invalidResponseError(errors.New("小红书知识清单不能为空"))
	}
	return decodedResponse{Model: model, Suggestions: []contracts.Suggestion{
		{Field: "knowledge_points", Value: mustJSON(output.KnowledgePoints)},
	}}, nil
}

// decodeXiaohongshuRewriteResponse 校验小红书笔记及其知识点覆盖声明。
func decodeXiaohongshuRewriteResponse(content, model string) (decodedResponse, error) {
	var output struct {
		Title           string   `json:"title"`
		BodyHTML        string   `json:"body_html"`
		CoveredPointIDs []string `json:"covered_point_ids"`
		Topics          string   `json:"topics"`
		SourceNote      string   `json:"source_note"`
		CommentCopy     string   `json:"comment_copy"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	if strings.TrimSpace(output.Title) == "" || strings.TrimSpace(output.BodyHTML) == "" || len(output.CoveredPointIDs) == 0 {
		return decodedResponse{}, invalidResponseError(errors.New("小红书标题、正文或知识点覆盖不能为空"))
	}
	return decodedResponse{Model: model, Suggestions: []contracts.Suggestion{
		{Field: "title", Value: mustJSON(strings.TrimSpace(output.Title))},
		{Field: "body_html", Value: mustJSON(output.BodyHTML)},
		{Field: "covered_point_ids", Value: mustJSON(output.CoveredPointIDs)},
		{Field: "topics", Value: mustJSON(output.Topics)},
		{Field: "source_note", Value: mustJSON(strings.TrimSpace(output.SourceNote))},
		{Field: "comment_copy", Value: mustJSON(strings.TrimSpace(output.CommentCopy))},
	}}, nil
}

// decodeXiaohongshuResponse 校验小红书适配结果并转换为统一的结构化建议项。
func decodeXiaohongshuResponse(content, model string) (decodedResponse, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &fields); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	for _, required := range []string{"title", "body_html", "topics", "source_note", "comment_copy"} {
		if _, exists := fields[required]; !exists {
			return decodedResponse{}, invalidResponseError(errors.New("小红书结构化响应缺少字段: " + required))
		}
	}
	var output struct {
		Title       string `json:"title"`
		BodyHTML    string `json:"body_html"`
		Topics      string `json:"topics"`
		SourceNote  string `json:"source_note"`
		CommentCopy string `json:"comment_copy"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return decodedResponse{}, invalidResponseError(err)
	}
	if strings.TrimSpace(output.Title) == "" || strings.TrimSpace(output.BodyHTML) == "" {
		return decodedResponse{}, invalidResponseError(errors.New("小红书标题或正文不能为空"))
	}
	return decodedResponse{Model: model, Suggestions: []contracts.Suggestion{
		{Field: "title", Value: mustJSON(strings.TrimSpace(output.Title))},
		{Field: "body_html", Value: mustJSON(output.BodyHTML)},
		{Field: "topics", Value: mustJSON(output.Topics)},
		{Field: "source_note", Value: mustJSON(strings.TrimSpace(output.SourceNote))},
		{Field: "comment_copy", Value: mustJSON(strings.TrimSpace(output.CommentCopy))},
	}}, nil
}

func invalidResponseError(cause error) *contracts.ProviderError {
	return &contracts.ProviderError{Code: "openai.response_invalid", Category: contracts.ErrorPermanent, Message: "AI 返回了无效的结构化响应", Cause: cause}
}

func mustJSON(value any) json.RawMessage {
	content, _ := json.Marshal(value)
	return content
}
