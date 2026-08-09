package editorial

import (
	"context"
	"path/filepath"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestPublicationSettingsDefaultEmptyAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := inksqlite.Open(ctx, filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO workspaces(id,name,data_dir,last_used_at,created_at,updated_at) VALUES('w1','测试','/tmp','2026-01-01','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadPublicationSettings(ctx, db, "w1")
	if err != nil || settings.ExcludedSections == nil || len(settings.ExcludedSections) != 0 {
		t.Fatalf("旧工作区默认设置错误: %+v err=%v", settings, err)
	}
	saved, err := SavePublicationSettings(ctx, db, "w1", PublicationSettings{ExcludedSections: []string{" 相关链接 ", "参考资料", "相关链接", ""}})
	if err != nil || len(saved.ExcludedSections) != 2 || saved.ExcludedSections[0] != "相关链接" {
		t.Fatalf("保存设置错误: %+v err=%v", saved, err)
	}
	loaded, err := LoadPublicationSettings(ctx, db, "w1")
	if err != nil || len(loaded.ExcludedSections) != 2 || loaded.ExcludedSections[1] != "参考资料" {
		t.Fatalf("读取设置错误: %+v err=%v", loaded, err)
	}
}
