package hugo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectSiteUsesConfiguredContentDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hugo.toml"), []byte("contentDir = 'articles'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	site, err := InspectSite(root)
	if err != nil {
		t.Fatal(err)
	}
	if site.Root != root || site.ContentDir != "articles" {
		t.Fatalf("InspectSite() = %#v", site)
	}
}

func TestInspectSiteRejectsContentModuleMount(t *testing.T) {
	root := t.TempDir()
	config := "[[module.mounts]]\nsource = 'articles'\ntarget = 'content'\n"
	if err := os.WriteFile(filepath.Join(root, "hugo.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSite(root); err == nil {
		t.Fatal("content module mount 应明确拒绝")
	}
}
