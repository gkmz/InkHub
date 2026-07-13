package repository

import (
	"context"
	"testing"
)

func TestTemplateRepositoryActivatesOneVersionAtomically(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repository := NewTemplateRepository(db, "w1")
	if err := repository.Activate(context.Background(), "test-template", "1.0.0", "digest-v1", "/templates/v1"); err != nil {
		t.Fatalf("激活 v1: %v", err)
	}
	if err := repository.Activate(context.Background(), "test-template", "1.1.0", "digest-v2", "/templates/v2"); err != nil {
		t.Fatalf("激活 v2: %v", err)
	}
	rows, err := db.Query(`SELECT version,enabled FROM templates WHERE workspace_id='w1' AND template_id='test-template' ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	states := map[string]int{}
	for rows.Next() {
		var version string
		var enabled int
		_ = rows.Scan(&version, &enabled)
		states[version] = enabled
	}
	if states["1.0.0"] != 0 || states["1.1.0"] != 1 {
		t.Fatalf("活动版本状态不正确: %+v", states)
	}
}
