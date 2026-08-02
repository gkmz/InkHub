package githubassets

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	platformlogging "github.com/gkmz/InkHub/internal/platform/logging"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"go.uber.org/zap"
)

const maxGitHubResponseSize = 2 << 20

// Uploader 使用 GitHub Contents API 幂等保存公开图片。
type Uploader struct {
	config Config
	client *http.Client
	logger *zap.Logger
}

// New 创建只连接 GitHub 官方端点的图片上传器。
func New(config Config, client *http.Client, logger *zap.Logger) (*Uploader, error) {
	if config.Branch == "" {
		config.Branch = "main"
	}
	if config.Prefix == "" {
		config.Prefix = "inkhub"
	}
	if err := validateConfig(config, true); err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Uploader{config: config, client: client, logger: logger}, nil
}

// Validate 确认目标仓库公开且当前 Token 具备内容写权限。
func (u *Uploader) Validate(ctx context.Context) (err error) {
	defer func() { u.logError("validate", err) }()
	var value struct {
		Private     bool `json:"private"`
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	status, err := u.getJSON(ctx, repositoryURL(u.config), &value)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return githubError("github.permission_denied", "GitHub Token 没有仓库写权限", contracts.ErrorUnauthorizedResource, false)
	}
	if status != http.StatusOK {
		return githubError("github.config_invalid", "GitHub 图片仓库不可用", contracts.ErrorValidation, false)
	}
	if value.Private {
		return githubError("github.repository_private", "微信图片仓库必须是公开仓库", contracts.ErrorValidation, false)
	}
	if !value.Permissions.Push {
		return githubError("github.permission_denied", "GitHub Token 没有仓库写权限", contracts.ErrorUnauthorizedResource, false)
	}
	var branch map[string]any
	status, err = u.getJSON(ctx, repositoryURL(u.config)+"/branches/"+url.PathEscape(u.config.Branch), &branch)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return githubError("github.config_invalid", "GitHub 图片仓库分支不存在", contracts.ErrorValidation, false)
	}
	return nil
}

// Inspect 只读检查确定性目标是否存在且内容摘要一致。
func (u *Uploader) Inspect(ctx context.Context, request contracts.AssetUploadRequest) (result contracts.AssetUploadResult, found bool, err error) {
	defer func() { u.logError("inspect", err) }()
	assetPath, err := AssetPath(u.config.Prefix, request.Digest, request.Extension)
	if err != nil {
		return contracts.AssetUploadResult{}, false, githubError("github.config_invalid", "GitHub 图片目标无效", contracts.ErrorValidation, false)
	}
	var value struct {
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	status, err := u.getJSON(ctx, contentsURL(u.config, assetPath), &value)
	if err != nil {
		return contracts.AssetUploadResult{}, false, err
	}
	if status == http.StatusNotFound {
		return contracts.AssetUploadResult{}, false, nil
	}
	if status != http.StatusOK || value.Type != "file" || value.Encoding != "base64" {
		return contracts.AssetUploadResult{}, false, githubError("github.upload_failed", "读取 GitHub 图片目标失败", contracts.ErrorDependency, true)
	}
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(value.Content, "\n", ""))
	if err != nil {
		return contracts.AssetUploadResult{}, false, githubError("github.upload_failed", "GitHub 图片响应无效", contracts.ErrorDependency, true)
	}
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != request.Digest {
		return contracts.AssetUploadResult{}, false, githubError("github.asset_conflict", "GitHub 图片目标已有不同内容", contracts.ErrorConflict, false)
	}
	publicURL, err := RawURL(u.config, assetPath)
	if err != nil {
		return contracts.AssetUploadResult{}, false, githubError("github.config_invalid", "GitHub 图片公开地址无效", contracts.ErrorValidation, false)
	}
	return contracts.AssetUploadResult{URL: publicURL, Reused: true}, true, nil
}

// Upload 幂等创建摘要路径，并确认最终地址可以匿名访问。
func (u *Uploader) Upload(ctx context.Context, request contracts.AssetUploadRequest) (result contracts.AssetUploadResult, err error) {
	defer func() { u.logError("upload", err) }()
	if existing, found, err := u.Inspect(ctx, request); err != nil || found {
		return existing, err
	}
	content, err := os.ReadFile(request.LocalPath)
	if err != nil || len(content) == 0 || len(content) > 10<<20 {
		return contracts.AssetUploadResult{}, githubError("github.upload_failed", "读取待上传图片失败", contracts.ErrorValidation, false)
	}
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != request.Digest {
		return contracts.AssetUploadResult{}, githubError("github.asset_conflict", "待上传图片内容已变化", contracts.ErrorConflict, false)
	}
	assetPath, err := AssetPath(u.config.Prefix, request.Digest, request.Extension)
	if err != nil {
		return contracts.AssetUploadResult{}, githubError("github.config_invalid", "GitHub 图片目标无效", contracts.ErrorValidation, false)
	}
	payload, _ := json.Marshal(map[string]string{
		"message": "InkHub 上传微信文章图片", "content": base64.StdEncoding.EncodeToString(content), "branch": u.config.Branch,
	})
	status, err := u.sendJSON(ctx, http.MethodPut, contentsBaseURL(u.config, assetPath), payload)
	if err != nil {
		return contracts.AssetUploadResult{}, err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return contracts.AssetUploadResult{}, githubError("github.upload_failed", "上传 GitHub 图片失败", contracts.ErrorDependency, status >= 500)
	}
	publicURL, err := RawURL(u.config, assetPath)
	if err != nil {
		return contracts.AssetUploadResult{}, githubError("github.config_invalid", "GitHub 图片公开地址无效", contracts.ErrorValidation, false)
	}
	if err := u.verifyPublicURL(ctx, publicURL); err != nil {
		return contracts.AssetUploadResult{}, err
	}
	return contracts.AssetUploadResult{URL: publicURL}, nil
}

func (u *Uploader) getJSON(ctx context.Context, endpoint string, target any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, githubError("github.config_invalid", "GitHub 请求无效", contracts.ErrorValidation, false)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+u.config.Token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := u.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, githubError("github.upload_failed", "连接 GitHub 失败", contracts.ErrorDependency, true)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxGitHubResponseSize+1)
	content, err := io.ReadAll(limited)
	if err != nil || len(content) > maxGitHubResponseSize {
		return 0, githubError("github.upload_failed", "GitHub 响应无效", contracts.ErrorDependency, true)
	}
	if len(content) > 0 && json.Unmarshal(content, target) != nil && response.StatusCode == http.StatusOK {
		return 0, githubError("github.upload_failed", "GitHub 响应无效", contracts.ErrorDependency, true)
	}
	return response.StatusCode, nil
}

func (u *Uploader) sendJSON(ctx context.Context, method, endpoint string, payload []byte) (int, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return 0, githubError("github.config_invalid", "GitHub 请求无效", contracts.ErrorValidation, false)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+u.config.Token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := u.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, githubError("github.upload_failed", "连接 GitHub 失败", contracts.ErrorDependency, true)
	}
	defer response.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, maxGitHubResponseSize+1)); err != nil {
		return 0, githubError("github.upload_failed", "GitHub 响应无效", contracts.ErrorDependency, true)
	}
	return response.StatusCode, nil
}

func (u *Uploader) verifyPublicURL(ctx context.Context, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubError("github.public_url_unavailable", "GitHub 图片公开地址无效", contracts.ErrorValidation, false)
	}
	response, err := u.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return githubError("github.public_url_unavailable", "GitHub 图片暂时无法公开访问", contracts.ErrorDependency, true)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubError("github.public_url_unavailable", "GitHub 图片暂时无法公开访问", contracts.ErrorDependency, true)
	}
	return nil
}

// logError 记录 GitHub 边界失败的稳定上下文，不记录 Token、图片内容或完整响应。
func (u *Uploader) logError(operation string, err error) {
	if err == nil {
		return
	}
	fields := []zap.Field{
		zap.String("provider", "github-assets"),
		zap.String("operation", operation),
		zap.String("repository_owner", u.config.Owner),
		zap.String("repository", u.config.Repository),
	}
	fields = append(fields, platformlogging.ErrorFields(err)...)
	u.logger.Error("GitHub 图片 Provider 操作失败", fields...)
}

func repositoryURL(config Config) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s", url.PathEscape(config.Owner), url.PathEscape(config.Repository))
}

func contentsURL(config Config, assetPath string) string {
	return contentsBaseURL(config, assetPath) + "?ref=" + url.QueryEscape(config.Branch)
}

func contentsBaseURL(config Config, assetPath string) string {
	parts := strings.Split(assetPath, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return repositoryURL(config) + "/contents/" + strings.Join(parts, "/")
}

func githubError(code, message string, category contracts.ErrorCategory, retryable bool) *contracts.ProviderError {
	return &contracts.ProviderError{Code: code, Message: message, Category: category, Retryable: retryable}
}
