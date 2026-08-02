package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestArticleSuggestionsUsesPersistedTagCounts(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01'); INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,description,tags_json,keywords_json,content_hash,frontmatter_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','article_ONE','one.md','Go 服务','摘要','[]','[]','hash-v1','front','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at) VALUES('ai1','w1','openai-compatible','AI',1,'{"base_url":"https://example.com/v1","model":"test","timeout":30000000000,"secret_ref":"ai-key"}','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',1,'2026-01-01','2026-01-01'); INSERT INTO taxonomy_terms(id,workspace_id,provider_instance_id,kind,external_key,name,canonical_name,usage_count,source_revision,updated_at) VALUES('t1','w1','h1','tag','go','Go','Go',18,'r1','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	provider := &suggestionAI{response: contracts.AIResponse{InputContentHash: "hash-v1", Model: "test", Suggestions: []contracts.Suggestion{{Field: "tags", Value: json.RawMessage(`["go","Agent"]`), Rationale: "主题匹配"}}}}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: suggestionRuntime{ai: provider}})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/suggestions", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"Go"`) || !strings.Contains(response.Body.String(), `"usage_count":18`) || !strings.Contains(response.Body.String(), `"new_term":true`) {
		t.Fatalf("生成建议响应错误: %d %s", response.Code, response.Body.String())
	}
	if len(provider.request.Taxonomy.Tags) != 1 || provider.request.Taxonomy.Tags[0] != "Go" || provider.request.Article.Body != "" {
		t.Fatalf("AI 请求候选或隐私边界错误: %+v", provider.request)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ai_suggestions WHERE article_id='a1'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("建议未持久化: count=%d err=%v", count, err)
	}
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w2','当前工作区','/tmp','2026-02-01','2026-02-01','2026-02-01')`)
	if err != nil {
		t.Fatal(err)
	}
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, request.Clone(context.Background()))
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("旧工作区文章仍可生成建议: %d %s", blocked.Code, blocked.Body.String())
	}
}

func TestArticleSuggestionsKeepHistoryAcrossGenerations(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01'); INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,description,tags_json,keywords_json,content_hash,frontmatter_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','article_ONE','one.md','Go 服务','摘要','[]','[]','hash-v1','front','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at) VALUES('ai1','w1','openai-compatible','AI',1,'{"base_url":"https://example.com/v1","model":"test","timeout":30000000000,"secret_ref":"ai-key"}','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',1,'2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	provider := &suggestionAI{responses: []contracts.AIResponse{
		{InputContentHash: "hash-v1", Model: "test", Suggestions: []contracts.Suggestion{{Field: "tags", Value: json.RawMessage(`["first"]`)}}},
		{InputContentHash: "hash-v1", Model: "test", Suggestions: []contracts.Suggestion{{Field: "tags", Value: json.RawMessage(`["second"]`)}}},
	}}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: suggestionRuntime{ai: provider}})
	for index := 0; index < 2; index++ {
		request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/suggestions", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://localhost")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("第 %d 次生成失败: %d %s", index+1, response.Code, response.Body.String())
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ai_suggestions WHERE article_id='a1'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("建议历史记录数 = %d, err=%v", count, err)
	}
}

func TestSuggestionHistoryAndDetailExposeTypedValues(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01'); INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,description,tags_json,keywords_json,content_hash,frontmatter_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','article_ONE','one.md','Go 服务','摘要','[]','[]','hash-v1','front','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,enabled,config_json,created_at,updated_at) VALUES('ai1','w1','openai-compatible','AI',1,'{"base_url":"https://example.com/v1","model":"test","timeout":30000000000,"secret_ref":"ai-key"}','2026-01-01','2026-01-01'); INSERT INTO ai_suggestions(id,article_id,input_content_hash,provider_instance_id,workspace_id,suggestion_json,state,created_at,updated_at) VALUES('suggestion_typed','a1','hash-v1','ai1','w1','{"model":"test-model","items":[{"id":"description_1","field":"description","value":"新的描述","rationale":"主题匹配"},{"id":"keywords_1","field":"keywords","value":["go","ai"],"rationale":"关键词"},{"id":"tag_1","field":"tags","value":"Go","usage_count":18}]}','pending','2026-01-02','2026-01-02')`)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	history := httptest.NewRecorder()
	handler.ServeHTTP(history, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1/suggestions?limit=1", nil))
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"suggestion_count":3`) {
		t.Fatalf("建议历史响应错误: %d %s", history.Code, history.Body.String())
	}
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1/suggestions/suggestion_typed", nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"value":["go","ai"]`) || !strings.Contains(detail.Body.String(), `"name":"Go"`) {
		t.Fatalf("建议详情类型化值错误: %d %s", detail.Code, detail.Body.String())
	}
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1/suggestions/unknown", nil))
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("不存在的建议版本状态码 = %d, body=%s", blocked.Code, blocked.Body.String())
	}
}

type suggestionAI struct {
	request   contracts.AIRequest
	response  contracts.AIResponse
	responses []contracts.AIResponse
}

func (p *suggestionAI) Descriptor() contracts.AIDescriptor { return contracts.AIDescriptor{} }
func (p *suggestionAI) Validate(context.Context) error     { return nil }
func (p *suggestionAI) Generate(_ context.Context, request contracts.AIRequest) (contracts.AIResponse, error) {
	p.request = request
	if len(p.responses) > 0 {
		response := p.responses[0]
		p.responses = p.responses[1:]
		return response, nil
	}
	return p.response, nil
}

type suggestionRuntime struct{ ai contracts.AIProvider }

func (r suggestionRuntime) SupportsTaxonomy(contracts.ProviderType) bool { return false }
func (r suggestionRuntime) BuildAI(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.AIProvider, error) {
	return r.ai, nil
}
func (r suggestionRuntime) BuildSource(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.SourceProvider, error) {
	return nil, nil
}
func (r suggestionRuntime) BuildPublish(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.PublishProvider, error) {
	return nil, nil
}
func (r suggestionRuntime) BuildTaxonomy(context.Context, contracts.ProviderRef, contracts.ConfigView) (contracts.TaxonomyProvider, error) {
	return nil, nil
}
