package template

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIndexFallsBackToLastValidatedCache(t *testing.T) {
	t.Parallel()

	cache := filepath.Join(t.TempDir(), "index.json")
	content := `{"specVersion":"1.0","generatedAt":"2026-07-13T04:00:00Z","templates":[{"id":"inkhub-default","version":"1.0.0","downloadURL":"https://example.com/template.zip","packageSHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`
	if err := os.WriteFile(cache, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := LoadIndex(context.Background(), failingIndexFetcher{}, cache, DefaultIndexURL)
	if err != nil {
		t.Fatalf("索引降级: %v", err)
	}
	if len(index.Templates) != 1 || index.Templates[0].ID != "inkhub-default" {
		t.Fatalf("未返回缓存索引: %+v", index)
	}
}

type failingIndexFetcher struct{}

func (failingIndexFetcher) Fetch(context.Context, string) ([]byte, error) {
	return nil, errors.New("offline")
}
