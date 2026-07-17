package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/app/publication"
)

func TestPublicationWorkflowRoutesReturnSafeRecoveryAndHistory(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	api := fakePublicationWorkflowAPI{
		workflow: publication.WorkflowView{ArticleID: "a1", ContentHash: "secret-hash", Hugo: &publication.HugoWorkflowView{State: "ready", Progress: 100, Stage: "预览已准备", Preview: &publication.PreviewView{ID: "preview_1", JobID: "job_secret", ContentHash: "secret-hash", TargetPath: "content/posts/demo", State: "ready"}}},
		history:  publication.HistoryPage{Items: []publication.HistoryItem{{ID: "history_1", Channel: "hugo", State: "published", Title: "已同步到 Hugo", Detail: "博客内容已更新", OccurredAt: now}}, NextCursor: "next"},
	}
	handler := NewRuntimeHandler(nil, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{PublicationWorkflows: api})

	workflow := httptest.NewRecorder()
	handler.ServeHTTP(workflow, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1/publication-workflow", nil))
	if workflow.Code != http.StatusOK || !strings.Contains(workflow.Body.String(), `"target_path":"content/posts/demo"`) || !strings.Contains(workflow.Body.String(), `"preview_id":"preview_1"`) {
		t.Fatalf("工作流响应错误: %d %s", workflow.Code, workflow.Body.String())
	}
	for _, secret := range []string{"secret-hash", "job_secret", "result_json", "/secret/"} {
		if strings.Contains(workflow.Body.String(), secret) {
			t.Fatalf("工作流响应泄露 %q: %s", secret, workflow.Body.String())
		}
	}

	history := httptest.NewRecorder()
	handler.ServeHTTP(history, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/a1/publication-history?limit=20", nil))
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"title":"已同步到 Hugo"`) || !strings.Contains(history.Body.String(), `"next_cursor":"next"`) {
		t.Fatalf("历史响应错误: %d %s", history.Code, history.Body.String())
	}
}

type fakePublicationWorkflowAPI struct {
	workflow publication.WorkflowView
	history  publication.HistoryPage
	err      error
}

func (f fakePublicationWorkflowAPI) Find(context.Context, string) (publication.WorkflowView, error) {
	return f.workflow, f.err
}

func (f fakePublicationWorkflowAPI) History(context.Context, string, string, int) (publication.HistoryPage, error) {
	return f.history, f.err
}
