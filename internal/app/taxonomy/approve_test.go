package taxonomy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAndApplyTermChangeRequiresExplicitApply(t *testing.T) {
	t.Parallel()

	source := filepath.Join("..", "..", "..", "testdata", "hugo", "taxonomy", "data", "taxonomy.yaml")
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "taxonomy.yaml")
	if err := os.WriteFile(path, content, 0o640); err != nil {
		t.Fatal(err)
	}

	change, err := BuildTermChange(context.Background(), path, Term{Name: "content-workflow", Aliases: []string{"workflow"}})
	if err != nil {
		t.Fatalf("BuildTermChange() error = %v", err)
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(content) {
		t.Fatal("BuildTermChange() modified the authority file")
	}
	if err := ApplyTermChange(context.Background(), path, change); err != nil {
		t.Fatalf("ApplyTermChange() error = %v", err)
	}
	loaded, err := LoadAuthoritative(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Tags["content-workflow"].Name == "" || loaded.Aliases["workflow"] != "content-workflow" {
		t.Fatalf("loaded taxonomy = %#v", loaded)
	}
}
