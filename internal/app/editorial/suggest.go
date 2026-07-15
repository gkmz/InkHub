package editorial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gkmz/InkHub/internal/domain/article"
	domaineditorial "github.com/gkmz/InkHub/internal/domain/editorial"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

var (
	// ErrStaleSuggestion 表示 AI 响应或建议不再对应文章当前版本。
	ErrStaleSuggestion = errors.New("AI 建议对应的文章版本已过期")
	// ErrInvalidSuggestion 表示 Provider 返回了不符合字段契约的建议。
	ErrInvalidSuggestion = errors.New("AI 建议格式无效")
)

type (
	// SuggestionState 是 Domain 建议状态在 Application 层的别名。
	SuggestionState = domaineditorial.SuggestionState
	// SuggestionItem 是 Domain 字段建议在 Application 层的别名。
	SuggestionItem = domaineditorial.SuggestionItem
	// SuggestionSet 是 Domain 建议集合在 Application 层的别名。
	SuggestionSet = domaineditorial.SuggestionSet
)

const (
	SuggestionPending           = domaineditorial.SuggestionPending
	SuggestionPartiallyAccepted = domaineditorial.SuggestionPartiallyAccepted
	SuggestionAccepted          = domaineditorial.SuggestionAccepted
	SuggestionRejected          = domaineditorial.SuggestionRejected
	SuggestionExpired           = domaineditorial.SuggestionExpired
	SuggestionInvalid           = domaineditorial.SuggestionInvalid
)

// SuggestionStore 持久化 AI 建议及其字段级处理状态。
type SuggestionStore interface {
	Save(ctx context.Context, value SuggestionSet) error
}

// GenerateSuggestionOptions 描述一次受隐私设置约束的建议生成请求。
type GenerateSuggestionOptions struct {
	SuggestionID       string
	ProviderInstanceID string
	Article            article.Article
	Body               string
	AllowBody          bool
	TagCandidates      []TagCandidate
	Taxonomy           contracts.TaxonomyContext
}

// TagCandidate 是 Application 从 taxonomy 快照读取的可信 Tag 候选。
type TagCandidate struct {
	Name       string
	UsageCount int
}

// GenerateSuggestions 调用 AI Provider、验证结构化结果并保存待处理建议。
func GenerateSuggestions(ctx context.Context, provider contracts.AIProvider, store SuggestionStore, options GenerateSuggestionOptions) (SuggestionSet, error) {
	if provider == nil || store == nil || options.SuggestionID == "" || options.Article.ID == "" {
		return SuggestionSet{}, fmt.Errorf("生成 AI 建议所需参数不完整")
	}
	input := contracts.ArticleInput{
		Title: options.Article.Title, Description: options.Article.Description,
		Tags: append([]string(nil), options.Article.Tags...), Keywords: append([]string(nil), options.Article.Keywords...),
		Category: options.Article.Category, Series: options.Article.Series, Slug: options.Article.Slug,
	}
	if options.AllowBody {
		input.Body = options.Body
	}
	response, err := provider.Generate(ctx, contracts.AIRequest{
		Task: contracts.AITaskMetadata, Article: input, Taxonomy: options.Taxonomy,
		InputContentHash: options.Article.ContentHash, AllowBody: options.AllowBody,
	})
	if err != nil {
		return SuggestionSet{}, err
	}
	if response.InputContentHash != options.Article.ContentHash {
		return SuggestionSet{}, ErrStaleSuggestion
	}
	items, err := normalizeSuggestions(response.Suggestions, options.TagCandidates, options.SuggestionID)
	if err != nil {
		return SuggestionSet{}, err
	}
	result := SuggestionSet{
		ID: options.SuggestionID, ArticleID: options.Article.ID, WorkspaceID: options.Article.WorkspaceID,
		ProviderInstanceID: options.ProviderInstanceID, InputContentHash: options.Article.ContentHash,
		Model: response.Model, Items: items, State: SuggestionPending,
	}
	if err := store.Save(ctx, result); err != nil {
		return SuggestionSet{}, fmt.Errorf("保存 AI 建议: %w", err)
	}
	return result, nil
}

func normalizeSuggestions(input []contracts.Suggestion, candidates []TagCandidate, suggestionID string) ([]SuggestionItem, error) {
	known := make(map[string]TagCandidate, len(candidates))
	for _, candidate := range candidates {
		if key := normalizeTerm(candidate.Name); key != "" {
			known[key] = candidate
		}
	}
	items := make([]SuggestionItem, 0, len(input))
	for _, suggestion := range input {
		switch suggestion.Field {
		case "description", "category", "series", "slug":
			var value string
			if err := json.Unmarshal(suggestion.Value, &value); err != nil {
				return nil, fmt.Errorf("%w: 字段 %s 必须是字符串", ErrInvalidSuggestion, suggestion.Field)
			}
			items = append(items, newSuggestionItem(suggestionID, len(items), suggestion, mustRawJSON(value), false))
		case "keywords":
			var values []string
			if err := json.Unmarshal(suggestion.Value, &values); err != nil {
				return nil, fmt.Errorf("%w: keywords 必须是字符串数组", ErrInvalidSuggestion)
			}
			items = append(items, newSuggestionItem(suggestionID, len(items), suggestion, mustRawJSON(cleanStrings(values)), false))
		case "tags":
			var values []string
			if err := json.Unmarshal(suggestion.Value, &values); err != nil {
				return nil, fmt.Errorf("%w: tags 必须是字符串数组", ErrInvalidSuggestion)
			}
			for _, value := range cleanStrings(values) {
				candidate, exists := known[normalizeTerm(value)]
				if exists {
					value = candidate.Name
				}
				item := newSuggestionItem(suggestionID, len(items), suggestion, mustRawJSON(value), !exists)
				item.UsageCount = candidate.UsageCount
				items = append(items, item)
			}
		default:
			return nil, fmt.Errorf("%w: 不支持字段 %s", ErrInvalidSuggestion, suggestion.Field)
		}
	}
	return items, nil
}

func newSuggestionItem(setID string, index int, source contracts.Suggestion, value json.RawMessage, newTerm bool) SuggestionItem {
	return SuggestionItem{
		ID: fmt.Sprintf("%s_%d", setID, index+1), Field: source.Field, Value: value,
		Rationale: source.Rationale, Confidence: source.Confidence, NewTerm: newTerm,
	}
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := normalizeTerm(value)
		if value == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeTerm(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func mustRawJSON(value any) json.RawMessage {
	content, _ := json.Marshal(value)
	return content
}
