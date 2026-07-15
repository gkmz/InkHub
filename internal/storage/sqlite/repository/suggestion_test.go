package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	domaineditorial "github.com/gkmz/InkHub/internal/domain/editorial"
)

func TestSuggestionRepositorySavesAndUpdatesFieldState(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedSuggestionParents(t, db)
	repository := NewSuggestionRepository(db)
	want := domaineditorial.SuggestionSet{
		ID: "suggestion_1", ArticleID: "a1", WorkspaceID: "w1", ProviderInstanceID: "provider_ai",
		InputContentHash: "hash-v1", Model: "test-model", State: domaineditorial.SuggestionPending,
		Items: []domaineditorial.SuggestionItem{{ID: "item_1", Field: "tags", Value: json.RawMessage(`"Go"`), UsageCount: 18}},
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("保存建议: %v", err)
	}

	want.Items[0].Accepted = true
	want.State = domaineditorial.SuggestionAccepted
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("更新建议: %v", err)
	}
	got, err := repository.FindByID(context.Background(), "suggestion_1")
	if err != nil {
		t.Fatalf("查询建议: %v", err)
	}
	if got.Model != "test-model" || got.State != domaineditorial.SuggestionAccepted || !got.Items[0].Accepted || got.Items[0].UsageCount != 18 {
		t.Fatalf("建议往返结果不匹配: %+v", got)
	}
	latest, found, err := repository.FindLatestByArticle(context.Background(), "w1", "a1")
	if err != nil || !found || latest.ID != want.ID {
		t.Fatalf("按文章查询最近建议 = %+v, %v, %v", latest, found, err)
	}
}

func seedSuggestionParents(t *testing.T, db *sql.DB) {
	t.Helper()
	seedWorkspace(t, db)
	_, err := db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,indexed_at,created_at,updated_at)
VALUES ('a1','w1','s1','article_ONE','one.md','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at)
VALUES ('provider_ai','w1','openai-compatible','AI','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
}
