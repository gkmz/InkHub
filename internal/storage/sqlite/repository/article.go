package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
)

// ArticleRepository 保存和查询文章索引。
type ArticleRepository struct {
	db *sql.DB
}

// NewArticleRepository 创建文章 Repository。
func NewArticleRepository(db *sql.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// Upsert 按内部文章 ID 新增或更新索引快照。
func (r *ArticleRepository) Upsert(ctx context.Context, value article.Article) error {
	tags, err := json.Marshal(value.Tags)
	if err != nil {
		return fmt.Errorf("序列化 Tags: %w", err)
	}
	keywords, err := json.Marshal(value.Keywords)
	if err != nil {
		return fmt.Errorf("序列化 Keywords: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	indexedAt := value.IndexedAt.UTC().Format(time.RFC3339Nano)
	if value.IndexedAt.IsZero() {
		indexedAt = now
	}
	contentStage := value.ContentStage
	if contentStage == "" {
		contentStage = article.ContentStageDraft
	}
	// 索引快照和审核失效属于同一个内容变化事实，必须在同一事务提交。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始文章索引事务: %w", err)
	}
	defer tx.Rollback()
	identityInvalid := value.StableID.Validate() != nil
	if value.StableID != "" {
		var existingID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE workspace_id=? AND source_id=? AND stable_id=?`, value.WorkspaceID, value.SourceID, value.StableID).Scan(&existingID)
		if err == nil {
			// Stable ID 是重命名和移动后的唯一身份来源，内部主键继续保持不变。
			value.ID = existingID
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("解析文章稳定身份: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO articles(
id,workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,
slug,cover,source_mtime,source_size,source_fingerprint,content_hash,body_hash,frontmatter_hash,indexed_at,deleted_at,created_at,updated_at,content_stage,content_stage_issue)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET stable_id=excluded.stable_id,relative_path=excluded.relative_path,title=excluded.title,
description=excluded.description,category=excluded.category,series=excluded.series,tags_json=excluded.tags_json,
keywords_json=excluded.keywords_json,slug=excluded.slug,cover=excluded.cover,source_mtime=excluded.source_mtime,
source_size=excluded.source_size,source_fingerprint=excluded.source_fingerprint,content_hash=excluded.content_hash,body_hash=excluded.body_hash,frontmatter_hash=excluded.frontmatter_hash,
indexed_at=excluded.indexed_at,deleted_at=excluded.deleted_at,content_stage=excluded.content_stage,content_stage_issue=excluded.content_stage_issue,updated_at=excluded.updated_at`,
		value.ID, value.WorkspaceID, value.SourceID, value.StableID, value.RelativePath, value.Title, value.Description,
		value.Category, value.Series, string(tags), string(keywords), value.Slug, value.Cover, formatTime(value.SourceMTime),
		value.SourceSize, value.SourceFingerprint, value.ContentHash, value.BodyHash, value.FrontmatterHash, indexedAt, formatTime(value.DeletedAt), now, now,
		contentStage, value.ContentStageIssue)
	if err != nil {
		return fmt.Errorf("保存文章索引: %w", err)
	}
	// 新记录按正文 hash 判断；旧记录或测试数据没有正文 hash 时回退到发布内容 hash。
	approvalChanged := "((approved_body_hash<>? AND approved_body_hash<>'') OR (approved_body_hash='' AND approved_content_hash<>?))"
	args := []any{now, value.ID, value.BodyHash, value.ContentHash, identityInvalid}
	if value.BodyHash == "" {
		approvalChanged = "approved_content_hash<>?"
		args = []any{now, value.ID, value.ContentHash, identityInvalid}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE editorial_reviews SET state='changed',updated_at=? WHERE article_id=? AND state='approved' AND (`+approvalChanged+` OR ?)`, args...); err != nil {
		return fmt.Errorf("使旧审核失效: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交文章索引事务: %w", err)
	}
	return nil
}

// MarkMissing 将完整扫描中未出现的文章软删除，源文件不会被修改。
func (r *ArticleRepository) MarkMissing(ctx context.Context, workspaceID, sourceID string, seenPaths []string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := `UPDATE articles SET deleted_at=?,updated_at=? WHERE workspace_id=? AND source_id=? AND deleted_at IS NULL`
	args := []any{now, now, workspaceID, sourceID}
	if len(seenPaths) > 0 {
		placeholders := make([]string, len(seenPaths))
		for index, relativePath := range seenPaths {
			placeholders[index] = "?"
			args = append(args, relativePath)
		}
		query += ` AND relative_path NOT IN (` + strings.Join(placeholders, ",") + `)`
	}
	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("软删除未见文章: %w", err)
	}
	return nil
}

// FindByStableID 按工作区和稳定 ID 查询文章。
func (r *ArticleRepository) FindByStableID(ctx context.Context, workspaceID, stableID string) (article.Article, error) {
	var value article.Article
	var tags, keywords string
	var indexedAt string
	var sourceMTime, deletedAt sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id,workspace_id,source_id,stable_id,relative_path,title,description,
category,series,tags_json,keywords_json,slug,cover,source_mtime,source_size,source_fingerprint,content_hash,body_hash,frontmatter_hash,
indexed_at,deleted_at,content_stage,content_stage_issue FROM articles WHERE workspace_id=? AND stable_id=?`, workspaceID, stableID).Scan(
		&value.ID, &value.WorkspaceID, &value.SourceID, &value.StableID, &value.RelativePath, &value.Title,
		&value.Description, &value.Category, &value.Series, &tags, &keywords, &value.Slug, &value.Cover,
		&sourceMTime, &value.SourceSize, &value.SourceFingerprint, &value.ContentHash, &value.BodyHash, &value.FrontmatterHash, &indexedAt, &deletedAt,
		&value.ContentStage, &value.ContentStageIssue)
	if err != nil {
		return article.Article{}, fmt.Errorf("查询文章索引: %w", err)
	}
	if err := json.Unmarshal([]byte(tags), &value.Tags); err != nil {
		return article.Article{}, fmt.Errorf("解析 Tags: %w", err)
	}
	if err := json.Unmarshal([]byte(keywords), &value.Keywords); err != nil {
		return article.Article{}, fmt.Errorf("解析 Keywords: %w", err)
	}
	value.IndexedAt, err = time.Parse(time.RFC3339Nano, indexedAt)
	if err != nil {
		return article.Article{}, fmt.Errorf("解析索引时间: %w", err)
	}
	value.SourceMTime, err = parseTime(sourceMTime)
	if err != nil {
		return article.Article{}, fmt.Errorf("解析源文件时间: %w", err)
	}
	value.DeletedAt, err = parseTime(deletedAt)
	if err != nil {
		return article.Article{}, fmt.Errorf("解析删除时间: %w", err)
	}
	return value, nil
}

func formatTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
