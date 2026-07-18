package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestValidateChecksConfiguredAssetUploader(t *testing.T) {
	t.Parallel()
	expected := errors.New("repository private")
	provider, err := New(Config{StagingRoot: t.TempDir()}, staticLoader{}, validatingUploader{err: expected}, &memoryClipboard{})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Validate(context.Background()); !errors.Is(err, expected) {
		t.Fatalf("Validate 未检查图片仓库: %v", err)
	}
}

type validatingUploader struct{ err error }

func (uploader validatingUploader) Validate(context.Context) error { return uploader.err }
func (validatingUploader) Inspect(context.Context, AssetUploadRequest) (AssetUploadResult, bool, error) {
	return AssetUploadResult{}, false, nil
}
func (validatingUploader) Upload(context.Context, AssetUploadRequest) (AssetUploadResult, error) {
	return AssetUploadResult{}, nil
}

func TestPrepareRendersAndUploadsWithoutCopyingUntilDeliver(t *testing.T) {
	t.Parallel()

	clipboard := &memoryClipboard{}
	provider, err := New(Config{StagingRoot: t.TempDir()}, staticLoader{template: renderTemplate("default", `.inkhub-root p { color: #111111; }`)}, staticUploader{}, clipboard)
	if err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(t.TempDir(), "cover.png")
	imageFile, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(imageFile, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}
	input := contracts.PublishInput{
		OperationID: "wechat_1", ContentHash: "hash-v1", Body: "正文\n\n![封面](images/cover.png)",
		Article:      article.Article{StableID: "article_ONE", Title: "标题"},
		TemplateRef:  &contracts.TemplateRef{ID: "default", Version: "1.0.0"},
		ResourceRefs: []contracts.ResourceRef{{Original: "images/cover.png", Resolved: imagePath, Kind: "image"}},
	}
	artifact, err := provider.Prepare(context.Background(), input)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if clipboard.calls != 0 {
		t.Fatal("Prepare 不得写剪贴板")
	}
	htmlContent, err := os.ReadFile(artifact.Location)
	if err != nil || strings.Contains(string(htmlContent), "images/cover.png") || !strings.Contains(string(htmlContent), "https://cdn.example/") {
		t.Fatalf("图片未上传或 artifact 无效: html=%s err=%v", htmlContent, err)
	}
	result, err := provider.Deliver(context.Background(), artifact)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if clipboard.calls != 1 || result.State != "copied" || !result.ConfirmRequired {
		t.Fatalf("复制状态不正确: calls=%d result=%+v", clipboard.calls, result)
	}
}

func TestDeliverRejectsTamperedArtifactLocation(t *testing.T) {
	t.Parallel()

	staging := t.TempDir()
	clipboard := &memoryClipboard{}
	provider, err := New(Config{StagingRoot: staging}, staticLoader{template: renderTemplate("default", `.inkhub-root p { color: #111111; }`)}, nil, clipboard)
	if err != nil {
		t.Fatal(err)
	}
	input := contracts.PublishInput{
		OperationID: "wechat_tampered", ContentHash: "hash", Body: "正文",
		Article:     article.Article{StableID: "article_ONE", Title: "标题"},
		TemplateRef: &contracts.TemplateRef{ID: "default", Version: "1.0.0"},
	}
	artifact, err := provider.Prepare(context.Background(), input)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret.html")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(staging, input.OperationID, "artifact.json")
	content, _ := os.ReadFile(manifestPath)
	var manifest wechatManifest
	_ = json.Unmarshal(content, &manifest)
	manifest.Artifact.Location = outside
	manifest.HTMLDigest = digestText("secret")
	content, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	artifact.Location = outside
	if _, err := provider.Deliver(context.Background(), artifact); err == nil {
		t.Fatal("越界 artifact Location 应被拒绝")
	}
	if clipboard.calls != 0 {
		t.Fatal("越界文件被复制到剪贴板")
	}
}

func TestPrepareConvertsMermaidThroughControlledRenderer(t *testing.T) {
	t.Parallel()

	provider, err := New(Config{StagingRoot: t.TempDir(), Mermaid: staticMermaid{}}, staticLoader{template: renderTemplate("default", `.inkhub-root img { max-width: 100%; }`)}, nil, &memoryClipboard{})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := provider.Prepare(context.Background(), contracts.PublishInput{
		OperationID: "wechat_mermaid", ContentHash: "hash", Body: "```mermaid\ngraph TD; A-->B;\n```",
		Article:     article.Article{StableID: "article_ONE", Title: "标题"},
		TemplateRef: &contracts.TemplateRef{ID: "default", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("Prepare Mermaid: %v", err)
	}
	content, _ := os.ReadFile(artifact.Location)
	if !strings.Contains(string(content), "https://cdn.example/mermaid-") || strings.Contains(string(content), "```mermaid") {
		t.Fatalf("Mermaid 未转换为图片: %s", content)
	}
}

func TestPreflightRejectsTemplateForAnotherTarget(t *testing.T) {
	t.Parallel()
	provider, err := New(Config{StagingRoot: t.TempDir()}, staticLoader{}, nil, &memoryClipboard{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Preflight(context.Background(), contracts.PublishInput{OperationID: "operation_target", ContentHash: "hash", TemplateRef: &contracts.TemplateRef{ID: "other", Version: "1.0.0", Target: "hugo-partial"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready {
		t.Fatal("微信 Provider 不应接受其他目标模板")
	}
}

func TestInspectAssetsDoesNotUploadAndReturnsSafePlan(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cover.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	uploader := &countingUploader{}
	provider, err := New(Config{StagingRoot: t.TempDir()}, staticLoader{}, uploader, &memoryClipboard{})
	if err != nil {
		t.Fatal(err)
	}
	items, diagnostics, err := provider.InspectAssets(context.Background(), contracts.PublishInput{ResourceRefs: []contracts.ResourceRef{{Original: "images/cover.png", Resolved: path, Kind: "image"}}})
	if err != nil || len(diagnostics) != 0 || len(items) != 1 || items[0].Reference != "images/cover.png" || items[0].State != "upload" {
		t.Fatalf("图片计划错误: items=%+v diagnostics=%+v err=%v", items, diagnostics, err)
	}
	if uploader.inspectCalls != 1 || uploader.uploadCalls != 0 {
		t.Fatalf("只读计划发生写入: inspect=%d upload=%d", uploader.inspectCalls, uploader.uploadCalls)
	}
}

func TestInspectAssetsKeepsSourceImageDiagnostics(t *testing.T) {
	t.Parallel()
	provider, err := New(Config{StagingRoot: t.TempDir()}, staticLoader{}, nil, &memoryClipboard{})
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics, err := provider.InspectAssets(context.Background(), contracts.PublishInput{Diagnostics: []contracts.Diagnostic{{Code: "source.image_unresolved", Message: "文章图片无法解析或超出 Vault", Blocking: true}}})
	if err != nil || len(diagnostics) != 1 || !diagnostics[0].Blocking {
		t.Fatalf("Source 图片诊断丢失: %+v err=%v", diagnostics, err)
	}
}

type staticLoader struct{ template domaintemplate.Validated }

func (l staticLoader) Load(context.Context, contracts.TemplateRef) (domaintemplate.Validated, error) {
	return l.template, nil
}

type staticUploader struct{}

func (staticUploader) Inspect(_ context.Context, request AssetUploadRequest) (AssetUploadResult, bool, error) {
	return AssetUploadResult{URL: "https://cdn.example/" + request.Digest + request.Extension, Reused: true}, true, nil
}

type countingUploader struct {
	inspectCalls int
	uploadCalls  int
}

func (uploader *countingUploader) Inspect(context.Context, AssetUploadRequest) (AssetUploadResult, bool, error) {
	uploader.inspectCalls++
	return AssetUploadResult{}, false, nil
}

func (uploader *countingUploader) Upload(context.Context, AssetUploadRequest) (AssetUploadResult, error) {
	uploader.uploadCalls++
	return AssetUploadResult{URL: "https://cdn.example/image.png"}, nil
}

func (staticUploader) Upload(_ context.Context, request AssetUploadRequest) (AssetUploadResult, error) {
	return AssetUploadResult{URL: "https://cdn.example/" + request.Digest + request.Extension}, nil
}

type staticMermaid struct{}

func (staticMermaid) Render(_ context.Context, _ string, digest string) (string, error) {
	return "https://cdn.example/mermaid-" + digest + ".png", nil
}

type memoryClipboard struct {
	calls int
	html  string
}

func (c *memoryClipboard) CopyHTML(_ context.Context, html string) error {
	c.calls++
	c.html = html
	return nil
}

func digestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
