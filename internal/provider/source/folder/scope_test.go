package folder

import "testing"

func TestScopeIncludesManagedFoldersAndAppliesIgnoredFoldersFirst(t *testing.T) {
	t.Parallel()

	scope, err := NewScope([]string{"Areas", "Areas/写作", "Projects/OpenSource"}, []string{"Areas/私人记录"})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]bool{
		"Areas/文章.md":                   true,
		"Areas/写作/专题.md":                true,
		"Areas/私人记录/日记.md":              false,
		"Projects/OpenSource/README.md": true,
		"Projects/客户项目/记录.md":           false,
		"Resources/资料.md":               false,
		"Areas/.trash/旧稿.md":            false,
	}
	for path, want := range tests {
		if got := scope.Includes(path); got != want {
			t.Errorf("Includes(%q) = %v, want %v", path, got, want)
		}
	}
	if roots := scope.ContentRoots(); len(roots) != 2 || roots[0] != "Areas" || roots[1] != "Projects/OpenSource" {
		t.Fatalf("父目录未合并: %#v", roots)
	}
}

func TestScopeRejectsUnsafeOrUnexplainableRules(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		roots   []string
		ignored []string
	}{
		{name: "绝对路径", roots: []string{"/Users/me/Vault"}},
		{name: "当前目录", roots: []string{"."}},
		{name: "越界", roots: []string{"../private"}},
		{name: "系统目录", roots: []string{".obsidian"}},
		{name: "忽略不属于内容目录", roots: []string{"Areas"}, ignored: []string{"Resources"}},
		{name: "忽略内容目录本身", roots: []string{"Areas"}, ignored: []string{"Areas"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewScope(test.roots, test.ignored); err == nil {
				t.Fatal("无效目录规则必须被拒绝")
			}
		})
	}
}

func TestScopeWithNoContentRootsIncludesNothing(t *testing.T) {
	t.Parallel()

	scope, err := NewScope([]string{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Includes("Areas/文章.md") {
		t.Fatal("未授权内容目录时不得扫描文章")
	}
}
