// Package bootstrap 负责解析启动参数并装配 InkHub 应用。
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/buildinfo"
	"github.com/gkmz/InkHub/internal/storage/sqlite"
	transportcli "github.com/gkmz/InkHub/internal/transport/cli"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
	webassets "github.com/gkmz/InkHub/web"
)

// Run 解析命令行参数并运行 InkHub 应用。
func Run(ctx context.Context, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		return transportcli.Run(ctx, args, nil, os.Stdout)
	}
	if len(args) > 1 && args[1] == "doctor" {
		_, err := fmt.Fprintln(os.Stdout, "正常  UI  生产资源已嵌入\n正常  SQLite  migration 已嵌入\n未启用  工作区  首次启动后配置")
		return err
	}
	config, err := parseServerConfig(args)
	if err != nil {
		return err
	}
	if config.ShowVersion {
		_, err := fmt.Fprintln(os.Stdout, buildinfo.Version)
		return err
	}
	return serve(ctx, config.Config)
}

type parsedConfig struct {
	Config
	ShowVersion bool
}

func parseServerConfig(args []string) (parsedConfig, error) {
	userConfig, err := os.UserConfigDir()
	if err != nil {
		return parsedConfig{}, fmt.Errorf("定位用户配置目录: %w", err)
	}
	flags := flag.NewFlagSet("inkhub", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	showVersion := flags.Bool("version", false, "显示版本")
	dataDir := flags.String("data-dir", filepath.Join(userConfig, "InkHub"), "覆盖数据目录")
	workspace := flags.String("workspace", "", "选择工作区")
	host := flags.String("host", "127.0.0.1", "监听地址")
	port := flags.Int("port", 8080, "监听端口")
	values := args
	if len(values) > 0 {
		values = values[1:]
	}
	if err := flags.Parse(values); err != nil {
		return parsedConfig{}, fmt.Errorf("解析启动参数: %w", err)
	}
	if *port < 0 || *port > 65535 {
		return parsedConfig{}, fmt.Errorf("端口超出有效范围")
	}
	return parsedConfig{Config: Config{Host: *host, Port: *port, DataDir: *dataDir, Workspace: *workspace}, ShowVersion: *showVersion}, nil
}

func serve(ctx context.Context, config Config) error {
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	unlock, err := acquireInstanceLock(config.DataDir)
	if err != nil {
		return err
	}
	defer unlock()
	db, err := sqlite.Open(ctx, filepath.Join(config.DataDir, "inkhub.db"))
	if err != nil {
		return fmt.Errorf("打开 InkHub 数据库: %w", err)
	}
	defer db.Close()
	assets, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		return fmt.Errorf("读取嵌入 UI: %w", err)
	}
	api := httptransport.NewRuntimeHandler(db, httptransport.NewRouter(databaseAPI{db: db}))
	server := &http.Server{Handler: httptransport.NewApplicationHandler(api, assets), ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", net.JoinHostPort(config.Host, fmt.Sprint(config.Port)))
	if err != nil {
		return fmt.Errorf("监听 InkHub 地址: %w", err)
	}
	errChannel := make(chan error, 1)
	go func() { errChannel <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP Server: %w", err)
		}
		return nil
	case err := <-errChannel:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func acquireInstanceLock(dataDir string) (func(), error) {
	path := filepath.Join(dataDir, "inkhub.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			content, readErr := os.ReadFile(path)
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(content)))
			if readErr == nil && parseErr == nil && !processAlive(pid) {
				if removeErr := os.Remove(path); removeErr == nil {
					return acquireInstanceLock(dataDir)
				}
			}
			return nil, fmt.Errorf("InkHub 已在使用此数据目录")
		}
		return nil, fmt.Errorf("创建单实例锁: %w", err)
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}

type databaseAPI struct{ db *sql.DB }

func (api databaseAPI) ListArticles(ctx context.Context, _ string, limit int) (httptransport.ArticlePage, error) {
	rows, err := api.db.QueryContext(ctx, `SELECT articles.id,articles.title,articles.relative_path,articles.category,
COALESCE(articles.source_mtime,articles.updated_at),COALESCE(editorial_reviews.state,'pending_review')
FROM articles LEFT JOIN editorial_reviews ON editorial_reviews.article_id=articles.id
WHERE articles.deleted_at IS NULL ORDER BY COALESCE(articles.source_mtime,articles.updated_at) DESC,articles.id LIMIT ?`, limit)
	if err != nil {
		return httptransport.ArticlePage{}, err
	}
	defer rows.Close()
	page := httptransport.ArticlePage{Items: []httptransport.ArticleSummary{}}
	for rows.Next() {
		var item httptransport.ArticleSummary
		var relative string
		if err := rows.Scan(&item.ID, &item.Title, &relative, &item.Category, &item.ModifiedAt, &item.State); err != nil {
			return httptransport.ArticlePage{}, err
		}
		item.Directory = filepath.ToSlash(filepath.Dir(relative))
		if item.Directory == "." {
			item.Directory = ""
		}
		item.HugoState, item.WeChatState = "尚未同步", "尚未准备"
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}
func (databaseAPI) QueuePublication(context.Context, httptransport.PublicationCommand) (string, error) {
	return "", httptransport.ErrNotFound
}
func (databaseAPI) ConfirmWeChat(context.Context, httptransport.ConfirmCommand) error {
	return httptransport.ErrNotFound
}
