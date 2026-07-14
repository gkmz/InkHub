package httptransport

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewApplicationHandlerKeepsAPIRoutesOutOfSPA(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>InkHub SPA</html>")},
		"assets/app.js": {Data: []byte("console.log('inkhub')")},
	}
	api := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusTeapot)
	})
	handler := NewApplicationHandler(api, mustSub(t, assets, "."))

	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/missing", nil))
	if apiResponse.Code != http.StatusTeapot || strings.Contains(apiResponse.Body.String(), "InkHub SPA") {
		t.Fatalf("API 错误落入 SPA: code=%d body=%s", apiResponse.Code, apiResponse.Body.String())
	}

	uiResponse := httptest.NewRecorder()
	handler.ServeHTTP(uiResponse, httptest.NewRequest(http.MethodGet, "http://localhost/articles/a1", nil))
	if uiResponse.Code != http.StatusOK || !strings.Contains(uiResponse.Body.String(), "InkHub SPA") {
		t.Fatalf("未知 UI 路由未返回 React 入口: code=%d body=%s", uiResponse.Code, uiResponse.Body.String())
	}

	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, httptest.NewRequest(http.MethodGet, "http://localhost/assets/app.js", nil))
	if assetResponse.Code != http.StatusOK || !strings.Contains(assetResponse.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("静态资源响应错误: code=%d type=%s", assetResponse.Code, assetResponse.Header().Get("Content-Type"))
	}
}

func TestRecoveryHandlerRejectsAPIWhileStaticUIRemainsAvailable(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("recovery-ui")}}
	handler := NewApplicationHandler(NewRecoveryHandler(), mustSub(t, assets, "."))
	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader("{}")))
	if apiResponse.Code != http.StatusServiceUnavailable || !strings.Contains(apiResponse.Body.String(), "recovery.read_only") {
		t.Fatalf("恢复 API 边界错误: %d %s", apiResponse.Code, apiResponse.Body.String())
	}
	uiResponse := httptest.NewRecorder()
	handler.ServeHTTP(uiResponse, httptest.NewRequest(http.MethodGet, "http://localhost/settings", nil))
	if uiResponse.Code != http.StatusOK || !strings.Contains(uiResponse.Body.String(), "recovery-ui") {
		t.Fatalf("恢复 UI 不可用: %d %s", uiResponse.Code, uiResponse.Body.String())
	}
}

func mustSub(t *testing.T, source fs.FS, directory string) fs.FS {
	t.Helper()
	result, err := fs.Sub(source, directory)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
