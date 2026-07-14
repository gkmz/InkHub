package httptransport

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var wikiImagePattern = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)

type assetTokenPayload struct {
	ArticleID   string `json:"article_id"`
	Fingerprint string `json:"fingerprint"`
	Relative    string `json:"relative"`
}

func newAssetTokenKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("无法初始化本地资源签名密钥")
	}
	return key
}

func (h *runtimeHandler) renderArticlePreview(ctx context.Context, source *obsidian.Provider, document contracts.SourceDocument, articleID string) (string, error) {
	body := wikiImagePattern.ReplaceAllStringFunc(document.Body, func(match string) string {
		reference := wikiImagePattern.FindStringSubmatch(match)[1]
		asset, err := source.ResolveAsset(ctx, document.Ref, reference, obsidian.AssetWikiEmbed)
		if err != nil {
			return "![图片不可用]()"
		}
		if asset.RemoteURL != "" {
			return "![远程图片](" + asset.RemoteURL + ")"
		}
		return "![" + strings.SplitN(reference, "|", 2)[0] + "](" + h.assetURL(articleID, document.Fingerprint, asset.RelativePath) + ")"
	})
	markdown := goldmark.New()
	reader := text.NewReader([]byte(body))
	documentNode := markdown.Parser().Parse(reader)
	err := ast.Walk(documentNode, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		image, ok := node.(*ast.Image)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		destination := string(image.Destination)
		if strings.HasPrefix(destination, "/api/v1/articles/") {
			return ast.WalkContinue, nil
		}
		asset, err := source.ResolveAsset(ctx, document.Ref, destination, obsidian.AssetMarkdownImage)
		if err != nil {
			image.Destination = nil
			return ast.WalkContinue, nil
		}
		if asset.RemoteURL != "" {
			image.Destination = []byte(asset.RemoteURL)
		} else {
			image.Destination = []byte(h.assetURL(articleID, document.Fingerprint, asset.RelativePath))
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	if err := markdown.Renderer().Render(&rendered, reader.Source(), documentNode); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (h *runtimeHandler) assetURL(articleID, fingerprint, relative string) string {
	payload, _ := json.Marshal(assetTokenPayload{ArticleID: articleID, Fingerprint: fingerprint, Relative: relative})
	signature := hmac.New(sha256.New, h.assetTokenKey)
	_, _ = signature.Write(payload)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
	return "/api/v1/articles/" + articleID + "/assets/" + token
}

func (h *runtimeHandler) verifyAssetToken(token string) (assetTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return assetTokenPayload{}, fmt.Errorf("资源 token 格式无效")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return assetTokenPayload{}, err
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return assetTokenPayload{}, err
	}
	signature := hmac.New(sha256.New, h.assetTokenKey)
	_, _ = signature.Write(payload)
	if !hmac.Equal(provided, signature.Sum(nil)) {
		return assetTokenPayload{}, fmt.Errorf("资源 token 签名无效")
	}
	var value assetTokenPayload
	if json.Unmarshal(payload, &value) != nil || value.ArticleID == "" || value.Fingerprint == "" || value.Relative == "" {
		return assetTokenPayload{}, fmt.Errorf("资源 token 内容无效")
	}
	return value, nil
}

func (h *runtimeHandler) articleAsset(response http.ResponseWriter, request *http.Request) {
	remainder := strings.TrimPrefix(request.URL.Path, "/api/v1/articles/")
	parts := strings.SplitN(remainder, "/assets/", 2)
	if len(parts) != 2 {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	payload, err := h.verifyAssetToken(parts[1])
	if err != nil || payload.ArticleID != parts[0] {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	var sourceID, relative, fingerprint, root string
	err = h.db.QueryRowContext(request.Context(), `SELECT articles.source_id,articles.relative_path,articles.source_fingerprint,sources.root_path FROM articles JOIN sources ON sources.id=articles.source_id WHERE articles.id=? AND articles.deleted_at IS NULL`, payload.ArticleID).Scan(&sourceID, &relative, &fingerprint, &root)
	if err != nil || fingerprint != payload.Fingerprint {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	source, err := obsidian.New(obsidian.Config{SourceID: sourceID, Root: root})
	if err != nil {
		mapError(response, err)
		return
	}
	document, err := source.Read(request.Context(), contracts.SourceRef{SourceID: sourceID, RelativePath: relative})
	if err != nil || document.Fingerprint != fingerprint || !articleReferencesAsset(request.Context(), source, document, payload.Relative) {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	asset, err := source.ResolveAsset(request.Context(), document.Ref, payload.Relative, obsidian.AssetWikiEmbed)
	if err != nil || asset.AbsolutePath == "" {
		writeError(response, http.StatusNotFound, "resource.not_found", "请求的资源不存在")
		return
	}
	content, err := os.ReadFile(asset.AbsolutePath)
	if err != nil || len(content) > 25<<20 {
		writeError(response, http.StatusUnprocessableEntity, "asset.unavailable", "图片无法读取")
		return
	}
	contentType := http.DetectContentType(content)
	allowed := map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/avif": true}
	if !allowed[contentType] {
		writeError(response, http.StatusUnsupportedMediaType, "asset.type_unsupported", "图片格式不受支持")
		return
	}
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Cache-Control", "private, max-age=300")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(content)
}

func articleReferencesAsset(ctx context.Context, source *obsidian.Provider, document contracts.SourceDocument, relative string) bool {
	for _, match := range wikiImagePattern.FindAllStringSubmatch(document.Body, -1) {
		if asset, err := source.ResolveAsset(ctx, document.Ref, match[1], obsidian.AssetWikiEmbed); err == nil && asset.RelativePath == relative {
			return true
		}
	}
	reader := text.NewReader([]byte(document.Body))
	node := goldmark.New().Parser().Parse(reader)
	found := false
	_ = ast.Walk(node, func(current ast.Node, entering bool) (ast.WalkStatus, error) {
		image, ok := current.(*ast.Image)
		if entering && ok {
			if asset, err := source.ResolveAsset(ctx, document.Ref, string(image.Destination), obsidian.AssetMarkdownImage); err == nil && asset.RelativePath == relative {
				found = true
			}
		}
		return ast.WalkContinue, nil
	})
	return found
}
