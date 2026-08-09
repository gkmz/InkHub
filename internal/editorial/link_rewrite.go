package editorial

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var wikiLinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

type sourceSpan struct{ start, stop int }

func rewriteWikiLinks(body string, resolve func(target, alias string) linkReplacement) LinkResult {
	spans, protected := markdownTextSpans([]byte(body))
	if len(spans) == 0 {
		return LinkResult{Body: body}
	}
	var output strings.Builder
	links := make([]LinkOutcome, 0)
	position := 0
	for _, span := range spans {
		if span.start < position || span.stop <= span.start || span.stop > len(body) {
			continue
		}
		output.WriteString(body[position:span.start])
		textValue := body[span.start:span.stop]
		matches := wikiLinkPattern.FindAllStringSubmatchIndex(textValue, -1)
		textPosition := 0
		for _, match := range matches {
			start, stop := match[0], match[1]
			globalStart, globalStop := span.start+start, span.start+stop
			if (start > 0 && textValue[start-1] == '!') || (globalStart > 0 && body[globalStart-1] == '!') || overlapsProtected(globalStart, globalStop, protected) {
				continue
			}
			output.WriteString(textValue[textPosition:start])
			target := strings.TrimSpace(textValue[match[2]:match[3]])
			alias := ""
			if match[4] >= 0 {
				alias = strings.TrimSpace(textValue[match[4]:match[5]])
			}
			replacement := resolve(target, alias)
			output.WriteString(replacement.Text)
			links = append(links, LinkOutcome{Target: target, Label: DefaultLabel(target, alias), Status: replacement.Status, Blocking: replacement.Blocking})
			textPosition = stop
		}
		output.WriteString(textValue[textPosition:])
		position = span.stop
	}
	output.WriteString(body[position:])
	return LinkResult{Body: output.String(), Links: links, Diagnostics: linkDiagnostics(links)}
}

func markdownTextSpans(source []byte) ([]sourceSpan, []sourceSpan) {
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	spans, protected := make([]sourceSpan, 0), make([]sourceSpan, 0)
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Goldmark 使用 TextBlock 承载列表项正文，必须和普通段落、标题一起纳入 WikiLink 转换。
		if node.Kind() == ast.KindParagraph || node.Kind() == ast.KindHeading || node.Kind() == ast.KindTextBlock {
			lines := node.Lines()
			for index := 0; lines != nil && index < lines.Len(); index++ {
				segment := lines.At(index)
				if segment.Start < segment.Stop {
					spans = append(spans, sourceSpan{start: segment.Start, stop: segment.Stop})
				}
			}
		}
		if node.Kind() == ast.KindCodeSpan || node.Kind() == ast.KindLink || node.Kind() == ast.KindImage || node.Kind() == ast.KindAutoLink || node.Kind() == ast.KindRawHTML {
			if span, found := inlineNodeSpan(node); found {
				protected = append(protected, span)
			}
		}
		return ast.WalkContinue, nil
	})
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	sort.Slice(protected, func(i, j int) bool { return protected[i].start < protected[j].start })
	return spans, protected
}

func inlineNodeSpan(node ast.Node) (sourceSpan, bool) {
	start, stop := -1, -1
	_ = ast.Walk(node, func(current ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if textNode, ok := current.(*ast.Text); ok {
			segment := textNode.Segment
			if start < 0 || segment.Start < start {
				start = segment.Start
			}
			if segment.Stop > stop {
				stop = segment.Stop
			}
		}
		if rawHTML, ok := current.(*ast.RawHTML); ok {
			for index := 0; index < rawHTML.Segments.Len(); index++ {
				segment := rawHTML.Segments.At(index)
				if start < 0 || segment.Start < start {
					start = segment.Start
				}
				if segment.Stop > stop {
					stop = segment.Stop
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return sourceSpan{start: start, stop: stop}, start >= 0 && stop > start
}

func overlapsProtected(start, stop int, protected []sourceSpan) bool {
	for _, span := range protected {
		if span.start >= stop {
			return false
		}
		if span.stop > start && span.start < stop {
			return true
		}
	}
	return false
}

func resolveIndexedLink(ctx context.Context, resolver LinkResolver, target string) (LinkResolution, LinkStatus) {
	if resolver == nil {
		return LinkResolution{}, LinkStatusUnavailable
	}
	resolution, err := resolver.Resolve(ctx, target)
	if err != nil {
		var ambiguous *AmbiguousLinkError
		if errors.As(err, &ambiguous) {
			return LinkResolution{}, LinkStatusAmbiguous
		}
		return LinkResolution{}, LinkStatusUnavailable
	}
	if !resolution.Found {
		return LinkResolution{}, LinkStatusMissing
	}
	if strings.TrimSpace(resolution.StableID) == "" {
		return resolution, LinkStatusUnpublished
	}
	return resolution, LinkStatusConverted
}

func linkDiagnostics(links []LinkOutcome) []contracts.Diagnostic {
	converted := 0
	issues := make([]contracts.Diagnostic, 0)
	seen := make(map[string]bool)
	for _, link := range links {
		if link.Status == LinkStatusConverted {
			converted++
			continue
		}
		key := string(link.Status) + "\x00" + link.Target
		if seen[key] {
			continue
		}
		seen[key] = true
		code, message := linkDiagnosticText(link)
		issues = append(issues, contracts.Diagnostic{Code: code, Message: message, Blocking: link.Blocking})
	}
	diagnostics := make([]contracts.Diagnostic, 0, len(issues)+1)
	if converted > 0 {
		diagnostics = append(diagnostics, contracts.Diagnostic{Code: "internal_link.converted", Message: fmt.Sprintf("已转换 %d 个内部链接", converted)})
	}
	return append(diagnostics, issues...)
}

func linkDiagnosticText(link LinkOutcome) (string, string) {
	switch link.Status {
	case LinkStatusUnpublished:
		return "internal_link.unpublished", fmt.Sprintf("内部链接“%s”的目标尚未发布，已保留为纯文本", link.Label)
	case LinkStatusMissing:
		return "internal_link.missing", fmt.Sprintf("内部链接“%s”未找到目标文章，已保留为纯文本", link.Label)
	case LinkStatusAmbiguous:
		return "internal_link.ambiguous", fmt.Sprintf("内部链接“%s”匹配到多篇文章，请在 Obsidian 中改为完整路径", link.Label)
	default:
		return "internal_link.unavailable", fmt.Sprintf("内部链接“%s”暂时无法生成渠道链接，已保留为纯文本", link.Label)
	}
}

func linkDocumentTarget(target string) string {
	target = strings.TrimSpace(target)
	if index := strings.IndexAny(target, "#^"); index >= 0 {
		target = target[:index]
	}
	return strings.TrimSpace(target)
}

func markdownLabel(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)
	return replacer.Replace(strings.TrimSpace(value))
}
