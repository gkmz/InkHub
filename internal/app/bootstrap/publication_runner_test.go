package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

func TestPublicationRunnerPreparesWeChatArtifact(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	_ = os.Mkdir(filepath.Join(root, ".obsidian"), 0o700)
	content := "---\nid: article_RUNNER\ntitle: Runner 测试\ndescription: 微信准备\ntags: [Go]\nkeywords: [InkHub]\n---\n正文 [[target|相关文章]] 与 [[missing|未找到]]\n\n## 相关链接\n[[target|应该被排除]]"
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	site := t.TempDir()
	targetBundle := filepath.Join(site, "content", "posts", "target")
	if err := os.MkdirAll(targetBundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetBundle, "index.md"), []byte("---\nsource_id: article_TARGET\nslug: target\n---\n\n目标正文\n"), 0o600); err != nil {
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
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,slug,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a2','w1','s1','article_TARGET','target.md','目标文章','target','hash-target','front-target','2026-01-01','2026-01-01','2026-01-01','ready')`, nil},
		{`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,approved_frontmatter_hash,updated_at) VALUES('a1','approved','hash-current','front','2026-01-01')`, nil},
		{`INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',?,'2026-01-01','2026-01-01')`, []any{`{"root":"` + filepath.ToSlash(site) + `","base_url":"https://blog.example.com"}`}},
		{`INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('wx1','w1','wechat','微信',?,'2026-01-01','2026-01-01')`, []any{`{"template":"default","staging_root":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "wechat")) + `"}`}},
		{`INSERT INTO settings(workspace_id,key,value_json,created_at,updated_at) VALUES('w1','publication_content','{"excluded_sections":["相关链接"]}','2026-01-01','2026-01-01')`, nil},
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
	input, _, _, err := handler.loadInput(ctx, jobID, publicationPayload{ArticleID: "a1", ProviderID: "wx1", ContentHash: "hash-current"})
	if err != nil {
		t.Fatalf("加载输入: %v", err)
	}
	if !strings.Contains(input.Body, "[相关文章](https://blog.example.com/target/)") || strings.Contains(input.Body, "应该被排除") || strings.Contains(input.Body, "[[") {
		t.Fatalf("微信内部链接转换不正确: %s", input.Body)
	}
	if !containsDiagnostic(input.Diagnostics, "publish.section_excluded", false) {
		t.Fatalf("发布章节排除诊断缺失: %+v", input.Diagnostics)
	}
	if !containsDiagnostic(input.Diagnostics, "internal_link.converted", false) || !containsDiagnostic(input.Diagnostics, "internal_link.missing", false) {
		t.Fatalf("微信内部链接诊断不完整: %+v", input.Diagnostics)
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

func TestPublicationRunnerLoadsHugoWikiLinks(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: article_SOURCE\ntitle: Hugo 链接测试\n---\n正文 [[target|相关文章]] 与 [[draft|草稿文章]]"
	if err := os.WriteFile(filepath.Join(vault, "source.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	site := t.TempDir()
	targetBundle := filepath.Join(site, "content", "posts", "target")
	if err := os.MkdirAll(targetBundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetBundle, "index.md"), []byte("---\nsource_id: article_TARGET\n---\n\n目标正文\n"), 0o600); err != nil {
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
		{`INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian',?,'2026-01-01','2026-01-01')`, []any{vault}},
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a1','w1','s1','article_SOURCE','source.md','Hugo 链接测试','hash-current','front','2026-01-01','2026-01-01','2026-01-01','ready')`, nil},
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a2','w1','s1','article_TARGET','target.md','目标文章','hash-target','front-target','2026-01-01','2026-01-01','2026-01-01','ready')`, nil},
		{`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,frontmatter_hash,indexed_at,created_at,updated_at,content_stage) VALUES('a3','w1','s1','article_DRAFT','draft.md','草稿文章','hash-draft','front-draft','2026-01-01','2026-01-01','2026-01-01','ready')`, nil},
		{`INSERT INTO provider_instances(id,workspace_id,provider_type,name,config_json,created_at,updated_at) VALUES('h1','w1','hugo','Hugo',?,'2026-01-01','2026-01-01')`, []any{`{"root":"` + filepath.ToSlash(site) + `","staging_root":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "staging")) + `"}`}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := newProviderRuntime()
	if err != nil {
		t.Fatal(err)
	}
	handler := publicationJobHandler{db: db, runtime: runtime}
	input, _, _, err := handler.loadInput(ctx, "operation-hugo-links", publicationPayload{ArticleID: "a1", ProviderID: "h1", ContentHash: "hash-current"})
	if err != nil {
		t.Fatalf("加载 Hugo 输入: %v", err)
	}
	if !strings.Contains(input.Body, `[相关文章]({{< relref "posts/target/index.md" >}})`) || strings.Contains(input.Body, "[[") {
		t.Fatalf("Hugo 内部链接转换不正确: %s", input.Body)
	}
	if !strings.Contains(input.Body, "草稿文章") || !containsDiagnostic(input.Diagnostics, "internal_link.unpublished", false) {
		t.Fatalf("Hugo 未发布链接处理不正确: body=%s diagnostics=%+v", input.Body, input.Diagnostics)
	}
}

func containsDiagnostic(diagnostics []contracts.Diagnostic, code string, blocking bool) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Blocking == blocking {
			return true
		}
	}
	return false
}
