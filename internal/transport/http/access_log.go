package httptransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"time"

	platformlogging "github.com/gkmz/InkHub/internal/platform/logging"
	"go.uber.org/zap"
)

type requestIDContextKey struct{}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// RequestID 返回访问日志为当前 HTTP 请求分配的关联 ID。
func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

// AccessLog 为 HTTP 请求补充 request ID，并记录不含正文和查询参数的访问摘要。
func AccessLog(logger *zap.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		start := time.Now()
		requestID := request.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(requestID) {
			requestID = newRequestID()
		}
		response.Header().Set("X-Request-ID", requestID)
		request = request.WithContext(context.WithValue(request.Context(), requestIDContextKey{}, requestID))
		wrapped := &accessResponseWriter{ResponseWriter: response, status: http.StatusOK}

		next.ServeHTTP(wrapped, request)

		fields := []zap.Field{
			zap.String("request_id", requestID),
			zap.String("method", request.Method),
			zap.String("path", request.URL.Path),
			zap.Int("status", wrapped.status),
			zap.Int64("response_bytes", wrapped.bytes),
			platformlogging.Duration(start),
		}
		switch {
		case wrapped.status >= http.StatusInternalServerError:
			logger.Error("HTTP 请求完成", fields...)
		case wrapped.status >= http.StatusBadRequest:
			logger.Warn("HTTP 请求完成", fields...)
		default:
			logger.Info("HTTP 请求完成", fields...)
		}
	})
}

type accessResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (writer *accessResponseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.status = status
	writer.wroteHeader = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *accessResponseWriter) Write(content []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	written, err := writer.ResponseWriter.Write(content)
	writer.bytes += int64(written)
	return written, err
}

// Unwrap 让 net/http.ResponseController 继续访问底层 writer 的可选能力。
func (writer *accessResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func newRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	// 系统随机源异常时仍生成合法关联值；纳秒值只用于日志关联，不作为安全令牌。
	return time.Now().UTC().Format("20060102T150405.000000000")
}
