package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	if sections.Code != http.StatusOK || !strings.Contains(sections.Body.String(), `"article_count":2`) || strings.Contains(sections.Body.String(), "/secret/") {
		t.Fatalf("Section 响应错误: %d %s", sections.Code, sections.Body.String())
	}

	createRequest := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/hugo-previews", strings.NewReader(`{"content_hash":"hash","section":"posts"}`))
	createRequest.Header.Set("Origin", "http://localhost")
	createRequest.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, createRequest)
	if created.Code != http.StatusAccepted || !strings.Contains(created.Body.String(), `"id":"preview_1"`) {
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

type fakeHugoPreviewAPI struct {
	view         publication.PreviewView
	confirmCalls int
}

func (f *fakeHugoPreviewAPI) DiscoverSections(context.Context, string) (contracts.SectionDiscovery, error) {
	return contracts.SectionDiscovery{Sections: []contracts.PublishSection{{Name: "posts", ArticleCount: 2}}, ExistingTarget: "/secret/hugo/content/posts/demo"}, nil
}

func (f *fakeHugoPreviewAPI) Queue(context.Context, publication.PreviewRequest) (domainjob.Job, error) {
	return domainjob.Job{ID: "preview_1", State: domainjob.StateQueued}, nil
}

func (f *fakeHugoPreviewAPI) Find(context.Context, string) (publication.PreviewView, error) {
	return f.view, nil
}

func (f *fakeHugoPreviewAPI) Confirm(context.Context, publication.ConfirmPreviewRequest) (domainjob.Job, error) {
	f.confirmCalls++
	return domainjob.Job{ID: "delivery_1", State: domainjob.StateQueued}, nil
}
