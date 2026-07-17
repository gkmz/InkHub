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
		sections = append(sections, contracts.PublishSection{Name: name, ArticleCount: count})
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
		}
	}
	return result, nil
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
