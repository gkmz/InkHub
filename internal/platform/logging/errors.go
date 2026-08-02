package logging

import (
	"errors"
	"reflect"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"go.uber.org/zap"
)

// ErrorFields 将 ProviderError 的稳定诊断属性转换为结构化日志字段。
// 该函数只输出脱敏元数据，不输出 Provider 请求体、响应体或 Secret。
func ErrorFields(err error) []zap.Field {
	if err == nil {
		return nil
	}
	fields := []zap.Field{zap.Error(err)}
	var providerErr *contracts.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return fields
	}
	fields = append(fields,
		zap.String("provider_error_code", providerErr.Code),
		zap.String("provider_error_category", string(providerErr.Category)),
		zap.Bool("provider_error_retryable", providerErr.Retryable),
	)
	if providerErr.Field != "" {
		fields = append(fields, zap.String("provider_error_field", providerErr.Field))
	}
	if providerErr.Cause != nil {
		// Cause 只记录类型，避免底层错误意外携带路径、响应正文或凭据。
		fields = append(fields, zap.String("provider_error_cause_type", errorType(providerErr.Cause)))
	}
	if providerErr.UpstreamStatus != 0 {
		fields = append(fields, zap.Int("provider_upstream_status", providerErr.UpstreamStatus))
	}
	if providerErr.UpstreamCode != "" {
		fields = append(fields, zap.String("provider_upstream_code", providerErr.UpstreamCode))
	}
	if providerErr.UpstreamMessage != "" {
		fields = append(fields, zap.String("provider_upstream_message", providerErr.UpstreamMessage))
	}
	return fields
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return reflect.TypeOf(err).String()
}
