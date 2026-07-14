package httptransport

import (
	"bytes"
	"context"
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
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}))

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

type fakeDirectoryPicker struct {
	path  string
	title string
}

func (p *fakeDirectoryPicker) Pick(_ context.Context, title string) (string, error) {
	p.title = title
	return p.path, nil
}

type emptyRuntimeAPI struct{}

func (emptyRuntimeAPI) ListArticles(context.Context, string, int) (ArticlePage, error) {
	return ArticlePage{}, nil
}
func (emptyRuntimeAPI) QueuePublication(context.Context, PublicationCommand) (string, error) {
	return "", ErrNotFound
}
func (emptyRuntimeAPI) ConfirmWeChat(context.Context, ConfirmCommand) error    { return ErrNotFound }
func (emptyRuntimeAPI) MarkWeChatCopied(context.Context, ConfirmCommand) error { return ErrNotFound }
