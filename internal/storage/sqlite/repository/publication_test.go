package repository

import (
	"context"
	"database/sql"
	"testing"

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
