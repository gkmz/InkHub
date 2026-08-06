// Package wechat 实现微信公众号安全渲染和人工复制交付。
package wechat

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	contentmarkdown "github.com/gkmz/InkHub/internal/content/markdown"
	domaintemplate "github.com/gkmz/InkHub/internal/domain/template"
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
	engine := contentmarkdown.NewRenderer()
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
	// 微信正文不支持可靠的外链跳转，因此把外链同步整理为文末引用。
	appendLinkReferences(contextNode)
	if err := sanitizeTree(contextNode); err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := html.Render(&output, contextNode); err != nil {
		return "", fmt.Errorf("序列化微信 HTML: %w", err)
	}
	return output.String(), nil
}

type linkReference struct {
	index int
	text  string
	href  string
}

// appendLinkReferences 为正文外链追加编号，并在文章末尾生成引用章节。
func appendLinkReferences(root *html.Node) {
	links := collectReferenceLinks(root)
	if len(links) == 0 {
		return
	}
	references := make([]linkReference, 0, len(links))
	for index, link := range links {
		reference := linkReference{index: index + 1, text: strings.TrimSpace(nodeText(link)), href: attributeValue(link, "href")}
		if reference.text == "" {
			reference.text = reference.href
		}
		references = append(references, reference)
		// 微信正文不保留不可用的跳转，只保留链接文字和引用编号。
		removeAttribute(link, "href")
		sup := elementNode("sup", "margin-left:3px;color:#42b883;font-size:0.72em;font-weight:700;line-height:0;vertical-align:super")
		sup.AppendChild(&html.Node{Type: html.TextNode, Data: fmt.Sprintf("[%d]", reference.index)})
		link.Parent.InsertBefore(sup, link.NextSibling)
	}
	root.AppendChild(referenceSection(references))
}

func collectReferenceLinks(root *html.Node) []*html.Node {
	var links []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" && isReferenceLink(node) {
			links = append(links, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return links
}

func isReferenceLink(node *html.Node) bool {
	href := strings.TrimSpace(attributeValue(node, "href"))
	lower := strings.ToLower(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(lower, "javascript:") || hasAncestor(node, "pre", "code") {
		return false
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "img" {
			return false
		}
	}
	return true
}

func referenceSection(references []linkReference) *html.Node {
	section := elementNode("section", "margin-top:38px;padding:12px 16px;background-color:#f7fcf9;border-left:4px solid #42b883;border-radius:0 8px 8px 0")
	title := elementNode("h3", "margin:0 0 10px;padding:0;border:0;color:#1a2733;font-size:16px;font-weight:600")
	title.AppendChild(&html.Node{Type: html.TextNode, Data: "引用链接"})
	section.AppendChild(title)
	list := elementNode("ul", "margin:0;padding-left:0;list-style-type:none")
	for _, reference := range references {
		item := elementNode("li", "display:block;margin:7px 0;color:#5c6975;font-size:14px;line-height:1.55;word-break:break-word")
		label := elementNode("span", "color:#42b883;font-weight:700;white-space:nowrap")
		// 不换行空格把编号与引用标题首字绑定，避免编号孤立在上一行。
		label.AppendChild(&html.Node{Type: html.TextNode, Data: fmt.Sprintf("[%d]\u00a0", reference.index)})
		item.AppendChild(label)
		item.AppendChild(&html.Node{Type: html.TextNode, Data: reference.text + ": "})
		value := elementNode("span", "color:#34495e;word-break:break-all")
		value.AppendChild(&html.Node{Type: html.TextNode, Data: reference.href})
		item.AppendChild(value)
		list.AppendChild(item)
	}
	section.AppendChild(list)
	return section
}

func elementNode(tag, style string) *html.Node {
	return &html.Node{Type: html.ElementNode, Data: tag, Attr: []html.Attribute{{Key: "style", Val: style}}}
}

func attributeValue(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func removeAttribute(node *html.Node, key string) {
	attributes := node.Attr[:0]
	for _, attribute := range node.Attr {
		if attribute.Key != key {
			attributes = append(attributes, attribute)
		}
	}
	node.Attr = attributes
}

func hasAncestor(node *html.Node, tags ...string) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		for _, tag := range tags {
			if parent.Type == html.ElementNode && parent.Data == tag {
				return true
			}
		}
	}
	return false
}

func nodeText(node *html.Node) string {
	var result strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			result.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return result.String()
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
	// 从右向左逐层匹配后代选择器，避免把 `pre code` 的代码块样式误用到行内 code。
	parent := node.Parent
	for index := len(parts) - 2; index >= 0; index-- {
		for parent != nil && !matchesSimpleSelector(parent, parts[index]) {
			parent = parent.Parent
		}
		if parent == nil {
			return false
		}
		parent = parent.Parent
	}
	return true
}

func matchesSimpleSelector(node *html.Node, selector string) bool {
	if selector == ".inkhub-root" {
		return hasClass(node, "inkhub-root")
	}
	return node.Type == html.ElementNode && node.Data == selector
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
	"span": true,
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
