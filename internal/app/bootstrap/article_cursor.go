package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const maxArticleCursorLength = 1024

// articleCursor 保存内容库 keyset 分页所需的最后排序位置。
type articleCursor struct {
	// ContentStage 是列表排序的第一关键字，确保跨阶段分页保持稳定。
	ContentStage string `json:"content_stage"`
	ModifiedAt   string `json:"modified_at"`
	ID           string `json:"id"`
}

func encodeArticleCursor(cursor articleCursor) (string, error) {
	if err := validateArticleCursor(cursor); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("编码文章 Cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeArticleCursor(value string) (articleCursor, error) {
	if value == "" || len(value) > maxArticleCursorLength {
		return articleCursor{}, fmt.Errorf("文章 Cursor 长度无效")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return articleCursor{}, fmt.Errorf("解码文章 Cursor: %w", err)
	}
	var cursor articleCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return articleCursor{}, fmt.Errorf("解析文章 Cursor: %w", err)
	}
	if err := validateArticleCursor(cursor); err != nil {
		return articleCursor{}, err
	}
	return cursor, nil
}

func validateArticleCursor(cursor articleCursor) error {
	if cursor.ContentStage != "draft" && cursor.ContentStage != "ready" {
		return fmt.Errorf("文章 Cursor 内容阶段无效")
	}
	if cursor.ModifiedAt == "" || cursor.ID == "" || len(cursor.ModifiedAt) > 64 || len(cursor.ID) > 256 {
		return fmt.Errorf("文章 Cursor 字段无效")
	}
	return nil
}
