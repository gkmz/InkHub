package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gkmz/mymedia/tools/wechat-preview/config"
	"github.com/gkmz/mymedia/tools/wechat-preview/handlers"
	"github.com/gkmz/mymedia/tools/wechat-preview/markdown"
	"github.com/gkmz/mymedia/tools/wechat-preview/scanner"
	"github.com/gkmz/mymedia/tools/wechat-preview/server"
	"github.com/gkmz/mymedia/tools/wechat-preview/services"
	"github.com/gkmz/mymedia/tools/wechat-preview/utils"
)

//go:embed web
var embedFS embed.FS

func main() {
	// 解析命令行参数
	dirFlag := flag.String("dir", "", "Markdown articles directory (default: current directory)")
	hostFlag := flag.String("host", "127.0.0.1", "Server host (default: 127.0.0.1)")
	portFlag := flag.String("port", "8080", "Server port")
	flag.Parse()

	// 加载配置
	config.Load()

	// 确定文章目录
	postsDir := determinePostsDir(*dirFlag)
	fmt.Printf("Using Posts Dir: %s\n", postsDir)

	// 扫描文章
	articleScanner := scanner.NewScanner(postsDir)
	articles, err := articleScanner.Scan()
	if err != nil {
		fmt.Printf("扫描文章失败: %v\n", err)
		os.Exit(1)
	}

	// 探测项目根目录
	projectRoot := utils.FindProjectRoot(postsDir)
	if projectRoot == "" {
		projectRoot = postsDir
		fmt.Println("Warning: Could not detect project root. Using postsDir as root.")
	}
	fmt.Printf("Using Project Root: %s\n", projectRoot)

	// 初始化服务
	configDir := filepath.Join(projectRoot, "config")
	platformService := services.NewPlatformService(configDir)
	if err := platformService.Load(); err != nil {
		fmt.Printf("加载平台配置失败: %v\n", err)
		os.Exit(1)
	}

	statusService := services.NewStatusService(configDir)
	if err := statusService.Load(); err != nil {
		fmt.Printf("加载状态数据失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化 Markdown 处理器
	processor := markdown.NewProcessor(articles, projectRoot)

	// 初始化处理器
	handler := handlers.NewHandler(articles, processor, platformService, statusService, projectRoot)

	// 打印启动信息
	printStartupInfo(len(articles), len(platformService.GetAll()), postsDir)

	// 初始化服务器
	srv := server.NewServer(handler, embedFS)
	if err := srv.Setup(postsDir); err != nil {
		fmt.Printf("服务器初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 启动服务
	addr := *hostFlag + ":" + *portFlag
	fmt.Printf("Starting server on http://%s\n", addr)
	fmt.Printf("Press Ctrl+C to stop.\n")
	if err := srv.Run(addr); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
		os.Exit(1)
	}
}

// determinePostsDir 确定文章目录
func determinePostsDir(dirFlag string) string {
	var postsDir string
	if dirFlag != "" {
		postsDir = dirFlag
	} else if config.AppConfig.PostsDir != "" {
		postsDir = config.AppConfig.PostsDir
	} else {
		wd, _ := os.Getwd()
		postsDir = wd
	}

	absPostsDir, err := filepath.Abs(postsDir)
	if err != nil {
		fmt.Printf("Error getting absolute path for %s: %v\n", postsDir, err)
		os.Exit(1)
	}
	return absPostsDir
}

// printStartupInfo 打印启动信息
func printStartupInfo(articleCount, platformCount int, postsDir string) {
	fmt.Println("\n========================================")
	fmt.Printf("   Wechat Preview Tool - CLI Mode\n")
	fmt.Printf("   Articles: %d\n", articleCount)
	fmt.Printf("   Platforms: %d\n", platformCount)
	fmt.Printf("   Scanning: %s\n", postsDir)
	fmt.Println("========================================")
	fmt.Println()
}
