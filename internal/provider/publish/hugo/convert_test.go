package hugo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestConvertArticleMapsMetadataAndObsidianSyntax(t *testing.T) {
	t.Parallel()

	content, err := convertArticle(contracts.PublishInput{
		Article: article.Article{
			StableID: "article_ONE", RelativePath: "文章/示例.md", Title: "示例标题", Description: "文章摘要",
			Category: "AI应用开发", Series: "InkHub 开发日志", Tags: []string{"go", "hugo"},
			Keywords: []string{"InkHub", "Hugo"}, Slug: "inkhub-hugo", Cover: "images/cover.png",
		},
		Body: `参见 [[另一篇文章|相关内容]]。

> [!NOTE] 注意事项
> 这是提示内容。

![[cover.png]]`,
	})
	if err != nil {
		t.Fatalf("转换文章: %v", err)
	}
	text := string(content)
	for _, expected := range []string{
		"source_id: article_ONE", "source_path: 文章/示例.md", "keywords:", "- InkHub",
		`[相关内容]({{< relref "另一篇文章" >}})`, "> **注意事项**", "![](cover.png)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("转换结果缺少 %q:\n%s", expected, text)
		}
	}
}

func TestPlanResourcesRejectsConflictingBasenames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	first := filepath.Join(root, "a", "cover.png")
	second := filepath.Join(root, "b", "cover.png")
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := planResources([]contracts.ResourceRef{
		{Original: "images/cover.png", Resolved: first, Kind: "image"},
		{Original: "assets/cover.png", Resolved: second, Kind: "image"},
	})
	if err == nil {
		t.Fatal("同名不同内容资源应返回冲突")
	}
}

func TestFindBundleUsesStableSourceID(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "..", "..", "testdata", "hugo", "site")
	bundle, found, err := findBundle(root, "posts", "article_EXISTING")
	if err != nil {
		t.Fatalf("查找 bundle: %v", err)
	}
	if !found || filepath.Base(bundle) != "existing" {
		t.Fatalf("未按 source_id 找到已有 bundle: path=%q found=%v", bundle, found)
	}
}
