package obsidian

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeAttachmentLocationSupportsObsidianModes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		kind  AttachmentLocationKind
		path  string
	}{
		{name: "vault root slash", value: "/", kind: AttachmentAtVaultRoot},
		{name: "vault root dot", value: ".", kind: AttachmentAtVaultRoot},
		{name: "current folder", value: "./", kind: AttachmentAtCurrentFolder},
		{name: "current subfolder", value: "./attachments", kind: AttachmentAtCurrentSubfolder, path: "attachments"},
		{name: "configured folder", value: "assets", kind: AttachmentAtConfiguredFolder, path: "assets"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeAttachmentLocation(test.value)
			if got.Kind != test.kind || got.Path != test.path {
				t.Fatalf("normalizeAttachmentLocation(%q) = %#v", test.value, got)
			}
		})
	}
}

func TestReadObsidianSettingsNormalizesLinkPreferencesAndFingerprint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".obsidian", "app.json"), []byte(`{"attachmentFolderPath":"./attachments","newLinkFormat":"absolute","useMarkdownLinks":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := readObsidianSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AttachmentFolder.Kind != AttachmentAtCurrentSubfolder || settings.AttachmentFolder.Path != "attachments" {
		t.Fatalf("attachment settings = %#v", settings.AttachmentFolder)
	}
	if settings.LinkFormat != LinkFormatAbsolute || !settings.UseMarkdownLinks || settings.Fingerprint == "" {
		t.Fatalf("link settings = %#v", settings)
	}
}

func TestReadObsidianSettingsIgnoresUnrelatedAppPreferencesInFingerprint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	pathName := filepath.Join(root, ".obsidian", "app.json")
	if err := os.WriteFile(pathName, []byte(`{"attachmentFolderPath":"assets","newLinkFormat":"relative","useMarkdownLinks":false,"theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := readObsidianSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathName, []byte(`{"attachmentFolderPath":"assets","newLinkFormat":"relative","useMarkdownLinks":false,"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := readObsidianSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("无关 Obsidian 设置不应使资源解析指纹变化")
	}
}
