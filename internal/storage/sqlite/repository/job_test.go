package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	domainjob "github.com/gkmz/InkHub/internal/domain/job"
)

func TestJobRepositoryDeduplicatesActiveJobs(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repository := NewJobRepository(db)
	now := time.Now().UTC()
	first, created, err := repository.Enqueue(context.Background(), domainjob.Job{
		ID: "job_1", WorkspaceID: "w1", Kind: "analyze", DedupeKey: "article:a1:hash-v1",
		PayloadJSON: `{"article_id":"a1"}`, AvailableAt: now,
	})
	if err != nil || !created {
		t.Fatalf("首次入队: job=%+v created=%v err=%v", first, created, err)
	}
	second, created, err := repository.Enqueue(context.Background(), domainjob.Job{
		ID: "job_2", WorkspaceID: "w1", Kind: "analyze", DedupeKey: "article:a1:hash-v1",
		PayloadJSON: `{"article_id":"a1"}`, AvailableAt: now,
	})
	if err != nil || created || second.ID != "job_1" {
		t.Fatalf("重复入队未返回现有任务: job=%+v created=%v err=%v", second, created, err)
	}
}

func TestJobRepositoryClaimsQueuedJobOnlyOnce(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repository := NewJobRepository(db)
	now := time.Now().UTC()
	if _, _, err := repository.Enqueue(context.Background(), domainjob.Job{
		ID: "job_1", WorkspaceID: "w1", Kind: "scan", AvailableAt: now,
	}); err != nil {
		t.Fatalf("任务入队: %v", err)
	}

	start := make(chan struct{})
	results := make(chan bool, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, claimed, err := repository.ClaimNext(context.Background(), now.Add(time.Second))
			if err != nil {
				t.Errorf("领取任务: %v", err)
			}
			results <- claimed
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	claimedCount := 0
	for claimed := range results {
		if claimed {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("同一任务被领取 %d 次", claimedCount)
	}

	stored, err := repository.FindByID(context.Background(), "job_1")
	if err != nil {
		t.Fatalf("查询任务: %v", err)
	}
	if stored.State != domainjob.StateRunning || stored.Attempts != 1 || stored.StartedAt == nil {
		t.Fatalf("领取后的任务状态不正确: %+v", stored)
	}
}
