// Package markdown 提供与渠道无关的 Markdown 解析和内容检查。
package markdown

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Severity 是 Markdown 检查的严重程度。
type Severity string

const (
	SeverityBlocking    Severity = "blocking"
	SeverityRecommended Severity = "recommended"
)

// Finding 描述 Markdown 结构或资源问题。
type Finding struct {
	Code     string
	Severity Severity
	Message  string
}

// Check 使用 Goldmark AST 检查标题层级和本地图片。
func Check(content []byte, baseDir string) []Finding {
	document := goldmark.New().Parser().Parse(text.NewReader(content))
	var findings []Finding
	previousHeading := 0
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Heading:
			if previousHeading > 0 && value.Level > previousHeading+1 {
				findings = append(findings, Finding{Code: "markdown.heading_jump", Severity: SeverityRecommended, Message: "标题层级发生跳跃"})
			}
			previousHeading = value.Level
		case *ast.Image:
			findings = append(findings, checkImage(baseDir, string(value.Destination))...)
		}
		return ast.WalkContinue, nil
	})
	return findings
}

func checkImage(baseDir, destination string) []Finding {
	parsed, err := url.Parse(destination)
	if err != nil || parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "data" {
		return nil
	}
	localPath, err := url.PathUnescape(parsed.Path)
	if err != nil || localPath == "" {
		return nil
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return nil
	}
	resolved, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(localPath)))
	if err != nil {
		return nil
	}
	relative, err := filepath.Rel(base, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return []Finding{{Code: "markdown.image_outside_root", Severity: SeverityBlocking, Message: "图片路径超出文章目录"}}
	}
	if info, err := os.Stat(resolved); err != nil || info.IsDir() {
		return []Finding{{Code: "markdown.image_missing", Severity: SeverityBlocking, Message: "本地图片不存在: " + localPath}}
	}
	return nil
}
