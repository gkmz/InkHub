package hugo

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// DiscoverSections 扫描 Hugo content 下可用于发布的真实一级目录。
func (p *Provider) DiscoverSections(ctx context.Context, sourceID string) (contracts.SectionDiscovery, error) {
	if err := ctx.Err(); err != nil {
		return contracts.SectionDiscovery{}, err
	}
	contentRoot := filepath.Join(p.config.Root, "content")
	entries, err := os.ReadDir(contentRoot)
	if os.IsNotExist(err) {
		return contracts.SectionDiscovery{Sections: []contracts.PublishSection{}}, nil
	}
	if err != nil {
		return contracts.SectionDiscovery{}, providerError("hugo.content_unavailable", "Hugo content 目录不可用", contracts.ErrorValidation, false, err)
	}
	sections := make([]contracts.PublishSection, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !safeSegment(name) {
			continue
		}
		path := filepath.Join(contentRoot, name)
		if !withinOrEqual(path, contentRoot) {
			continue
		}
		count, countErr := countMarkdown(path)
		if countErr != nil {
			return contracts.SectionDiscovery{}, countErr
		}
		directories, directoryErr := discoverBundleDirectories(path)
		if directoryErr != nil {
			return contracts.SectionDiscovery{}, directoryErr
		}
		sections = append(sections, contracts.PublishSection{Name: name, ArticleCount: count, Directories: directories})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].Name < sections[j].Name })
	result := contracts.SectionDiscovery{Sections: sections}
	if sourceID != "" {
		target, section, found, findErr := findBundleBySourceID(p.config.Root, sourceID)
		if findErr != nil {
			return contracts.SectionDiscovery{}, findErr
		}
		if found {
			result.ExistingSection, result.ExistingTarget, result.SelectionLocked = section, target, true
			result.ExistingDirectory = bundleDirectory(p.config.Root, section, target)
		}
	}
	return result, nil
}

// discoverBundleDirectories 只返回 Section 直属的容器目录，排除直属文章 Bundle。
func discoverBundleDirectories(sectionRoot string) ([]contracts.PublishDirectory, error) {
	entries, err := os.ReadDir(sectionRoot)
	if err != nil {
		return nil, err
	}
	directories := make([]contracts.PublishDirectory, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !safeSegment(name) {
			continue
		}
		path := filepath.Join(sectionRoot, name)
		if info, statErr := os.Stat(filepath.Join(path, "index.md")); statErr == nil && !info.IsDir() {
			continue
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		count, countErr := countMarkdown(path)
		if countErr != nil {
			return nil, countErr
		}
		directories = append(directories, contracts.PublishDirectory{Path: name, ArticleCount: count})
	}
	sort.Slice(directories, func(i, j int) bool { return directories[i].Path < directories[j].Path })
	return directories, nil
}

// bundleDirectory 返回已有 Bundle 相对于 Section 的容器目录。
func bundleDirectory(root, section, target string) string {
	relative, err := filepath.Rel(filepath.Join(root, "content", section), target)
	if err != nil {
		return ""
	}
	directory := filepath.Dir(relative)
	if directory == "." || directory == string(filepath.Separator) {
		return ""
	}
	return filepath.ToSlash(directory)
}

func countMarkdown(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			count++
		}
		return nil
	})
	return count, err
}
