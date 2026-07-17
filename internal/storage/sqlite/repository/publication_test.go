package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/domain/publication"
)

func TestPublicationRepositorySavesProjectionAndEventAtomically(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedPublicationParents(t, db)
	repo := NewPublicationRepository(db)
	err := repo.SaveWithEvent(context.Background(), PublicationRecord{
		ID: "p1", ArticleID: "a1", ProviderInstanceID: "provider1", WorkspaceID: "w1",
		State: publication.StatePublished, ContentHash: "hash1",
	}, PublicationEvent{ID: "e1", Type: "published", ContentHash: "hash1"})
	if err != nil {
		t.Fatalf("SaveWithEvent() error = %v", err)
	}

	assertCount(t, db, "publications", 1)
	assertCount(t, db, "publication_events", 1)
}

func TestPublicationRepositoryRollsBackProjectionWhenEventFails(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedPublicationParents(t, db)
	repo := NewPublicationRepository(db)
	base := PublicationRecord{ID: "p1", ArticleID: "a1", ProviderInstanceID: "provider1", WorkspaceID: "w1", State: publication.StatePrepared}
	if err := repo.SaveWithEvent(context.Background(), base, PublicationEvent{ID: "e1", Type: "prepared"}); err != nil {
		t.Fatal(err)
	}
	base.State = publication.StatePublished
	if err := repo.SaveWithEvent(context.Background(), base, PublicationEvent{ID: "e1", Type: "published"}); err == nil {
		t.Fatal("duplicate event ID must fail")
	}

	var state string
	if err := db.QueryRow(`SELECT state FROM publications WHERE id='p1'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state == string(publication.StatePublished) {
		t.Fatal("projection update must roll back when event insert fails")
	}
}

func TestPublicationRepositoryPreservesLastRevisionOnFailure(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedPublicationParents(t, db)
	repo := NewPublicationRepository(db)
	base := PublicationRecord{ID: "p1", ArticleID: "a1", ProviderInstanceID: "provider1", WorkspaceID: "w1", State: publication.StatePublished, ContentHash: "hash1", ProviderRevision: "revision-success"}
	if err := repo.SaveWithEvent(context.Background(), base, PublicationEvent{ID: "e1", Type: "published", ContentHash: "hash1"}); err != nil {
		t.Fatal(err)
	}
	base.State, base.ContentHash, base.ProviderRevision = publication.StateFailed, "hash2", ""
	if err := repo.SaveWithEvent(context.Background(), base, PublicationEvent{ID: "e2", Type: "failed", ContentHash: "hash2"}); err != nil {
		t.Fatal(err)
	}
	var revision string
	if err := db.QueryRow(`SELECT provider_revision FROM publications WHERE id='p1'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != "revision-success" {
		t.Fatalf("失败覆盖了最后成功 revision: %q", revision)
	}
}

func TestPublicationRepositoryListsEventsWithStableCursor(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedPublicationParents(t, db)
	if _, err := db.Exec(`INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('wechat1','w1','wechat','微信','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	repo := NewPublicationRepository(db)
	events := []struct {
		record PublicationRecord
		event  PublicationEvent
	}{
		{PublicationRecord{ID: "p_hugo", ArticleID: "a1", ProviderInstanceID: "provider1", WorkspaceID: "w1", State: publication.StatePublished}, PublicationEvent{ID: "e_hugo", Type: "published"}},
		{PublicationRecord{ID: "p_wechat", ArticleID: "a1", ProviderInstanceID: "wechat1", WorkspaceID: "w1", State: publication.StatePrepared}, PublicationEvent{ID: "e_wechat_prepared", Type: "prepared"}},
		{PublicationRecord{ID: "p_wechat", ArticleID: "a1", ProviderInstanceID: "wechat1", WorkspaceID: "w1", State: publication.StateConfirmed}, PublicationEvent{ID: "e_wechat_confirmed", Type: "confirmed"}},
	}
	for _, value := range events {
		if err := repo.SaveWithEvent(context.Background(), value.record, value.event); err != nil {
			t.Fatal(err)
		}
	}
	when := "2026-07-17T12:00:00Z"
	if _, err := db.Exec(`UPDATE publication_events SET created_at=?`, when); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ListEvents(context.Background(), "w1", "a1", PublicationEventCursor{}, 2)
	if err != nil || len(first.Items) != 2 || !first.HasMore || first.Items[0].ID != "e_wechat_prepared" || first.Items[1].ID != "e_wechat_confirmed" {
		t.Fatalf("历史第一页错误: %+v err=%v", first, err)
	}
	second, err := repo.ListEvents(context.Background(), "w1", "a1", PublicationEventCursor{CreatedAt: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC), ID: first.Items[1].ID}, 2)
	if err != nil || len(second.Items) != 1 || second.HasMore || second.Items[0].ID != "e_hugo" || second.Items[0].ProviderType != "hugo" {
		t.Fatalf("历史第二页错误: %+v err=%v", second, err)
	}
}

func assertCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
