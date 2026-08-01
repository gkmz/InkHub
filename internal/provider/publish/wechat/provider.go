package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

var wechatOperationPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Config 定义微信 Provider 的本地 staging 配置。
type Config struct {
	StagingRoot string
	ArtifactTTL time.Duration
	Variables   map[string]any
	Mermaid     MermaidRenderer
}

// MermaidRenderer 将 Mermaid 源码转换为可公开访问的 HTTPS 图片。
type MermaidRenderer interface {
	Render(ctx context.Context, source, digest string) (string, error)
}

// TemplateLoader 读取一个已经安装且不可变的模板版本。
type TemplateLoader interface {
	Load(ctx context.Context, ref contracts.TemplateRef) (domaintemplate.Validated, error)
}

// AssetUploadRequest 是通用上传请求的微信包兼容别名。
type AssetUploadRequest = contracts.AssetUploadRequest

// AssetUploadResult 是通用上传结果的微信包兼容别名。
type AssetUploadResult = contracts.AssetUploadResult

// AssetUploader 按内容摘要检查或上传文章图片并返回公网 URL。
type AssetUploader interface {
	Inspect(ctx context.Context, request AssetUploadRequest) (AssetUploadResult, bool, error)
	Upload(ctx context.Context, request AssetUploadRequest) (AssetUploadResult, error)
}

// Clipboard 只在用户显式交付时复制格式化 HTML。
type Clipboard interface {
	CopyHTML(ctx context.Context, html string) error
}

// Provider 准备和复制微信公众号 HTML，不声称自动发布成功。
type Provider struct {
	config    Config
	templates TemplateLoader
	uploader  AssetUploader
	clipboard Clipboard
}

var _ contracts.PublishProvider = (*Provider)(nil)

// New 创建微信 Publish Provider。
func New(config Config, templates TemplateLoader, uploader AssetUploader, clipboard Clipboard) (*Provider, error) {
	if templates == nil || clipboard == nil {
		return nil, fmt.Errorf("微信模板加载器或剪贴板为空")
	}
	staging, err := filepath.Abs(config.StagingRoot)
	if err != nil {
		return nil, fmt.Errorf("解析微信 staging: %w", err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, fmt.Errorf("创建微信 staging: %w", err)
	}
	if config.ArtifactTTL <= 0 {
		config.ArtifactTTL = time.Hour
	}
	config.StagingRoot = staging
	return &Provider{config: config, templates: templates, uploader: uploader, clipboard: clipboard}, nil
}

// Descriptor 返回微信人工交付渠道能力。
func (p *Provider) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{Descriptor: contracts.Descriptor{
		Type: contracts.ProviderWeChat, DisplayName: "微信公众号", Version: "1",
		Capabilities: []contracts.Capability{
			contracts.CapabilityPreview, contracts.CapabilityImages, contracts.CapabilityManualConfirmation,
		},
	}, DeliveryMode: contracts.DeliveryPrepareOnly}
}

// Validate 检查 Provider 的本地依赖和已配置图片仓库。
func (p *Provider) Validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if validator, ok := p.uploader.(interface{ Validate(context.Context) error }); ok {
		return validator.Validate(ctx)
	}
	return nil
}

// Preflight 检查文章、模板引用和本地图片上传能力。
func (p *Provider) Preflight(ctx context.Context, input contracts.PublishInput) (contracts.PreflightResult, error) {
	if err := ctx.Err(); err != nil {
		return contracts.PreflightResult{}, err
	}
	diagnostics := publicationDiagnostics(input.Diagnostics)
	if !wechatOperationPattern.MatchString(input.OperationID) || input.ContentHash == "" || input.TemplateRef == nil {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "wechat.input_invalid", Message: "微信发布输入缺少 OperationID、内容版本或模板", Blocking: true})
	} else if input.TemplateRef.Target != "" && input.TemplateRef.Target != domaintemplate.TargetWeChatHTML {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "wechat.template_target_invalid", Message: "所选模板不适用于微信公众号", Blocking: true})
	}
	if len(input.ResourceRefs) > 0 && p.uploader == nil {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "wechat.uploader_missing", Message: "文章包含本地图片但未配置图片上传", Blocking: true})
	}
	return contracts.PreflightResult{Diagnostics: diagnostics, Ready: !hasBlockingDiagnostic(diagnostics)}, nil
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

// InspectAssets 只读检查本地图片及远端复用状态，不产生外部写入。
func (p *Provider) InspectAssets(ctx context.Context, input contracts.PublishInput) ([]contracts.AssetPlanItem, []contracts.Diagnostic, error) {
	items := make([]contracts.AssetPlanItem, 0, len(input.ResourceRefs))
	diagnostics := append([]contracts.Diagnostic(nil), input.Diagnostics...)
	for _, resource := range input.ResourceRefs {
		info, err := InspectImage(resource.Resolved)
		if err != nil {
			diagnostics = append(diagnostics, contracts.Diagnostic{Code: "wechat.image_invalid", Message: err.Error(), Blocking: true})
			continue
		}
		item := contracts.AssetPlanItem{Reference: resource.Original, MediaType: info.MediaType, Size: info.Size, State: "upload"}
		if p.uploader == nil {
			diagnostics = append(diagnostics, contracts.Diagnostic{Code: "wechat.uploader_missing", Message: "文章包含本地图片，请先配置公开图片仓库", Blocking: true})
			items = append(items, item)
			continue
		}
		result, found, err := p.uploader.Inspect(ctx, contracts.AssetUploadRequest{LocalPath: resource.Resolved, Digest: info.Digest, MediaType: info.MediaType, Extension: info.Extension})
		if err != nil {
			diagnostics = append(diagnostics, contracts.Diagnostic{Code: "wechat.image_inspect_failed", Message: "暂时无法检查图片仓库", Blocking: true})
			items = append(items, item)
			continue
		}
		if found && result.URL != "" {
			item.State = "reuse"
		}
		items = append(items, item)
	}
	return items, diagnostics, nil
}

type wechatManifest struct {
	Artifact       contracts.PreparedArtifact `json:"artifact"`
	TemplateDigest string                     `json:"template_digest"`
	HTMLDigest     string                     `json:"html_digest"`
}

// Prepare 上传图片并生成安全 HTML artifact，但不写入剪贴板。
func (p *Provider) Prepare(ctx context.Context, input contracts.PublishInput) (contracts.PreparedArtifact, error) {
	preflight, err := p.Preflight(ctx, input)
	if err != nil {
		return contracts.PreparedArtifact{}, err
	}
	if !preflight.Ready {
		return contracts.PreparedArtifact{}, providerError("wechat.preflight_failed", "微信发布前检查未通过", contracts.ErrorValidation, nil)
	}
	operationRoot := filepath.Join(p.config.StagingRoot, input.OperationID)
	manifestPath := filepath.Join(operationRoot, "artifact.json")
	if existing, ok, conflict := loadWechatManifest(manifestPath, input.ContentHash); conflict {
		return contracts.PreparedArtifact{}, providerError("wechat.operation_conflict", "微信 OperationID 已绑定其他内容版本", contracts.ErrorConflict, nil)
	} else if ok {
		if existing.Artifact.Location != filepath.Join(operationRoot, "content.html") {
			return contracts.PreparedArtifact{}, providerError("wechat.artifact_unauthorized", "微信 artifact 路径越界", contracts.ErrorUnauthorizedResource, nil)
		}
		return existing.Artifact, nil
	}
	validated, err := p.templates.Load(ctx, *input.TemplateRef)
	if err != nil {
		return contracts.PreparedArtifact{}, providerError("wechat.template_invalid", "加载微信模板失败", contracts.ErrorValidation, err)
	}
	if input.TemplateRef.Digest != "" && validated.Digest != input.TemplateRef.Digest {
		return contracts.PreparedArtifact{}, providerError("wechat.template_conflict", "微信模板摘要不匹配", contracts.ErrorConflict, nil)
	}
	if !validated.Manifest.CompatibleWith(string(contracts.ProviderWeChat), domaintemplate.TargetWeChatHTML, domaintemplate.RendererWeChatHTMLV1) || (input.TemplateRef.Target != "" && input.TemplateRef.Target != domaintemplate.TargetWeChatHTML) {
		return contracts.PreparedArtifact{}, providerError("wechat.template_target_invalid", "微信模板目标不兼容", contracts.ErrorValidation, nil)
	}
	body, err := p.uploadResources(ctx, input.Body, input.ResourceRefs)
	if err != nil {
		return contracts.PreparedArtifact{}, err
	}
	body, err = p.renderMermaid(ctx, body)
	if err != nil {
		return contracts.PreparedArtifact{}, err
	}
	htmlContent, err := Render(validated, body, p.config.Variables)
	if err != nil {
		return contracts.PreparedArtifact{}, providerError("wechat.render_failed", "渲染微信内容失败", contracts.ErrorValidation, err)
	}
	if err := os.MkdirAll(operationRoot, 0o755); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("创建微信 artifact 目录: %w", err)
	}
	htmlPath := filepath.Join(operationRoot, "content.html")
	if err := filesystem.AtomicWrite(htmlPath, []byte(htmlContent), nil); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("保存微信 HTML: %w", err)
	}
	expiresAt := time.Now().UTC().Add(p.config.ArtifactTTL)
	artifact := contracts.PreparedArtifact{
		OperationID: input.OperationID, ContentHash: input.ContentHash, ProviderRevision: validated.Digest,
		Location: htmlPath, ExpiresAt: &expiresAt,
	}
	sum := sha256.Sum256([]byte(htmlContent))
	manifest := wechatManifest{Artifact: artifact, TemplateDigest: validated.Digest, HTMLDigest: hex.EncodeToString(sum[:])}
	encoded, _ := json.Marshal(manifest)
	if err := filesystem.AtomicWrite(manifestPath, encoded, nil); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("保存微信 artifact manifest: %w", err)
	}
	return artifact, nil
}

var mermaidFencePattern = regexp.MustCompile("(?s)```mermaid[ \\t]*\\n(.*?)\\n```")

func (p *Provider) renderMermaid(ctx context.Context, body string) (string, error) {
	matches := mermaidFencePattern.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return body, nil
	}
	if p.config.Mermaid == nil {
		return "", providerError("wechat.mermaid_unavailable", "文章包含 Mermaid，但未配置转换器", contracts.ErrorValidation, nil)
	}
	var result strings.Builder
	position := 0
	for _, match := range matches {
		source := body[match[2]:match[3]]
		sum := sha256.Sum256([]byte(source))
		digest := hex.EncodeToString(sum[:])
		remote, err := p.config.Mermaid.Render(ctx, source, digest)
		if err != nil {
			return "", providerError("wechat.mermaid_failed", "Mermaid 转换失败", contracts.ErrorDependency, err)
		}
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return "", providerError("wechat.mermaid_url_invalid", "Mermaid 转换返回了不安全 URL", contracts.ErrorValidation, err)
		}
		result.WriteString(body[position:match[0]])
		result.WriteString("![Mermaid](" + remote + ")")
		position = match[1]
	}
	result.WriteString(body[position:])
	return result.String(), nil
}

// Deliver 将已准备 HTML 复制到剪贴板，并要求用户人工确认草稿。
func (p *Provider) Deliver(ctx context.Context, artifact contracts.PreparedArtifact) (contracts.DeliveryResult, error) {
	operationRoot := filepath.Join(p.config.StagingRoot, artifact.OperationID)
	manifestPath := filepath.Join(operationRoot, "artifact.json")
	manifest, ok, conflict := loadWechatManifest(manifestPath, artifact.ContentHash)
	expectedLocation := filepath.Join(operationRoot, "content.html")
	if conflict || !ok || manifest.Artifact.Location != expectedLocation || artifact.Location != expectedLocation {
		return contracts.DeliveryResult{}, providerError("wechat.artifact_invalid", "微信 artifact 无效或已过期", contracts.ErrorConflict, nil)
	}
	content, err := os.ReadFile(manifest.Artifact.Location)
	if err != nil {
		return contracts.DeliveryResult{}, providerError("wechat.artifact_missing", "微信 HTML artifact 不存在", contracts.ErrorNotFound, err)
	}
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != manifest.HTMLDigest {
		return contracts.DeliveryResult{}, providerError("wechat.artifact_conflict", "微信 HTML artifact 已被修改", contracts.ErrorConflict, nil)
	}
	if err := p.clipboard.CopyHTML(ctx, string(content)); err != nil {
		return contracts.DeliveryResult{}, providerError("wechat.clipboard_failed", "复制微信内容失败", contracts.ErrorDependency, err)
	}
	return contracts.DeliveryResult{
		State: "copied", ProviderRevision: manifest.TemplateDigest, Location: manifest.Artifact.Location, ConfirmRequired: true,
	}, nil
}

func (p *Provider) uploadResources(ctx context.Context, body string, resources []contracts.ResourceRef) (string, error) {
	uploaded := make(map[string]string, len(resources))
	for _, resource := range resources {
		info, err := InspectImage(resource.Resolved)
		if err != nil {
			return "", err
		}
		remote, exists := uploaded[info.Digest]
		if !exists {
			result, uploadErr := p.uploader.Upload(ctx, AssetUploadRequest{
				LocalPath: resource.Resolved, Digest: info.Digest, MediaType: info.MediaType, Extension: info.Extension,
			})
			if uploadErr != nil {
				return "", providerError("wechat.image_upload_failed", "上传微信图片失败", contracts.ErrorTemporary, uploadErr)
			}
			remote = result.URL
			uploaded[info.Digest] = remote
		}
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return "", providerError("wechat.image_url_invalid", "图片上传返回了不安全 URL", contracts.ErrorValidation, err)
		}
		body = rewriteImageReference(body, resource.Original, remote)
	}
	return body, nil
}

func rewriteImageReference(body, original, remote string) string {
	// 只重写图片语法中的目标，避免相同文本出现在普通正文时被误改。
	body = strings.ReplaceAll(body, "]("+original+")", "]("+remote+")")
	body = strings.ReplaceAll(body, "![["+original+"]]", "![图片]("+remote+")")
	return body
}

func loadWechatManifest(path, contentHash string) (wechatManifest, bool, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return wechatManifest{}, false, false
	}
	var manifest wechatManifest
	if json.Unmarshal(content, &manifest) != nil {
		return wechatManifest{}, false, true
	}
	if manifest.Artifact.ContentHash != contentHash {
		return manifest, false, true
	}
	return manifest, true, false
}

func hasBlockingDiagnostic(values []contracts.Diagnostic) bool {
	for _, value := range values {
		if value.Blocking {
			return true
		}
	}
	return false
}

func providerError(code, message string, category contracts.ErrorCategory, cause error) *contracts.ProviderError {
	return &contracts.ProviderError{Code: code, Category: category, Message: message, Cause: cause}
}
