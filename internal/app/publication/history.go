package publication

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrHistoryCursorInvalid 表示发布历史 cursor 无效或不属于当前文章。
	ErrHistoryCursorInvalid = errors.New("发布历史分页位置无效")
)

// HistoryEventCursor 是 Application 与 Store 之间的分页位置。
type HistoryEventCursor struct {
	CreatedAt time.Time
	ID        string
}

// HistoryEvent 是 Store 返回的渠道事件事实。
type HistoryEvent struct {
	ID           string
	ProviderType string
	Type         string
	PayloadJSON  string
	CreatedAt    time.Time
}

// HistoryEventPage 是 Store 返回的事件页。
type HistoryEventPage struct {
	Items   []HistoryEvent
	HasMore bool
}

// PublicationHistoryStore 提供跨渠道发布事件的稳定分页。
type PublicationHistoryStore interface {
	ListPublicationEvents(ctx context.Context, workspaceID, articleID string, cursor HistoryEventCursor, limit int) (HistoryEventPage, error)
}

// HistoryArticle 是查询统一历史所需的渠道无关文章身份。
type HistoryArticle struct {
	ArticleID   string
	WorkspaceID string
}

// HistoryResolver 按当前工作区解析文章，不依赖具体发布渠道。
type HistoryResolver interface {
	ResolveHistoryArticle(ctx context.Context, articleID string) (HistoryArticle, error)
}

// HistoryItem 是浏览器可见的统一发布历史项。
type HistoryItem struct {
	ID         string
	Channel    string
	State      string
	Title      string
	Detail     string
	OccurredAt time.Time
}

// HistoryPage 是带不透明下一页位置的发布历史。
type HistoryPage struct {
	Items      []HistoryItem
	NextCursor string
}

// PublicationHistoryService 将渠道事件映射为安全中文时间线。
type PublicationHistoryService struct {
	resolver HistoryResolver
	store    PublicationHistoryStore
}

// NewPublicationHistoryService 创建统一发布历史服务。
func NewPublicationHistoryService(resolver HistoryResolver, store PublicationHistoryStore) *PublicationHistoryService {
	return &PublicationHistoryService{resolver: resolver, store: store}
}

// History 查询当前工作区文章的统一发布历史。
func (s *PublicationHistoryService) History(ctx context.Context, articleID, cursorValue string, limit int) (HistoryPage, error) {
	if s == nil || s.resolver == nil || s.store == nil || articleID == "" || limit < 1 || limit > 50 {
		return HistoryPage{}, fmt.Errorf("发布历史请求无效")
	}
	article, err := s.resolver.ResolveHistoryArticle(ctx, articleID)
	if err != nil {
		return HistoryPage{}, err
	}
	if article.ArticleID != articleID || article.WorkspaceID == "" {
		return HistoryPage{}, fmt.Errorf("发布历史文章身份无效")
	}
	cursor, err := decodeHistoryCursor(cursorValue, article.WorkspaceID, article.ArticleID)
	if err != nil {
		return HistoryPage{}, err
	}
	events, err := s.store.ListPublicationEvents(ctx, article.WorkspaceID, article.ArticleID, cursor, limit)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Items: make([]HistoryItem, 0, len(events.Items))}
	for _, event := range events.Items {
		page.Items = append(page.Items, historyItem(event))
	}
	if events.HasMore && len(events.Items) > 0 {
		last := events.Items[len(events.Items)-1]
		page.NextCursor, err = encodeHistoryCursor(historyCursor{WorkspaceID: article.WorkspaceID, ArticleID: article.ArticleID, CreatedAt: last.CreatedAt, EventID: last.ID})
		if err != nil {
			return HistoryPage{}, err
		}
	}
	return page, nil
}

type historyCursor struct {
	WorkspaceID string    `json:"workspace_id"`
	ArticleID   string    `json:"article_id"`
	CreatedAt   time.Time `json:"created_at"`
	EventID     string    `json:"event_id"`
}

func encodeHistoryCursor(cursor historyCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("编码发布历史 cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeHistoryCursor(value, workspaceID, articleID string) (HistoryEventCursor, error) {
	if value == "" {
		return HistoryEventCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return HistoryEventCursor{}, ErrHistoryCursorInvalid
	}
	var cursor historyCursor
	if json.Unmarshal(decoded, &cursor) != nil || cursor.WorkspaceID != workspaceID || cursor.ArticleID != articleID || cursor.CreatedAt.IsZero() || cursor.EventID == "" {
		return HistoryEventCursor{}, ErrHistoryCursorInvalid
	}
	return HistoryEventCursor{CreatedAt: cursor.CreatedAt, ID: cursor.EventID}, nil
}

func historyItem(event HistoryEvent) HistoryItem {
	channel, state, title, detail := event.ProviderType, event.Type, "发布状态已更新", "渠道已记录处理结果"
	switch event.ProviderType + "/" + event.Type {
	case "hugo/marked_published":
		state, title, detail = "published", "已标记为 Hugo 已发表", "已记录外部发表状态"
	case "wechat/marked_published":
		state, title, detail = "published", "已标记为微信已发表", "已记录外部发表状态"
	case "hugo/published":
		title, detail = "已同步到 Hugo", "博客内容已更新"
	case "hugo/failed":
		title, detail = "Hugo 同步失败", "未能完成博客内容更新"
	case "wechat/prepared":
		title, detail = "微信内容已准备", "格式化内容可以预览和复制"
	case "wechat/copied":
		title, detail = "微信内容已复制", "格式化内容已写入剪贴板"
	case "wechat/confirmed":
		title, detail = "已确认保存微信草稿", "用户已确认草稿保存"
	case "wechat/failed":
		title, detail = "微信内容处理失败", "未能完成微信内容准备"
	}
	sum := sha256.Sum256([]byte(event.ID))
	return HistoryItem{ID: "history_" + hex.EncodeToString(sum[:8]), Channel: channel, State: state, Title: title, Detail: detail, OccurredAt: event.CreatedAt}
}
