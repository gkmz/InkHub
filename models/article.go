package models

import "time"

// Article 文章元数据
type Article struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Series    string    `json:"series"`
	Path      string    `json:"path"`
	RelPath   string    `json:"relPath"` // 相对 posts 的路径，用于定位图片
	Slug      string    `json:"slug"`
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ArticleDetail 文章详情
type ArticleDetail struct {
	Article
	HTML        string `json:"html"`
	RawMarkdown string `json:"rawMarkdown"`
}
