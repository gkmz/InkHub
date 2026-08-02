// Package job 编排可恢复的本地持久化后台任务。
package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	platformlogging "github.com/gkmz/InkHub/internal/platform/logging"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"go.uber.org/zap"
)

// Store 定义 Runner 所需的短事务持久化操作。
type Store interface {
	Enqueue(ctx context.Context, value domainjob.Job) (domainjob.Job, bool, error)
	ClaimNext(ctx context.Context, now time.Time) (domainjob.Job, bool, error)
	FindByID(ctx context.Context, id string) (domainjob.Job, error)
	UpdateProgress(ctx context.Context, id string, progress int, now time.Time) error
	Succeed(ctx context.Context, id, resultJSON string, now time.Time) error
	Retry(ctx context.Context, id string, availableAt time.Time, code, message string, now time.Time) error
	Fail(ctx context.Context, id, code, message string, now time.Time) error
	Cancel(ctx context.Context, id string, now time.Time) error
	ListRunningBefore(ctx context.Context, before time.Time) ([]domainjob.Job, error)
	RequeueRecovered(ctx context.Context, id string, availableAt, now time.Time) error
	FailRecovered(ctx context.Context, id string, now time.Time) error
}

// Config 定义 Runner 的 worker 数量、轮询间隔、时钟和结构化日志。
type Config struct {
	Workers      int
	PollInterval time.Duration
	Now          func() time.Time
	Logger       *zap.Logger
}

// EnqueueRequest 描述一个只包含可重建参数的任务意图。
type EnqueueRequest struct {
	ID                 string
	WorkspaceID        string
	Kind               string
	DedupeKey          string
	PayloadJSON        string
	ArticleID          string
	ProviderInstanceID string
	ContentHash        string
}

// Handler 执行单个已领取任务，返回可持久化的有限结果。
type Handler func(ctx context.Context, execution *Execution) (resultJSON string, err error)

// Failure 是任务最终失败后可交给业务层持久化的安全摘要。
type Failure struct {
	Code    string
	Message string
	Attempt int
}

// HandlerOptions 定义任务处理器的重试安全边界。
type HandlerOptions struct {
	Handle            Handler
	MaxAttempts       int
	RetrySafe         bool
	Backoff           func(attempt int) time.Duration
	OnTerminalFailure func(context.Context, domainjob.Job, Failure) error
}

// Execution 向 Handler 暴露任务快照和受控进度更新。
type Execution struct {
	Job   domainjob.Job
	store Store
	now   func() time.Time
}

// ReportProgress 持久化运行中任务的进度，不允许 Handler 直接操作 Repository。
func (e *Execution) ReportProgress(ctx context.Context, progress int) error {
	return e.store.UpdateProgress(ctx, e.Job.ID, progress, e.now())
}

// Runner 使用有限 worker 执行 SQLite 中的持久化任务。
type Runner struct {
	store    Store
	config   Config
	mu       sync.RWMutex
	handlers map[string]HandlerOptions
	locks    *keyedLocker
	runCtx   context.Context
	stop     context.CancelFunc
	wait     sync.WaitGroup
	activeMu sync.Mutex
	active   map[string]context.CancelFunc
	logger   *zap.Logger
}

// NewRunner 创建尚未启动 worker 的任务 Runner。
func NewRunner(store Store, config Config) *Runner {
	if config.Workers <= 0 {
		config.Workers = 2
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 200 * time.Millisecond
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}
	return &Runner{
		store: store, config: config, handlers: make(map[string]HandlerOptions),
		locks: newKeyedLocker(), active: make(map[string]context.CancelFunc), logger: config.Logger,
	}
}

// Register 注册一个类型化任务处理器。
func (r *Runner) Register(kind string, options HandlerOptions) {
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 1
		if options.RetrySafe {
			options.MaxAttempts = 3
		}
	}
	if options.Backoff == nil {
		options.Backoff = defaultBackoff
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = options
}

// Enqueue 持久化任务意图，并对活动任务执行确定性去重。
func (r *Runner) Enqueue(ctx context.Context, request EnqueueRequest) (domainjob.Job, bool, error) {
	if request.PayloadJSON == "" {
		request.PayloadJSON = "{}"
	}
	if !json.Valid([]byte(request.PayloadJSON)) {
		return domainjob.Job{}, false, fmt.Errorf("任务 payload 必须是有效 JSON")
	}
	payload, err := mergeTargetMetadata(request.PayloadJSON, request.ArticleID, request.ProviderInstanceID)
	if err != nil {
		r.logger.Warn("后台任务入队请求无效", append(requestLogFields(request), platformlogging.ErrorFields(err)...)...)
		return domainjob.Job{}, false, err
	}
	request.PayloadJSON = payload
	if request.DedupeKey == "" {
		request.DedupeKey = BuildDedupeKey(request.Kind, request.ArticleID, request.ProviderInstanceID, request.ContentHash)
	}
	value, created, err := r.store.Enqueue(ctx, domainjob.Job{
		ID: request.ID, WorkspaceID: request.WorkspaceID, Kind: request.Kind,
		DedupeKey: request.DedupeKey, PayloadJSON: request.PayloadJSON, AvailableAt: r.config.Now(),
	})
	if err != nil {
		fields := append(requestLogFields(request), zap.String("error_code", "job_enqueue_failed"))
		fields = append(fields, platformlogging.ErrorFields(err)...)
		r.logger.Error("后台任务入队失败", fields...)
		return domainjob.Job{}, false, err
	}
	r.logger.Info("后台任务已入队", append(requestLogFields(request), zap.Bool("created", created))...)
	return value, created, nil
}

// BuildDedupeKey 根据任务目标和内容版本生成稳定去重键。
func BuildDedupeKey(kind, articleID, providerInstanceID, contentHash string) string {
	if articleID == "" && providerInstanceID == "" && contentHash == "" {
		return ""
	}
	encoded, _ := json.Marshal([]string{kind, articleID, providerInstanceID, contentHash})
	sum := sha256.Sum256(encoded)
	return "job:" + hex.EncodeToString(sum[:])
}

func mergeTargetMetadata(payloadJSON, articleID, providerInstanceID string) (string, error) {
	if articleID == "" && providerInstanceID == "" {
		return payloadJSON, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil || payload == nil {
		return "", fmt.Errorf("带文章或 Provider 目标的任务 payload 必须是 JSON 对象")
	}
	for key, expected := range map[string]string{
		"article_id": articleID, "provider_instance_id": providerInstanceID,
	} {
		if expected == "" {
			continue
		}
		if current, exists := payload[key]; exists {
			var value string
			if json.Unmarshal(current, &value) != nil || value != expected {
				return "", fmt.Errorf("任务 payload 中的 %s 与目标不一致", key)
			}
		}
		payload[key], _ = json.Marshal(expected)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("编码任务 payload: %w", err)
	}
	return string(encoded), nil
}

// RunOne 原子领取并同步执行一个当前可运行任务。
func (r *Runner) RunOne(ctx context.Context) (bool, error) {
	value, claimed, err := r.store.ClaimNext(ctx, r.config.Now())
	if err != nil || !claimed {
		if err != nil {
			fields := []zap.Field{zap.String("error_code", "job_claim_failed")}
			fields = append(fields, platformlogging.ErrorFields(err)...)
			r.logger.Error("领取后台任务失败", fields...)
		}
		return claimed, err
	}
	start := time.Now()
	fields := jobLogFields(value)
	r.logger.Info("后台任务开始执行", fields...)
	r.mu.RLock()
	options, exists := r.handlers[value.Kind]
	r.mu.RUnlock()
	if !exists || options.Handle == nil {
		err := r.store.Fail(ctx, value.ID, "job.handler_missing", "任务处理器未注册", r.config.Now())
		logFields := append(fields, zap.String("error_code", "job.handler_missing"), elapsedField(start))
		logFields = append(logFields, platformlogging.ErrorFields(err)...)
		r.logger.Error("后台任务执行失败", logFields...)
		return true, err
	}

	taskCtx, cancel := context.WithCancel(ctx)
	r.activeMu.Lock()
	r.active[value.ID] = cancel
	r.activeMu.Unlock()
	defer func() {
		cancel()
		r.activeMu.Lock()
		delete(r.active, value.ID)
		r.activeMu.Unlock()
	}()
	release, err := r.locks.acquire(taskCtx, lockKeys(value.PayloadJSON))
	if err != nil {
		finishErr := r.finishInterrupted(ctx, value, options)
		if finishErr != nil {
			fields := append(jobLogFields(value), platformlogging.ErrorFields(finishErr)...)
			r.logger.Error("后台任务中断状态保存失败", fields...)
		}
		return true, finishErr
	}
	defer release()

	execution := &Execution{Job: value, store: r.store, now: r.config.Now}
	result, handleErr := options.Handle(taskCtx, execution)
	if taskCtx.Err() != nil {
		stored, findErr := r.store.FindByID(context.Background(), value.ID)
		if findErr == nil && stored.State == domainjob.StateCancelled {
			return true, nil
		}
		finishErr := r.finishInterrupted(ctx, value, options)
		if finishErr != nil {
			logFields := append(fields, platformlogging.ErrorFields(finishErr)...)
			r.logger.Error("后台任务中断状态保存失败", logFields...)
		}
		return true, finishErr
	}
	if handleErr == nil {
		if result == "" {
			result = "{}"
		}
		if !json.Valid([]byte(result)) {
			err := r.store.Fail(ctx, value.ID, "job.result_invalid", "任务返回了无效结果", r.config.Now())
			logFields := append(fields, zap.String("error_code", "job.result_invalid"), elapsedField(start))
			logFields = append(logFields, platformlogging.ErrorFields(err)...)
			r.logger.Error("后台任务执行失败", logFields...)
			return true, err
		}
		err := r.store.Succeed(ctx, value.ID, result, r.config.Now())
		if err != nil {
			logFields := append(fields, zap.String("error_code", "job_succeed_store_failed"), elapsedField(start))
			logFields = append(logFields, platformlogging.ErrorFields(err)...)
			r.logger.Error("保存后台任务结果失败", logFields...)
			return true, err
		}
		r.logger.Info("后台任务执行成功", append(fields, elapsedField(start))...)
		return true, nil
	}
	code, message, retryable := classifyError(handleErr)
	if retryable && options.RetrySafe && value.Attempts < options.MaxAttempts {
		availableAt := r.config.Now().Add(options.Backoff(value.Attempts))
		err := r.store.Retry(ctx, value.ID, availableAt, code, message, r.config.Now())
		logFields := append(fields, zap.String("error_code", code), zap.Int("attempt", value.Attempts), elapsedField(start))
		logFields = append(logFields, platformlogging.ErrorFields(handleErr)...)
		if err != nil {
			logFields = append(logFields, platformlogging.ErrorFields(err)...)
		}
		r.logger.Warn("后台任务等待重试", logFields...)
		return true, err
	}
	err = r.store.Fail(ctx, value.ID, code, message, r.config.Now())
	logFields := append(fields, zap.String("error_code", code), zap.Int("attempt", value.Attempts), elapsedField(start))
	logFields = append(logFields, platformlogging.ErrorFields(handleErr)...)
	if err != nil {
		logFields = append(logFields, platformlogging.ErrorFields(err)...)
	}
	r.logger.Error("后台任务执行失败", logFields...)
	if err != nil {
		return true, err
	}
	if options.OnTerminalFailure != nil {
		failure := Failure{Code: code, Message: message, Attempt: value.Attempts}
		if callbackErr := options.OnTerminalFailure(ctx, value, failure); callbackErr != nil {
			logFields := append(fields, zap.String("error_code", "job.failure_event_failed"))
			logFields = append(logFields, platformlogging.ErrorFields(callbackErr)...)
			r.logger.Error("保存任务失败事实失败", logFields...)
			return true, callbackErr
		}
	}
	return true, nil
}

func requestLogFields(request EnqueueRequest) []zap.Field {
	return []zap.Field{
		zap.String("job_id", request.ID),
		zap.String("workspace_id", request.WorkspaceID),
		zap.String("job_kind", request.Kind),
		zap.String("article_id", request.ArticleID),
		zap.String("provider_id", request.ProviderInstanceID),
	}
}

func jobLogFields(value domainjob.Job) []zap.Field {
	// payload 只解析关联 ID，正文和其他任务参数绝不进入日志字段。
	var target struct {
		ArticleID string `json:"article_id"`
		Provider  string `json:"provider_instance_id"`
	}
	_ = json.Unmarshal([]byte(value.PayloadJSON), &target)
	return []zap.Field{
		zap.String("job_id", value.ID),
		zap.String("workspace_id", value.WorkspaceID),
		zap.String("job_kind", value.Kind),
		zap.String("article_id", target.ArticleID),
		zap.String("provider_id", target.Provider),
	}
}

func elapsedField(start time.Time) zap.Field {
	return zap.Int64("duration_ms", time.Since(start).Milliseconds())
}

// Start 启动有限数量的 worker；重复启动会返回错误。
func (r *Runner) Start(ctx context.Context) error {
	if err := validateRunner(r); err != nil {
		return err
	}
	r.mu.Lock()
	if r.stop != nil {
		r.mu.Unlock()
		return fmt.Errorf("Job Runner 已启动")
	}
	r.runCtx, r.stop = context.WithCancel(ctx)
	r.mu.Unlock()
	for range r.config.Workers {
		r.wait.Add(1)
		go r.worker()
	}
	return nil
}

// Cancel 取消排队或当前进程正在执行的任务。
func (r *Runner) Cancel(ctx context.Context, id string) error {
	r.activeMu.Lock()
	cancel := r.active[id]
	r.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return r.store.Cancel(ctx, id, r.config.Now())
}

// Shutdown 停止领取新任务并等待现有 worker 完成中断处理。
func (r *Runner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	stop := r.stop
	if stop == nil {
		r.mu.Unlock()
		return nil
	}
	stop()
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.wait.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		r.mu.Lock()
		r.stop = nil
		r.runCtx = nil
		r.mu.Unlock()
		return nil
	}
}

func (r *Runner) worker() {
	defer r.wait.Done()
	for {
		worked, err := r.RunOne(r.runCtx)
		if r.runCtx.Err() != nil {
			return
		}
		if worked && err == nil {
			continue
		}
		// Repository 异常和空队列都使用有限轮询，避免数据库故障时 worker 快速空转。
		timer := time.NewTimer(r.config.PollInterval)
		select {
		case <-r.runCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (r *Runner) finishInterrupted(parent context.Context, value domainjob.Job, options HandlerOptions) error {
	if parent.Err() == nil {
		return r.store.Cancel(context.Background(), value.ID, r.config.Now())
	}
	if options.RetrySafe && value.Attempts < options.MaxAttempts {
		return r.store.Retry(context.Background(), value.ID, r.config.Now(), "job.shutdown", "应用关闭后重新排队", r.config.Now())
	}
	return r.store.Fail(context.Background(), value.ID, "job.recovery_unsafe", "任务中断后无法安全自动重试", r.config.Now())
}

func classifyError(err error) (code, message string, retryable bool) {
	var providerErr *contracts.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Code, providerErr.Message, providerErr.Retryable
	}
	if errors.Is(err, context.Canceled) {
		return "job.cancelled", "任务已取消", false
	}
	return "job.failed", "任务执行失败", false
}

func defaultBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

func validateRunner(r *Runner) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("Job Runner 或 Store 为空")
	}
	return nil
}
