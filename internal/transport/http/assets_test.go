package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	inksqlite "github.com/gkmz/InkHub/internal/storage/sqlite"
)

func TestRuntimeHandlerRendersAndServesReferencedVaultImages(t *testing.T) {
	db, err := inksqlite.Open(context.Background(), filepath.Join(t.TempDir(), "inkhub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(vault, "Areas"), 0o700); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	if err := os.WriteFile(filepath.Join(vault, "Areas", "image.png"), png, 0o600); err != nil {
		t.Fatal(err)
	}
	body := "# 图片\n\n![标准](image.png)\n\n![[image.png|640]]\n\n![远程](https://example.com/remote.png)"
	if err := os.WriteFile(filepath.Join(vault, "Areas", "article.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{1}, 32)
	handler := NewRuntimeHandler(db, NewRouter(emptyRuntimeAPI{}), RuntimeOptions{AssetTokenKey: key, ProviderRuntime: testProviderRuntime(t)})
	createBody := `{"name":"图片空间","vault_path":"` + filepath.ToSlash(vault) + `","content_roots":["Areas"],"ignored_folders":[],"wechat_template":"default","ai_enabled":false}`
	create := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/workspaces", strings.NewReader(createBody))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Origin", "http://localhost")
	create.Header.Set("Idempotency-Key", "asset-workspace")
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("创建图片工作区失败: %s", createResponse.Body.String())
	}
	var articleID string
	if err := db.QueryRow(`SELECT id FROM articles WHERE relative_path='Areas/article.md'`).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/articles/"+articleID, nil))
	var detailBody struct {
		PreviewHTML string `json:"preview_html"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatal(err)
	}
	assetURL := regexp.MustCompile(`/api/v1/articles/[^" ]+/assets/[^" ]+`).FindString(detailBody.PreviewHTML)
	if assetURL == "" || !strings.Contains(detailBody.PreviewHTML, "https://example.com/remote.png") {
		t.Fatalf("文章预览未改写图片: %s", detail.Body.String())
	}
	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "http://localhost"+assetURL, nil))
	if asset.Code != http.StatusOK || asset.Header().Get("Content-Type") != "image/png" || asset.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(asset.Body.Bytes(), png) {
		t.Fatalf("图片响应错误: code=%d headers=%v body=%x", asset.Code, asset.Header(), asset.Body.Bytes())
	}
	tamperedURL := assetURL[:len(assetURL)-1] + "A"
	tampered := httptest.NewRecorder()
	handler.ServeHTTP(tampered, httptest.NewRequest(http.MethodGet, "http://localhost"+tamperedURL, nil))
	if tampered.Code != http.StatusNotFound {
		t.Fatalf("篡改 token 未拒绝: %d", tampered.Code)
	}
	var fingerprint string
	if err := db.QueryRow(`SELECT source_fingerprint FROM articles WHERE id=?`, articleID).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	unreferencedURL := (&runtimeHandler{assetTokenKey: key}).assetURL(articleID, fingerprint, "Areas/unreferenced.png")
	unreferenced := httptest.NewRecorder()
	handler.ServeHTTP(unreferenced, httptest.NewRequest(http.MethodGet, "http://localhost"+unreferencedURL, nil))
	if unreferenced.Code != http.StatusNotFound {
		t.Fatalf("未引用资源未拒绝: %d", unreferenced.Code)
	}
}
