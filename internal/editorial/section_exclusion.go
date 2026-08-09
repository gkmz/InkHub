package editorial

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ExcludedSection 描述一次发布转换中实际移除的同名章节。
type ExcludedSection struct {
	Title       string
	Occurrences int
	BlockCount  int
}

// SectionExclusionResult 保存裁剪后的 Markdown 和可供渠道预览展示的诊断数据。
type SectionExclusionResult struct {
	Body     string
	Excluded []ExcludedSection
}

// ApplyPublicationSectionExclusions 在渠道转换前裁剪正文，并过滤随章节移除的资源与图片诊断。
func ApplyPublicationSectionExclusions(ctx context.Context, db *sql.DB, workspaceID string, document contracts.SourceDocument) (contracts.SourceDocument, []ExcludedSection, error) {
	settings, err := LoadPublicationSettings(ctx, db, workspaceID)
	if err != nil {
		return contracts.SourceDocument{}, nil, err
	}
	result := ExcludePublishSections(document.Body, settings.ExcludedSections)
	document.Body = result.Body
	if len(result.Excluded) == 0 {
		return document, nil, nil
	}
	document.ResourceRefs = filterResourceRefs(document.ResourceRefs, document.Body)
	document.Diagnostics = filterImageDiagnostics(document.Diagnostics, document.Body)
	for _, item := range result.Excluded {
		message := fmt.Sprintf("发布时已排除章节：%s（包含 %d 个内容块）", item.Title, item.BlockCount)
		if item.Occurrences > 1 {
			message = fmt.Sprintf("发布时已排除章节：%s（%d 处，包含 %d 个内容块）", item.Title, item.Occurrences, item.BlockCount)
		}
		document.Diagnostics = append(document.Diagnostics, contracts.Diagnostic{Code: "publish.section_excluded", Message: message, Blocking: false})
	}
	return document, result.Excluded, nil
}

func filterResourceRefs(values []contracts.ResourceRef, body string) []contracts.ResourceRef {
	result := make([]contracts.ResourceRef, 0, len(values))
	for _, value := range values {
		if strings.Contains(body, value.Original) {
			result = append(result, value)
		}
	}
	return result
}

func filterImageDiagnostics(values []contracts.Diagnostic, body string) []contracts.Diagnostic {
	result := make([]contracts.Diagnostic, 0, len(values))
	for _, value := range values {
		if value.Code == "source.image_unresolved" {
			separator := strings.LastIndex(value.Message, ":")
			if separator >= 0 && !strings.Contains(body, strings.TrimSpace(value.Message[separator+1:])) {
				continue
			}
		}
		result = append(result, value)
	}
	return result
}

type sectionRange struct {
	start  int
	end    int
	title  string
	blocks int
}

// ExcludePublishSections 按标题精确匹配发布时排除的章节，并连同全部子章节裁剪。
func ExcludePublishSections(body string, titles []string) SectionExclusionResult {
	normalized := normalizeExcludedTitles(titles)
	if strings.TrimSpace(body) == "" || len(normalized) == 0 {
		return SectionExclusionResult{Body: body}
	}
	source := []byte(body)
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	blocks := rootBlocks(document)
	ranges := make([]sectionRange, 0)
	for index, node := range blocks {
		heading, ok := node.(*ast.Heading)
		if !ok || heading.Lines().Len() == 0 {
			continue
		}
		title := markdownHeadingText(heading, source)
		if _, matched := normalized[title]; !matched {
			continue
		}
		endIndex := len(blocks)
		for next := index + 1; next < len(blocks); next++ {
			candidate, isHeading := blocks[next].(*ast.Heading)
			if isHeading && candidate.Level <= heading.Level {
				endIndex = next
				break
			}
		}
		end := len(source)
		if endIndex < len(blocks) && blocks[endIndex].Lines().Len() > 0 {
			end = markdownLineStart(source, blocks[endIndex].Lines().At(0).Start)
		}
		start := markdownLineStart(source, heading.Lines().At(0).Start)
		blockCount := endIndex - index - 1
		// 紧邻章节标题的水平线属于章节分隔装饰，裁剪章节时不能把它单独留在正文末尾。
		if ruleStart, found := adjacentHorizontalRule(source, start); found {
			start = ruleStart
			blockCount++
		}
		ranges = append(ranges, sectionRange{
			start:  start,
			end:    end,
			title:  title,
			blocks: blockCount,
		})
	}
	if len(ranges) == 0 {
		return SectionExclusionResult{Body: body}
	}

	// 父章节命中时，其范围会覆盖内部同名子章节；合并范围避免重复裁剪和重复计数。
	merged := mergeSectionRanges(ranges)
	var output strings.Builder
	last := 0
	for _, item := range merged {
		output.Write(source[last:item.start])
		last = item.end
	}
	output.Write(source[last:])
	return SectionExclusionResult{Body: output.String(), Excluded: summarizeExcludedSections(ranges, merged)}
}

func markdownLineStart(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	for offset > 0 && source[offset-1] != '\n' {
		offset--
	}
	return offset
}

func adjacentHorizontalRule(source []byte, headingStart int) (int, bool) {
	lineEnd := headingStart
	for lineEnd > 0 && (source[lineEnd-1] == ' ' || source[lineEnd-1] == '\t' || source[lineEnd-1] == '\n' || source[lineEnd-1] == '\r') {
		lineEnd--
	}
	lineStart := markdownLineStart(source, lineEnd)
	line := strings.TrimSpace(string(source[lineStart:lineEnd]))
	if len(line) < 3 || (line[0] != '-' && line[0] != '*' && line[0] != '_') {
		return 0, false
	}
	for _, character := range line {
		if character != rune(line[0]) {
			return 0, false
		}
	}
	return lineStart, true
}

func markdownHeadingText(heading *ast.Heading, source []byte) string {
	var value strings.Builder
	_ = ast.Walk(heading, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if item, ok := node.(*ast.Text); ok {
			value.Write(item.Segment.Value(source))
			if item.SoftLineBreak() || item.HardLineBreak() {
				value.WriteByte(' ')
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(value.String())
}

func normalizeExcludedTitles(titles []string) map[string]struct{} {
	result := make(map[string]struct{}, len(titles))
	for _, title := range titles {
		if value := strings.TrimSpace(title); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func rootBlocks(document ast.Node) []ast.Node {
	result := make([]ast.Node, 0)
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		result = append(result, node)
	}
	return result
}

func mergeSectionRanges(ranges []sectionRange) []sectionRange {
	merged := make([]sectionRange, 0, len(ranges))
	for _, item := range ranges {
		if len(merged) > 0 && item.start < merged[len(merged)-1].end {
			continue
		}
		merged = append(merged, item)
	}
	return merged
}

func summarizeExcludedSections(all, applied []sectionRange) []ExcludedSection {
	appliedStarts := make(map[int]struct{}, len(applied))
	for _, item := range applied {
		appliedStarts[item.start] = struct{}{}
	}
	order := make([]string, 0)
	summary := make(map[string]*ExcludedSection)
	for _, item := range all {
		if _, ok := appliedStarts[item.start]; !ok {
			continue
		}
		entry := summary[item.title]
		if entry == nil {
			order = append(order, item.title)
			entry = &ExcludedSection{Title: item.title}
			summary[item.title] = entry
		}
		entry.Occurrences++
		entry.BlockCount += item.blocks
	}
	result := make([]ExcludedSection, 0, len(order))
	for _, title := range order {
		result = append(result, *summary[title])
	}
	return result
}
