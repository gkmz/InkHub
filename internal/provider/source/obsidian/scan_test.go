package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestScanReportsDuplicateStableIDsWithoutDroppingDocuments(t *testing.T) {
	t.Parallel()

	root := copyFixtureVault(t, "valid")
	content, _ := os.ReadFile(filepath.Join(root, "文章.md"))
	if err := os.WriteFile(filepath.Join(root, "重复.md"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".obsidian", "ignored.md"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	provider, _ := New(Config{Root: root, SourceID: "source1"})
	result, err := provider.Scan(context.Background(), contracts.ScanCursor{})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(result.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(result.Documents))
	}
	for _, document := range result.Documents {
		if len(document.Diagnostics) != 1 || !document.Diagnostics[0].Blocking || document.Diagnostics[0].Code != "obsidian.duplicate_stable_id" {
			t.Fatalf("diagnostics = %#v", document.Diagnostics)
		}
	}
}

func TestScanUsesCursorForUnchangedVault(t *testing.T) {
	t.Parallel()

	root := copyFixtureVault(t, "valid")
	provider, _ := New(Config{Root: root, SourceID: "source1"})
	first, err := provider.Scan(context.Background(), contracts.ScanCursor{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Scan(context.Background(), first.Next)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Documents) != 0 || second.Next.Revision != first.Next.Revision {
		t.Fatalf("unchanged scan = %#v", second)
	}
}

func TestScanInvalidatesCursorWhenObsidianSettingsChange(t *testing.T) {
	t.Parallel()
	root := copyFixtureVault(t, "valid")
	settingsPath := filepath.Join(root, ".obsidian", "app.json")
	if err := os.WriteFile(settingsPath, []byte(`{"attachmentFolderPath":"assets"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, _ := New(Config{Root: root, SourceID: "source1"})
	first, err := provider.Scan(context.Background(), contracts.ScanCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"attachmentFolderPath":"./attachments"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := provider.Scan(context.Background(), first.Next)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Documents) == 0 || second.Next.Revision == first.Next.Revision {
		t.Fatalf("settings change must invalidate cursor: first=%#v second=%#v", first.Next, second.Next)
	}
}
