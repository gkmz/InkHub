// Package httptransport 提供版本化的本机 JSON API。
package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	platformlogging "github.com/gkmz/InkHub/internal/platform/logging"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"go.uber.org/zap"
)

var (
	// ErrStaleContent 表示命令引用了过期文章版本。
	ErrStaleContent = errors.New("文章内容版本已过期")
	// ErrNotFound 表示请求的本地资源不存在。
	ErrNotFound = errors.New("资源不存在")
	// ErrInvalidCursor 表示列表分页位置损坏或不是服务端签发的结构。
	ErrInvalidCursor = errors.New("分页 Cursor 无效")
	// ErrDispositionContentChanged 表示批量处置包含已经变化的文章。
	ErrDispositionContentChanged = errors.New("批量处置文章内容已变化")
	// ErrDispositionInvalid 表示批量处置命令无效。
	ErrDispositionInvalid = errors.New("批量处置请求无效")
	// ErrDispositionChannelUnavailable 表示批量处置选择了未启用渠道。
	ErrDispositionChannelUnavailable = errors.New("批量处置渠道不可用")
	// ErrArticleNotReady 表示文章尚未由作者明确标记为已就绪。
	ErrArticleNotReady = errors.New("文章尚未标记为已就绪")
)

// ArticleSummary 是内容库列表使用的脱敏 DTO。
type ArticleSummary struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Directory         string `json:"directory"`
	Category          string `json:"category"`
	ModifiedAt        string `json:"modified_at"`
	State             string `json:"state"`
	HugoState         string `json:"hugo_state"`
	WeChatState       string `json:"wechat_state"`
	XiaohongshuState  string `json:"xiaohongshu_state"`
	ContentVersion    string `json:"content_version"`
	Disposition       string `json:"disposition,omitempty"`
	ContentStage      string `json:"content_stage"`
	ContentStageIssue string `json:"content_stage_issue,omitempty"`
	NextAction        string `json:"next_action,omitempty"`
}

// ArticleListQuery 描述内容库的分页、搜索和固定枚举筛选条件。
type ArticleListQuery struct {
	Cursor       string
	Limit        int
	Search       string
	State        string
	Disposition  string
	ContentStage string
}

// ArticlePage 是基于不透明 Cursor 的文章分页结果。
type ArticlePage struct {
	Items             []ArticleSummary `json:"items"`
	NextCursor        string           `json:"next_cursor,omitempty"`
	AvailableChannels []string         `json:"available_channels"`
}

// DashboardView 是工作台按行动优先级划分的互斥文章分组。
type DashboardView struct {
	Failed          []ArticleSummary `json:"failed"`
	Changed         []ArticleSummary `json:"changed"`
	NeedsReview     []ArticleSummary `json:"needs_review"`
	ReadyToPublish  []ArticleSummary `json:"ready_to_publish"`
	LatestReady     []ArticleSummary `json:"latest_ready"`
	RecentlyHandled []ArticleSummary `json:"recently_handled"`
}

// PublicationCommand 描述一个进入后台队列的渠道操作。
type PublicationCommand struct {
	ArticleID          string `json:"article_id"`
	ProviderInstanceID string `json:"provider_instance_id"`
	Channel            string `json:"channel"`
	ContentHash        string `json:"content_hash"`
}

// ConfirmCommand 描述微信草稿人工确认。
type ConfirmCommand struct {
	ArticleID          string `json:"article_id"`
	ProviderInstanceID string `json:"provider_instance_id"`
	ContentHash        string `json:"content_hash"`
}

// ArticleVersion 是批量命令引用的文章和用户已见内容版本。
type ArticleVersion struct {
	ID             string `json:"id"`
	ContentVersion string `json:"content_version"`
}

// BatchDispositionCommand 描述文章批量处置请求。
type BatchDispositionCommand struct {
	Operation string           `json:"operation"`
	Articles  []ArticleVersion `json:"articles"`
	Channels  []string         `json:"channels,omitempty"`
}

// BatchDispositionResult 汇总文章批量处置结果。
type BatchDispositionResult struct {
	Processed int `json:"processed"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
}

// API 是 HTTP Adapter 调用的 Application 用例集合。
type API interface {
	ListArticles(ctx context.Context, query ArticleListQuery) (ArticlePage, error)
	Dashboard(ctx context.Context) (DashboardView, error)
	QueuePublication(ctx context.Context, command PublicationCommand) (string, error)
	MarkWeChatCopied(ctx context.Context, command ConfirmCommand) error
	ConfirmWeChat(ctx context.Context, command ConfirmCommand) error
	BatchDisposition(ctx context.Context, command BatchDispositionCommand) (BatchDispositionResult, error)
}

// NewRouter 创建带本机同源边界的 `/api/v1` Router。
func NewRouter(api API) http.Handler {
	router := &router{api: api}
	return localOnly(router)
}

type router struct{ api API }

// ServeHTTP 路由版本化 API，并在写请求前执行同源校验。
func (r *router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/articles":
		r.listArticles(response, request)
	case request.Method == http.MethodGet && request.URL.Path == "/api/v1/dashboard":
		r.dashboard(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/articles/batch-disposition":
		if !validateWriteRequest(response, request) {
			return
		}
		r.batchDisposition(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/publications":
		if !validateWriteRequest(response, request) {
			return
		}
		r.queuePublication(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/wechat/confirm":
		if !validateWriteRequest(response, request) {
			return
		}
		r.confirmWeChat(response, request)
	case request.Method == http.MethodPost && request.URL.Path == "/api/v1/wechat/copied":
		if !validateWriteRequest(response, request) {
			return
		}
		r.markWeChatCopied(response, request)
	default:
		writeError(response, http.StatusNotFound, "route.not_found", "接口不存在")
	}
}

func (r *router) dashboard(response http.ResponseWriter, request *http.Request) {
	view, err := r.api.Dashboard(request.Context())
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, view)
}

func (r *router) batchDisposition(response http.ResponseWriter, request *http.Request) {
	var command BatchDispositionCommand
	if err := decodeJSON(request, &command); err != nil || len(command.Articles) < 1 || len(command.Articles) > 100 {
		writeError(response, http.StatusBadRequest, "request.invalid", "文章批量处置请求无效")
		return
	}
	for _, article := range command.Articles {
		if strings.TrimSpace(article.ID) == "" || strings.TrimSpace(article.ContentVersion) == "" {
			writeError(response, http.StatusBadRequest, "request.invalid", "文章批量处置请求无效")
			return
		}
	}
	result, err := r.api.BatchDisposition(request.Context(), command)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (r *router) markWeChatCopied(response http.ResponseWriter, request *http.Request) {
	var command ConfirmCommand
	if err := decodeJSON(request, &command); err != nil || command.ArticleID == "" || command.ProviderInstanceID == "" || command.ContentHash == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "微信复制请求无效")
		return
	}
	if err := r.api.MarkWeChatCopied(request.Context(), command); err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"state": "copied"})
}

func (r *router) listArticles(response http.ResponseWriter, request *http.Request) {
	limit := 50
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(response, http.StatusBadRequest, "request.invalid", "分页参数无效")
			return
		}
		limit = parsed
	}
	state := request.URL.Query().Get("state")
	disposition := request.URL.Query().Get("disposition")
	contentStage := request.URL.Query().Get("stage")
	if !validArticleStateFilter(state) || !validArticleDispositionFilter(disposition) || !validArticleContentStageFilter(contentStage) {
		writeError(response, http.StatusBadRequest, "request.invalid", "文章筛选参数无效")
		return
	}
	query := ArticleListQuery{
		Cursor:       request.URL.Query().Get("cursor"),
		Limit:        limit,
		Search:       strings.TrimSpace(request.URL.Query().Get("q")),
		State:        state,
		Disposition:  disposition,
		ContentStage: contentStage,
	}
	page, err := r.api.ListArticles(request.Context(), query)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, page)
}

func validArticleContentStageFilter(value string) bool {
	return value == "" || value == "draft" || value == "ready"
}

func validArticleStateFilter(value string) bool {
	switch value {
	case "", "draft", "incomplete", "pending_review", "approved", "changed", "blocked":
		return true
	default:
		return false
	}
}

func validArticleDispositionFilter(value string) bool {
	switch value {
	case "", "published", "ignored", "unresolved":
		return true
	default:
		return false
	}
}

func (r *router) queuePublication(response http.ResponseWriter, request *http.Request) {
	var command PublicationCommand
	if err := decodeJSON(request, &command); err != nil || command.ArticleID == "" || command.ProviderInstanceID == "" || command.ContentHash == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "发布请求无效")
		return
	}
	jobID, err := r.api.QueuePublication(request.Context(), command)
	if err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (r *router) confirmWeChat(response http.ResponseWriter, request *http.Request) {
	var command ConfirmCommand
	if err := decodeJSON(request, &command); err != nil || command.ArticleID == "" || command.ProviderInstanceID == "" || command.ContentHash == "" {
		writeError(response, http.StatusBadRequest, "request.invalid", "微信确认请求无效")
		return
	}
	if err := r.api.ConfirmWeChat(request.Context(), command); err != nil {
		mapError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"state": "confirmed"})
}

func decodeJSON(request *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("请求包含多个 JSON 值")
	}
	return nil
}

func validateWriteRequest(response http.ResponseWriter, request *http.Request) bool {
	if !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		writeError(response, http.StatusUnsupportedMediaType, "request.content_type", "写请求必须使用 JSON")
		return false
	}
	origin := request.Header.Get("Origin")
	parsed, err := url.Parse(origin)
	expectedScheme := request.URL.Scheme
	if expectedScheme == "" {
		expectedScheme = "http"
		if request.TLS != nil {
			expectedScheme = "https"
		}
	}
	if err != nil || !strings.EqualFold(parsed.Scheme, expectedScheme) || !strings.EqualFold(parsed.Host, request.Host) {
		writeError(response, http.StatusForbidden, "request.origin_forbidden", "写请求来源不受信任")
		return false
	}
	return true
}

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		host := request.Host
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			host = parsedHost
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && !net.ParseIP(host).IsLoopback() {
			writeError(response, http.StatusForbidden, "request.host_forbidden", "仅允许本机访问")
			return
		}
		next.ServeHTTP(response, request)
	})
}

func mapError(response http.ResponseWriter, err error) {
	status, code, message := mappedError(err)
	logHTTPError(response, err, status, code)
	writeError(response, status, code, message)
}

func mappedError(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, ErrStaleContent):
		return http.StatusConflict, "content.stale", "文章内容已变化，请刷新后重试"
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound, "resource.not_found", "请求的资源不存在"
	case errors.Is(err, ErrInvalidCursor):
		return http.StatusBadRequest, "request.cursor_invalid", "分页位置无效，请重新加载列表"
	case errors.Is(err, ErrDispositionContentChanged):
		return http.StatusConflict, "disposition.content_changed", "部分文章已更新，请刷新后重新选择"
	case errors.Is(err, ErrDispositionChannelUnavailable):
		return http.StatusUnprocessableEntity, "disposition.channel_unavailable", "所选发布渠道未配置或未启用"
	case errors.Is(err, ErrDispositionInvalid):
		return http.StatusBadRequest, "request.invalid", "文章批量处置请求无效"
	case errors.Is(err, ErrArticleNotReady):
		return http.StatusUnprocessableEntity, "article.not_ready", "文章尚未标记为已就绪"
	}
	var providerErr *contracts.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		status := http.StatusInternalServerError
		switch providerErr.Category {
		case contracts.ErrorValidation:
			status = http.StatusBadRequest
		case contracts.ErrorConflict:
			status = http.StatusConflict
		case contracts.ErrorNotFound:
			status = http.StatusNotFound
		case contracts.ErrorDependency:
			status = http.StatusBadGateway
		case contracts.ErrorTemporary:
			status = http.StatusServiceUnavailable
		case contracts.ErrorPermanent:
			status = http.StatusUnprocessableEntity
		case contracts.ErrorUnauthorizedResource:
			status = http.StatusForbidden
		}
		return status, providerErr.Code, providerErr.Message
	}
	return http.StatusInternalServerError, "internal.error", "操作失败"
}

func logHTTPError(response http.ResponseWriter, err error, status int, code string) {
	holder, ok := response.(interface{ requestForLogging() *http.Request })
	if !ok || holder.requestForLogging() == nil {
		return
	}
	request := holder.requestForLogging()
	fields := []zap.Field{
		zap.String("request_id", RequestID(request.Context())),
		zap.String("method", request.Method),
		zap.String("path", request.URL.Path),
		zap.Int("status", status),
		zap.String("error_code", code),
	}
	fields = append(fields, platformlogging.ErrorFields(err)...)
	logger := RequestLogger(request.Context())
	if status >= http.StatusInternalServerError {
		logger.Error("HTTP 请求处理失败", fields...)
		return
	}
	logger.Warn("HTTP 请求处理失败", fields...)
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	if setter, ok := response.(interface{ setErrorCode(string) }); ok {
		setter.setErrorCode(code)
	}
	writeJSON(response, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
