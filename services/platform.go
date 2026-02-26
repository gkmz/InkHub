package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hankmor/mymedia/tools/wechat-preview/models"
)

// PlatformService 平台服务
type PlatformService struct {
	configPath string
	platforms  []models.Platform
}

// NewPlatformService 创建平台服务
func NewPlatformService(configDir string) *PlatformService {
	return &PlatformService{
		configPath: filepath.Join(configDir, "platforms.json"),
	}
}

// Load 加载平台配置
func (s *PlatformService) Load() error {
	// 如果配置文件不存在，创建默认配置
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		if err := s.createDefaultConfig(); err != nil {
			return fmt.Errorf("创建默认配置失败: %w", err)
		}
	}

	// 读取配置文件
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config models.PlatformConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	s.platforms = config.Platforms
	return nil
}

// createDefaultConfig 创建默认配置
func (s *PlatformService) createDefaultConfig() error {
	defaultConfig := models.PlatformConfig{
		Platforms: []models.Platform{
			{
				ID:    "wechat",
				Name:  "微信公众号",
				Icon:  "💬",
				Color: "#07C160",
			},
			{
				ID:    "zhihu",
				Name:  "知乎",
				Icon:  "🎓",
				Color: "#0084FF",
			},
			{
				ID:    "juejin",
				Name:  "掘金",
				Icon:  "💎",
				Color: "#1E80FF",
			},
			{
				ID:    "csdn",
				Name:  "CSDN",
				Icon:  "📝",
				Color: "#FC5531",
			},
		},
	}

	// 确保目录存在
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 写入文件
	data, err := json.MarshalIndent(defaultConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.configPath, data, 0644)
}

// GetAll 获取所有平台
func (s *PlatformService) GetAll() []models.Platform {
	return s.platforms
}

// GetByID 根据 ID 获取平台
func (s *PlatformService) GetByID(id string) *models.Platform {
	for i := range s.platforms {
		if s.platforms[i].ID == id {
			return &s.platforms[i]
		}
	}
	return nil
}
