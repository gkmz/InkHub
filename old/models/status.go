package models

import "time"

// PublishInfo 发布信息
type PublishInfo struct {
	Published   bool      `json:"published"`
	PublishedAt time.Time `json:"publishedAt"`
	URL         string    `json:"url,omitempty"`
}

// ArticleStatus 文章发布状态
type ArticleStatus struct {
	Platforms map[string]*PublishInfo `json:"platforms"` // key: platform ID
}

// PublishStatusData 所有文章的发布状态
type PublishStatusData struct {
	Articles map[string]*ArticleStatus `json:"articles"` // key: article ID
}
