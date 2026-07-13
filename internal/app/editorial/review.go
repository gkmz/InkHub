package editorial

import (
	"context"
	"fmt"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	domaineditorial "github.com/gkmz/InkHub/internal/domain/editorial"
)

// Review 是文章当前人工审核投影。
type Review struct {
	ArticleID               string
	State                   domaineditorial.State
	ApprovedContentHash     string
	ApprovedFrontmatterHash string
	ApprovedAt              time.Time
}

// ReviewStore 保存人工审核投影。
type ReviewStore interface {
	Save(ctx context.Context, review Review) error
}

// ReviewArticle 校验文章与检查结果并记录当前内容版本已审核。
func ReviewArticle(ctx context.Context, store ReviewStore, value article.Article, findings []Finding) (Review, error) {
	if err := value.StableID.Validate(); err != nil {
		return Review{}, err
	}
	if value.ContentHash == "" || value.FrontmatterHash == "" {
		return Review{}, fmt.Errorf("文章内容版本不完整")
	}
	for _, finding := range findings {
		if finding.Severity == SeverityBlocking {
			return Review{}, fmt.Errorf("存在阻断检查项: %s", finding.Code)
		}
	}
	review := Review{
		ArticleID: value.ID, State: domaineditorial.StateApproved,
		ApprovedContentHash: value.ContentHash, ApprovedFrontmatterHash: value.FrontmatterHash,
		ApprovedAt: time.Now().UTC(),
	}
	if err := store.Save(ctx, review); err != nil {
		return Review{}, fmt.Errorf("保存审核记录: %w", err)
	}
	return review, nil
}
