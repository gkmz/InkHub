package folder

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Scope 表示用户明确授权的内容目录和其中的忽略目录。
type Scope struct {
	contentRoots     []string
	ignoredFolders   []string
	ignoredFileNames map[string]bool
}

// NewScope 校验、规范化并合并可解释的 Vault 相对目录规则。
func NewScope(contentRoots, ignoredFolders []string) (Scope, error) {
	return NewScopeWithFileNames(contentRoots, ignoredFolders, DefaultIgnoredFileNames())
}

// DefaultIgnoredFileNames 返回新工作区默认跳过的索引文件名。
func DefaultIgnoredFileNames() []string {
	return []string{"index.md", "_index.md"}
}

// NewScopeWithFileNames 创建包含目录和精确文件名规则的内容范围。
func NewScopeWithFileNames(contentRoots, ignoredFolders, ignoredFileNames []string) (Scope, error) {
	roots, err := normalizeRules(contentRoots)
	if err != nil {
		return Scope{}, fmt.Errorf("内容目录: %w", err)
	}
	roots = removeNestedRules(roots)
	ignored, err := normalizeRules(ignoredFolders)
	if err != nil {
		return Scope{}, fmt.Errorf("忽略目录: %w", err)
	}
	for _, candidate := range ignored {
		valid := false
		for _, root := range roots {
			if candidate != root && isWithin(candidate, root) {
				valid = true
				break
			}
		}
		if !valid {
			return Scope{}, fmt.Errorf("%q 必须位于某个内容目录内部", candidate)
		}
	}
	fileNames := make(map[string]bool, len(ignoredFileNames))
	for _, value := range ignoredFileNames {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" || name != filepath.Base(name) || !strings.EqualFold(filepath.Ext(name), ".md") {
			return Scope{}, fmt.Errorf("忽略文件名 %q 必须是完整 Markdown 文件名", value)
		}
		fileNames[name] = true
	}
	return Scope{contentRoots: roots, ignoredFolders: removeNestedRules(ignored), ignoredFileNames: fileNames}, nil
}

// ContentRoots 返回规范化且已移除冗余子目录的内容目录。
func (s Scope) ContentRoots() []string {
	return append(make([]string, 0, len(s.contentRoots)), s.contentRoots...)
}

// IgnoredFolders 返回规范化且已移除冗余子目录的忽略目录。
func (s Scope) IgnoredFolders() []string {
	return append(make([]string, 0, len(s.ignoredFolders)), s.ignoredFolders...)
}

// IgnoredFileNames 返回稳定排序的精确忽略文件名。
func (s Scope) IgnoredFileNames() []string {
	result := make([]string, 0, len(s.ignoredFileNames))
	for name := range s.ignoredFileNames {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// Includes 判断 Vault 相对文件是否属于用户授权的内容范围。
func (s Scope) Includes(relativePath string) bool {
	candidate, err := normalizeRelative(relativePath)
	if err != nil || containsSystemDirectory(candidate) {
		return false
	}
	if s.ignoredFileNames[strings.ToLower(path.Base(candidate))] {
		return false
	}
	included := false
	for _, root := range s.contentRoots {
		if isWithin(candidate, root) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, ignored := range s.ignoredFolders {
		if isWithin(candidate, ignored) {
			return false
		}
	}
	return true
}

func normalizeRules(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		normalized, err := normalizeRelative(value)
		if err != nil {
			return nil, err
		}
		if containsSystemDirectory(normalized) {
			return nil, fmt.Errorf("系统目录 %q 不能配置", value)
		}
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result, nil
}

func normalizeRelative(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "." || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return "", fmt.Errorf("%q 必须是非空 Vault 相对路径", value)
	}
	normalized := path.Clean(value)
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("%q 超出 Vault", value)
	}
	return normalized, nil
}

func removeNestedRules(values []string) []string {
	result := make([]string, 0, len(values))
	for _, candidate := range values {
		redundant := false
		for _, parent := range result {
			if isWithin(candidate, parent) {
				redundant = true
				break
			}
		}
		if !redundant {
			result = append(result, candidate)
		}
	}
	return result
}

func isWithin(candidate, directory string) bool {
	return candidate == directory || strings.HasPrefix(candidate, directory+"/")
}

func containsSystemDirectory(relative string) bool {
	for _, segment := range strings.Split(relative, "/") {
		if segment == ".obsidian" || segment == ".git" || segment == ".trash" || strings.HasPrefix(segment, ".inkhub-") {
			return true
		}
	}
	return false
}
