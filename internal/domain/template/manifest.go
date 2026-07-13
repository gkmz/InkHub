// Package template 定义无执行代码的微信模板规范与安全校验。
package template

// Manifest 是微信模板规范 1.0 的声明文件。
type Manifest struct {
	SpecVersion   string              `yaml:"specVersion"`
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
