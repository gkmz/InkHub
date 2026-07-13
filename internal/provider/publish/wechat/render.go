// Package wechat 实现微信公众号安全渲染和人工复制交付。
package wechat

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Render 使用已校验模板渲染、内联并清理 Markdown HTML。
func Render(validated domaintemplate.Validated, markdown string, variables map[string]any) (string, error) {
	css, err := domaintemplate.ResolveCSS(validated, variables)
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	engine := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := engine.Convert([]byte(markdown), &rendered); err != nil {
		return "", fmt.Errorf("渲染微信 Markdown: %w", err)
	}
	contextNode := &html.Node{Type: html.ElementNode, DataAtom: atom.Section, Data: "section", Attr: []html.Attribute{{Key: "class", Val: "inkhub-root"}}}
	children, err := html.ParseFragment(strings.NewReader(rendered.String()), contextNode)
	if err != nil {
		return "", fmt.Errorf("解析微信 HTML: %w", err)
	}
	for _, child := range children {
		contextNode.AppendChild(child)
	}
	if err := inlineCSS(contextNode, css); err != nil {
		return "", err
	}
	if err := sanitizeTree(contextNode); err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := html.Render(&output, contextNode); err != nil {
		return "", fmt.Errorf("序列化微信 HTML: %w", err)
	}
	return output.String(), nil
}

type cssRule struct {
	selector string
	styles   map[string]string
}

var renderRulePattern = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)

func inlineCSS(root *html.Node, css string) error {
	var rules []cssRule
	for _, match := range renderRulePattern.FindAllStringSubmatch(css, -1) {
		styles := map[string]string{}
		for _, declaration := range strings.Split(match[2], ";") {
			parts := strings.SplitN(strings.TrimSpace(declaration), ":", 2)
			if len(parts) == 2 {
				styles[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		for _, selector := range strings.Split(match[1], ",") {
			rules = append(rules, cssRule{selector: strings.TrimSpace(selector), styles: styles})
		}
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			styles := map[string]string{}
			for _, rule := range rules {
				if matchesSelector(node, rule.selector) {
					for property, value := range rule.styles {
						styles[property] = value
					}
				}
			}
			if len(styles) > 0 {
				node.Attr = append(node.Attr, html.Attribute{Key: "style", Val: serializeStyles(styles)})
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nil
}

func matchesSelector(node *html.Node, selector string) bool {
	parts := strings.Fields(selector)
	if len(parts) == 0 || parts[0] != ".inkhub-root" {
		return false
	}
	if len(parts) == 1 {
		return hasClass(node, "inkhub-root")
	}
	if node.Data != parts[len(parts)-1] {
		return false
	}
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if hasClass(parent, "inkhub-root") {
			return true
		}
	}
	return false
}

func hasClass(node *html.Node, class string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == "class" && containsWord(attribute.Val, class) {
			return true
		}
	}
	return false
}

func containsWord(value, target string) bool {
	for _, word := range strings.Fields(value) {
		if word == target {
			return true
		}
	}
	return false
}

func serializeStyles(styles map[string]string) string {
	keys := make([]string, 0, len(styles))
	for key := range styles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+":"+styles[key])
	}
	return strings.Join(parts, ";")
}

var allowedHTMLTags = map[string]bool{
	"section": true, "p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "ul": true, "ol": true, "li": true, "table": true, "thead": true, "tbody": true,
	"tr": true, "th": true, "td": true, "a": true, "img": true, "code": true, "pre": true, "strong": true,
	"em": true, "hr": true, "br": true, "del": true, "sup": true, "sub": true,
}

func sanitizeTree(root *html.Node) error {
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type == html.CommentNode {
			node.Parent.RemoveChild(node)
			return nil
		}
		if node.Type == html.ElementNode {
			if !allowedHTMLTags[node.Data] {
				return fmt.Errorf("微信 HTML 包含不安全元素: %s", node.Data)
			}
			attributes := node.Attr[:0]
			for _, attribute := range node.Attr {
				switch attribute.Key {
				case "style", "alt", "title":
					attributes = append(attributes, attribute)
				case "href":
					if !safeURL(attribute.Val, true) {
						return fmt.Errorf("微信链接 URL 不安全")
					}
					attributes = append(attributes, attribute)
				case "src":
					if !safeURL(attribute.Val, false) {
						return fmt.Errorf("微信图片 URL 不安全")
					}
					attributes = append(attributes, attribute)
				}
			}
			node.Attr = attributes
		}
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			if err := walk(child); err != nil {
				return err
			}
			child = next
		}
		return nil
	}
	return walk(root)
}

func safeURL(value string, allowMail bool) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	if parsed.Scheme == "https" || parsed.Scheme == "http" {
		return parsed.Host != ""
	}
	return allowMail && parsed.Scheme == "mailto"
}
