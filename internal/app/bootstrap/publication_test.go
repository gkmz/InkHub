package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

func TestDatabaseAPIQueuesOnlyCurrentApprovedPublication(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test','/tmp','2026-01-01','2026-01-01','2026-01-01');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a1','w1','s1','article_TEST','a.md','hash-current','front','2026-01-01','2026-01-01','2026-01-01','ready');
INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-current','front','2026-01-01');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h1','w1','hugo','Hugo','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	api := newDatabaseAPI(db)
	command := httptransport.PublicationCommand{ArticleID: "a1", ProviderInstanceID: "h1", Channel: "hugo", ContentHash: "hash-current"}
	first, err := api.QueuePublication(context.Background(), command)
	if err != nil || first == "" {
		t.Fatalf("当前审核版本未入队: id=%s err=%v", first, err)
	}
	second, err := api.QueuePublication(context.Background(), command)
	if err != nil || second != first {
		t.Fatalf("重复请求未去重: first=%s second=%s err=%v", first, second, err)
	}
	command.ContentHash = "hash-old"
	if _, err := api.QueuePublication(context.Background(), command); err == nil {
		t.Fatal("旧内容版本不应进入队列")
	}
	command.ContentHash = "hash-current"
	if _, err := db.Exec(`UPDATE editorial_reviews SET state='changed' WHERE article_id='a1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := api.QueuePublication(context.Background(), command); err != httptransport.ErrReviewRequired {
		t.Fatalf("审核状态已失效时仍可入队: %v", err)
	}
}

func TestDatabaseAPIRejectsProviderFromAnotherWorkspace(t *testing.T) {
	t.Parallel()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','one','/tmp','2026-01-01','2026-01-01','2026-01-01'),('w2','two','/tmp','2026-01-01','2026-01-01','2026-01-01');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian','/tmp','2026-01-01','2026-01-01');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a1','w1','s1','article_TEST','a.md','hash-current','front','2026-01-01','2026-01-01','2026-01-01','ready');
INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-current','front','2026-01-01');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES('h2','w2','hugo','Other Hugo','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newDatabaseAPI(db).QueuePublication(context.Background(), httptransport.PublicationCommand{ArticleID: "a1", ProviderInstanceID: "h2", ContentHash: "hash-current"})
	if err != httptransport.ErrNotFound {
		t.Fatalf("跨工作区 Provider 应被拒绝: %v", err)
	}
}
