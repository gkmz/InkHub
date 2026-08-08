package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/app/publication"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestHugoPreviewRoutesReturnSafeViewsAndProtectConfirmation(t *testing.T) {
	expires := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	api := &fakeHugoPreviewAPI{view: publication.PreviewView{ID: "preview_1", ArticleID: "a1", ContentHash: "hash", Section: "posts", TargetPath: "content/posts/demo", Change: "added", State: "ready", JobID: "preview_1", ExpiresAt: &expires, Files: []publication.PreviewFile{{RelativePath: "index.md", MediaType: "text/markdown", Size: 12}}}}
	handler := NewRuntimeHandler(nil, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{HugoPreviews: api})

	sections := httptest.NewRecorder()
	handler.ServeHTTP(sections, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1/hugo-sections", nil))
	if sections.Code != http.StatusOK || !strings.Contains(sections.Body.String(), `"article_count":2`) || !strings.Contains(sections.Body.String(), `"path":"ai"`) || !strings.Contains(sections.Body.String(), `"existing_directory":"ai"`) || strings.Contains(sections.Body.String(), "/secret/") {
		t.Fatalf("Section 响应错误: %d %s", sections.Code, sections.Body.String())
	}

	createRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/hugo-previews", strings.NewReader(`{"content_hash":"hash","section":"posts","directory":"ai"}`))
	createRequest.Header.Set("Origin", "http://localhost")
	createRequest.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, createRequest)
	if created.Code != http.StatusAccepted || !strings.Contains(created.Body.String(), `"id":"preview_1"`) || api.queued.Directory != "ai" {
		t.Fatalf("创建预览响应错误: %d %s", created.Code, created.Body.String())
	}

	preview := httptest.NewRecorder()
	handler.ServeHTTP(preview, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/hugo-previews/preview_1", nil))
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"target_path":"content/posts/demo"`) || strings.Contains(preview.Body.String(), "/secret/") {
		t.Fatalf("预览安全视图错误: %d %s", preview.Code, preview.Body.String())
	}

	blockedRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/hugo-previews/preview_1/confirm", nil)
	blockedRequest.Header.Set("Origin", "https://evil.example")
	blockedRequest.Header.Set("Content-Type", "application/json")
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, blockedRequest)
	if blocked.Code != http.StatusForbidden || api.confirmCalls != 0 {
		t.Fatalf("跨源确认未阻止: code=%d calls=%d", blocked.Code, api.confirmCalls)
	}

	confirmRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/hugo-previews/preview_1/confirm", nil)
	confirmRequest.Header.Set("Origin", "http://localhost")
	confirmRequest.Header.Set("Content-Type", "application/json")
	confirmed := httptest.NewRecorder()
	handler.ServeHTTP(confirmed, confirmRequest)
	if confirmed.Code != http.StatusAccepted || api.confirmCalls != 1 || !strings.Contains(confirmed.Body.String(), `"job_id":"delivery_1"`) {
		t.Fatalf("确认响应错误: %d %s calls=%d", confirmed.Code, confirmed.Body.String(), api.confirmCalls)
	}
}

func TestSafeHugoPreviewViewIncludesActionableFailure(t *testing.T) {
	view := safeHugoPreviewView(publication.PreviewView{ID: "preview_failed", State: "failed", Failure: &publication.PublicationFailure{Stage: "preflight", Code: "source.image_unresolved", Message: "图片引用无法解析", Action: "修复图片引用后重试", Retryable: true}})
	failure, ok := view["failure"].(map[string]any)
	if !ok || failure["stage"] != "preflight" || failure["code"] != "source.image_unresolved" || failure["action"] != "修复图片引用后重试" || failure["retryable"] != true {
		t.Fatalf("失败视图未安全序列化: %+v", view)
	}
}

func TestHugoPreviewRenderServesOnlyScopedCurrentArticle(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "index.html")
	html := `<!doctype html><html><head><link rel="stylesheet" href="/css/site.css"></head><body><img src="https://blog.example.com/images/cover.png"><script>alert(1)</script></body></html>`
	if err := os.WriteFile(page, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	api := &fakeHugoPreviewAPI{
		view:       publication.PreviewView{ID: "preview_1", State: "ready", PreviewURL: "https://blog.example.com/posts/demo/", RenderPath: "posts/demo/index.html"},
		renderFile: publication.PreviewRenderFile{AbsolutePath: page, MediaType: "text/html; charset=utf-8"},
	}
	handler := NewRuntimeHandler(nil, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{HugoPreviews: api})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/hugo-previews/preview_1/render/posts/demo/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("读取 Hugo 渲染预览: code=%d body=%s", response.Code, response.Body.String())
	}
	rootPath := "/api/v1/hugo-previews/preview_1/render"
	if !strings.Contains(response.Body.String(), `href="`+rootPath+`/css/site.css"`) || !strings.Contains(response.Body.String(), `src="`+rootPath+`/images/cover.png"`) {
		t.Fatalf("预览资源链接未隔离改写: %s", response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "script-src 'none'") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("预览安全响应头不完整: %+v", response.Header())
	}
}

type fakeHugoPreviewAPI struct {
	view         publication.PreviewView
	renderFile   publication.PreviewRenderFile
	confirmCalls int
	queued       publication.PreviewRequest
}

func (f *fakeHugoPreviewAPI) DiscoverSections(context.Context, string) (contracts.SectionDiscovery, error) {
	return contracts.SectionDiscovery{Sections: []contracts.PublishSection{{Name: "posts", ArticleCount: 2, Directories: []contracts.PublishDirectory{{Path: "ai", ArticleCount: 2}}}}, ExistingDirectory: "ai", ExistingTarget: "/secret/hugo/content/posts/ai/demo"}, nil
}

func (f *fakeHugoPreviewAPI) Queue(_ context.Context, request publication.PreviewRequest) (domainjob.Job, error) {
	f.queued = request
	return domainjob.Job{ID: "preview_1", State: domainjob.StateQueued}, nil
}

func (f *fakeHugoPreviewAPI) Find(context.Context, string) (publication.PreviewView, error) {
	return f.view, nil
}

func (f *fakeHugoPreviewAPI) ResolveRenderFile(context.Context, string, string) (publication.PreviewRenderFile, error) {
	return f.renderFile, nil
}

func (f *fakeHugoPreviewAPI) Confirm(context.Context, publication.ConfirmPreviewRequest) (domainjob.Job, error) {
	f.confirmCalls++
	return domainjob.Job{ID: "delivery_1", State: domainjob.StateQueued}, nil
}
