package handlers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gkmz/InkHub/old/models"
	"github.com/gkmz/InkHub/old/services"
)

type renderedArticle struct {
	Article     *models.Article
	HTML        string
	RawMarkdown string
}

// findArticleByID 按稳定文章 ID 查找文章，避免每个 handler 重复线性扫描。
func (h *Handler) findArticleByID(id string) (*models.Article, bool) {
	article, ok := h.articleByID[id]
	return article, ok
}

// renderArticle 统一详情页和详情 API 的 Markdown 渲染流水线。
func (h *Handler) renderArticle(article *models.Article, mermaidTheme string) (*renderedArticle, error) {
	content, err := os.ReadFile(article.Path)
	if err != nil {
		return nil, fmt.Errorf("读取文章失败: %w", err)
	}

	articleDir := filepath.Dir(article.Path)
	rawMarkdown := h.Processor.RemoveFrontmatter(string(content))
	markdownContent := h.Processor.RemoveTitle(rawMarkdown)
	markdownContent = h.Processor.ReplaceRelRef(markdownContent)
	markdownContent = h.Processor.ResolveWikiLinks(markdownContent)
	markdownContent = h.Processor.ResolveAbstract(markdownContent)

	if preprocessed, err := services.PreprocessMermaidBlocks(markdownContent, h.ProjectRoot, articleDir, mermaidTheme); err == nil {
		markdownContent = preprocessed
	}

	htmlContent, err := h.Processor.Convert(markdownContent)
	if err != nil {
		return nil, fmt.Errorf("渲染文章失败: %w", err)
	}

	htmlContent = h.Processor.OptimizeListItems(htmlContent)
	htmlContent = h.Processor.ProcessImagePaths(htmlContent, articleDir)

	return &renderedArticle{
		Article:     article,
		HTML:        htmlContent,
		RawMarkdown: rawMarkdown,
	}, nil
}

// renderPublishHTML 保持发布接口原有输出结构，只收敛后端 HTML 渲染步骤。
func (h *Handler) renderPublishHTML(markdownContent string) (string, string, error) {
	cleanMarkdown := h.Processor.RemoveFrontmatter(markdownContent)
	cleanMarkdown = h.Processor.RemoveTitle(cleanMarkdown)

	htmlContent, err := h.Processor.Convert(cleanMarkdown)
	if err != nil {
		return "", "", fmt.Errorf("渲染失败: %w", err)
	}

	htmlContent = h.Processor.OptimizeListItems(htmlContent)
	return cleanMarkdown, htmlContent, nil
}
