package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestRuntimeHandlerCreatesWorkspaceIdempotentlyAndRestoresSession(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	article := "---\nid: article_01JTEST\ntitle: 真实扫描文章\ndescription: 测试首次扫描\ntags: [Go]\nkeywords: [InkHub]\n---\n正文"
	if err := os.MkdirAll(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "Areas", "文章.md"), []byte(article), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "Areas", "index.md"), []byte(article), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshCalls := 0
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t), AfterWorkspaceCreated: func(context.Context) (string, error) {
		refreshCalls++
		return "ready", nil
	}})

	session := httptest.NewRecorder()
	handler.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/session", nil))
	if !strings.Contains(session.Body.String(), `"has_workspace":false`) {
		t.Fatalf("初始会话错误: %s", session.Body.String())
	}

	body := []byte(`{"name":"写作空间","vault_path":"` + filepath.ToSlash(vault) + `","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`)
	for range 2 {
		request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://localhost")
		request.Header.Set("Idempotency-Key", "same-request")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("创建工作区失败: %d %s", response.Code, response.Body.String())
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workspaces`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("工作区未幂等创建: count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE title='真实扫描文章'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("首次扫描未写入文章: count=%d err=%v", count, err)
	}
	if refreshCalls != 2 {
		t.Fatalf("工作区创建后未触发 taxonomy 刷新: calls=%d", refreshCalls)
	}
	var articleID string
	if err := db.QueryRow(`SELECT id FROM articles LIMIT 1`).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/"+articleID, nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "真实扫描文章") || !strings.Contains(detail.Body.String(), "正文") {
		t.Fatalf("文章详情错误: %d %s", detail.Code, detail.Body.String())
	}
	metadataBody := `{"metadata":{"title":"写回后的标题","description":"新摘要","category":"工程","series":"InkHub","tags":["Go"],"keywords":["本地"],"slug":"updated-title","cover":""}}`
	metadataRequest := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/articles/"+articleID+"/metadata", strings.NewReader(metadataBody))
	metadataRequest.Header.Set("Content-Type", "application/json")
	metadataRequest.Header.Set("Origin", "http://localhost")
	metadataResponse := httptest.NewRecorder()
	handler.ServeHTTP(metadataResponse, metadataRequest)
	updated, readErr := os.ReadFile(filepath.Join(vault, "Areas", "文章.md"))
	if metadataResponse.Code != http.StatusOK || readErr != nil || !strings.Contains(string(updated), "写回后的标题") {
		t.Fatalf("元数据未原子写回: code=%d body=%s file=%s err=%v", metadataResponse.Code, metadataResponse.Body.String(), updated, readErr)
	}
	scopeRequest := httptest.NewRequest(http.MethodPut, "http://localhost/api/v1/settings/content-scope", strings.NewReader(`{"content_roots":["Areas"],"ignored_folders":[],"ignored_file_names":["toc.md"]}`))
	scopeRequest.Header.Set("Content-Type", "application/json")
	scopeRequest.Header.Set("Origin", "http://localhost")
	scopeResponse := httptest.NewRecorder()
	handler.ServeHTTP(scopeResponse, scopeRequest)
	var sourceConfig string
	if err := db.QueryRow(`SELECT config_json FROM sources LIMIT 1`).Scan(&sourceConfig); scopeResponse.Code != http.StatusOK || err != nil || !strings.Contains(sourceConfig, `"ignored_file_names":["toc.md"]`) {
		t.Fatalf("忽略文件名未持久化: code=%d config=%s err=%v", scopeResponse.Code, sourceConfig, err)
	}
}

func TestRuntimeHandlerRefreshesWorkspace(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	articlePath := filepath.Join(vault, "Areas", "文章.md")
	draft := "---\nid: article_REFRESH\ntitle: 手动刷新\npublish:\n  status: draft\n---\n正文\n"
	if err := os.WriteFile(articlePath, []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
	createRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(`{"name":"刷新测试","vault_path":"`+filepath.ToSlash(vault)+`","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Origin", "http://localhost")
	createRequest.Header.Set("Idempotency-Key", "refresh-test")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建刷新测试工作区失败: %d %s", createResponse.Code, createResponse.Body.String())
	}
	var stage string
	if err := db.QueryRow(`SELECT content_stage FROM articles WHERE title='手动刷新'`).Scan(&stage); err != nil || stage != "draft" {
		t.Fatalf("初始文章阶段错误: stage=%s err=%v", stage, err)
	}
	ready := strings.Replace(draft, "status: draft", "status: ready", 1)
	if err := os.WriteFile(articlePath, []byte(ready), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspace/refresh", strings.NewReader("{}"))
	refreshRequest.Header.Set("Content-Type", "application/json")
	refreshRequest.Header.Set("Origin", "http://localhost")
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK || !strings.Contains(refreshResponse.Body.String(), `"indexed":1`) {
		t.Fatalf("工作区刷新失败: %d %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	if err := db.QueryRow(`SELECT content_stage FROM articles WHERE title='手动刷新'`).Scan(&stage); err != nil || stage != "ready" {
		t.Fatalf("刷新后文章阶段错误: stage=%s err=%v", stage, err)
	}
}

func TestRuntimeHandlerInspectsVaultDirectoriesWithoutExposingFileNames(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"Areas/写作/秘密标题.md", "Areas/另一篇.md", "Resources/资料.md"} {
		full := filepath.Join(vault, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("private body"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	body := `{"vault_path":"` + filepath.ToSlash(vault) + `"}`
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/inspect", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"Areas"`) || !strings.Contains(response.Body.String(), `"markdown_count":2`) {
		t.Fatalf("目录检查响应错误: code=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "秘密标题") || strings.Contains(response.Body.String(), "private body") {
		t.Fatalf("目录检查泄露文章信息: %s", response.Body.String())
	}
}

func TestRuntimeSettingsNormalizesLegacyNullScopeArrays(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','旧空间',?,?,?,?)`, t.TempDir(), now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sources(id,workspace_id,provider_type,root_path,config_json,created_at,updated_at) VALUES('s1','w1','obsidian',?,'{"content_roots":["Areas"],"ignored_folders":null}',?,?)`, vault, now, now); err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/settings", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ignored_folders":[]`) {
		t.Fatalf("旧 null 配置未规范化: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeHandlerPicksDirectoryThroughInjectedNativeAdapter(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	picker := &fakeDirectoryPicker{path: "/Users/test/Documents/Vault"}
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{DirectoryPicker: picker})

	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/pick", strings.NewReader(`{"purpose":"hugo"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"/Users/test/Documents/Vault"`) {
		t.Fatalf("目录选择接口响应错误: code=%d body=%s", response.Code, response.Body.String())
	}
	if picker.title != "选择 Hugo 项目根目录" {
		t.Fatalf("目录选择器标题错误: %q", picker.title)
	}

	invalid := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/pick", strings.NewReader(`{"purpose":"unknown"}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalid.Header.Set("Origin", "http://localhost")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest || !strings.Contains(invalidResponse.Body.String(), "request.invalid") {
		t.Fatalf("未知目录用途未拒绝: code=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}

	blocked := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/directories/pick", strings.NewReader(`{}`))
	blocked.Header.Set("Content-Type", "application/json")
	blocked.Header.Set("Origin", "https://evil.example")
	blockedResponse := httptest.NewRecorder()
	handler.ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("跨源目录选择请求未拒绝: %d", blockedResponse.Code)
	}
}

func TestRuntimeDashboardPassesThroughToCoreRouter(t *testing.T) {
	t.Parallel()
	core := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, http.StatusOK, map[string]string{"source": "core"})
	})
	response := httptest.NewRecorder()
	NewRuntimeHandler(nil, core).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/dashboard", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source":"core"`) {
		t.Fatalf("工作台请求未进入核心 Router: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRuntimeArticleDetailReturnsOnlyEffectiveDisposition(t *testing.T) {
	tests := []struct {
		name            string
		kind            string
		dispositionHash string
		wantDisposition bool
		wantKind        string
		wantChannels    []string
	}{
		{name: "当前版本已发表返回渠道", kind: "published", dispositionHash: "hash-1", wantDisposition: true, wantKind: "published", wantChannels: []string{"hugo", "wechat"}},
		{name: "旧版本已发表不再有效", kind: "published", dispositionHash: "hash-old", wantDisposition: false},
		{name: "忽略跨版本持续且不返回渠道", kind: "ignored", dispositionHash: "hash-old", wantDisposition: true, wantKind: "ignored", wantChannels: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			vault := t.TempDir()
			if err := os.Mkdir(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(vault, "article.md"), []byte("---\nid: stable-1\ntitle: 处置详情\n---\n正文"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','当前','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES('s1','w1','obsidian',?,'2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at) VALUES('a1','w1','s1','stable-1','article.md','处置详情','hash-1','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES
('h1','w1','hugo','Hugo','2026-07-30','2026-07-30'),('m1','w1','wechat','微信','2026-07-30','2026-07-30');
INSERT INTO publications(id,article_id,provider_instance_id,workspace_id,state,content_hash,created_at,updated_at) VALUES
				('p1','a1','h1','w1','published','hash-1','2026-07-30','2026-07-30'),('p2','a1','m1','w1','published','hash-1','2026-07-30','2026-07-30')`, vault)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,created_at,updated_at) VALUES('a1','w1',?,?,'2026-07-30','2026-07-30')`, test.kind, test.dispositionHash); err != nil {
				t.Fatal(err)
			}
			handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{ProviderRuntime: testProviderRuntime(t)})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("文章详情响应错误: code=%d body=%s", response.Code, response.Body.String())
			}
			var body struct {
				Disposition *struct {
					Kind     string   `json:"kind"`
					Channels []string `json:"channels"`
				} `json:"disposition"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if !test.wantDisposition {
				if body.Disposition != nil {
					t.Fatalf("旧处置不应返回: %+v", body.Disposition)
				}
				return
			}
			if body.Disposition == nil || body.Disposition.Kind != test.wantKind || !equalStrings(body.Disposition.Channels, test.wantChannels) {
				t.Fatalf("disposition=%+v want kind=%s channels=%v", body.Disposition, test.wantKind, test.wantChannels)
			}
		})
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type fakeDirectoryPicker struct {
	path  string
	title string
}

func (p *fakeDirectoryPicker) Pick(_ context.Context, title string) (string, error) {
	p.title = title
	return p.path, nil
}

type emptyRuntimeAPI struct{}

func (emptyRuntimeAPI) ListArticles(context.Context, ArticleListQuery) (ArticlePage, error) {
	return ArticlePage{}, nil
}

func (emptyRuntimeAPI) BatchDisposition(context.Context, BatchDispositionCommand) (BatchDispositionResult, error) {
	return BatchDispositionResult{}, nil
}
func (emptyRuntimeAPI) Dashboard(context.Context) (DashboardView, error) {
	return DashboardView{}, nil
}
func (emptyRuntimeAPI) QueuePublication(context.Context, PublicationCommand) (string, error) {
	return "", ErrNotFound
}
func (emptyRuntimeAPI) ConfirmWeChat(context.Context, ConfirmCommand) error    { return ErrNotFound }
func (emptyRuntimeAPI) MarkWeChatCopied(context.Context, ConfirmCommand) error { return ErrNotFound }
