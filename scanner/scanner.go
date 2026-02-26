package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hankmor/mymedia/tools/wechat-preview/models"
)

// Scanner 文章扫描器
type Scanner struct {
	postsDir string
}

// NewScanner 创建扫描器
func NewScanner(postsDir string) *Scanner {
	return &Scanner{postsDir: postsDir}
}

// Scan 扫描文章目录
func (s *Scanner) Scan() ([]models.Article, error) {
	articles := []models.Article{}

	err := filepath.WalkDir(s.postsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 处理 .md 和 .adoc 文件
		ext := filepath.Ext(path)
		if d.IsDir() || (ext != ".md" && ext != ".adoc") {
			return nil
		}

		// 读取文件获取标题和 Slug
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		title, slug := extractMetadata(string(content))
		if title == "" {
			title = filepath.Base(path)
		}

		// 如果没有 slug，使用文件名 (无扩展名)
		if slug == "" {
			slug = strings.TrimSuffix(filepath.Base(path), ext)
		}

		// 提取系列名（从目录结构）
		relPath, _ := filepath.Rel(s.postsDir, path)
		parts := strings.Split(relPath, string(os.PathSeparator))
		series := "其他"
		if len(parts) > 1 {
			series = parts[0]
		}

		// 获取修改时间和创建时间
		updatedAt := time.Now()
		createdAt := time.Now()
		info, err := d.Info()
		if err == nil && info != nil {
			updatedAt = info.ModTime()
			createdAt = getFileCreationTime(path)
		}

		// 生成 ID（去掉 posts/ 前缀和 .md/.adoc 后缀，替换路径分隔符为下划线）
		id := strings.TrimSuffix(relPath, ext)
		id = strings.ReplaceAll(id, string(os.PathSeparator), "_")

		articles = append(articles, models.Article{
			ID:        id,
			Title:     title,
			Series:    series,
			Path:      path,
			RelPath:   relPath,
			Slug:      slug,
			UpdatedAt: updatedAt,
			CreatedAt: createdAt,
		})

		return nil
	})

	// 按时间倒序排序 (最近的在前面)
	sort.Slice(articles, func(i, j int) bool {
		return articles[i].CreatedAt.After(articles[j].CreatedAt)
	})

	return articles, err
}

// extractMetadata 从 Markdown/Adoc 内容提取标题和 Slug
func extractMetadata(content string) (string, string) {
	var title, slug string

	// 1. 尝试从 Frontmatter 提取 (YAML)
	if strings.HasPrefix(content, "---") {
		lines := strings.Split(content, "\n")
		// 查找第二个 ---
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "---" {
				break
			}
			if strings.HasPrefix(line, "title:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "title:"))
				title = strings.Trim(val, "\"'")
			}
			if strings.HasPrefix(line, "slug:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "slug:"))
				slug = strings.Trim(val, "\"'")
			}
		}
	}

	// 2. 如果标题为空，回退到查找第一个 H1 (Markdown) or = (Adoc)
	if title == "" {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimPrefix(line, "# ")
				break
			}
			if strings.HasPrefix(line, "= ") {
				title = strings.TrimPrefix(line, "= ")
				break
			}
		}
	}

	return title, slug
}
