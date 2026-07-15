// Package taxonomy 编排 Provider taxonomy 发现与持久化快照刷新。
package taxonomy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// SnapshotStore 持久化最近成功快照和独立刷新状态。
type SnapshotStore interface {
	FindSnapshot(ctx context.Context, workspaceID, providerID string) (contracts.TaxonomySnapshot, bool, error)
	ReplaceSnapshot(ctx context.Context, workspaceID string, snapshot contracts.TaxonomySnapshot, now time.Time) error
	MarkRefreshSucceeded(ctx context.Context, workspaceID, providerID string, now time.Time) error
	MarkRefreshFailed(ctx context.Context, workspaceID, providerID, code, message string, now time.Time) error
}

// Service 编排增量发现，并确保失败时保留最近成功快照。
type Service struct {
	store SnapshotStore
	now   func() time.Time
}

// NewService 创建 taxonomy 刷新服务。
func NewService(store SnapshotStore, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

// Refresh 使用缓存 revision 发现 taxonomy，并持久化完整成功快照。
func (s *Service) Refresh(ctx context.Context, workspaceID string, ref contracts.ProviderRef, provider contracts.TaxonomyProvider) (contracts.TaxonomySnapshot, error) {
	if s == nil || s.store == nil || workspaceID == "" || ref.ID == "" || ref.Type == "" || provider == nil {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("taxonomy 刷新参数不完整")
	}
	descriptor := provider.Descriptor()
	if descriptor.Type != ref.Type {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("Taxonomy Provider 类型不匹配")
	}
	current, found, err := s.store.FindSnapshot(ctx, workspaceID, ref.ID)
	if err != nil {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("读取 taxonomy 缓存: %w", err)
	}
	cursor := contracts.TaxonomyCursor{}
	if found {
		cursor.Revision = current.Revision
	}
	discovered, discoverErr := provider.Discover(ctx, cursor)
	if discoverErr != nil {
		code := "taxonomy.refresh_failed"
		var providerErr *contracts.ProviderError
		if errors.As(discoverErr, &providerErr) && providerErr.Code != "" {
			code = providerErr.Code
		}
		markErr := s.store.MarkRefreshFailed(ctx, workspaceID, ref.ID, code, discoverErr.Error(), s.now())
		return current, errors.Join(discoverErr, markErr)
	}
	if discovered.ProviderRef.Type == "" {
		discovered.ProviderRef.Type = ref.Type
	}
	if discovered.ProviderRef.ID == "" {
		discovered.ProviderRef.ID = ref.ID
	}
	if discovered.ProviderRef != ref {
		return current, fmt.Errorf("Taxonomy Provider 返回了其他实例的快照")
	}
	if discovered.Unchanged {
		if !found {
			return current, fmt.Errorf("首次 taxonomy 发现不能返回未变化")
		}
		if err := s.store.MarkRefreshSucceeded(ctx, workspaceID, ref.ID, s.now()); err != nil {
			return current, err
		}
		return current, nil
	}
	if err := s.store.ReplaceSnapshot(ctx, workspaceID, discovered, s.now()); err != nil {
		return current, err
	}
	return discovered, nil
}

// PlanChange 生成可审阅的 Provider 原生变更，不产生文件副作用。
func (s *Service) PlanChange(ctx context.Context, ref contracts.ProviderRef, provider contracts.TaxonomyProvider, command contracts.TaxonomyCommand) (contracts.TaxonomyChangeSet, error) {
	if err := validateChangeRequest(ref, provider, command); err != nil {
		return contracts.TaxonomyChangeSet{}, err
	}
	change, err := provider.PlanChange(ctx, command)
	if err != nil {
		return contracts.TaxonomyChangeSet{}, err
	}
	if change.ProviderRef != ref || change.ExpectedRevision != command.ExpectedRevision || len(change.Files) == 0 {
		return contracts.TaxonomyChangeSet{}, fmt.Errorf("Taxonomy Provider 返回了无效变更计划")
	}
	return change, nil
}

// ApplyChange 重新规划并应用已确认命令，成功后替换 SQLite 快照。
func (s *Service) ApplyChange(ctx context.Context, workspaceID string, ref contracts.ProviderRef, provider contracts.TaxonomyProvider, command contracts.TaxonomyCommand) (contracts.TaxonomySnapshot, error) {
	if s == nil || s.store == nil || workspaceID == "" {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("taxonomy 应用依赖不完整")
	}
	change, err := s.PlanChange(ctx, ref, provider, command)
	if err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	snapshot, err := provider.ApplyChange(ctx, change)
	if err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	if snapshot.ProviderRef != ref || snapshot.Revision == "" || !snapshot.Complete {
		return contracts.TaxonomySnapshot{}, fmt.Errorf("Taxonomy Provider 返回了无效应用结果")
	}
	if err := s.store.ReplaceSnapshot(ctx, workspaceID, snapshot, s.now()); err != nil {
		return contracts.TaxonomySnapshot{}, err
	}
	return snapshot, nil
}

func validateChangeRequest(ref contracts.ProviderRef, provider contracts.TaxonomyProvider, command contracts.TaxonomyCommand) error {
	if ref.ID == "" || ref.Type == "" || provider == nil || command.Kind == "" || command.ExpectedRevision == "" || command.Term.Kind == "" || command.Term.Key == "" || command.Term.Name == "" {
		return fmt.Errorf("taxonomy 变更参数不完整")
	}
	descriptor := provider.Descriptor()
	if descriptor.Type != ref.Type || !descriptor.Writable {
		return fmt.Errorf("Taxonomy Provider 不支持写入")
	}
	return nil
}
