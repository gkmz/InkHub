package httptransport

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAccessLogRecordsRequestMetadataAndRequestID(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	handler := AccessLog(zap.New(core), http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if RequestID(request.Context()) != "request-123" {
			t.Fatalf("RequestID() = %q", RequestID(request.Context()))
		}
		response.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(response, "ok")
	}))
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles?secret=query-value", strings.NewReader("body-secret"))
	request.Header.Set("X-Request-ID", "request-123")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Header().Get("X-Request-ID") != "request-123" {
		t.Fatalf("response request id = %q", response.Header().Get("X-Request-ID"))
	}
	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["method"] != http.MethodPost || fields["path"] != "/api/v1/articles" || fields["status"] != int64(http.StatusCreated) || fields["response_bytes"] != int64(2) || fields["request_id"] != "request-123" {
		t.Fatalf("fields = %#v", fields)
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Fatalf("duration_ms missing: %#v", fields)
	}
	if strings.Contains(entries[0].Message+entries[0].ContextMap()["path"].(string), "secret") {
		t.Fatalf("日志包含查询参数或请求体: %#v", entries[0])
	}
}

func TestAccessLogIncludesErrorCodeAndProviderDiagnostics(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.DebugLevel)
	handler := AccessLog(zap.New(core), http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		mapError(response, &contracts.ProviderError{
			Code: "openai.timeout", Category: contracts.ErrorTemporary, Message: "AI 请求超时", Retryable: true,
			Cause: errors.New("upstream timeout"),
		})
	}))
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/articles/a1/suggestions", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"openai.timeout"`) {
		t.Fatalf("provider error response = %d %s", response.Code, response.Body.String())
	}
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("log count = %d, want handler and access entries", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["error_code"] != "openai.timeout" || fields["provider_error_category"] != "temporary" || fields["provider_error_retryable"] != true || fields["provider_error_cause_type"] != "*errors.errorString" {
		t.Fatalf("provider diagnostics = %#v", fields)
	}
	accessFields := entries[1].ContextMap()
	if accessFields["error_code"] != "openai.timeout" || accessFields["status"] != int64(http.StatusServiceUnavailable) {
		t.Fatalf("access diagnostics = %#v", accessFields)
	}
	if strings.Contains(entries[0].Message, "upstream timeout") {
		t.Fatalf("日志不应重复写入底层错误正文: %#v", entries[0])
	}
}

func TestAccessLogReplacesInvalidRequestID(t *testing.T) {
	t.Parallel()

	core, _ := observer.New(zap.InfoLevel)
	handler := AccessLog(zap.New(core), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodGet, "http://localhost/health", nil)
	request.Header.Set("X-Request-ID", "invalid request id with spaces")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	requestID := response.Header().Get("X-Request-ID")
	if requestID == "" || requestID == request.Header.Get("X-Request-ID") {
		t.Fatalf("generated request id = %q", requestID)
	}
}

func TestAccessLogSkipsSuccessfulStaticAssets(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	handler := AccessLog(zap.New(core), http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(response, "asset")
	}))
	request := httptest.NewRequest(http.MethodGet, "http://localhost/assets/index.js", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("static response status = %d", response.Code)
	}
	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("successful static request should be omitted, got %#v", entries)
	}
}

func TestAccessLogKeepsFailedStaticAssets(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.WarnLevel)
	handler := AccessLog(zap.New(core), http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNotFound)
	}))
	request := httptest.NewRequest(http.MethodGet, "http://localhost/assets/missing.js", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if len(observed.All()) != 1 {
		t.Fatalf("failed static request should be logged, got %d entries", len(observed.All()))
	}
}
