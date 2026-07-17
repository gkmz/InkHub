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

func TestJobRepositoryRequeuesOnlyMatchingFailedJob(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedWorkspace(t, db)
	repository := NewJobRepository(db)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	if _, _, err := repository.Enqueue(context.Background(), domainjob.Job{ID: "preview_1", WorkspaceID: "w1", Kind: "hugo_preview", PayloadJSON: `{"article_id":"a1"}`, AvailableAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := repository.ClaimNext(context.Background(), now); err != nil || !claimed {
		t.Fatalf("领取任务: claimed=%v err=%v", claimed, err)
	}
	if err := repository.Fail(context.Background(), "preview_1", "hugo.failed", "构建失败", now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequeueFailed(context.Background(), "preview_1", "other", "hugo_preview", now.Add(time.Second)); err == nil {
		t.Fatal("工作区不匹配时仍重排任务")
	}
	requeued, err := repository.RequeueFailed(context.Background(), "preview_1", "w1", "hugo_preview", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if requeued.State != domainjob.StateQueued || requeued.Attempts != 1 || requeued.PayloadJSON != `{"article_id":"a1"}` || requeued.StartedAt != nil || requeued.FinishedAt != nil || requeued.ErrorCode != "" {
		t.Fatalf("重排任务状态错误: %+v", requeued)
	}
	if _, err := repository.RequeueFailed(context.Background(), "preview_1", "w1", "hugo_preview", now.Add(2*time.Second)); err == nil {
		t.Fatal("queued 任务被重复重排")
	}
}

func TestJobRepositoryFindsLatestTargetJobByCurrentIdentity(t *testing.T) {
	t.Parallel()

	db := openRepositoryTestDB(t)
	seedPublicationParents(t, db)
	repository := NewJobRepository(db)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	jobs := []domainjob.Job{
		{ID: "preview_old_hash", WorkspaceID: "w1", Kind: "hugo_preview", PayloadJSON: `{"article_id":"a1","provider_instance_id":"provider1","content_hash":"old"}`, AvailableAt: now},
		{ID: "preview_current", WorkspaceID: "w1", Kind: "hugo_preview", PayloadJSON: `{"article_id":"a1","provider_instance_id":"provider1","content_hash":"hash"}`, AvailableAt: now.Add(time.Second)},
		{ID: "preview_other_article", WorkspaceID: "w1", Kind: "hugo_preview", PayloadJSON: `{"article_id":"other","provider_instance_id":"provider1","content_hash":"hash"}`, AvailableAt: now.Add(2 * time.Second)},
	}
	for _, job := range jobs {
		if _, _, err := repository.Enqueue(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	found, ok, err := repository.FindLatestTargetJob(context.Background(), "w1", "a1", "provider1", "hash", "hugo_preview")
	if err != nil || !ok || found.ID != "preview_current" {
		t.Fatalf("当前目标任务查询错误: job=%+v found=%v err=%v", found, ok, err)
	}
	if _, ok, err := repository.FindLatestTargetJob(context.Background(), "w1", "a1", "provider1", "missing", "hugo_preview"); err != nil || ok {
		t.Fatalf("不存在的内容版本返回了任务: found=%v err=%v", ok, err)
	}
}
