package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestWatchReportsCreatedMarkdownFile(t *testing.T) {
	root := copyFixtureVault(t, "valid")
	provider, err := New(Config{Root: root, SourceID: "source1", PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan contracts.SourceChange, 4)
	done := make(chan error, 1)
	go func() { done <- provider.Watch(ctx, changes) }()

	time.Sleep(30 * time.Millisecond)
	content, _ := os.ReadFile(filepath.Join(root, "文章.md"))
	if err := os.WriteFile(filepath.Join(root, "新增.md"), content, 0o640); err != nil {
		t.Fatal(err)
	}

	select {
	case change := <-changes:
		if change.Kind != contracts.SourceCreated || change.Ref.RelativePath != "新增.md" {
			t.Fatalf("change = %#v", change)
		}
	case <-time.After(time.Second):
		t.Fatal("Watch() did not report the created file")
	}
}
