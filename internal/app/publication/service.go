// Package publication 编排渠道任务和人工确认，不实现具体渠道转换。
package publication

import (
	"context"
	"errors"
	"fmt"

	"github.com/gkmz/InkHub/internal/domain/article"
	domainpublication "github.com/gkmz/InkHub/internal/domain/publication"
)

var (
	// ErrContentChanged 表示审核或渠道结果不再对应文章当前版本。
	ErrContentChanged = errors.New("文章内容已变化")
	// ErrConfirmationInvalid 表示微信草稿尚未复制或状态不允许确认。
	ErrConfirmationInvalid = errors.New("微信草稿状态不允许确认")
)

// Channel 是 Application 支持的发布渠道。
type Channel string

const (
	ChannelHugo   Channel = "hugo"
	ChannelWeChat Channel = "wechat"
)

// JobIntent 是提交给持久化任务队列的可重建发布意图。
type JobIntent struct {
	ID                 string
	WorkspaceID        string
	Kind               string
	ArticleID          string
	ProviderInstanceID string
	ContentHash        string
}

// JobQueue 持久化长任务并返回稳定 Job ID。
type JobQueue interface {
	Enqueue(ctx context.Context, intent JobIntent) (string, error)
}

// Record 是 Application 使用的渠道当前投影。
type Record struct {
	ID                 string
	WorkspaceID        string
	ArticleID          string
	ProviderInstanceID string
	State              domainpublication.State
	ContentHash        string
}

// Store 查询渠道投影并原子保存投影与事件。
type Store interface {
	Find(ctx context.Context, articleID, providerInstanceID string) (Record, error)
	SaveWithEvent(ctx context.Context, record Record, eventType string) error
}

// Service 编排发布入队和微信人工确认。
type Service struct {
	queue JobQueue
	store Store
}

// NewService 创建发布 Application Service。
func NewService(queue JobQueue, store Store) *Service { return &Service{queue: queue, store: store} }

// QueueRequest 描述一次经过审核的渠道任务请求。
type QueueRequest struct {
	JobID               string
	ProviderInstanceID  string
	Channel             Channel
	Article             article.Article
	ApprovedContentHash string
}

// Queue 校验当前审核版本并将耗时渠道操作持久化入队。
func (s *Service) Queue(ctx context.Context, request QueueRequest) (string, error) {
	if s == nil || s.queue == nil || request.JobID == "" || request.ProviderInstanceID == "" || request.Article.ID == "" {
		return "", fmt.Errorf("发布任务参数不完整")
	}
	if request.Article.ContentHash == "" || request.ApprovedContentHash != request.Article.ContentHash {
		return "", ErrContentChanged
	}
	kind := ""
	switch request.Channel {
	case ChannelHugo:
		kind = "hugo_sync"
	case ChannelWeChat:
		kind = "wechat_prepare"
	default:
		return "", fmt.Errorf("不支持发布渠道: %s", request.Channel)
	}
	return s.queue.Enqueue(ctx, JobIntent{
		ID: request.JobID, WorkspaceID: request.Article.WorkspaceID, Kind: kind,
		ArticleID: request.Article.ID, ProviderInstanceID: request.ProviderInstanceID,
		ContentHash: request.Article.ContentHash,
	})
}

// ConfirmRequest 描述用户确认已在微信后台保存当前草稿。
type ConfirmRequest struct {
	ArticleID          string
	ProviderInstanceID string
	CurrentContentHash string
}

// ConfirmWeChatDraft 将当前内容版本的 copied 投影确认为草稿已保存。
func (s *Service) ConfirmWeChatDraft(ctx context.Context, request ConfirmRequest) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("发布 Store 为空")
	}
	record, err := s.store.Find(ctx, request.ArticleID, request.ProviderInstanceID)
	if err != nil {
		return fmt.Errorf("查询微信渠道状态: %w", err)
	}
	if record.ContentHash != request.CurrentContentHash {
		return ErrContentChanged
	}
	if record.State != domainpublication.StateCopied {
		return ErrConfirmationInvalid
	}
	record.State = domainpublication.StateConfirmed
	if err := s.store.SaveWithEvent(ctx, record, "confirmed"); err != nil {
		return fmt.Errorf("保存微信草稿确认: %w", err)
	}
	return nil
}
