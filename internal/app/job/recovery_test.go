package job

import (
	"context"
	"testing"
	"time"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
)

func TestRunnerRecoversOnlyRetrySafeRunningJobs(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	old := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	for _, value := range []domainjob.Job{
		{ID: "job_safe", WorkspaceID: "w1", Kind: "analyze", AvailableAt: old},
		{ID: "job_unsafe", WorkspaceID: "w1", Kind: "atomic_replace", AvailableAt: old},
	} {
		if _, _, err := store.Enqueue(context.Background(), value); err != nil {
			t.Fatalf("任务入队: %v", err)
		}
		if _, claimed, err := store.ClaimNext(context.Background(), old); err != nil || !claimed {
			t.Fatalf("领取任务: claimed=%v err=%v", claimed, err)
		}
	}
	runner := NewRunner(store, Config{Now: func() time.Time { return old.Add(2 * time.Hour) }})
	runner.Register("analyze", HandlerOptions{RetrySafe: true, Handle: func(context.Context, *Execution) (string, error) { return `{}`, nil }})
	runner.Register("atomic_replace", HandlerOptions{RetrySafe: false, Handle: func(context.Context, *Execution) (string, error) { return `{}`, nil }})
	if err := runner.Recover(context.Background(), old.Add(time.Hour)); err != nil {
		t.Fatalf("恢复任务: %v", err)
	}
	safe, _ := store.FindByID(context.Background(), "job_safe")
	unsafe, _ := store.FindByID(context.Background(), "job_unsafe")
	if safe.State != domainjob.StateQueued {
		t.Fatalf("可恢复任务状态=%s", safe.State)
	}
	if unsafe.State != domainjob.StateFailed || unsafe.ErrorCode != "job.recovery_unsafe" {
		t.Fatalf("不可安全恢复任务状态不正确: %+v", unsafe)
	}
}
