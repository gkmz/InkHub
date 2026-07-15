package folder

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMarkdownPathsReturnsStablePathsAndHonorsExclusions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, path := range []string{"b.md", filepath.Join("notes", "a.md"), filepath.Join(".obsidian", "ignored.md"), "other.txt"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("content"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	source, err := New(Config{Root: root, ExcludedDirs: map[string]bool{".obsidian": true}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := source.MarkdownPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b.md", "notes/a.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MarkdownPaths() = %#v, want %#v", got, want)
	}
}

func TestMarkdownPathsOnlyReturnsConfiguredContentScope(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, relative := range []string{"Areas/文章.md", "Areas/私人/日记.md", "Resources/资料.md"} {
		full := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("content"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	source, err := New(Config{Root: root, ContentRoots: []string{"Areas"}, IgnoredFolders: []string{"Areas/私人"}, IgnoredFileNames: []string{"index.md"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := source.MarkdownPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Areas/文章.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MarkdownPaths() = %#v, want %#v", got, want)
	}
}

func TestSnapshotDetectsSameSizeContentChangeWithUnchangedMTime(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "article.md")
	stamp := time.Unix(1_700_000_000, 0)
	if err := os.WriteFile(path, []byte("first"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	source, _ := New(Config{Root: root})
	before, err := source.snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("other"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	after, err := source.snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before["article.md"] == after["article.md"] {
		t.Fatal("snapshot must detect content changes even when size and mtime are unchanged")
	}
}
