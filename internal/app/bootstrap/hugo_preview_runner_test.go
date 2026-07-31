package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/app/publication"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

func TestHugoPreviewJobPreparesThenDeliverJobPublishesSameArtifact(t *testing.T) {
	ctx := context.Background()
	site := t.TempDir()
	if err := os.CopyFS(site, os.DirFS(filepath.Join("..", "..", "..", "testdata", "hugo", "site"))); err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	articleText := "---\nid: article_PREVIEW\ntitle: Hugo Preview\ndescription: 预览确认\ntags: [go]\nkeywords: [InkHub]\npublish:\n  slug: hugo-preview\n---\n# 正文"
	if err := os.WriteFile(filepath.Join(vault, "preview.md"), []byte(articleText), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := inksqlite.Open(ctx, filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	staging := filepath.Join(t.TempDir(), "staging")
	insertHugoPreviewFixture(t, db, site, vault, staging)

	jobs := repository.NewJobRepository(db)
	previewID := "preview_0123456789abcdef01234567"
	payload, _ := json.Marshal(map[string]string{"preview_id": previewID, "article_id": "a1", "provider_instance_id": "h1", "content_hash": "hash-current", "section": "posts"})
	if _, _, err := jobs.Enqueue(ctx, domainjob.Job{ID: previewID, WorkspaceID: "w1", Kind: "hugo_preview", DedupeKey: "preview", PayloadJSON: string(payload), AvailableAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	runtime, err := newProviderRuntime()
	if err != nil {
		t.Fatal(err)
	}
	runner := newPublicationRunner(db, nil, runtime)
	if worked, err := runner.RunOne(ctx); err != nil || !worked {
		t.Fatalf("执行 Hugo 预览任务: worked=%v err=%v", worked, err)
	}
	preview, err := jobs.FindByID(ctx, previewID)
	if err != nil || preview.State != domainjob.StateSucceeded {
		t.Fatalf("Hugo 预览未成功: state=%s err=%v message=%s", preview.State, err, preview.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(site, "content", "posts", "hugo-preview", "index.md")); !os.IsNotExist(err) {
		t.Fatalf("确认前修改了正式 Hugo content: %v", err)
	}
	var result publication.HugoPreviewResult
	if err := json.Unmarshal([]byte(preview.ResultJSON), &result); err != nil || result.Artifact.OperationID != previewID {
		t.Fatalf("预览 Artifact 无效: %+v err=%v", result, err)
	}

	deliveryID := "delivery_0123456789abcdef01234567"
	deliveryPayload, _ := json.Marshal(map[string]string{"preview_id": previewID, "article_id": "a1", "provider_instance_id": "h1", "content_hash": "hash-current"})
	if _, _, err := jobs.Enqueue(ctx, domainjob.Job{ID: deliveryID, WorkspaceID: "w1", Kind: "hugo_deliver", DedupeKey: "delivery", PayloadJSON: string(deliveryPayload), AvailableAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if worked, err := runner.RunOne(ctx); err != nil || !worked {
		t.Fatalf("执行 Hugo 交付任务: worked=%v err=%v", worked, err)
	}
	delivery, err := jobs.FindByID(ctx, deliveryID)
	if err != nil || delivery.State != domainjob.StateSucceeded {
		t.Fatalf("Hugo 交付未成功: state=%s err=%v message=%s", delivery.State, err, delivery.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(site, "content", "posts", "hugo-preview", "index.md")); err != nil {
		t.Fatalf("确认后未写入正式 Hugo bundle: %v", err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM publications WHERE article_id='a1' AND provider_instance_id='h1'`).Scan(&state); err != nil || state != "published" {
		t.Fatalf("交付后 publication 状态错误: state=%s err=%v", state, err)
	}
}

func TestHugoPreviewJobPersistsTerminalFailureEvent(t *testing.T) {
	ctx := context.Background()
	site := t.TempDir()
	if err := os.CopyFS(site, os.DirFS(filepath.Join("..", "..", "..", "testdata", "hugo", "site"))); err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "preview.md"), []byte("---\nid: article_PREVIEW\ntitle: Preview\npublish:\n  slug: preview\n---\n正文"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := inksqlite.Open(ctx, filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insertHugoPreviewFixture(t, db, site, vault, filepath.Join(t.TempDir(), "staging"))
	payload := `{"preview_id":"preview_invalid","article_id":"a1","provider_instance_id":"h1","content_hash":"hash-current","section":"missing"}`
	jobs := repository.NewJobRepository(db)
	if _, _, err := jobs.Enqueue(ctx, domainjob.Job{ID: "preview_invalid", WorkspaceID: "w1", Kind: "hugo_preview", PayloadJSON: payload, AvailableAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	runtime, err := newProviderRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := newPublicationRunner(db, nil, runtime).RunOne(ctx); err != nil || !worked {
		t.Fatalf("执行失败预览: worked=%v err=%v", worked, err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM publications WHERE article_id='a1' AND provider_instance_id='h1'`).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("终态失败投影错误: state=%s err=%v", state, err)
	}
	var count int
	var eventPayload string
	if err := db.QueryRow(`SELECT COUNT(*),MAX(payload_json) FROM publication_events WHERE event_type='failed'`).Scan(&count, &eventPayload); err != nil || count != 1 {
		t.Fatalf("终态失败事件错误: count=%d payload=%s err=%v", count, eventPayload, err)
	}
	if strings.Contains(eventPayload, site) || strings.Contains(eventPayload, vault) || !strings.Contains(eventPayload, `"error_code"`) {
		t.Fatalf("终态失败事件未脱敏: %s", eventPayload)
	}
}

func insertHugoPreviewFixture(t *testing.T, db *sql.DB, site, vault, staging string) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test',?,'2026-01-01','2026-01-01','2026-01-01')`, []any{t.TempDir()}},
		{`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian',?,'2026-01-01','2026-01-01')`, []any{vault}},
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,description,tags_json,keywords_json,slug,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a1','w1','s1','article_PREVIEW','preview.md','Hugo Preview','预览确认','["go"]','["InkHub"]','hugo-preview','hash-current','front','2026-01-01','2026-01-01','2026-01-01','ready')`, nil},
		{`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-current','front','2026-01-01')`, nil},
		{`INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',?,'2026-01-01','2026-01-01')`, []any{`{"root":"` + filepath.ToSlash(site) + `","staging_root":"` + filepath.ToSlash(staging) + `","section":"posts"}`}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}
