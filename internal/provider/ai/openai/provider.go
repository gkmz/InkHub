// Package openai 实现 OpenAI-compatible 结构化 AI Provider。
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

const defaultMaxResponseBytes int64 = 2 << 20

// Config 定义 OpenAI-compatible Provider 的非敏感配置。
type Config struct {
	BaseURL          string        `json:"base_url"`
	Model            string        `json:"model"`
	Timeout          time.Duration `json:"timeout"`
	MaxInputBytes    int64         `json:"max_input_bytes,omitempty"`
	MaxResponseBytes int64         `json:"max_response_bytes,omitempty"`
}

// Factory 构建 OpenAI-compatible Provider。
type Factory struct {
	client *http.Client
}

// NewFactory 使用给定 HTTP Client 创建工厂。
func NewFactory(client *http.Client) *Factory {
	if client == nil {
		client = http.DefaultClient
	}
	return &Factory{client: client}
}

// Type 返回工厂的稳定 Provider 类型。
func (f *Factory) Type() contracts.ProviderType { return contracts.ProviderOpenAI }

// Descriptor 返回不包含 Secret 的稳定能力描述。
func (f *Factory) Descriptor() contracts.AIDescriptor {
	return contracts.AIDescriptor{
		Descriptor: contracts.Descriptor{
			Type:         contracts.ProviderOpenAI,
			DisplayName:  "OpenAI Compatible",
			Version:      "1",
			ConfigSchema: openAIConfigSchema,
			Capabilities: []contracts.Capability{contracts.CapabilityStructuredOutput},
			SecretKeys:   []string{"api_key"},
		},
		OutputSchema: metadataOutputSchema,
	}
}

// Build 严格解码配置并解析 API Key 引用。
func (f *Factory) Build(_ context.Context, ref contracts.ProviderRef, view contracts.ConfigView, secrets contracts.SecretResolver) (contracts.AIProvider, error) {
	if ref.Type != contracts.ProviderOpenAI {
		return nil, validationError("provider_type", "Provider 类型不匹配", nil)
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(view.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, validationError("config", "AI Provider 配置无效", err)
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	secretRef := view.SecretRefs["api_key"]
	if secretRef != "" && secrets == nil {
		return nil, validationError("api_key", "API Key 解析器不可用", nil)
	}
	if config.MaxResponseBytes == 0 {
		config.MaxResponseBytes = defaultMaxResponseBytes
	}

	// 复制 Client，避免修改调用方共享实例的超时和重定向策略。
	client := *f.client
	client.Timeout = config.Timeout
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Provider{ref: ref, config: config, secretRef: secretRef, secrets: secrets, client: &client}, nil
}

// Provider 通过 OpenAI-compatible HTTP API 生成结构化建议。
type Provider struct {
	ref       contracts.ProviderRef
	config    Config
	secretRef string
	secrets   contracts.SecretResolver
	client    *http.Client
}

// Descriptor 返回当前实例的稳定能力描述。
func (p *Provider) Descriptor() contracts.AIDescriptor {
	descriptor := NewFactory(p.client).Descriptor()
	descriptor.Models = []string{p.config.Model}
	descriptor.MaxInputBytes = p.config.MaxInputBytes
	return descriptor
}

// Validate 检查当前实例配置，不发起探测性网络请求。
func (p *Provider) Validate(context.Context) error { return validateConfig(p.config) }

// Generate 发送经过隐私裁剪的请求并解析结构化建议。
func (p *Provider) Generate(ctx context.Context, request contracts.AIRequest) (contracts.AIResponse, error) {
	payload, err := p.buildRequest(request)
	if err != nil {
		return contracts.AIResponse{}, err
	}
	response, err := p.do(ctx, payload)
	if err != nil {
		return contracts.AIResponse{}, err
	}
	structured, err := decodeResponse(response)
	if err != nil {
		return contracts.AIResponse{}, err
	}
	return contracts.AIResponse{
		InputContentHash: request.InputContentHash,
		Model:            structured.Model,
		Suggestions:      structured.Suggestions,
	}, nil
}

func validateConfig(config Config) error {
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return validationError("base_url", "AI Base URL 必须是有效的 HTTP(S) 地址", err)
	}
	if strings.TrimSpace(config.Model) == "" {
		return validationError("model", "AI 模型不能为空", nil)
	}
	if config.Timeout <= 0 {
		return validationError("timeout", "AI 请求超时必须大于零", nil)
	}
	if config.MaxInputBytes < 0 || config.MaxResponseBytes < 0 {
		return validationError("limits", "AI 输入输出限制不能为负数", nil)
	}
	return nil
}

func validationError(field, message string, cause error) *contracts.ProviderError {
	return &contracts.ProviderError{Code: "openai.config_invalid", Category: contracts.ErrorValidation, Message: message, Field: field, Cause: cause}
}

func (p *Provider) endpoint() string {
	base := strings.TrimRight(p.config.BaseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

const openAIConfigSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["base_url","model","timeout"],
  "properties":{
    "base_url":{"type":"string","format":"uri"},
    "model":{"type":"string","minLength":1},
    "timeout":{"type":"integer","minimum":1},
    "max_input_bytes":{"type":"integer","minimum":0},
    "max_response_bytes":{"type":"integer","minimum":0}
  }
}`

const metadataOutputSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["description","category","series","recommended_tags","new_tag_candidates","slug","primary_keyword","secondary_keywords","reasons"],
  "properties":{
    "description":{"type":"string"},
    "category":{"type":"string"},
    "series":{"type":"string"},
    "recommended_tags":{"type":"array","items":{"type":"string"}},
    "new_tag_candidates":{"type":"array","items":{"type":"string"}},
    "slug":{"type":"string"},
    "primary_keyword":{"type":"string"},
    "secondary_keywords":{"type":"array","items":{"type":"string"}},
    "reasons":{"type":"object","additionalProperties":{"type":"string"}}
  }
}`

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

func (p *Provider) buildRequest(request contracts.AIRequest) ([]byte, error) {
	article := request.Article
	if !request.AllowBody {
		article.Body = ""
	}
	input := struct {
		Task     contracts.AITask          `json:"task"`
		Article  contracts.ArticleInput    `json:"article"`
		Taxonomy contracts.TaxonomyContext `json:"taxonomy"`
		Schema   json.RawMessage           `json:"output_schema,omitempty"`
	}{Task: request.Task, Article: article, Taxonomy: request.Taxonomy}
	if request.OutputSchema != "" {
		input.Schema = json.RawMessage(request.OutputSchema)
	}
	content, err := json.Marshal(input)
	if err != nil {
		return nil, &contracts.ProviderError{Code: "openai.request_invalid", Category: contracts.ErrorValidation, Message: "无法编码 AI 请求", Cause: err}
	}
	if p.config.MaxInputBytes > 0 && int64(len(content)) > p.config.MaxInputBytes {
		return nil, &contracts.ProviderError{Code: "openai.input_too_large", Category: contracts.ErrorValidation, Message: "AI 输入超过配置限制"}
	}
	payload, err := json.Marshal(chatRequest{
		Model:          p.config.Model,
		Messages:       []chatMessage{{Role: "user", Content: string(content)}},
		ResponseFormat: &responseFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, &contracts.ProviderError{Code: "openai.request_invalid", Category: contracts.ErrorInternal, Message: "无法编码 AI 请求", Cause: err}
	}
	return payload, nil
}

func (p *Provider) do(ctx context.Context, payload []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, validationError("base_url", "无法创建 AI 请求", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if p.secretRef != "" {
		secret, err := p.secrets.Resolve(ctx, p.secretRef)
		if err != nil {
			return nil, &contracts.ProviderError{Code: "openai.secret_unavailable", Category: contracts.ErrorDependency, Message: "无法读取 AI API Key", Cause: err}
		}
		// 复制后立即清零本地副本，避免 Secret 在 Provider 实例中跨请求驻留。
		apiKey := append([]byte(nil), secret.Bytes...)
		request.Header.Set("Authorization", "Bearer "+string(apiKey))
		for index := range apiKey {
			apiKey[index] = 0
		}
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, mapNetworkError(ctx, err)
	}
	defer response.Body.Close()

	body, tooLarge, err := readLimited(response.Body, p.config.MaxResponseBytes)
	if err != nil {
		return nil, &contracts.ProviderError{Code: "openai.response_read_failed", Category: contracts.ErrorTemporary, Message: "读取 AI 响应失败", Retryable: true, Cause: err}
	}
	if tooLarge {
		return nil, &contracts.ProviderError{Code: "openai.response_too_large", Category: contracts.ErrorPermanent, Message: "AI 响应超过大小限制"}
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, &contracts.ProviderError{Code: "openai.rate_limited", Category: contracts.ErrorTemporary, Message: "AI 服务请求过于频繁", Retryable: true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retryable := response.StatusCode >= 500
		category := contracts.ErrorPermanent
		if retryable {
			category = contracts.ErrorTemporary
		}
		return nil, &contracts.ProviderError{Code: "openai.request_failed", Category: category, Message: fmt.Sprintf("AI 服务返回状态 %d", response.StatusCode), Retryable: retryable}
	}
	return body, nil
}
