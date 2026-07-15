package template

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDirectoryAcceptsStrictTemplate(t *testing.T) {
	t.Parallel()

	root := makeTemplateFixture(t, safeCSS)
	validated, err := ValidateDirectory(root)
	if err != nil {
		t.Fatalf("校验模板: %v", err)
	}
	if validated.Manifest.ID != "test-template" || validated.Digest == "" {
		t.Fatalf("校验结果不完整: %+v", validated)
	}
	if validated.Manifest.Target != TargetWeChatHTML || validated.Manifest.Renderer != RendererWeChatHTMLV1 {
		t.Fatalf("1.0 模板未迁移默认目标: %+v", validated.Manifest)
	}
}

func TestValidateDirectoryAcceptsExplicitTemplateTargetV11(t *testing.T) {
	t.Parallel()
	root := makeTemplateFixture(t, safeCSS)
	path := filepath.Join(root, "template.yaml")
	content, _ := os.ReadFile(path)
	updated := strings.Replace(string(content), `specVersion: "1.0"`, `specVersion: "1.1"
target: wechat-html
format: css
renderer: wechat-html-v1
compatibility:
  providers: [wechat]
  rendererVersion: "1"`, 1)
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	validated, err := ValidateDirectory(root)
	if err != nil {
		t.Fatalf("校验 1.1 模板: %v", err)
	}
	if validated.Manifest.SpecVersion != "1.1" || validated.Manifest.Target != TargetWeChatHTML {
		t.Fatalf("显式模板目标丢失: %+v", validated.Manifest)
	}
	if !validated.Manifest.CompatibleWith("wechat", TargetWeChatHTML, RendererWeChatHTMLV1) || validated.Manifest.CompatibleWith("hugo", TargetWeChatHTML, RendererWeChatHTMLV1) {
		t.Fatal("模板兼容性判断错误")
	}
}

func TestValidateDirectoryRejectsDuplicateKeyAliasAndDangerousCSS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		css      string
		manifest func(string) string
	}{
		{name: "重复 YAML key", css: safeCSS, manifest: func(value string) string { return value + "id: duplicate\n" }},
		{name: "YAML alias", css: safeCSS, manifest: func(value string) string {
			return strings.Replace(value, "name: Test Template", "name: &name Test Template\ndescription: *name", 1)
		}},
		{name: "脚本 URL", css: `.inkhub-root p { background-color: url(javascript:alert(1)); }`},
		{name: "越界 selector", css: `body .inkhub-root p { color: #111111; }`},
		{name: "禁止属性", css: `.inkhub-root p { position: fixed; }`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeTemplateFixture(t, test.css)
			if test.manifest != nil {
				path := filepath.Join(root, "template.yaml")
				content, _ := os.ReadFile(path)
				if err := os.WriteFile(path, []byte(test.manifest(string(content))), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ValidateDirectory(root); err == nil {
				t.Fatal("危险模板应被拒绝")
			}
		})
	}
}

func TestValidateDirectoryRejectsDigestMismatchAndActiveContent(t *testing.T) {
	t.Parallel()

	root := makeTemplateFixture(t, safeCSS)
	if err := os.WriteFile(filepath.Join(root, "styles.css"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateDirectory(root); err == nil {
		t.Fatal("摘要不匹配应失败")
	}

	root = makeTemplateFixture(t, safeCSS)
	if err := os.WriteFile(filepath.Join(root, "script.js"), []byte("alert(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateDirectory(root); err == nil {
		t.Fatal("主动内容格式应失败")
	}
}

func TestValidateDirectoryChecksAssetLicenseAndMediaSignature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   []byte
		license   string
		wantValid bool
	}{
		{name: "合法 PNG asset", content: nil, license: "Apache-2.0", wantValid: true},
		{name: "缺少许可证", content: nil, license: "", wantValid: false},
		{name: "伪造 PNG", content: []byte("not-a-png"), license: "Apache-2.0", wantValid: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := makeTemplateFixture(t, safeCSS)
			preview, _ := os.ReadFile(filepath.Join(root, "preview.png"))
			asset := test.content
			if asset == nil {
				asset = preview
			}
			if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "assets", "logo.png"), asset, 0o644); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(root, "template.yaml")
			manifest, _ := os.ReadFile(manifestPath)
			addition := `assets:
  - path: assets/logo.png
    mediaType: image/png
    sha256: ` + digest(asset) + `
    source: https://example.com
    license: ` + test.license + `
files:
  - path: assets/logo.png
    sha256: ` + digest(asset) + "\n"
			manifest = []byte(strings.Replace(string(manifest), "files:\n", addition, 1))
			if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ValidateDirectory(root)
			if test.wantValid && err != nil {
				t.Fatalf("合法 asset 被拒绝: %v", err)
			}
			if !test.wantValid && err == nil {
				t.Fatal("不安全 asset 应被拒绝")
			}
		})
	}
}

const safeCSS = `.inkhub-root { color: {{ color.accentColor }}; font-family: {{ font-family.bodyFont }}; }
.inkhub-root p { margin-bottom: 16px; line-height: 1.8; }
.inkhub-root h1 { color: #1677ff; font-size: 24px; }`

func makeTemplateFixture(t *testing.T, css string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "test-template")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	preview := filepath.Join(root, "preview.png")
	file, err := os.Create(preview)
	if err != nil {
		t.Fatal(err)
	}
	imageValue := image.NewRGBA(image.Rect(0, 0, 1200, 1600))
	imageValue.Set(0, 0, color.White)
	if err := png.Encode(file, imageValue); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	files := map[string][]byte{
		"styles.css": []byte(css),
		"preview.md": []byte("# 标题\n\n正文。"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	previewContent, _ := os.ReadFile(preview)
	manifest := `specVersion: "1.0"
id: test-template
name: Test Template
description: 测试模板
author:
  name: InkHub
  url: https://example.com
license: Apache-2.0
version: 1.0.0
inkhubVersion: ">=1.0.0 <2.0.0"
entry: styles.css
preview:
  markdown: preview.md
  image: preview.png
elements: [paragraph, heading-1]
variables:
  accentColor:
    type: color
    label: 强调色
    default: "#1677ff"
  bodyFont:
    type: font-family
    label: 正文字体
    default: system-sans
    options: [system-sans, system-serif]
files:
  - path: styles.css
    sha256: ` + digest(files["styles.css"]) + `
  - path: preview.md
    sha256: ` + digest(files["preview.md"]) + `
  - path: preview.png
    sha256: ` + digest(previewContent) + "\n"
	if err := os.WriteFile(filepath.Join(root, "template.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
