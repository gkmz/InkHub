package repository

import (
	"context"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestTaxonomyRepositoryReplacesSnapshotAndKeepsLastSuccessOnFailure(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h1','w1','hugo','Hugo','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	repository := NewTaxonomyRepository(db)
	snapshot := contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: "h1", Type: contracts.ProviderHugo}, Revision: "revision-1", Complete: true, Terms: []contracts.TaxonomyTerm{{Kind: "topic", Key: "go", Name: "Go", CanonicalName: "Go", UsageCount: 2, Metadata: map[string]string{"description": "Go 文章"}}}}
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	if err := repository.ReplaceSnapshot(context.Background(), "w1", snapshot, now); err != nil {
		t.Fatalf("保存 taxonomy snapshot: %v", err)
	}
	if err := repository.MarkRefreshFailed(context.Background(), "w1", "h1", "hugo.read_failed", "读取失败", now.Add(time.Minute)); err != nil {
		t.Fatalf("记录刷新失败: %v", err)
	}
	loaded, status, err := repository.GetSnapshot(context.Background(), "w1", "h1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != "revision-1" || len(loaded.Terms) != 1 || status.State != "failed" || status.LastSuccessAt == nil {
		t.Fatalf("失败刷新丢失最近成功快照: snapshot=%+v status=%+v", loaded, status)
	}
}

func TestTaxonomyRepositoryRejectsCrossWorkspaceSnapshot(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w2','other','/tmp/other','2026-01-01','2026-01-01','2026-01-01'); INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h2','w2','hugo','Hugo','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	err := NewTaxonomyRepository(db).ReplaceSnapshot(context.Background(), "w1", contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: "h2", Type: contracts.ProviderHugo}, Revision: "r", Complete: true}, time.Now().UTC())
	if err == nil {
		t.Fatal("跨工作区 taxonomy snapshot 应被拒绝")
	}
}

func TestTaxonomyRepositoryRejectsIncompleteTerm(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	if _, err := db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h1','w1','hugo','Hugo','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	err := NewTaxonomyRepository(db).ReplaceSnapshot(context.Background(), "w1", contracts.TaxonomySnapshot{ProviderRef: contracts.ProviderRef{ID: "h1", Type: contracts.ProviderHugo}, Revision: "r", Complete: true, Terms: []contracts.TaxonomyTerm{{Kind: "tag", Name: "Go"}}}, time.Now())
	if err == nil {
		t.Fatal("缺少 external key 的 term 应被拒绝")
	}
}
