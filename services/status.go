package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hankmor/mymedia/tools/wechat-preview/models"
)

// StatusService 状态服务
type StatusService struct {
	dataPath string
	data     *models.PublishStatusData
	mu       sync.RWMutex
}

// NewStatusService 创建状态服务
func NewStatusService(configDir string) *StatusService {
	return &StatusService{
		dataPath: filepath.Join(configDir, "publish-status.json"),
		data: &models.PublishStatusData{
			Articles: make(map[string]*models.ArticleStatus),
		},
	}
}

// Load 加载状态数据
func (s *StatusService) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果文件不存在，创建空数据
	if _, err := os.Stat(s.dataPath); os.IsNotExist(err) {
		return s.saveUnsafe()
	}

	// 读取文件
	data, err := os.ReadFile(s.dataPath)
	if err != nil {
		return fmt.Errorf("读取状态文件失败: %w", err)
	}

	// 解析 JSON
	if err := json.Unmarshal(data, s.data); err != nil {
		return fmt.Errorf("解析状态文件失败: %w", err)
	}

	// 确保 map 已初始化
	if s.data.Articles == nil {
		s.data.Articles = make(map[string]*models.ArticleStatus)
	}

	return nil
}

// Save 保存状态数据
func (s *StatusService) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnsafe()
}

// saveUnsafe 保存数据（不加锁，内部使用）
func (s *StatusService) saveUnsafe() error {
	// 确保目录存在
	dir := filepath.Dir(s.dataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 序列化
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// 写入文件
	return os.WriteFile(s.dataPath, data, 0644)
}

// GetArticleStatus 获取文章状态
func (s *StatusService) GetArticleStatus(articleID string) *models.ArticleStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status, exists := s.data.Articles[articleID]
	if !exists {
		return &models.ArticleStatus{
			Platforms: make(map[string]*models.PublishInfo),
		}
	}
	return status
}

// GetAllStatus 获取所有文章状态
func (s *StatusService) GetAllStatus() map[string]*models.ArticleStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Articles
}

// MarkPublished 标记为已发布
func (s *StatusService) MarkPublished(articleID, platformID string, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 确保文章状态存在
	if s.data.Articles[articleID] == nil {
		s.data.Articles[articleID] = &models.ArticleStatus{
			Platforms: make(map[string]*models.PublishInfo),
		}
	}

	// 更新发布信息
	s.data.Articles[articleID].Platforms[platformID] = &models.PublishInfo{
		Published:   true,
		PublishedAt: time.Now(),
		URL:         url,
	}

	return s.saveUnsafe()
}

// UnmarkPublished 取消发布标记
func (s *StatusService) UnmarkPublished(articleID, platformID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果文章状态不存在，直接返回
	if s.data.Articles[articleID] == nil {
		return nil
	}

	// 删除平台发布信息
	delete(s.data.Articles[articleID].Platforms, platformID)

	// 如果文章没有任何平台发布信息，删除文章状态
	if len(s.data.Articles[articleID].Platforms) == 0 {
		delete(s.data.Articles, articleID)
	}

	return s.saveUnsafe()
}
