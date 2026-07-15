package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

func TestPublicationRunnerDeliversHugoWithRealBuild(t *testing.T) {
	ctx := context.Background()
	site := t.TempDir()
	if err := os.CopyFS(site, os.DirFS(filepath.Join("..", "..", "..", "testdata", "hugo", "site"))); err != nil {
		t.Fatal(err)
	}
	vault := t.TempDir()
	_ = os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700)
	articleText := "---\nid: article_HUGORUN\ntitle: Hugo Runner\ndescription: 真实构建\ntags: [go, hugo]\nkeywords: [InkHub]\npublish:\n  category: AI应用开发\n  series: InkHub 开发日志\n  slug: hugo-runner\n---\n# 正文"
	if err := os.WriteFile(filepath.Join(vault, "hugo.md"), []byte(articleText), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := inksqlite.Open(ctx, filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	staging := filepath.Join(t.TempDir(), "staging")
	statements := []struct {
		query string
		args  []any
	}{{`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test',?,'2026-01-01','2026-01-01','2026-01-01')`, []any{t.TempDir()}}, {`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian',?,'2026-01-01','2026-01-01')`, []any{vault}}, {`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,description,category,series,tags_json,keywords_json,slug,content_hash,frontmatter_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','article_HUGORUN','hugo.md','Hugo Runner','真实构建','AI应用开发','InkHub 开发日志','["go","hugo"]','["InkHub"]','hugo-runner','hash-current','front','2026-01-01','2026-01-01','2026-01-01')`, nil}, {`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-current','front','2026-01-01')`, nil}, {`INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',?,'2026-01-01','2026-01-01')`, []any{`{"root":"` + filepath.ToSlash(site) + `","staging_root":"` + filepath.ToSlash(staging) + `","section":"posts"}`}}}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	api := newDatabaseAPI(db)
	_, err = api.QueuePublication(ctx, httptransport.PublicationCommand{ArticleID: "a1", ProviderInstanceID: "h1", Channel: "hugo", ContentHash: "hash-current"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newProviderRuntime()
	if err != nil {
		t.Fatal(err)
	}
	runner := newPublicationRunner(db, nil, runtime)
	worked, err := runner.RunOne(ctx)
	if err != nil || !worked {
		t.Fatalf("执行 Hugo: worked=%v err=%v", worked, err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM publications WHERE article_id='a1' AND provider_instance_id='h1'`).Scan(&state); err != nil || state != "published" {
		var jobState, code, message string
		_ = db.QueryRow(`SELECT state,COALESCE(error_code,''),COALESCE(error_message,'') FROM jobs WHERE kind='publication'`).Scan(&jobState, &code, &message)
		t.Fatalf("Hugo 未发布: state=%s err=%v job=%s %s %s", state, err, jobState, code, message)
	}
	if _, err := os.Stat(filepath.Join(site, "content", "posts", "hugo-runner", "index.md")); err != nil {
		t.Fatalf("Hugo bundle 不存在: %v", err)
	}
}
