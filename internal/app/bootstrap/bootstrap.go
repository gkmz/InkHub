// Package bootstrap 负责解析启动参数并装配 InkHub 应用。
package bootstrap

import (
	"context"
	"crypto/rand"
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
	platformsecrets "github.com/gkmz/InkHub/internal/platform/secrets"
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
	return serve(ctx, config.Config, config.Logging, config.Origins["data_dir"])
}

type parsedConfig struct {
	Config
	Logging     platformlogging.Config
	Origins     map[string]string
	ShowVersion bool
}

func parseServerConfig(args []string) (parsedConfig, error) {
	userConfig, err := os.UserConfigDir()
	if err != nil {
		return parsedConfig{}, fmt.Errorf("定位用户配置目录: %w", err)
	}
	defaults := Config{Host: "127.0.0.1", Port: 8080, DataDir: filepath.Join(userConfig, "InkHub")}
	flags := flag.NewFlagSet("inkhub", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	showVersion := flags.Bool("version", false, "显示版本")
	dataDir := flags.String("data-dir", "", "覆盖数据目录")
	workspace := flags.String("workspace", "", "选择工作区")
	host := flags.String("host", "", "监听地址")
	port := flags.Int("port", 0, "监听端口")
	values := args
	if len(values) > 0 {
		values = values[1:]
	}
	if err := flags.Parse(values); err != nil {
		return parsedConfig{}, fmt.Errorf("解析启动参数: %w", err)
	}
	cliFields := make(map[string]bool)
	flags.Visit(func(value *flag.Flag) {
		switch value.Name {
		case "data-dir":
			cliFields["data_dir"] = true
		case "workspace", "host", "port":
			cliFields[value.Name] = true
		}
	})
	environment := Layer{Name: "environment", Set: make(map[string]bool)}
	if environmentDataDir, ok := os.LookupEnv("INKHUB_DATA_DIR"); ok && strings.TrimSpace(environmentDataDir) != "" {
		environment.Values.DataDir = strings.TrimSpace(environmentDataDir)
		environment.Set["data_dir"] = true
	}
	merged := MergeConfig(
		Layer{Name: "default", Values: defaults},
		environment,
		Layer{Name: "cli", Values: Config{Host: *host, Port: *port, DataDir: *dataDir, Workspace: *workspace}, Set: cliFields},
	)
	if !filepath.IsAbs(merged.Config.DataDir) {
		setting := "数据目录"
		switch merged.Origins["data_dir"] {
		case "environment":
			setting = "INKHUB_DATA_DIR"
		case "cli":
			setting = "--data-dir"
		}
		return parsedConfig{}, fmt.Errorf("%s 必须使用绝对路径", setting)
	}
	merged.Config.DataDir = filepath.Clean(merged.Config.DataDir)
	if merged.Config.Port < 0 || merged.Config.Port > 65535 {
		return parsedConfig{}, fmt.Errorf("端口超出有效范围")
	}
	logConfig, err := platformlogging.ParseConfig(merged.Config.DataDir, os.LookupEnv)
	if err != nil {
		return parsedConfig{}, fmt.Errorf("解析日志配置: %w", err)
	}
	return parsedConfig{
		Config:      merged.Config,
		Logging:     logConfig,
		Origins:     merged.Origins,
		ShowVersion: *showVersion,
	}, nil
}

func loadDotEnv(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("加载 .env: %w", err)
	}
	return nil
}

func serve(ctx context.Context, config Config, logConfig platformlogging.Config, dataDirSource string) error {
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
	logFields := startupConfigLogFields(config, dataDirSource)
	logFields = append(logFields,
		zap.String("component", "bootstrap"),
		zap.String("log_level", logConfig.Level),
		zap.Int("log_max_size_mib", logConfig.MaxSize),
		zap.Bool("log_console", logConfig.Console),
	)
	logger.Info("InkHub 启动", logFields...)
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
	secretStore := platformsecrets.NewSystemStore()
	providerRuntime, err := newProviderRuntime(secretStoreResolver{store: secretStore})
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
	if snapshots, taxonomyErr := RefreshRecentTaxonomy(ctx, db, providerRuntime); taxonomyErr != nil {
		logger.Warn("启动 taxonomy 刷新失败", zap.String("error_code", "taxonomy_refresh_failed"), zap.Error(taxonomyErr))
	} else {
		logger.Info("启动 taxonomy 刷新完成", zap.String("component", "taxonomy"), zap.Int("snapshot_count", len(snapshots)))
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
	hugoPreviews := newHugoPreviewAPI(db, providerRuntime)
	publicationWorkflows := newPublicationWorkflowAPI(db, hugoPreviews)
	planKey := make([]byte, 32)
	if _, err := rand.Read(planKey); err != nil {
		return fmt.Errorf("生成微信计划签名密钥: %w", err)
	}
	wechatPlans, err := newWeChatPlanAPI(db, providerRuntime, planKey)
	if err != nil {
		return fmt.Errorf("装配微信准备计划: %w", err)
	}
	api := httptransport.NewRuntimeHandler(db, httptransport.NewRouter(newDatabaseAPI(db)), httptransport.RuntimeOptions{
		DataDir:              config.DataDir,
		ProviderRuntime:      providerRuntime,
		AISecrets:            secretStore,
		HugoPreviews:         hugoPreviews,
		PublicationWorkflows: publicationWorkflows,
		WeChatPlans:          wechatPlans,
		AssetTokenKey:        planKey,
		RefreshTaxonomy: func(refreshCtx context.Context) error {
			_, refreshErr := RefreshRecentTaxonomy(refreshCtx, db, providerRuntime)
			return refreshErr
		},
		AfterWorkspaceCreated: func(refreshCtx context.Context) (string, error) {
			snapshots, refreshErr := RefreshRecentTaxonomy(refreshCtx, db, providerRuntime)
			if len(snapshots) == 0 && refreshErr == nil {
				return "not_enabled", nil
			}
			return "ready", refreshErr
		},
	})
	return runHTTPServer(ctx, config, logger, httptransport.NewApplicationHandler(api, assets))
}

func startupConfigLogFields(config Config, dataDirSource string) []zap.Field {
	return []zap.Field{
		zap.String("data_dir", config.DataDir),
		zap.String("data_dir_source", dataDirSource),
	}
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
