package models

// Platform 发布平台
type Platform struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// PlatformConfig 平台配置
type PlatformConfig struct {
	Platforms []Platform `json:"platforms"`
}
