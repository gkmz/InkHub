package taxonomy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAuthoritativeBuildsAliasIndex(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "testdata", "hugo", "taxonomy", "data", "taxonomy.yaml")
	result, err := LoadAuthoritative(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadAuthoritative() error = %v", err)
	}
	if result.Aliases["golang"] != "go" || !result.Tags["go"].Core {
		t.Fatalf("taxonomy = %#v", result)
	}
}

func TestLoadAuthoritativeRejectsDuplicateAliasInSameTerm(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "taxonomy.yaml")
	content := "version: 1\ncategories: []\nseries: []\ntags:\n  - name: go\n    aliases: [golang, golang]\n    core: true\n    allowLowFrequency: false\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthoritative(context.Background(), path); err == nil {
		t.Fatal("LoadAuthoritative() must reject duplicate aliases")
	}
}
