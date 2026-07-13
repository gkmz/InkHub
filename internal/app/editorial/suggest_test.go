package editorial

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestGenerateSuggestionsAppliesPrivacyAndMarksUnknownTags(t *testing.T) {
	t.Parallel()

	provider := &capturingAI{response: contracts.AIResponse{
		InputContentHash: "hash-v1",
		Model:            "test-model",
		Suggestions: []contracts.Suggestion{
			{Field: "description", Value: json.RawMessage(`"新摘要"`)},
			{Field: "tags", Value: json.RawMessage(`["Go","Agent"]`)},
		},
	}}
	store := &memorySuggestionStore{}
	result, err := GenerateSuggestions(context.Background(), provider, store, GenerateSuggestionOptions{
		SuggestionID:       "suggestion_1",
		ProviderInstanceID: "provider_ai",
		Article: article.Article{
			ID: "article_1", WorkspaceID: "workspace_1", Title: "标题", Description: "旧摘要", ContentHash: "hash-v1",
		},
		Body:        "不得发送的正文",
		AllowBody:   false,
		AllowedTags: []string{"Go"},
		Taxonomy:    contracts.TaxonomyContext{Tags: []string{"Go"}},
	})
	if err != nil {
		t.Fatalf("生成建议: %v", err)
	}
	if provider.request.Article.Body != "" || provider.request.AllowBody {
		t.Fatalf("隐私模式仍发送正文: %+v", provider.request)
	}
	if len(result.Items) != 3 {
		t.Fatalf("Tag 建议应拆分为可采纳字段项: %+v", result.Items)
	}
	if !findItem(result.Items, "tags", "Agent").NewTerm {
		t.Fatalf("未知 Tag 未标记为待准入: %+v", result.Items)
	}
	if store.saved.ID != "suggestion_1" || store.saved.State != SuggestionPending {
		t.Fatalf("建议未持久化: %+v", store.saved)
	}
}

func TestGenerateSuggestionsRejectsStaleProviderResponse(t *testing.T) {
	t.Parallel()

	provider := &capturingAI{response: contracts.AIResponse{InputContentHash: "old-hash"}}
	_, err := GenerateSuggestions(context.Background(), provider, &memorySuggestionStore{}, GenerateSuggestionOptions{
		SuggestionID: "suggestion_1",
		Article:      article.Article{ID: "article_1", ContentHash: "current-hash"},
	})
	if !errors.Is(err, ErrStaleSuggestion) {
		t.Fatalf("过期响应应被拒绝: %T %v", err, err)
	}
}

func TestAcceptSuggestionWritesOneFieldAndRequiresNewTagAdmission(t *testing.T) {
	t.Parallel()

	value := json.RawMessage(`"新摘要"`)
	record := SuggestionSet{
		ID: "suggestion_1", ArticleID: "article_1", InputContentHash: "hash-v1", State: SuggestionPending,
		Items: []SuggestionItem{
			{ID: "item_description", Field: "description", Value: value},
			{ID: "item_new_tag", Field: "tags", Value: json.RawMessage(`"Agent"`), NewTerm: true},
		},
	}
	writer := &capturingMetadataWriter{}
	store := &memorySuggestionStore{saved: record}
	current := article.Article{ID: "article_1", SourceID: "source_1", StableID: "article_STABLE", RelativePath: "文章.md", ContentHash: "hash-v1", FrontmatterHash: "frontmatter-v1"}

	updated, err := AcceptSuggestion(context.Background(), writer, store, staticTagGate{allowed: map[string]bool{"Go": true}}, current, record, "item_description")
	if err != nil {
		t.Fatalf("采纳摘要建议: %v", err)
	}
	if writer.command.Patch.Description == nil || *writer.command.Patch.Description != "新摘要" {
		t.Fatalf("没有生成字段级摘要 patch: %+v", writer.command.Patch)
	}
	if writer.command.Patch.Tags != nil || updated.State != SuggestionPartiallyAccepted {
		t.Fatalf("采纳单字段不应改写其他字段: patch=%+v state=%s", writer.command.Patch, updated.State)
	}

	_, err = AcceptSuggestion(context.Background(), writer, store, staticTagGate{allowed: map[string]bool{"Go": true}}, current, record, "item_new_tag")
	if !errors.Is(err, ErrTaxonomyAdmissionRequired) {
		t.Fatalf("未知 Tag 未准入时应拒绝写回: %T %v", err, err)
	}
}

func TestAcceptSuggestionRejectsChangedArticle(t *testing.T) {
	t.Parallel()

	record := SuggestionSet{
		ID: "suggestion_1", ArticleID: "article_1", InputContentHash: "old-hash", State: SuggestionPending,
		Items: []SuggestionItem{{ID: "item", Field: "slug", Value: json.RawMessage(`"new-slug"`)}},
	}
	_, err := AcceptSuggestion(context.Background(), &capturingMetadataWriter{}, &memorySuggestionStore{}, staticTagGate{}, article.Article{
		ID: "article_1", ContentHash: "new-hash",
	}, record, "item")
	if !errors.Is(err, ErrStaleSuggestion) {
		t.Fatalf("文章变化后仍采纳了旧建议: %T %v", err, err)
	}
}

type capturingAI struct {
	request  contracts.AIRequest
	response contracts.AIResponse
}

func (p *capturingAI) Descriptor() contracts.AIDescriptor { return contracts.AIDescriptor{} }
func (p *capturingAI) Validate(context.Context) error     { return nil }
func (p *capturingAI) Generate(_ context.Context, request contracts.AIRequest) (contracts.AIResponse, error) {
	p.request = request
	return p.response, nil
}

type memorySuggestionStore struct {
	saved SuggestionSet
}

func (s *memorySuggestionStore) Save(_ context.Context, value SuggestionSet) error {
	s.saved = value
	return nil
}

type capturingMetadataWriter struct {
	command contracts.MetadataWriteCommand
}

func (w *capturingMetadataWriter) WriteMetadata(_ context.Context, command contracts.MetadataWriteCommand) (contracts.SourceDocument, error) {
	w.command = command
	return contracts.SourceDocument{}, nil
}

type staticTagGate struct {
	allowed map[string]bool
}

func (g staticTagGate) IsTagAllowed(_ context.Context, tag string) (bool, error) {
	return g.allowed[tag], nil
}

func findItem(items []SuggestionItem, field, stringValue string) SuggestionItem {
	for _, item := range items {
		var value string
		if item.Field == field && json.Unmarshal(item.Value, &value) == nil && value == stringValue {
			return item
		}
	}
	return SuggestionItem{}
}
