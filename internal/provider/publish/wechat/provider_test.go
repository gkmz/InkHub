package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestPrepareRendersAndUploadsWithoutCopyingUntilDeliver(t *testing.T) {
	t.Parallel()

	clipboard := &memoryClipboard{}
	provider, err := New(Config{StagingRoot: t.TempDir()}, staticLoader{template: renderTemplate("default", `.inkhub-root p { color: #111111; }`)}, staticUploader{}, clipboard)
	if err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
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

type staticLoader struct{ template domaintemplate.Validated }

func (l staticLoader) Load(context.Context, contracts.TemplateRef) (domaintemplate.Validated, error) {
	return l.template, nil
}

type staticUploader struct{}

func (staticUploader) Upload(_ context.Context, _ string, digest string) (string, error) {
	return "https://cdn.example/" + digest + ".png", nil
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
