// Package disposition 定义文章管理处置状态。
package disposition

import "time"

// Kind 是文章当前管理处置类型。
type Kind string

const (
	// KindPublished 表示当前内容版本已在外部渠道发表。
	KindPublished Kind = "published"
	// KindIgnored 表示文章被长期排除在日常管理之外。
	KindIgnored Kind = "ignored"
)

// Record 是文章当前处置投影。
type Record struct {
	ArticleID   string
	WorkspaceID string
	Kind        Kind
	ContentHash string
	ClearedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Effective 判断处置对当前内容版本是否有效。
func (r Record) Effective(currentHash string) bool {
	if r.ClearedAt != nil {
		return false
	}
	return r.Kind == KindIgnored || r.Kind == KindPublished && r.ContentHash == currentHash
}
