package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
	httptransport "github.com/gkmz/InkHub/internal/transport/http"
)

func TestDashboardGroupsCurrentWorkspaceWithoutDuplicates(t *testing.T) {
	t.Parallel()
	api := seedDashboardAPI(t)

	view, err := api.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertDashboardIDs(t, view.Failed, "failed-published", "failed")
	assertDashboardIDs(t, view.Changed, "outdated", "stale", "changed")
	assertDashboardIDs(t, view.NeedsReview, "pending")
	assertDashboardIDs(t, view.RecentlyHandled, "published", "approved")
	if view.Failed[0].Disposition != "published" || view.Failed[0].WeChatState != "处理失败" {
		t.Fatalf("未选择渠道的失败被已发表处置掩盖: %+v", view.Failed[0])
	}
}

func TestDashboardLimitsRecentlyHandledByHandlingTime(t *testing.T) {
	t.Parallel()
	api := seedDashboardAPI(t)
	for index := 0; index < 11; index++ {
		id := fmt.Sprintf("recent-%02d", index)
		handledAt := fmt.Sprintf("2026-07-31T%02d:00:00Z", index)
		_, err := api.db.Exec(`INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at,source_mtime) VALUES(?,?,?,?,?,?,?,'2026-07-31','2026-07-31','2026-07-31','2026-07-31')`, id, "w2", "s2", id, id+".md", id, id+"-hash")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := api.db.Exec(`INSERT INTO editorial_reviews(article_id,state,approved_content_hash,updated_at) VALUES(?,'approved',?,?)`, id, id+"-hash", handledAt); err != nil {
			t.Fatal(err)
		}
	}

	view, err := api.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(view.RecentlyHandled) != 10 || view.RecentlyHandled[0].ID != "recent-10" || view.RecentlyHandled[9].ID != "recent-01" {
		t.Fatalf("最近处理排序或上限错误: %+v", view.RecentlyHandled)
	}
}

func seedDashboardAPI(t *testing.T) databaseAPI {
	t.Helper()
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES
('w1','旧工作区','/tmp','2026-07-01','2026-07-01','2026-07-01'),
('w2','当前工作区','/tmp','2026-07-30','2026-07-30','2026-07-30');
INSERT INTO sources(id,workspace_id,provider_type,root_path,created_at,updated_at) VALUES
('s1','w1','obsidian','/tmp/old','2026-07-01','2026-07-01'),
('s2','w2','obsidian','/tmp/current','2026-07-30','2026-07-30');
INSERT INTO provider_instances(id,workspace_id,provider_type,name,created_at,updated_at) VALUES
('h2','w2','hugo','Hugo','2026-07-30','2026-07-30'),
('m2','w2','wechat','微信','2026-07-30','2026-07-30');
INSERT INTO articles(id,workspace_id,source_id,stable_id,relative_path,title,content_hash,indexed_at,created_at,updated_at,source_mtime,deleted_at) VALUES
('old','w1','s1','old','old.md','旧文章','old','2026-07-01','2026-07-01','2026-07-01','2026-07-31T14:00:00Z',NULL),
('ignored','w2','s2','ignored','ignored.md','忽略','i2','2026-07-30','2026-07-30','2026-07-30','2026-07-30T14:00:00Z',NULL),
('failed-published','w2','s2','failed-published','failed-published.md','部分渠道失败','fp1','2026-07-30','2026-07-30','2026-07-30','2026-07-30T13:00:00Z',NULL),
('failed','w2','s2','failed','failed.md','失败','f1','2026-07-30','2026-07-30','2026-07-30','2026-07-30T12:00:00Z',NULL),
('outdated','w2','s2','outdated','outdated.md','渠道过期','o2','2026-07-30','2026-07-30','2026-07-30','2026-07-30T11:00:00Z',NULL),
('stale','w2','s2','stale','stale.md','外部发表已过期','s2','2026-07-30','2026-07-30','2026-07-30','2026-07-30T10:00:00Z',NULL),
('changed','w2','s2','changed','changed.md','内容更新','c2','2026-07-30','2026-07-30','2026-07-30','2026-07-30T09:00:00Z',NULL),
('pending','w2','s2','pending','pending.md','待审核','p1','2026-07-30','2026-07-30','2026-07-30','2026-07-30T08:00:00Z',NULL),
('published','w2','s2','published','published.md','已发表','x1','2026-07-30','2026-07-30','2026-07-30','2026-07-30T07:00:00Z',NULL),
('approved','w2','s2','approved','approved.md','已审核','a1','2026-07-30','2026-07-30','2026-07-30','2026-07-30T06:00:00Z',NULL),
('deleted','w2','s2','deleted','deleted.md','已删除','d1','2026-07-30','2026-07-30','2026-07-30','2026-07-30T15:00:00Z','2026-07-30');
INSERT INTO editorial_reviews(article_id,state,approved_content_hash,updated_at) VALUES
('failed','blocked',NULL,'2026-07-30T12:00:00Z'),
('changed','changed','c1','2026-07-30T09:00:00Z'),
('approved','approved','a1','2026-07-30T19:00:00Z');
INSERT INTO article_dispositions(article_id,workspace_id,kind,content_hash,created_at,updated_at) VALUES
('ignored','w2','ignored','i1','2026-07-30','2026-07-30T22:00:00Z'),
('failed-published','w2','published','fp1','2026-07-30','2026-07-30T21:00:00Z'),
('stale','w2','published','s1','2026-07-30','2026-07-30T20:00:00Z'),
('published','w2','published','x1','2026-07-30','2026-07-30T23:00:00Z');
INSERT INTO publications(id,article_id,provider_instance_id,workspace_id,state,content_hash,created_at,updated_at) VALUES
('pub-fp-h','failed-published','h2','w2','published','fp1','2026-07-30','2026-07-30'),
('pub-fp-m','failed-published','m2','w2','failed','fp1','2026-07-30','2026-07-30'),
('pub-outdated','outdated','h2','w2','published','o1','2026-07-30','2026-07-30')`)
	if err != nil {
		t.Fatal(err)
	}
	return newDatabaseAPI(db)
}

func assertDashboardIDs(t *testing.T, items []httptransport.ArticleSummary, want ...string) {
	t.Helper()
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("文章 IDs = %v, want %v", got, want)
	}
}
