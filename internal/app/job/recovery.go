package job

import (
	"context"
	"fmt"
	"time"
)

// Recover 处理应用启动前遗留的超时 running 任务。
func (r *Runner) Recover(ctx context.Context, staleBefore time.Time) error {
	values, err := r.store.ListRunningBefore(ctx, staleBefore)
	if err != nil {
		return err
	}
	for _, value := range values {
		r.mu.RLock()
		options, exists := r.handlers[value.Kind]
		r.mu.RUnlock()
		if exists && options.RetrySafe && value.Attempts < options.MaxAttempts {
			if err := r.store.RequeueRecovered(ctx, value.ID, r.config.Now(), r.config.Now()); err != nil {
				return fmt.Errorf("重新排队恢复任务 %s: %w", value.ID, err)
			}
			continue
		}
		if err := r.store.FailRecovered(ctx, value.ID, r.config.Now()); err != nil {
			return fmt.Errorf("标记不可恢复任务 %s: %w", value.ID, err)
		}
	}
	return nil
}
