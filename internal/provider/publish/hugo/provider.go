// Package hugo 实现幂等、可恢复的 Hugo Publish Provider。
package hugo

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Config 定义 Hugo Provider 的非敏感本机配置。
type Config struct {
	Root        string
	StagingRoot string
	Section     string
	BaseURL     string
	ArtifactTTL time.Duration
}

// Builder 在指定 Hugo 根目录执行受控构建。
type Builder interface {
	Build(ctx context.Context, root string) (revision string, err error)
}

// Provider 将标准文章转换为 Hugo page bundle。
type Provider struct {
	config  Config
	builder Builder
	replace replaceBundleFunc
}

var _ contracts.PublishProvider = (*Provider)(nil)

// New 创建并校验 Hugo Publish Provider。
func New(config Config, builder Builder) (*Provider, error) {
	if builder == nil {
		return nil, fmt.Errorf("Hugo Builder 为空")
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("解析 Hugo 根目录: %w", err)
	}
	staging, err := filepath.Abs(config.StagingRoot)
	if err != nil {
		return nil, fmt.Errorf("解析 Hugo staging 目录: %w", err)
	}
	if !hasHugoConfig(root) {
		return nil, fmt.Errorf("目录不是有效 Hugo 站点: %s", root)
	}
	if staging == root || isWithin(staging, root) || isWithin(root, staging) {
		return nil, fmt.Errorf("Hugo 根目录与 staging 目录不能互相包含")
	}
	if config.Section == "" {
		config.Section = "posts"
	}
	if !safeSegment(config.Section) {
		return nil, fmt.Errorf("Hugo section 无效: %s", config.Section)
	}
	if config.ArtifactTTL <= 0 {
		config.ArtifactTTL = time.Hour
	}
	if config.BaseURL != "" {
		parsed, parseErr := url.Parse(config.BaseURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("Hugo Base URL 无效")
		}
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, fmt.Errorf("创建 Hugo staging 目录: %w", err)
	}
	config.Root = root
	config.StagingRoot = staging
	return &Provider{config: config, builder: builder, replace: replaceBundle}, nil
}

// Descriptor 返回 Hugo 渠道能力声明。
func (p *Provider) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderHugo, DisplayName: "Hugo", Version: "1",
		Capabilities: []contracts.Capability{
			contracts.CapabilityPreview, contracts.CapabilityTaxonomy, contracts.CapabilityCanonical,
		},
	}, DeliveryMode: contracts.DeliveryAutomatic}
}

// Validate 检查 Hugo 根目录、配置和 staging 可用性。
func (p *Provider) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !hasHugoConfig(p.config.Root) {
		return fmt.Errorf("Hugo 配置文件不可用")
	}
	return nil
}

// Preflight 检查标准文章输入，不产生文件副作用。
func (p *Provider) Preflight(ctx context.Context, input contracts.PublishInput) (contracts.PreflightResult, error) {
	if err := ctx.Err(); err != nil {
		return contracts.PreflightResult{}, err
	}
	diagnostics := publicationDiagnostics(input.Diagnostics)
	if input.OperationID == "" || !operationIDPattern.MatchString(input.OperationID) {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "hugo.operation_invalid", Message: "Hugo OperationID 无效", Blocking: true})
	}
	if input.ContentHash == "" {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "hugo.content_version_missing", Message: "Hugo 文章缺少内容版本", Blocking: true})
	}
	if input.Article.StableID == "" {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "hugo.stable_id_missing", Message: "Hugo 文章缺少稳定 ID", Blocking: true})
	} else if input.Article.StableID.Validate() != nil {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "hugo.stable_id_invalid", Message: "Hugo 文章稳定 ID 格式无效", Blocking: true})
	}
	if input.Article.Title == "" {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "hugo.title_missing", Message: "Hugo 文章缺少标题", Blocking: true})
	}
	return contracts.PreflightResult{Diagnostics: diagnostics, Ready: !hasBlocking(diagnostics)}, nil
}

// publicationDiagnostics 将索引阶段的资源问题提升为发布阶段阻断问题。
func publicationDiagnostics(values []contracts.Diagnostic) []contracts.Diagnostic {
	diagnostics := append([]contracts.Diagnostic(nil), values...)
	for index := range diagnostics {
		if diagnostics[index].Code == "source.image_unresolved" {
			diagnostics[index].Blocking = true
		}
	}
	return diagnostics
}

func providerError(code, message string, category contracts.ErrorCategory, retryable bool, cause error) *contracts.ProviderError {
	return &contracts.ProviderError{Code: code, Category: category, Message: message, Retryable: retryable, Cause: cause}
}

func hasBlocking(values []contracts.Diagnostic) bool {
	for _, value := range values {
		if value.Blocking {
			return true
		}
	}
	return false
}

func hasHugoConfig(root string) bool {
	for _, name := range []string{"hugo.yaml", "hugo.yml", "hugo.toml", "hugo.json", "config.yaml", "config.yml", "config.toml", "config.json"} {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func safeSegment(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

func isWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
