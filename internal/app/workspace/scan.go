// Package workspace 编排工作区初始化和内容扫描用例。
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Source 提供工作区扫描需要的内容源能力。
type Source interface {
	Scan(ctx context.Context, cursor contracts.ScanCursor) (contracts.ScanResult, error)
	Read(ctx context.Context, ref contracts.SourceRef) (contracts.SourceDocument, error)
}

// ArticleStore 保存可重建文章索引。
type ArticleStore interface {
	Upsert(ctx context.Context, value article.Article) error
	MarkMissing(ctx context.Context, workspaceID, sourceID string, seenPaths []string) error
}

// ScanReport 汇总一次扫描结果。
type ScanReport struct {
	Indexed int
	Failed  int
	Next    contracts.ScanCursor
}

// ScanOptions 提供索引文章所需的工作区上下文。
type ScanOptions struct {
	WorkspaceID string
	SourceID    string
}

// ScanWorkspace 扫描内容源并独立更新每篇有效文章。
func ScanWorkspace(ctx context.Context, source Source, store ArticleStore, options ScanOptions, cursor contracts.ScanCursor) (ScanReport, error) {
	if options.WorkspaceID == "" {
		return ScanReport{}, fmt.Errorf("工作区 ID 不能为空")
	}
	result, err := source.Scan(ctx, cursor)
	if err != nil {
		return ScanReport{}, fmt.Errorf("扫描工作区: %w", err)
	}
	report := ScanReport{Next: result.Next}
	seenPaths := make([]string, 0, len(result.Documents))
	for _, reference := range result.Documents {
		seenPaths = append(seenPaths, reference.Ref.RelativePath)
		if hasBlockingDiagnostic(reference.Diagnostics) {
			report.Failed++
			continue
		}
		document, err := source.Read(ctx, reference.Ref)
		if err != nil {
			report.Failed++
			continue
		}
		document.Article.WorkspaceID = options.WorkspaceID
		document.Article.SourceFingerprint = document.Fingerprint
		document.Article.ID = articleIndexID(document.Article.SourceID, string(document.Article.StableID), document.Article.RelativePath)
		document.Article.ContentHash, err = article.NormalizeAndHash(article.HashInput{
			Body: document.Body, Title: document.Article.Title, Description: document.Article.Description,
			Tags: document.Article.Tags, Keywords: document.Article.Keywords, Category: document.Article.Category,
			Series: document.Article.Series, URL: document.Article.URL, PublishDate: document.Article.PublishDate,
			Slug: document.Article.Slug, Cover: document.Article.Cover,
		})
		if err != nil {
			report.Failed++
			continue
		}
		document.Article.FrontmatterHash, err = article.NormalizeAndHash(article.HashInput{
			Title: document.Article.Title, Description: document.Article.Description, Tags: document.Article.Tags,
			Keywords: document.Article.Keywords, Category: document.Article.Category, Series: document.Article.Series,
			URL: document.Article.URL, PublishDate: document.Article.PublishDate, Slug: document.Article.Slug, Cover: document.Article.Cover,
		})
		if err != nil {
			report.Failed++
			continue
		}
		if err := store.Upsert(ctx, document.Article); err != nil {
			report.Failed++
			continue
		}
		report.Indexed++
	}
	unchanged := cursor.Revision != "" && cursor.Revision == result.Next.Revision && len(result.Documents) == 0
	if result.Complete && !unchanged {
		if err := store.MarkMissing(ctx, options.WorkspaceID, options.SourceID, seenPaths); err != nil {
			return report, fmt.Errorf("更新文章索引范围: %w", err)
		}
	}
	return report, nil
}

func articleIndexID(sourceID, stableID, relativePath string) string {
	identity := stableID
	if identity == "" {
		identity = relativePath
	}
	sum := sha256.Sum256([]byte(sourceID + "\x00" + identity))
	return "idx_" + hex.EncodeToString(sum[:16])
}

func hasBlockingDiagnostic(diagnostics []contracts.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Blocking {
			return true
		}
	}
	return false
}
