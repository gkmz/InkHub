package template

import (
	"fmt"
	"regexp"
	"strings"
)

var allowedCSSProperties = map[string]bool{
	"color": true, "background-color": true, "font-family": true, "font-size": true, "font-style": true,
	"font-weight": true, "line-height": true, "text-align": true, "text-decoration": true, "white-space": true,
	"word-break": true, "overflow-wrap": true, "margin": true, "margin-top": true, "margin-right": true,
	"margin-bottom": true, "margin-left": true, "padding": true, "padding-top": true, "padding-right": true,
	"padding-bottom": true, "padding-left": true, "border": true, "border-width": true, "border-style": true,
	"border-color": true, "border-top": true, "border-right": true, "border-bottom": true, "border-left": true,
	"border-radius": true, "width": true, "max-width": true, "min-width": true, "height": true, "display": true,
	"vertical-align": true, "list-style-type": true, "border-collapse": true, "border-spacing": true, "box-sizing": true,
	"tab-size": true,
}

var cssRulePattern = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)

func validateCSS(css string, variables map[string]Variable) error {
	clean := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	if strings.Contains(clean, "@") || strings.Count(clean, "{") != strings.Count(clean, "}") {
		return fmt.Errorf("模板 CSS 包含禁止 at-rule 或结构无效")
	}
	matches := cssRulePattern.FindAllStringSubmatch(clean, -1)
	if len(matches) == 0 {
		return fmt.Errorf("模板 CSS 没有有效规则")
	}
	for _, match := range matches {
		for _, selector := range strings.Split(match[1], ",") {
			selector = strings.TrimSpace(selector)
			if !strings.HasPrefix(selector, ".inkhub-root") || strings.ContainsAny(selector, "#*>+~:[]") {
				return fmt.Errorf("模板 CSS selector 不安全: %s", selector)
			}
		}
		for _, declaration := range strings.Split(match[2], ";") {
			declaration = strings.TrimSpace(declaration)
			if declaration == "" {
				continue
			}
			parts := strings.SplitN(declaration, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("模板 CSS 声明无效")
			}
			property := strings.TrimSpace(strings.ToLower(parts[0]))
			value := strings.TrimSpace(parts[1])
			if !allowedCSSProperties[property] {
				return fmt.Errorf("模板 CSS 属性不允许: %s", property)
			}
			lower := strings.ToLower(value)
			for _, forbidden := range []string{"url(", "javascript:", "expression(", "var(", "env(", "calc(", "!important"} {
				if strings.Contains(lower, forbidden) {
					return fmt.Errorf("模板 CSS 值不安全: %s", property)
				}
			}
			if strings.Contains(value, "{{") {
				if err := validatePlaceholder(property, value, variables); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validatePlaceholder(property, value string, variables map[string]Variable) error {
	match := regexp.MustCompile(`^\{\{ ([a-z-]+)\.([A-Za-z][A-Za-z0-9]*) \}\}$`).FindStringSubmatch(value)
	if match == nil {
		return fmt.Errorf("模板 CSS 占位符必须占据完整属性值")
	}
	variable, exists := variables[match[2]]
	if !exists || variable.Type != match[1] {
		return fmt.Errorf("模板 CSS 引用了未声明或类型不匹配的变量")
	}
	if variable.Type == "boolean" && property != "display" {
		return fmt.Errorf("boolean 变量只能用于 display")
	}
	return nil
}
