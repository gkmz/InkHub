package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	GitHubToken      string
	GitHubRepo       string // e.g., "username/repo"
	GitHubBranch     string // default "main"
	GitHubPathPrefix string
	PostsDir         string
	BaseURL          string // e.g., "https://hankmo.com"
}

var AppConfig *Config

func Load() {
	// 尝试加载 .env 文件，如果不存在也不报错（可能通过系统环境变量注入）
	_ = godotenv.Load()

	githubOwner := strings.TrimSpace(os.Getenv("GITHUB_OWNER"))
	githubRepo := strings.TrimSpace(os.Getenv("GITHUB_REPO"))
	if githubOwner != "" && githubRepo != "" && !strings.Contains(githubRepo, "/") {
		githubRepo = githubOwner + "/" + githubRepo
	}

	AppConfig = &Config{
		GitHubToken:      strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubRepo:       githubRepo,
		GitHubBranch:     strings.TrimSpace(os.Getenv("GITHUB_BRANCH")),
		GitHubPathPrefix: strings.TrimSpace(os.Getenv("GITHUB_PATH_PREFIX")),
		PostsDir:         strings.TrimSpace(os.Getenv("POSTS_DIR")),
		BaseURL:          strings.TrimSpace(os.Getenv("POSTS_BASE_URL")),
	}

	// 自动去除 .git 后缀
	if before, ok := strings.CutSuffix(AppConfig.GitHubRepo, ".git"); ok {
		AppConfig.GitHubRepo = before
	}

	if AppConfig.GitHubBranch == "" {
		AppConfig.GitHubBranch = "main"
	}

	if AppConfig.GitHubToken == "" {
		log.Println("⚠️  Warning: GITHUB_TOKEN not found. Upload feature will be disabled.")
	}
}
