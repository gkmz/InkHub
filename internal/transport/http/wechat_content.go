package httptransport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type wechatContentArtifact struct {
	OperationID string `json:"OperationID"`
	ContentHash string `json:"ContentHash"`
	Location    string `json:"Location"`
}

type wechatContentManifest struct {
	Artifact   wechatContentArtifact `json:"artifact"`
	HTMLDigest string                `json:"html_digest"`
}

// wechatContent 只返回当前工作区、文章版本和启用 Provider 对应的完整微信产物。
func (h *runtimeHandler) wechatContent(response http.ResponseWriter, request *http.Request) {
	articleID := strings.TrimPrefix(request.URL.Path, "/api/v1/wechat/content/")
	var jobID, contentHash, providerConfig, resultJSON string
	err := h.db.QueryRowContext(request.Context(), `SELECT jobs.id,articles.content_hash,provider_instances.config_json,jobs.result_json
FROM articles
JOIN workspaces ON workspaces.id=articles.workspace_id
  AND workspaces.id=(SELECT id FROM workspaces ORDER BY last_used_at DESC LIMIT 1)
JOIN provider_instances ON provider_instances.workspace_id=articles.workspace_id
  AND provider_instances.provider_type='wechat' AND provider_instances.enabled=1
JOIN jobs ON jobs.workspace_id=articles.workspace_id AND jobs.kind='wechat_prepare' AND jobs.state='succeeded'
  AND json_extract(jobs.payload_json,'$.article_id')=articles.id
  AND json_extract(jobs.payload_json,'$.provider_instance_id')=provider_instances.id
  AND json_extract(jobs.payload_json,'$.content_hash')=articles.content_hash
WHERE articles.id=? AND articles.deleted_at IS NULL
ORDER BY jobs.finished_at DESC,jobs.id DESC LIMIT 1`, articleID).Scan(&jobID, &contentHash, &providerConfig, &resultJSON)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	content, err := readVerifiedWeChatContent(jobID, contentHash, providerConfig, resultJSON)
	if err != nil {
		mapError(response, ErrNotFound)
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"html": string(content)})
}

func readVerifiedWeChatContent(jobID, contentHash, providerConfig, resultJSON string) ([]byte, error) {
	var config struct {
		StagingRoot string `json:"staging_root"`
	}
	var result struct {
		Location string `json:"location"`
	}
	if json.Unmarshal([]byte(providerConfig), &config) != nil || json.Unmarshal([]byte(resultJSON), &result) != nil || config.StagingRoot == "" {
		return nil, ErrNotFound
	}
	root, err := filepath.Abs(config.StagingRoot)
	if err != nil || filepath.Base(jobID) != jobID {
		return nil, ErrNotFound
	}
	operationRoot := filepath.Join(root, jobID)
	if !resolvedWithin(root, operationRoot) {
		return nil, ErrNotFound
	}
	expectedContent := filepath.Join(operationRoot, "content.html")
	if filepath.Clean(result.Location) != expectedContent {
		return nil, ErrNotFound
	}
	manifestContent, err := readRegularFile(filepath.Join(operationRoot, "artifact.json"))
	if err != nil {
		return nil, ErrNotFound
	}
	var manifest wechatContentManifest
	if json.Unmarshal(manifestContent, &manifest) != nil || manifest.Artifact.OperationID != jobID || manifest.Artifact.ContentHash != contentHash || filepath.Clean(manifest.Artifact.Location) != expectedContent || manifest.HTMLDigest == "" {
		return nil, ErrNotFound
	}
	content, err := readRegularFile(expectedContent)
	if err != nil {
		return nil, ErrNotFound
	}
	sum := sha256.Sum256(content)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), manifest.HTMLDigest) {
		return nil, ErrNotFound
	}
	return content, nil
}

func resolvedWithin(root, target string) bool {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedTarget)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = errors.New("文件类型无效")
		}
		return nil, err
	}
	return os.ReadFile(path)
}
