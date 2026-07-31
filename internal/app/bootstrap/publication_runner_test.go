package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

func TestPublicationRunnerPreparesWeChatArtifact(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	_ = os.Mkdir(filepath.Join(root, ".obsidian"), 0o700)
	content := "---\nid: article_RUNNER\ntitle: Runner 测试\ndescription: 微信准备\ntags: [Go]\nkeywords: [InkHub]\n---\n正文"
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := inksqlite.Open(ctx, filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','test',?,'2026-01-01','2026-01-01','2026-01-01')`, []any{t.TempDir()}},
		{`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian',?,'2026-01-01','2026-01-01')`, []any{root}},
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,description,tags_json,keywords_json,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a1','w1','s1','article_RUNNER','a.md','Runner 测试','微信准备','["Go"]','["InkHub"]','hash-current','front','2026-01-01','2026-01-01','2026-01-01','ready')`, nil},
		{`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-current','front','2026-01-01')`, nil},
		{`INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('wx1','w1','wechat','微信',?,'2026-01-01','2026-01-01')`, []any{`{"template":"default","staging_root":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "wechat")) + `"}`}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	api := newDatabaseAPI(db)
	jobID, err := api.QueuePublication(ctx, httptransport.PublicationCommand{ArticleID: "a1", ProviderInstanceID: "wx1", Channel: "wechat", ContentHash: "hash-current"})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newProviderRuntime()
	if err != nil {
		t.Fatalf("注册 Provider: %v", err)
	}
	handler := publicationJobHandler{db: db, publications: repository.NewPublicationRepository(db), runtime: runtime}
	if _, _, _, err := handler.loadInput(ctx, jobID, publicationPayload{ArticleID: "a1", ProviderID: "wx1", ContentHash: "hash-current"}); err != nil {
		t.Fatalf("加载输入: %v", err)
	}
	runner := newPublicationRunner(db, nil, runtime)
	worked, err := runner.RunOne(ctx)
	if err != nil || !worked {
		t.Fatalf("执行微信任务: worked=%v err=%v", worked, err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM publications WHERE article_id='a1' AND provider_instance_id='wx1'`).Scan(&state); err != nil || state != "prepared" {
		var jobState, code, message string
		_ = db.QueryRow(`SELECT state,COALESCE(error_code,''),COALESCE(error_message,'') FROM jobs WHERE id=?`, jobID).Scan(&jobState, &code, &message)
		t.Fatalf("微信未准备: state=%s err=%v job=%s job_state=%s code=%s message=%s", state, err, jobID, jobState, code, message)
	}
}
