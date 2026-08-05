package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestProviderGeneratesStructuredSuggestionsWithoutSendingBodyInPrivacyMode(t *testing.T) {
	t.Parallel()

	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedBody, _ = io.ReadAll(request.Body)
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"model":"test-model","choices":[{"message":{"content":"{\"description\":\"新的摘要\",\"category\":\"AI应用开发\",\"series\":\"\",\"recommended_tags\":[\"Go\"],\"new_tag_candidates\":[\"Agent\"],\"slug\":\"new-slug\",\"primary_keyword\":\"Go AI\",\"secondary_keywords\":[\"Agent\"],\"reasons\":{\"description\":\"更清楚\"}}"}}]}`)
	}))
	defer server.Close()

	provider := buildTestProvider(t, server.URL, 1024*1024, 2*time.Second)
	result, err := provider.Generate(context.Background(), contracts.AIRequest{
		Task: contracts.AITaskMetadata,
		Article: contracts.ArticleInput{
			Title: "标题", Description: "旧摘要", Body: "不得发送的完整正文", Tags: []string{"Go"},
		},
		Taxonomy:         contracts.TaxonomyContext{Tags: []string{"Go"}},
		InputContentHash: "hash-v1",
		AllowBody:        false,
	})
	if err != nil {
		t.Fatalf("生成建议: %v", err)
	}
	if strings.Contains(string(receivedBody), "不得发送的完整正文") {
		t.Fatalf("隐私模式泄露了正文: %s", receivedBody)
	}
	var requestPayload struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(receivedBody, &requestPayload); err != nil || len(requestPayload.Messages) != 1 {
		t.Fatalf("AI 请求消息结构无效: %s", receivedBody)
	}
	if !strings.Contains(strings.ToLower(requestPayload.Messages[0].Content), "json") || !strings.Contains(requestPayload.Messages[0].Content, "output_schema") {
		t.Fatalf("AI 请求未声明 JSON 输出和结构约束: %s", requestPayload.Messages[0].Content)
	}
	if result.InputContentHash != "hash-v1" || result.Model != "test-model" {
		t.Fatalf("响应上下文不匹配: %+v", result)
	}
	if !hasSuggestion(result.Suggestions, "description", false) || !hasSuggestion(result.Suggestions, "tags", true) {
		t.Fatalf("结构化建议不完整: %+v", result.Suggestions)
	}
}

func TestProviderGeneratesXiaohongshuStructuredContent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, `{"model":"xhs-model","choices":[{"message":{"content":"{\"title\":\"给 Agent 装上研发流程\",\"body_html\":\"<p>提炼后的短文案</p>\",\"topics\":\"#AI编程 #效率工具\",\"source_note\":\"根据原文整理\",\"comment_copy\":\"你会怎么做？\"}"}}]}`)
	}))
	defer server.Close()
	provider := buildTestProvider(t, server.URL, 1024*1024, 2*time.Second)
	result, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskXiaohongshu, Article: contracts.ArticleInput{Title: "原文标题", Body: "完整原文"}, AllowBody: true, InputContentHash: "hash-xhs", OutputSchema: `{"type":"object"}`})
	if err != nil {
		t.Fatalf("生成小红书文案: %v", err)
	}
	if result.Model != "xhs-model" || len(result.Suggestions) != 5 {
		t.Fatalf("小红书结构化响应错误: %+v", result)
	}
	if !hasSuggestion(result.Suggestions, "body_html", false) || !hasSuggestion(result.Suggestions, "topics", false) {
		t.Fatalf("小红书字段缺失: %+v", result.Suggestions)
	}
}

func TestProviderMapsRateLimitToRetryableError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(response, `{"error":{"message":"secret upstream detail"}}`)
	}))
	defer server.Close()

	provider := buildTestProvider(t, server.URL, 1024, time.Second)
	_, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskMetadata})
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("期望 ProviderError，得到 %T: %v", err, err)
	}
	if !providerErr.Retryable || providerErr.Category != contracts.ErrorTemporary || providerErr.Code != "openai.rate_limited" {
		t.Fatalf("限流错误映射不正确: %+v", providerErr)
	}
	if strings.Contains(providerErr.Error(), "secret upstream detail") {
		t.Fatalf("错误消息泄露上游响应: %q", providerErr.Error())
	}
	if providerErr.UpstreamStatus != http.StatusTooManyRequests || providerErr.UpstreamMessage != "secret upstream detail" {
		t.Fatalf("上游诊断字段缺失: %+v", providerErr)
	}
}

func TestProviderRejectsOversizedAndInvalidResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		body  string
		limit int64
		code  string
	}{
		{name: "响应过大", body: strings.Repeat("x", 256), limit: 32, code: "openai.response_too_large"},
		{name: "无效 JSON", body: `{not-json`, limit: 1024, code: "openai.response_invalid"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()

			provider := buildTestProvider(t, server.URL, test.limit, time.Second)
			_, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskMetadata})
			var providerErr *contracts.ProviderError
			if !errors.As(err, &providerErr) || providerErr.Code != test.code {
				t.Fatalf("期望错误 %q，得到 %T: %v", test.code, err, err)
			}
		})
	}
}

func TestProviderRejectsIncompleteStructuredResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, `{"model":"test","choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	provider := buildTestProvider(t, server.URL, 1024, time.Second)
	_, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskMetadata})
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "openai.response_invalid" {
		t.Fatalf("缺少必填结构化字段应被拒绝: %T %v", err, err)
	}
}

func TestProviderHonorsTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(response, `{}`)
	}))
	defer server.Close()

	provider := buildTestProvider(t, server.URL, 1024, 10*time.Millisecond)
	_, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskMetadata})
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "openai.timeout" || !providerErr.Retryable {
		t.Fatalf("超时错误映射不正确: %T %+v", err, providerErr)
	}
}

func TestFactoryRejectsUnknownConfigFields(t *testing.T) {
	t.Parallel()

	factory := NewFactory(http.DefaultClient)
	_, err := factory.Build(context.Background(), contracts.ProviderRef{ID: "ai", Type: contracts.ProviderOpenAI}, contracts.ConfigView{
		Data: json.RawMessage(`{"base_url":"https://example.com","model":"test","unknown":true}`),
	}, staticSecrets{})
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != contracts.ErrorValidation {
		t.Fatalf("未知配置字段应返回 validation: %T %v", err, err)
	}
}

func TestProviderAcceptsV1BaseURLAndSendsStringMessageContent(t *testing.T) {
	t.Parallel()

	var requestPath string
	var contentKind string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestPath = request.URL.Path
		var payload struct {
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(request.Body).Decode(&payload)
		if len(payload.Messages) > 0 {
			switch payload.Messages[0].Content.(type) {
			case string:
				contentKind = "string"
			default:
				contentKind = "other"
			}
		}
		_, _ = io.WriteString(response, emptyStructuredChatResponse)
	}))
	defer server.Close()

	provider := buildTestProvider(t, server.URL+"/v1", 1024, time.Second)
	if _, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskMetadata}); err != nil {
		t.Fatalf("生成建议: %v", err)
	}
	if requestPath != "/v1/chat/completions" {
		t.Fatalf("Base URL 路径拼接错误: %q", requestPath)
	}
	if contentKind != "string" {
		t.Fatalf("OpenAI message.content 必须是字符串，得到 %q", contentKind)
	}
}

func TestProviderResolvesAPIKeyForEachCall(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, emptyStructuredChatResponse)
	}))
	defer server.Close()

	resolver := &countingSecrets{}
	factory := NewFactory(http.DefaultClient)
	config, _ := json.Marshal(Config{BaseURL: server.URL, Model: "test", Timeout: time.Second, MaxResponseBytes: 1024})
	provider, err := factory.Build(context.Background(), contracts.ProviderRef{ID: "ai", Type: contracts.ProviderOpenAI}, contracts.ConfigView{
		Data: config, SecretRefs: map[string]string{"api_key": "secret-ref"},
	}, resolver)
	if err != nil {
		t.Fatalf("构建 Provider: %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("Build 不应让 Secret 长期驻留 Provider: calls=%d", resolver.calls)
	}
	if _, err := provider.Generate(context.Background(), contracts.AIRequest{Task: contracts.AITaskMetadata}); err != nil {
		t.Fatalf("生成建议: %v", err)
	}
	if resolver.calls != 1 {
		t.Fatalf("每次调用应解析一次 Secret: calls=%d", resolver.calls)
	}
}

func buildTestProvider(t *testing.T, baseURL string, maxResponseBytes int64, timeout time.Duration) contracts.AIProvider {
	t.Helper()
	factory := NewFactory(http.DefaultClient)
	config, err := json.Marshal(Config{
		BaseURL:          baseURL,
		Model:            "test-model",
		Timeout:          timeout,
		MaxResponseBytes: maxResponseBytes,
	})
	if err != nil {
		t.Fatalf("编码配置: %v", err)
	}
	provider, err := factory.Build(context.Background(), contracts.ProviderRef{ID: "ai", Type: contracts.ProviderOpenAI}, contracts.ConfigView{
		Data:       config,
		SecretRefs: map[string]string{"api_key": "secret-ref"},
	}, staticSecrets{})
	if err != nil {
		t.Fatalf("构建 Provider: %v", err)
	}
	return provider
}

func hasSuggestion(suggestions []contracts.Suggestion, field string, newTerm bool) bool {
	for _, suggestion := range suggestions {
		if suggestion.Field == field && suggestion.NewTerm == newTerm {
			return true
		}
	}
	return false
}

type staticSecrets struct{}

func (staticSecrets) Resolve(context.Context, string) (contracts.SecretValue, error) {
	return contracts.SecretValue{Bytes: []byte("test-key")}, nil
}

type countingSecrets struct {
	calls int
}

const emptyStructuredChatResponse = `{"model":"test","choices":[{"message":{"content":"{\"description\":\"\",\"category\":\"\",\"series\":\"\",\"recommended_tags\":[],\"new_tag_candidates\":[],\"slug\":\"\",\"primary_keyword\":\"\",\"secondary_keywords\":[],\"reasons\":{}}"}}]}`

func (s *countingSecrets) Resolve(context.Context, string) (contracts.SecretValue, error) {
	s.calls++
	return contracts.SecretValue{Bytes: []byte("test-key")}, nil
}
