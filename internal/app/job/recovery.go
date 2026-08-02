package job

import (
	"context"
	"fmt"
	"time"

	platformlogging "github.com/gkmz/InkHub/internal/platform/logging"
	"go.uber.org/zap"
)

// Recover 处理应用启动前遗留的超时 running 任务。
func (r *Runner) Recover(ctx context.Context, staleBefore time.Time) error {
	values, err := r.store.ListRunningBefore(ctx, staleBefore)
	if err != nil {
		fields := append([]zap.Field{zap.String("error_code", "job_recovery_list_failed")}, platformlogging.ErrorFields(err)...)
		r.logger.Error("读取待恢复后台任务失败", fields...)
		return err
	}
	for _, value := range values {
		r.mu.RLock()
		options, exists := r.handlers[value.Kind]
		r.mu.RUnlock()
		if exists && options.RetrySafe && value.Attempts < options.MaxAttempts {
			if err := r.store.RequeueRecovered(ctx, value.ID, r.config.Now(), r.config.Now()); err != nil {
				fields := append(jobLogFields(value), zap.String("error_code", "job_recovery_requeue_failed"))
				fields = append(fields, platformlogging.ErrorFields(err)...)
				r.logger.Error("重新排队恢复任务失败", fields...)
				return fmt.Errorf("重新排队恢复任务 %s: %w", value.ID, err)
			}
			continue
		}
		if err := r.store.FailRecovered(ctx, value.ID, r.config.Now()); err != nil {
			fields := append(jobLogFields(value), zap.String("error_code", "job_recovery_fail_failed"))
			fields = append(fields, platformlogging.ErrorFields(err)...)
			r.logger.Error("标记不可恢复任务失败", fields...)
			return fmt.Errorf("标记不可恢复任务 %s: %w", value.ID, err)
		}
	}
	return nil
}
