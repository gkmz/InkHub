package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	workspaceapp "github.com/gkmz/InkHub/internal/app/workspace"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

type persistedSourceScope struct {
	ContentRoots     []string `json:"content_roots"`
	IgnoredFolders   []string `json:"ignored_folders"`
	IgnoredFileNames []string `json:"ignored_file_names"`
}

// RescanRecentWorkspace 按最近工作区保存的目录规则恢复文章索引。
func RescanRecentWorkspace(ctx context.Context, db *sql.DB, runtime contracts.ProviderRuntime) (workspaceapp.ScanReport, error) {
	var workspaceID, sourceID, providerType, root, configJSON string
	err := db.QueryRowContext(ctx, `SELECT workspaces.id,sources.id,sources.provider_type,sources.root_path,sources.config_json FROM workspaces JOIN sources ON sources.workspace_id=workspaces.id ORDER BY workspaces.last_used_at DESC LIMIT 1`).Scan(&workspaceID, &sourceID, &providerType, &root, &configJSON)
	if err == sql.ErrNoRows {
		return workspaceapp.ScanReport{}, nil
	}
	if err != nil {
		return workspaceapp.ScanReport{}, fmt.Errorf("读取最近工作区 Source: %w", err)
	}
	config := persistedSourceScope{ContentRoots: []string{}, IgnoredFolders: []string{}}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return workspaceapp.ScanReport{}, fmt.Errorf("解析内容目录配置: %w", err)
	}
	if len(config.ContentRoots) == 0 {
		return workspaceapp.ScanReport{}, nil
	}
	if config.IgnoredFileNames == nil {
		config.IgnoredFileNames = []string{"index.md", "_index.md"}
	}
	view, err := sourceConfigView(root, []byte(configJSON))
	if err != nil {
		return workspaceapp.ScanReport{}, err
	}
	source, err := runtime.BuildSource(ctx, contracts.ProviderRef{ID: sourceID, Type: contracts.ProviderType(providerType)}, view)
	if err != nil {
		return workspaceapp.ScanReport{}, err
	}
	return workspaceapp.ScanWorkspace(ctx, source, repository.NewArticleRepository(db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{})
}
