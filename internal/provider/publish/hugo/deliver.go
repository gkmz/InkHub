package hugo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

type replaceBundleFunc func(candidate, target, backup string) error

type deliveryManifest struct {
	ContentHash string                   `json:"content_hash"`
	Result      contracts.DeliveryResult `json:"result"`
}

// ValidatePreparedArtifact 验证 staging manifest、目标边界和 Artifact 身份，不修改正式 Hugo 内容。
func (p *Provider) ValidatePreparedArtifact(_ context.Context, artifact contracts.PreparedArtifact) error {
	_, _, err := p.validateArtifact(artifact)
	return err
}

// Deliver 原子替换真实 bundle，并在真实站点构建失败时恢复旧内容。
func (p *Provider) Deliver(ctx context.Context, artifact contracts.PreparedArtifact) (contracts.DeliveryResult, error) {
	prepared, operationRoot, err := p.validateArtifact(artifact)
	if err != nil {
		return contracts.DeliveryResult{}, err
	}
	deliveredPath := filepath.Join(operationRoot, "delivered.json")
	if result, ok := loadDelivery(deliveredPath, artifact.ContentHash); ok {
		return result, nil
	}

	parent := filepath.Dir(prepared.TargetPath)
	base := filepath.Base(prepared.TargetPath)
	candidate := filepath.Join(parent, "."+base+".inkhub-"+artifact.OperationID+".tmp")
	backup := filepath.Join(parent, "."+base+".inkhub-"+artifact.OperationID+".bak")
	if err := recoverBundle(prepared.TargetPath, backup); err != nil {
		return contracts.DeliveryResult{}, providerError("hugo.recovery_failed", "恢复 Hugo 旧 bundle 失败", contracts.ErrorInternal, false, err)
	}
	if err := os.RemoveAll(candidate); err != nil {
		return contracts.DeliveryResult{}, fmt.Errorf("清理 Hugo 候选 bundle: %w", err)
	}
	if err := copyTree(prepared.Location, candidate); err != nil {
		return contracts.DeliveryResult{}, providerError("hugo.delivery_failed", "创建 Hugo 候选 bundle 失败", contracts.ErrorInternal, false, err)
	}
	if err := p.replace(candidate, prepared.TargetPath, backup); err != nil {
		_ = os.RemoveAll(candidate)
		if restoreErr := recoverBundle(prepared.TargetPath, backup); restoreErr != nil {
			return contracts.DeliveryResult{}, providerError("hugo.recovery_failed", "Hugo 替换失败且旧 bundle 恢复失败", contracts.ErrorInternal, false, restoreErr)
		}
		return contracts.DeliveryResult{}, providerError("hugo.replace_failed", "替换 Hugo bundle 失败", contracts.ErrorInternal, false, err)
	}

	revision, buildErr := p.builder.Build(ctx, p.config.Root)
	if buildErr != nil {
		_ = os.RemoveAll(prepared.TargetPath)
		if restoreErr := recoverBundle(prepared.TargetPath, backup); restoreErr != nil {
			return contracts.DeliveryResult{}, providerError("hugo.recovery_failed", "Hugo 构建失败且旧 bundle 恢复失败", contracts.ErrorInternal, false, restoreErr)
		}
		return contracts.DeliveryResult{}, providerError("hugo.build_failed", "Hugo 真实站点构建失败，已恢复旧内容", contracts.ErrorPermanent, false, buildErr)
	}
	if err := os.RemoveAll(backup); err != nil {
		return contracts.DeliveryResult{}, providerError("hugo.cleanup_failed", "Hugo 发布成功但备份清理失败", contracts.ErrorInternal, false, err)
	}
	result := contracts.DeliveryResult{State: "published", ProviderRevision: revision, Location: prepared.TargetPath}
	manifest, _ := json.Marshal(deliveryManifest{ContentHash: artifact.ContentHash, Result: result})
	if err := filesystem.AtomicWrite(deliveredPath, manifest, nil); err != nil {
		return contracts.DeliveryResult{}, providerError("hugo.delivery_record_failed", "Hugo 已更新但交付记录保存失败", contracts.ErrorInternal, false, err)
	}
	return result, nil
}

func (p *Provider) validateArtifact(artifact contracts.PreparedArtifact) (contracts.PreparedArtifact, string, error) {
	if !operationIDPattern.MatchString(artifact.OperationID) {
		return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_invalid", "Hugo artifact OperationID 无效", contracts.ErrorValidation, false, nil)
	}
	operationRoot := filepath.Join(p.config.StagingRoot, artifact.OperationID)
	content, err := os.ReadFile(filepath.Join(operationRoot, "artifact.json"))
	if err != nil {
		return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_missing", "Hugo artifact 不存在", contracts.ErrorNotFound, false, err)
	}
	var manifest artifactManifest
	if json.Unmarshal(content, &manifest) != nil {
		return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_invalid", "Hugo artifact 记录无效", contracts.ErrorValidation, false, nil)
	}
	prepared := manifest.Artifact
	if prepared.OperationID != artifact.OperationID || prepared.ContentHash != artifact.ContentHash ||
		prepared.Location != artifact.Location || prepared.TargetPath != artifact.TargetPath {
		return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_conflict", "Hugo artifact 与已准备记录不一致", contracts.ErrorConflict, false, nil)
	}
	stagedSite := filepath.Join(operationRoot, "site")
	contentRoot := filepath.Join(p.config.Root, "content")
	if !withinOrEqual(prepared.Location, stagedSite) || !withinOrEqual(prepared.TargetPath, contentRoot) {
		return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_unauthorized", "Hugo artifact 路径越界", contracts.ErrorUnauthorizedResource, false, nil)
	}
	resolvedTarget := filepath.Join(p.config.Root, filepath.FromSlash(prepared.TargetRelativePath))
	if prepared.TargetRelativePath == "" || resolvedTarget != prepared.TargetPath {
		return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_conflict", "Hugo artifact 相对目标不匹配", contracts.ErrorConflict, false, nil)
	}
	for _, file := range prepared.Files {
		if file.RelativePath == "" || !withinOrEqual(filepath.Join(prepared.Location, filepath.FromSlash(file.RelativePath)), prepared.Location) {
			return contracts.PreparedArtifact{}, "", providerError("hugo.artifact_unauthorized", "Hugo artifact 文件路径越界", contracts.ErrorUnauthorizedResource, false, nil)
		}
	}
	return prepared, operationRoot, nil
}

func loadDelivery(path, contentHash string) (contracts.DeliveryResult, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return contracts.DeliveryResult{}, false
	}
	var manifest deliveryManifest
	if json.Unmarshal(content, &manifest) != nil || manifest.ContentHash != contentHash {
		return contracts.DeliveryResult{}, false
	}
	return manifest.Result, true
}

func replaceBundle(candidate, target, backup string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(candidate, target); err != nil {
		return err
	}
	return nil
}

func recoverBundle(target, backup string) error {
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	// 备份存在说明上次替换未完成；先移除不可信候选，再恢复最后确认的旧 bundle。
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(backup, target)
}

func withinOrEqual(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && (relative == "." || (relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}
