package hugo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestCLIBuilderBuildsPreparedAndDeliveredSite(t *testing.T) {
	if _, err := exec.LookPath("hugo"); err != nil {
		t.Skip("本机未安装 Hugo，跳过集成测试")
	}

	root := copyHugoFixture(t)
	staging := filepath.Join(t.TempDir(), "staging")
	provider, err := New(Config{
		Root: root, StagingRoot: staging, Section: "posts", BaseURL: "https://example.com", ArtifactTTL: time.Hour,
	}, CLIBuilder{Executable: "hugo", MaxOutputBytes: 1024 * 1024})
	if err != nil {
		t.Fatalf("创建 Hugo Provider: %v", err)
	}
	input := contracts.PublishInput{
		OperationID: "integration_1", ContentHash: "hash-integration", TargetSection: "posts", Body: "这是真实 Hugo 构建正文。",
		Article: article.Article{
			StableID: "article_INTEGRATION", RelativePath: "集成.md", Title: "集成测试",
			Category: "AI应用开发", Tags: []string{"go"}, Slug: "integration-test",
		},
	}
	artifact, err := provider.Prepare(context.Background(), input)
	if err != nil {
		t.Fatalf("真实 Hugo Prepare: %v", err)
	}
	stagedHTML := filepath.Join(staging, input.OperationID, "site", "public", "posts", "integration-test", "index.html")
	content, err := os.ReadFile(stagedHTML)
	if err != nil || !strings.Contains(string(content), "真实 Hugo 构建正文") {
		t.Fatalf("staging HTML 不正确: content=%s err=%v", content, err)
	}
	if _, err := provider.Deliver(context.Background(), artifact); err != nil {
		t.Fatalf("真实 Hugo Deliver: %v", err)
	}
	realHTML := filepath.Join(root, "public", "posts", "integration-test", "index.html")
	content, err = os.ReadFile(realHTML)
	if err != nil || !strings.Contains(string(content), "真实 Hugo 构建正文") {
		t.Fatalf("真实站点 HTML 不正确: content=%s err=%v", content, err)
	}
}
