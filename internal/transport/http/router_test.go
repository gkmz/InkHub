package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouterListsCursorPageWithoutAbsolutePaths(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{page: ArticlePage{Items: []ArticleSummary{{ID: "a1", Title: "标题", State: "approved"}}, NextCursor: "next"}}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/articles?cursor=cursor&limit=20", nil)
	response := httptest.NewRecorder()
	NewRouter(api).ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "/Users/") {
		t.Fatalf("文章列表响应不正确: code=%d body=%s", response.Code, response.Body.String())
	}
	if api.listQuery.Cursor != "cursor" || api.listQuery.Limit != 20 || !strings.Contains(response.Body.String(), `"next_cursor":"next"`) {
		t.Fatalf("Cursor 未透传: api=%+v body=%s", api, response.Body.String())
	}
}

func TestRouterParsesArticleListQueryAndRejectsUnknownFilters(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles?q=SQLite+Tips&state=pending_review&disposition=published&limit=25", nil)
	NewRouter(api).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("列表查询响应状态 = %d, body=%s", response.Code, response.Body.String())
	}
	want := ArticleListQuery{Search: "SQLite Tips", State: "pending_review", Disposition: "published", Limit: 25}
	if api.listQuery != want {
		t.Fatalf("列表查询参数 = %+v, want %+v", api.listQuery, want)
	}

	for _, path := range []string{
		"/api/v1/articles?state=unknown",
		"/api/v1/articles?disposition=unknown",
	} {
		invalidAPI := &fakeAPI{}
		invalidResponse := httptest.NewRecorder()
		NewRouter(invalidAPI).ServeHTTP(invalidResponse, httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil))
		if invalidResponse.Code != http.StatusBadRequest || invalidAPI.listCalls != 0 {
			t.Fatalf("非法筛选未被拒绝: path=%s code=%d calls=%d body=%s", path, invalidResponse.Code, invalidAPI.listCalls, invalidResponse.Body.String())
		}
	}
}

func TestRouterMapsInvalidCursorToBadRequest(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewRouter(&fakeAPI{err: ErrInvalidCursor}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles?cursor=broken", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"request.cursor_invalid"`) {
		t.Fatalf("Cursor 错误映射不正确: code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRouterRequiresSameOriginForWritesAndReturnsJobID(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{jobID: "job_1"}
	body := []byte(`{"article_id":"a1","provider_instance_id":"hugo_1","channel":"hugo","content_hash":"hash"}`)
	blocked := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/publications", bytes.NewReader(body))
	blocked.Header.Set("Content-Type", "application/json")
	blocked.Header.Set("Origin", "https://evil.example")
	blockedResponse := httptest.NewRecorder()
	NewRouter(api).ServeHTTP(blockedResponse, blocked)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("跨源写请求未拒绝: %d", blockedResponse.Code)
	}
	schemeMismatch := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/publications", bytes.NewReader(body))
	schemeMismatch.Header.Set("Content-Type", "application/json")
	schemeMismatch.Header.Set("Origin", "https://127.0.0.1")
	schemeResponse := httptest.NewRecorder()
	NewRouter(api).ServeHTTP(schemeResponse, schemeMismatch)
	if schemeResponse.Code != http.StatusForbidden {
		t.Fatalf("不同 scheme 的 Origin 未拒绝: %d", schemeResponse.Code)
	}

	allowed := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/publications", bytes.NewReader(body))
	allowed.Header.Set("Content-Type", "application/json")
	allowed.Header.Set("Origin", "http://127.0.0.1")
	allowedResponse := httptest.NewRecorder()
	NewRouter(api).ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusAccepted || !strings.Contains(allowedResponse.Body.String(), `"job_id":"job_1"`) {
		t.Fatalf("发布入队响应不正确: code=%d body=%s", allowedResponse.Code, allowedResponse.Body.String())
	}
}

func TestRouterMapsValidationAndStaleErrorsToStableCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		api    *fakeAPI
		code   int
		stable string
	}{
		{name: "未知字段", body: `{"article_id":"a1","unknown":true}`, api: &fakeAPI{}, code: 400, stable: "request.invalid"},
		{name: "内容过期", body: `{"article_id":"a1","provider_instance_id":"hugo_1","channel":"hugo","content_hash":"old"}`, api: &fakeAPI{err: ErrStaleContent}, code: 409, stable: "content.stale"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/publications", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://localhost")
			response := httptest.NewRecorder()
			NewRouter(test.api).ServeHTTP(response, request)
			if response.Code != test.code || !strings.Contains(response.Body.String(), test.stable) {
				t.Fatalf("错误映射不正确: code=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRouterAppliesBatchDispositionAndReturnsCounts(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{dispositionResult: BatchDispositionResult{Processed: 2, Changed: 1, Unchanged: 1}}
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/batch-disposition", strings.NewReader(`{"operation":"published","articles":[{"id":"a1","content_version":"v1"},{"id":"a2","content_version":"v2"}],"channels":["hugo","wechat"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost")
	response := httptest.NewRecorder()
	NewRouter(api).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"processed":2`) {
		t.Fatalf("批量处置响应不正确: code=%d body=%s", response.Code, response.Body.String())
	}
	if api.dispositionCommand.Operation != "published" || len(api.dispositionCommand.Articles) != 2 || len(api.dispositionCommand.Channels) != 2 {
		t.Fatalf("批量处置请求未透传: %+v", api.dispositionCommand)
	}
}

func TestRouterMapsBatchDispositionErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "内容变化", err: ErrDispositionContentChanged, status: http.StatusConflict, code: "disposition.content_changed"},
		{name: "文章不存在", err: ErrNotFound, status: http.StatusNotFound, code: "resource.not_found"},
		{name: "渠道不可用", err: ErrDispositionChannelUnavailable, status: http.StatusUnprocessableEntity, code: "disposition.channel_unavailable"},
		{name: "命令无效", err: ErrDispositionInvalid, status: http.StatusBadRequest, code: "request.invalid"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/batch-disposition", strings.NewReader(`{"operation":"ignored","articles":[{"id":"a1","content_version":"v1"}]}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://localhost")
			response := httptest.NewRecorder()
			NewRouter(&fakeAPI{dispositionErr: test.err}).ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("code=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type fakeAPI struct {
	page               ArticlePage
	listQuery          ArticleListQuery
	listCalls          int
	jobID              string
	err                error
	dispositionCommand BatchDispositionCommand
	dispositionResult  BatchDispositionResult
	dispositionErr     error
}

func (a *fakeAPI) ListArticles(_ context.Context, query ArticleListQuery) (ArticlePage, error) {
	a.listQuery = query
	a.listCalls++
	return a.page, a.err
}

func (a *fakeAPI) QueuePublication(context.Context, PublicationCommand) (string, error) {
	return a.jobID, a.err
}

func (a *fakeAPI) ConfirmWeChat(context.Context, ConfirmCommand) error    { return a.err }
func (a *fakeAPI) MarkWeChatCopied(context.Context, ConfirmCommand) error { return a.err }

func (a *fakeAPI) BatchDisposition(_ context.Context, command BatchDispositionCommand) (BatchDispositionResult, error) {
	a.dispositionCommand = command
	return a.dispositionResult, a.dispositionErr
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &value)
	return value
}
