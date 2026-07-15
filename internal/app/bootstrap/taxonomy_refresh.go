package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appTaxonomy "github.com/gkmz/InkHub/internal/app/taxonomy"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// RefreshRecentTaxonomy 增量刷新最近工作区所有支持 taxonomy 的 Provider。
func RefreshRecentTaxonomy(ctx context.Context, db *sql.DB, runtime contracts.ProviderRuntime) ([]contracts.TaxonomySnapshot, error) {
	if db == nil || runtime == nil {
		return nil, fmt.Errorf("taxonomy 刷新依赖为空")
	}
	var workspaceID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1`).Scan(&workspaceID); err == sql.ErrNoRows {
		return []contracts.TaxonomySnapshot{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("读取最近工作区: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT id,provider_type,config_json FROM provider_instances WHERE workspace_id=? AND enabled=1 ORDER BY id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("读取 Taxonomy Provider 实例: %w", err)
	}
	defer rows.Close()
	store := repository.NewTaxonomyRepository(db)
	service := appTaxonomy.NewService(store, time.Now)
	var snapshots []contracts.TaxonomySnapshot
	var refreshErrors []error
	for rows.Next() {
		var id, providerType, configJSON string
		if err := rows.Scan(&id, &providerType, &configJSON); err != nil {
			return snapshots, err
		}
		typeID := contracts.ProviderType(providerType)
		if !runtime.SupportsTaxonomy(typeID) {
			continue
		}
		view, viewErr := providerConfigView([]byte(configJSON))
		if viewErr != nil {
			_ = store.MarkRefreshFailed(ctx, workspaceID, id, "taxonomy.config_invalid", viewErr.Error(), time.Now())
			refreshErrors = append(refreshErrors, viewErr)
			continue
		}
		ref := contracts.ProviderRef{ID: id, Type: typeID}
		provider, buildErr := runtime.BuildTaxonomy(ctx, ref, view)
		if buildErr != nil {
			_ = store.MarkRefreshFailed(ctx, workspaceID, id, "taxonomy.provider_build_failed", buildErr.Error(), time.Now())
			refreshErrors = append(refreshErrors, buildErr)
			continue
		}
		snapshot, refreshErr := service.Refresh(ctx, workspaceID, ref, provider)
		if snapshot.Revision != "" {
			snapshots = append(snapshots, snapshot)
		}
		if refreshErr != nil {
			refreshErrors = append(refreshErrors, refreshErr)
		}
	}
	if err := rows.Err(); err != nil {
		refreshErrors = append(refreshErrors, err)
	}
	return snapshots, errors.Join(refreshErrors...)
}
