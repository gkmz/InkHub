package job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunnerCompletesJobAndPersistsProgress(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	runner := NewRunner(store, Config{Now: func() time.Time { return now }})
	runner.Register("analyze", HandlerOptions{
		MaxAttempts: 3,
		RetrySafe:   true,
		Handle: func(ctx context.Context, execution *Execution) (string, error) {
			if err := execution.ReportProgress(ctx, 45); err != nil {
				return "", err
			}
			return `{"ok":true}`, nil
		},
	})
	queued, created, err := runner.Enqueue(context.Background(), EnqueueRequest{
		ID: "job_1", WorkspaceID: "w1", Kind: "analyze", PayloadJSON: `{}`,
	})
	if err != nil || !created {
		t.Fatalf("任务入队: job=%+v created=%v err=%v", queued, created, err)
	}
	worked, err := runner.RunOne(context.Background())
	if err != nil || !worked {
		t.Fatalf("执行任务: worked=%v err=%v", worked, err)
	}
	stored, err := store.FindByID(context.Background(), "job_1")
	if err != nil {
		t.Fatalf("查询任务: %v", err)
	}
	if stored.State != domainjob.StateSucceeded || stored.Progress != 100 || stored.ResultJSON != `{"ok":true}` || stored.FinishedAt == nil {
		t.Fatalf("成功任务状态不完整: %+v", stored)
	}
}

func TestRunnerLogsJobLifecycleWithoutPayload(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	core, observed := observer.New(zap.InfoLevel)
	runner := NewRunner(store, Config{Logger: zap.New(core)})
	runner.Register("publish", HandlerOptions{Handle: func(context.Context, *Execution) (string, error) {
		return `{}`, nil
	}})
	if _, _, err := runner.Enqueue(context.Background(), EnqueueRequest{
		ID: "job_1", WorkspaceID: "w1", Kind: "publish", ArticleID: "article_1",
		ProviderInstanceID: "provider_1", PayloadJSON: `{"private":"正文秘密"}`,
	}); err != nil {
		t.Fatalf("任务入队: %v", err)
	}
	if _, err := runner.RunOne(context.Background()); err != nil {
		t.Fatalf("执行任务: %v", err)
	}

	entries := observed.All()
	if len(entries) < 3 {
		t.Fatalf("日志数量 = %d, want enqueue/start/success", len(entries))
	}
	for _, entry := range entries {
		if strings.Contains(entry.Message+fmt.Sprint(entry.ContextMap()), "正文秘密") {
			t.Fatalf("任务日志泄露 payload: %#v", entry)
		}
	}
	fields := entries[len(entries)-1].ContextMap()
	if fields["job_id"] != "job_1" || fields["workspace_id"] != "w1" || fields["article_id"] != "article_1" || fields["provider_id"] != "provider_1" {
		t.Fatalf("任务关联字段 = %#v", fields)
	}
}

func TestRunnerRetriesOnlyRetrySafeJobs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		retrySafe bool
		wantState domainjob.State
		wantDelay time.Duration
	}{
		{name: "网络任务退避重试", retrySafe: true, wantState: domainjob.StateQueued, wantDelay: time.Minute},
		{name: "原子替换任务不盲目重试", retrySafe: false, wantState: domainjob.StateFailed},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := openJobStore(t)
			runner := NewRunner(store, Config{Now: func() time.Time { return now }})
			runner.Register("publish", HandlerOptions{
				MaxAttempts: 3,
				RetrySafe:   test.retrySafe,
				Backoff:     func(int) time.Duration { return time.Minute },
				Handle: func(context.Context, *Execution) (string, error) {
					return "", &contracts.ProviderError{
						Code: "provider.temporary", Category: contracts.ErrorTemporary,
						Message: "服务暂时不可用", Retryable: true,
					}
				},
			})
			if _, _, err := runner.Enqueue(context.Background(), EnqueueRequest{ID: "job_1", WorkspaceID: "w1", Kind: "publish"}); err != nil {
				t.Fatalf("任务入队: %v", err)
			}
			if _, err := runner.RunOne(context.Background()); err != nil {
				t.Fatalf("执行任务: %v", err)
			}
			stored, err := store.FindByID(context.Background(), "job_1")
			if err != nil {
				t.Fatalf("查询任务: %v", err)
			}
			if stored.State != test.wantState {
				t.Fatalf("任务状态=%s，期望 %s", stored.State, test.wantState)
			}
			if test.wantDelay > 0 && !stored.AvailableAt.Equal(now.Add(test.wantDelay)) {
				t.Fatalf("退避时间=%s，期望 %s", stored.AvailableAt, now.Add(test.wantDelay))
			}
			if stored.ErrorCode != "provider.temporary" || stored.ErrorMessage != "服务暂时不可用" {
				t.Fatalf("错误摘要未持久化或未脱敏: %+v", stored)
			}
		})
	}
}

func TestRunnerRejectsInvalidPayloadAndResultJSON(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	runner := NewRunner(store, Config{})
	runner.Register("invalid_result", HandlerOptions{
		Handle: func(context.Context, *Execution) (string, error) { return `{invalid`, nil },
	})
	if _, _, err := runner.Enqueue(context.Background(), EnqueueRequest{
		ID: "invalid_payload", WorkspaceID: "w1", Kind: "invalid_result", PayloadJSON: `{invalid`,
	}); err == nil {
		t.Fatal("无效 payload JSON 应在入队前被拒绝")
	}
	if _, _, err := runner.Enqueue(context.Background(), EnqueueRequest{
		ID: "invalid_result", WorkspaceID: "w1", Kind: "invalid_result", PayloadJSON: `{}`,
	}); err != nil {
		t.Fatalf("任务入队: %v", err)
	}
	if _, err := runner.RunOne(context.Background()); err != nil {
		t.Fatalf("执行任务: %v", err)
	}
	stored, err := store.FindByID(context.Background(), "invalid_result")
	if err != nil {
		t.Fatalf("查询任务: %v", err)
	}
	if stored.State != domainjob.StateFailed || stored.ErrorCode != "job.result_invalid" {
		t.Fatalf("无效结果不应标记成功: %+v", stored)
	}
}

func TestRunnerBuildsDeterministicDedupeKey(t *testing.T) {
	t.Parallel()

	store := openJobStore(t)
	runner := NewRunner(store, Config{})
	request := EnqueueRequest{
		ID: "job_1", WorkspaceID: "w1", Kind: "analyze", ArticleID: "article_1",
		ProviderInstanceID: "provider_ai", ContentHash: "hash-v1", PayloadJSON: `{}`,
	}
	first, created, err := runner.Enqueue(context.Background(), request)
	if err != nil || !created || first.DedupeKey == "" {
		t.Fatalf("首次入队未生成去重键: job=%+v created=%v err=%v", first, created, err)
	}
	request.ID = "job_2"
	second, created, err := runner.Enqueue(context.Background(), request)
	if err != nil || created || second.ID != "job_1" {
		t.Fatalf("相同业务目标未去重: job=%+v created=%v err=%v", second, created, err)
	}
	changed := BuildDedupeKey("analyze", "article_1", "provider_ai", "hash-v2")
	if changed == first.DedupeKey {
		t.Fatal("content hash 变化后去重键必须变化")
	}
}

func openJobStore(t *testing.T) *repository.JobRepository {
	t.Helper()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedJobWorkspace(t, db)
	return repository.NewJobRepository(db)
}

func seedJobWorkspace(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at)
VALUES ('w1','test','/tmp/test','2026-01-01','2026-01-01','2026-01-01')`)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
}
