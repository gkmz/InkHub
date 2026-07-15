// Package template 定义无执行代码的多目标模板规范与安全校验。
package template

const (
	// TargetWeChatHTML 表示微信公众号安全 HTML 渲染目标。
	TargetWeChatHTML = "wechat-html"
	// RendererWeChatHTMLV1 表示微信 HTML Renderer 契约版本 1。
	RendererWeChatHTMLV1 = "wechat-html-v1"
)

// Manifest 是模板资源包的声明文件。
type Manifest struct {
	SpecVersion   string              `yaml:"specVersion"`
	Target        string              `yaml:"target"`
	Format        string              `yaml:"format"`
	Renderer      string              `yaml:"renderer"`
	Compatibility Compatibility       `yaml:"compatibility"`
	ID            string              `yaml:"id"`
	Name          string              `yaml:"name"`
	Description   string              `yaml:"description"`
	Author        Author              `yaml:"author"`
	License       string              `yaml:"license"`
	Version       string              `yaml:"version"`
	InkHubVersion string              `yaml:"inkhubVersion"`
	Entry         string              `yaml:"entry"`
	Preview       Preview             `yaml:"preview"`
	Elements      []string            `yaml:"elements"`
	Variables     map[string]Variable `yaml:"variables"`
	Assets        []Asset             `yaml:"assets"`
	Files         []FileDigest        `yaml:"files"`
}

// Compatibility 声明模板允许的 Provider 和 Renderer 版本。
type Compatibility struct {
	Providers       []string `yaml:"providers"`
	RendererVersion string   `yaml:"rendererVersion"`
}

// Author 描述模板作者与可选主页。
type Author struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Preview 指向标准预览 Markdown 和 PNG。
type Preview struct {
	Markdown string `yaml:"markdown"`
	Image    string `yaml:"image"`
}

// Variable 定义一个类型安全的模板变量。
type Variable struct {
	Type      string   `yaml:"type"`
	Label     string   `yaml:"label"`
	Default   any      `yaml:"default"`
	Options   []string `yaml:"options"`
	Min       *float64 `yaml:"min"`
	Max       *float64 `yaml:"max"`
	Unit      string   `yaml:"unit"`
	TrueValue string   `yaml:"trueValue"`
}

// Asset 描述模板静态资源的来源、许可和摘要。
type Asset struct {
	Path      string `yaml:"path"`
	MediaType string `yaml:"mediaType"`
	SHA256    string `yaml:"sha256"`
	Source    string `yaml:"source"`
	License   string `yaml:"license"`
}

// FileDigest 绑定模板文件路径和 SHA-256。
type FileDigest struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

// Validated 是通过完整安全校验的不可变模板快照。
type Validated struct {
	Root     string
	Manifest Manifest
	CSS      string
	Digest   string
}

// CompatibleWith 判断模板是否可由指定 Provider 和 Renderer 使用。
func (m Manifest) CompatibleWith(provider, target, renderer string) bool {
	if m.Target == "" && m.Renderer == "" && m.SpecVersion != "1.1" {
		return provider == "wechat" && target == TargetWeChatHTML && renderer == RendererWeChatHTMLV1
	}
	if m.Target != target || m.Renderer != renderer {
		return false
	}
	for _, allowed := range m.Compatibility.Providers {
		if allowed == provider {
			return true
		}
	}
	return false
}
