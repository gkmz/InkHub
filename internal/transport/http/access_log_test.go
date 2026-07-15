package httptransport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
