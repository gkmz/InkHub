package template

import (
	"fmt"
	"strconv"
	"strings"
)

var safeFontStacks = map[string]string{
	"system-sans":  `-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif`,
	"system-serif": `Georgia,"Times New Roman",serif`,
	"system-mono":  `ui-monospace,SFMono-Regular,Menlo,monospace`,
}

// ResolveCSS 校验用户变量并生成不含占位符的确定性 CSS。
func ResolveCSS(validated Validated, values map[string]any) (string, error) {
	css := validated.CSS
	for name, variable := range validated.Manifest.Variables {
		value := variable.Default
		if provided, exists := values[name]; exists {
			value = provided
		}
		resolved, err := resolveVariable(variable, value)
		if err != nil {
			return "", fmt.Errorf("解析模板变量 %s: %w", name, err)
		}
		token := "{{ " + variable.Type + "." + name + " }}"
		css = strings.ReplaceAll(css, token, resolved)
	}
	if strings.Contains(css, "{{") {
		return "", fmt.Errorf("模板 CSS 存在未解析变量")
	}
	return css, nil
}

func resolveVariable(variable Variable, value any) (string, error) {
	switch variable.Type {
	case "color":
		text, ok := value.(string)
		if !ok || !regexpColor.MatchString(text) {
			return "", fmt.Errorf("颜色值无效")
		}
		return strings.ToLower(text), nil
	case "font-family":
		text, ok := value.(string)
		stack, exists := safeFontStacks[text]
		if !ok || !exists || !contains(variable.Options, text) {
			return "", fmt.Errorf("字体 token 无效")
		}
		return stack, nil
	case "enum":
		text, ok := value.(string)
		if !ok || !contains(variable.Options, text) {
			return "", fmt.Errorf("枚举值无效")
		}
		return text, nil
	case "font-size", "spacing":
		number, ok := numericValue(value)
		if !ok || variable.Min == nil || variable.Max == nil || number < *variable.Min || number > *variable.Max {
			return "", fmt.Errorf("数值超出范围")
		}
		return strconv.FormatFloat(number, 'f', -1, 64) + variable.Unit, nil
	case "boolean":
		enabled, ok := value.(bool)
		if !ok {
			return "", fmt.Errorf("布尔值无效")
		}
		if enabled {
			return variable.TrueValue, nil
		}
		return "none", nil
	default:
		return "", fmt.Errorf("不支持变量类型")
	}
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case float64:
		return number, true
	default:
		return 0, false
	}
}
