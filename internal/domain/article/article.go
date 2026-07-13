// Package article 定义 InkHub 的标准文章模型和内容版本规则。
package article

import (
	"fmt"
	"regexp"
	"time"
)

var stableIDPattern = regexp.MustCompile(`^article_[A-Za-z0-9]+$`)

// StableID 是跟随源文章移动和改名的稳定身份。
type StableID string

// Validate 校验稳定文章 ID 的固定格式。
func (id StableID) Validate() error {
	if !stableIDPattern.MatchString(string(id)) {
		return fmt.Errorf("无效的稳定文章 ID: %q", id)
	}
	return nil
}

// Article 是跨 Source 和 Publish Provider 使用的标准文章模型。
type Article struct {
	ID                string
	WorkspaceID       string
	SourceID          string
	StableID          StableID
	RelativePath      string
	Title             string
	Description       string
	Category          string
	Series            string
	Tags              []string
	Keywords          []string
	Slug              string
	Cover             string
	SourceMTime       *time.Time
	SourceSize        int64
	SourceFingerprint string
	ContentHash       string
	FrontmatterHash   string
	IndexedAt         time.Time
	DeletedAt         *time.Time
}
