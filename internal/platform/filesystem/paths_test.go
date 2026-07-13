package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAuthorizedFSRejectsSiblingPrefixAndSymlinkEscape(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "vault")
	sibling := filepath.Join(base, "vault-private")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{root, sibling, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	afs, err := NewAuthorizedFS([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := afs.Resolve(filepath.Join(sibling, "secret.md")); err == nil {
		t.Fatal("sibling-prefix path must be rejected")
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(root, "escape")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		if _, err := afs.Resolve(filepath.Join(link, "secret.md")); err == nil {
			t.Fatal("symlink escape must be rejected")
		}
	}
}

func TestAtomicWritePreservesPermissions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "article.md")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("new"), nil); err != nil {
		t.Fatalf("AtomicWrite() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions = %o, want 640", info.Mode().Perm())
	}
	content, _ := os.ReadFile(path)
	if string(content) != "new" {
		t.Fatalf("content = %q, want new", content)
	}
}
