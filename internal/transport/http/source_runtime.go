package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func (h *runtimeHandler) buildSource(ctx context.Context, sourceID string, overrideConfig []byte) (contracts.SourceProvider, error) {
	if h.providerRuntime == nil {
		return nil, fmt.Errorf("Source Provider Runtime 未配置")
	}
	var providerType, root, configJSON string
	if err := h.db.QueryRowContext(ctx, `SELECT provider_type,root_path,config_json FROM sources WHERE id=?`, sourceID).Scan(&providerType, &root, &configJSON); err != nil {
		return nil, err
	}
	data := []byte(configJSON)
	if overrideConfig != nil {
		data = overrideConfig
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 Source Provider 配置: %w", err)
	}
	raw["root"] = root
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("编码 Source Provider 配置: %w", err)
	}
	return h.providerRuntime.BuildSource(ctx, contracts.ProviderRef{ID: sourceID, Type: contracts.ProviderType(providerType)}, contracts.ConfigView{Data: encoded, AllowedRoots: []string{root}})
}

func sourceConflict(err error) bool {
	var providerErr *contracts.ProviderError
	return errors.As(err, &providerErr) && providerErr.Category == contracts.ErrorConflict
}
