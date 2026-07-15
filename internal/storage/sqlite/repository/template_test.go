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
	if err := repository.Activate(context.Background(), "test-template", "1.0.0", "digest-v1", "/templates/v1", "wechat-html", "css", "wechat-html-v1"); err != nil {
		t.Fatalf("激活 v1: %v", err)
	}
	if err := repository.Activate(context.Background(), "test-template", "1.1.0", "digest-v2", "/templates/v2", "wechat-html", "css", "wechat-html-v1"); err != nil {
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
	var target, format, renderer string
	if err := db.QueryRow(`SELECT target,format,renderer FROM templates WHERE workspace_id='w1' AND enabled=1`).Scan(&target, &format, &renderer); err != nil || target != "wechat-html" || format != "css" || renderer != "wechat-html-v1" {
		t.Fatalf("模板目标未持久化: target=%s format=%s renderer=%s err=%v", target, format, renderer, err)
	}
}

func TestTemplateRepositoryKeepsDifferentTargetsActive(t *testing.T) {
	t.Parallel()
	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repository := NewTemplateRepository(db, "w1")
	if err := repository.Activate(context.Background(), "wechat-template", "1.0.0", "digest-w", "/templates/w", "wechat-html", "css", "wechat-html-v1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Activate(context.Background(), "blog-template", "1.0.0", "digest-b", "/templates/b", "hugo-partial", "go-template", "hugo-partial-v1"); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM templates WHERE workspace_id='w1' AND enabled=1`).Scan(&active); err != nil || active != 2 {
		t.Fatalf("不同 target 不应互相停用: active=%d err=%v", active, err)
	}
}
