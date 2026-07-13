package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

func TestRunnerSerializesJobsForSameArticle(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	runner := NewRunner(store, Config{Workers: 2, PollInterval: time.Millisecond})
	var active atomic.Int32
	var maximum atomic.Int32
	runner.Register("analyze", HandlerOptions{
		RetrySafe: true,
		Handle: func(context.Context, *Execution) (string, error) {
			current := active.Add(1)
			for {
				seen := maximum.Load()
				if current <= seen || maximum.CompareAndSwap(seen, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return `{}`, nil
		},
	})
	for _, id := range []string{"job_1", "job_2"} {
		if _, _, err := runner.Enqueue(context.Background(), EnqueueRequest{
			ID: id, WorkspaceID: "w1", Kind: "analyze", PayloadJSON: `{"article_id":"article_1"}`,
		}); err != nil {
			t.Fatalf("任务入队: %v", err)
		}
	}
	if err := runner.Start(context.Background()); err != nil {
		t.Fatalf("启动 Runner: %v", err)
	}
	waitForJobState(t, store, "job_1", domainjob.StateSucceeded)
	waitForJobState(t, store, "job_2", domainjob.StateSucceeded)
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatalf("关闭 Runner: %v", err)
	}
	if maximum.Load() != 1 {
		t.Fatalf("同一文章任务发生并发执行: max=%d", maximum.Load())
	}
}

func TestRunnerCancelPropagatesContextAndPersistsCancellation(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	runner := NewRunner(store, Config{Workers: 1, PollInterval: time.Millisecond})
	started := make(chan struct{})
	runner.Register("analyze", HandlerOptions{
		RetrySafe: true,
		Handle: func(ctx context.Context, _ *Execution) (string, error) {
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if _, _, err := runner.Enqueue(context.Background(), EnqueueRequest{ID: "job_1", WorkspaceID: "w1", Kind: "analyze"}); err != nil {
		t.Fatalf("任务入队: %v", err)
	}
	if err := runner.Start(context.Background()); err != nil {
		t.Fatalf("启动 Runner: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Handler 未启动")
	}
	if err := runner.Cancel(context.Background(), "job_1"); err != nil {
		t.Fatalf("取消任务: %v", err)
	}
	waitForJobState(t, store, "job_1", domainjob.StateCancelled)
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatalf("关闭 Runner: %v", err)
	}
}

func waitForJobState(t *testing.T, store *repository.JobRepository, id string, state domainjob.State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		value, err := store.FindByID(context.Background(), id)
		if err == nil && value.State == state {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	value, _ := store.FindByID(context.Background(), id)
	t.Fatalf("任务 %s 未进入 %s，当前=%s", id, state, value.State)
}
