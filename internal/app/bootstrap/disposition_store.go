package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appdisposition "github.com/gkmz/InkHub/internal/app/disposition"
)

type dispositionStore struct {
	db *sql.DB
}

func newDispositionService(db *sql.DB) *appdisposition.Service {
	return appdisposition.NewService(&dispositionStore{db: db})
}

func (s *dispositionStore) Apply(ctx context.Context, command appdisposition.Command) (appdisposition.Result, error) {
	if s == nil || s.db == nil {
		return appdisposition.Result{}, appdisposition.ErrInvalidCommand
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return appdisposition.Result{}, fmt.Errorf("开始文章处置事务: %w", err)
	}
	defer tx.Rollback()

	workspaceID, err := currentDispositionWorkspace(ctx, tx)
	if err != nil {
		return appdisposition.Result{}, err
	}
	versions, err := validateDispositionArticles(ctx, tx, workspaceID, command.Articles)
	if err != nil {
		return appdisposition.Result{}, err
	}
	providers, err := resolveDispositionProviders(ctx, tx, workspaceID, command)
	if err != nil {
		return appdisposition.Result{}, err
	}

	result := appdisposition.Result{Processed: len(command.Articles)}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, article := range command.Articles {
		changed, applyErr := applyArticleDisposition(ctx, tx, workspaceID, article.ID, versions[article.ID], command, providers, now)
		if applyErr != nil {
			return appdisposition.Result{}, applyErr
		}
		if changed {
			result.Changed++
		} else {
			result.Unchanged++
		}
	}
	if err := tx.Commit(); err != nil {
		return appdisposition.Result{}, fmt.Errorf("提交文章处置事务: %w", err)
	}
	return result, nil
}

func currentDispositionWorkspace(ctx context.Context, tx *sql.Tx) (string, error) {
	var workspaceID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM workspaces ORDER BY last_used_at DESC,id LIMIT 1`).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", appdisposition.ErrArticleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("查询当前工作区: %w", err)
	}
	return workspaceID, nil
}

func validateDispositionArticles(ctx context.Context, tx *sql.Tx, workspaceID string, articles []appdisposition.ArticleVersion) (map[string]string, error) {
	versions := make(map[string]string, len(articles))
	for _, article := range articles {
		var current string
		err := tx.QueryRowContext(ctx, `SELECT content_hash FROM articles WHERE id=? AND workspace_id=? AND deleted_at IS NULL`, article.ID, workspaceID).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appdisposition.ErrArticleNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("查询待处置文章: %w", err)
		}
		if current != article.ContentVersion {
			return nil, appdisposition.ErrContentChanged
		}
		versions[article.ID] = current
	}
	return versions, nil
}

func resolveDispositionProviders(ctx context.Context, tx *sql.Tx, workspaceID string, command appdisposition.Command) (map[string]string, error) {
	providers := make(map[string]string, len(command.Channels))
	if command.Operation != appdisposition.OperationPublished {
		return providers, nil
	}
	for _, channel := range command.Channels {
		var providerID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM provider_instances WHERE workspace_id=? AND provider_type=? AND enabled=1`, workspaceID, channel).Scan(&providerID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appdisposition.ErrChannelUnavailable
		}
		if err != nil {
			return nil, fmt.Errorf("查询处置发布渠道: %w", err)
		}
		providers[channel] = providerID
	}
	return providers, nil
}

func applyArticleDisposition(ctx context.Context, tx *sql.Tx, workspaceID, articleID, contentHash string, command appdisposition.Command, providers map[string]string, now string) (bool, error) {
	switch command.Operation {
	case appdisposition.OperationPublished:
		return markArticlePublished(ctx, tx, workspaceID, articleID, contentHash, command.Channels, providers, now)
	case appdisposition.OperationIgnored:
		return ignoreArticle(ctx, tx, workspaceID, articleID, contentHash, now)
	case appdisposition.OperationRestore:
		return restoreArticle(ctx, tx, workspaceID, articleID, now)
	default:
		return false, appdisposition.ErrInvalidCommand
	}
}

func markArticlePublished(ctx context.Context, tx *sql.Tx, workspaceID, articleID, contentHash string, channels []string, providers map[string]string, now string) (bool, error) {
	changed, err := dispositionDiffers(ctx, tx, articleID, "published", contentHash)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,cleared_at,created_at,updated_at)
VALUES(?,?,'published',?,NULL,?,?)
ON CONFLICT(article_id) DO UPDATE SET workspace_id=excluded.workspace_id,kind='published',content_hash=excluded.content_hash,cleared_at=NULL,updated_at=excluded.updated_at`, articleID, workspaceID, contentHash, now, now); err != nil {
		return false, fmt.Errorf("保存已发表处置: %w", err)
	}
	for _, channel := range channels {
		providerID := providers[channel]
		var existingState, existingHash string
		err := tx.QueryRowContext(ctx, `SELECT state,content_hash FROM publications WHERE article_id=? AND provider_instance_id=?`, articleID, providerID).Scan(&existingState, &existingHash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("查询渠道发布投影: %w", err)
		}
		if existingState != "published" || existingHash != contentHash {
			changed = true
		}
		publicationID := stableAPIID("publication", articleID, providerID)
		if _, err := tx.ExecContext(ctx, `INSERT INTO publications(id,article_id,provider_instance_id,workspace_id,state,content_hash,last_processed_at,created_at,updated_at)
VALUES(?,?,?,?,'published',?,?,?,?)
ON CONFLICT(article_id,provider_instance_id) DO UPDATE SET state='published',content_hash=excluded.content_hash,last_error_code=NULL,last_error_message=NULL,last_processed_at=excluded.last_processed_at,updated_at=excluded.updated_at`, publicationID, articleID, providerID, workspaceID, contentHash, now, now, now); err != nil {
			return false, fmt.Errorf("保存渠道已发表投影: %w", err)
		}
		eventID := stableAPIID("event", publicationID, "marked_published", contentHash)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO publication_events(id,publication_id,event_type,content_hash,payload_json,created_at) VALUES(?,?,'marked_published',?,'{"source":"user","external":true}',?)`, eventID, publicationID, contentHash, now); err != nil {
			return false, fmt.Errorf("保存外部发表事件: %w", err)
		}
	}
	return changed, nil
}

func ignoreArticle(ctx context.Context, tx *sql.Tx, workspaceID, articleID, contentHash, now string) (bool, error) {
	changed, err := dispositionDiffers(ctx, tx, articleID, "ignored", contentHash)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,cleared_at,created_at,updated_at)
VALUES(?,?,'ignored',?,NULL,?,?)
ON CONFLICT(article_id) DO UPDATE SET workspace_id=excluded.workspace_id,kind='ignored',content_hash=excluded.content_hash,cleared_at=NULL,updated_at=excluded.updated_at`, articleID, workspaceID, contentHash, now, now); err != nil {
		return false, fmt.Errorf("保存忽略处置: %w", err)
	}
	return changed, nil
}

func restoreArticle(ctx context.Context, tx *sql.Tx, workspaceID, articleID, now string) (bool, error) {
	result, err := tx.ExecContext(ctx, `UPDATE article_dispositions SET cleared_at=?,updated_at=? WHERE article_id=? AND workspace_id=? AND kind='ignored' AND cleared_at IS NULL`, now, now, articleID, workspaceID)
	if err != nil {
		return false, fmt.Errorf("恢复文章管理: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取恢复文章结果: %w", err)
	}
	return rows > 0, nil
}

func dispositionDiffers(ctx context.Context, tx *sql.Tx, articleID, kind, contentHash string) (bool, error) {
	var existingKind, existingHash string
	var clearedAt sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT kind,content_hash,cleared_at FROM article_dispositions WHERE article_id=?`, articleID).Scan(&existingKind, &existingHash, &clearedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("查询文章处置: %w", err)
	}
	return existingKind != kind || existingHash != contentHash || clearedAt.Valid, nil
}
