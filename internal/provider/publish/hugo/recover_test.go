package hugo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestRecoverRejectsTamperedTargetOutsideHugoRoot(t *testing.T) {
	t.Parallel()

	root := copyHugoFixture(t)
	staging := filepath.Join(t.TempDir(), "staging")
	provider, err := New(Config{Root: root, StagingRoot: staging, Section: "posts"}, &fakeBuilder{})
	if err != nil {
		t.Fatal(err)
	}
	operationRoot := filepath.Join(staging, "operation_tampered")
	if err := os.MkdirAll(operationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(outside), ".victim.inkhub-operation_tampered.bak")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(artifactManifest{Artifact: contracts.PreparedArtifact{
		OperationID: "operation_tampered", ContentHash: "hash", TargetPath: outside,
	}})
	if err := os.WriteFile(filepath.Join(operationRoot, "artifact.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := provider.Recover(context.Background()); err == nil {
		t.Fatal("篡改后的越界恢复路径应被拒绝")
	}
	if content, err := os.ReadFile(filepath.Join(outside, "keep.txt")); err != nil || string(content) != "keep" {
		t.Fatalf("越界目标被修改: content=%q err=%v", content, err)
	}
}
