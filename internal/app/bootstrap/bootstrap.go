// Package bootstrap 负责解析启动参数并装配 InkHub 应用。
package bootstrap

import (
	"context"
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
	platformlogging "github.com/gkmz/InkHub/internal/platform/logging"
	"github.com/gkmz/InkHub/internal/storage/sqlite"
	transportcli "github.com/gkmz/InkHub/internal/transport/cli"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
	webassets "github.com/gkmz/InkHub/web"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
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
	if err := loadDotEnv(".env"); err != nil {
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
	return serve(ctx, config.Config, config.Logging)
}

type parsedConfig struct {
	Config
	Logging     platformlogging.Config
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
	logConfig, err := platformlogging.ParseConfig(*dataDir, os.LookupEnv)
	if err != nil {
		return parsedConfig{}, fmt.Errorf("解析日志配置: %w", err)
	}
	return parsedConfig{
		Config:      Config{Host: *host, Port: *port, DataDir: *dataDir, Workspace: *workspace},
		Logging:     logConfig,
		ShowVersion: *showVersion,
	}, nil
}

func loadDotEnv(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("加载 .env: %w", err)
	}
	return nil
}

func serve(ctx context.Context, config Config, logConfig platformlogging.Config) error {
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return fmt.Errorf("创建数据目录: %w", err)
	}
	unlock, err := acquireInstanceLock(config.DataDir)
	if err != nil {
		return err
	}
	defer unlock()
	logger, closeLogger, err := platformlogging.New(logConfig, os.Stderr)
	if err != nil {
		return fmt.Errorf("初始化日志: %w", err)
	}
	defer func() { _ = closeLogger() }()
	logger.Info("InkHub 启动",
		zap.String("component", "bootstrap"),
		zap.String("log_level", logConfig.Level),
		zap.Int("log_max_size_mib", logConfig.MaxSize),
		zap.Bool("log_console", logConfig.Console),
	)
	assets, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		return fmt.Errorf("读取嵌入 UI: %w", err)
	}
	db, err := sqlite.Open(ctx, filepath.Join(config.DataDir, "inkhub.db"))
	if err != nil {
		// migration 或完整性检查失败时仍提供 UI，但所有 API 都进入只读恢复边界。
		logger.Error("数据库不可用，进入恢复模式", zap.String("error_code", "database_open_failed"), zap.Error(err))
		return runHTTPServer(ctx, config, logger, httptransport.NewApplicationHandler(httptransport.NewRecoveryHandler(), assets))
	}
	defer db.Close()
	logger.Info("数据库已打开", zap.String("component", "sqlite"))
	providerRuntime, err := newProviderRuntime()
	if err != nil {
		return fmt.Errorf("注册内置 Provider: %w", err)
	}
	// 启动重扫只修复可重建索引；失败不阻止用户进入设置修正 Vault 或目录规则。
	if report, scanErr := RescanRecentWorkspace(ctx, db, providerRuntime); scanErr != nil {
		logger.Warn("启动重扫失败", zap.String("error_code", "workspace_rescan_failed"), zap.Error(scanErr))
	} else {
		logger.Info("启动重扫完成",
			zap.String("component", "workspace_scan"),
			zap.Int("indexed_count", report.Indexed),
			zap.Int("failed_count", report.Failed),
		)
	}
	runner := newPublicationRunner(db, logger, providerRuntime)
	if err := runner.Recover(ctx, time.Now().UTC().Add(-time.Minute)); err != nil {
		logger.Error("恢复后台任务失败", zap.String("error_code", "job_recovery_failed"), zap.Error(err))
		return fmt.Errorf("恢复后台任务: %w", err)
	}
	logger.Info("后台任务恢复完成", zap.String("component", "job_runner"))
	if err := runner.Start(ctx); err != nil {
		logger.Error("启动后台任务失败", zap.String("error_code", "job_runner_start_failed"), zap.Error(err))
		return fmt.Errorf("启动后台任务: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Shutdown(shutdownCtx)
	}()
	api := httptransport.NewRuntimeHandler(db, httptransport.NewRouter(newDatabaseAPI(db)), httptransport.RuntimeOptions{DataDir: config.DataDir})
	return runHTTPServer(ctx, config, logger, httptransport.NewApplicationHandler(api, assets))
}

func runHTTPServer(ctx context.Context, config Config, logger *zap.Logger, handler http.Handler) error {
	server := &http.Server{Handler: httptransport.AccessLog(logger, handler), ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", net.JoinHostPort(config.Host, fmt.Sprint(config.Port)))
	if err != nil {
		logger.Error("监听 HTTP 地址失败", zap.String("error_code", "http_listen_failed"), zap.Error(err))
		return fmt.Errorf("监听 InkHub 地址: %w", err)
	}
	logger.Info("HTTP 服务已启动", zap.String("component", "http_server"), zap.String("address", listener.Addr().String()))
	errChannel := make(chan error, 1)
	go func() { errChannel <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		logger.Info("正在关闭 HTTP 服务", zap.String("component", "http_server"))
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
		logger.Error("HTTP 服务异常退出", zap.String("error_code", "http_server_failed"), zap.Error(err))
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
