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

func decodeResponse(content []byte) (decodedResponse, error) {
	var response chatResponse
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	if err := decoder.Decode(&response); err != nil || len(response.Choices) == 0 {
		return decodedResponse{}, invalidResponseError(err)
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

func invalidResponseError(cause error) *contracts.ProviderError {
	return &contracts.ProviderError{Code: "openai.response_invalid", Category: contracts.ErrorPermanent, Message: "AI 返回了无效的结构化响应", Cause: cause}
}

func mustJSON(value any) json.RawMessage {
	content, _ := json.Marshal(value)
	return content
}
