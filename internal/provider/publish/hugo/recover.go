package hugo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gkmz/InkHub/internal/provider/contracts"
)

// Recover 扫描未完成 operation，并恢复遗留的真实 bundle 备份。
func (p *Provider) Recover(ctx context.Context) error {
	entries, err := os.ReadDir(p.config.StagingRoot)
	if err != nil {
		return fmt.Errorf("读取 Hugo staging: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || !operationIDPattern.MatchString(entry.Name()) {
			continue
		}
		operationRoot := filepath.Join(p.config.StagingRoot, entry.Name())
		content, err := os.ReadFile(filepath.Join(operationRoot, "artifact.json"))
		if err != nil {
			continue
		}
		var manifest artifactManifest
		if json.Unmarshal(content, &manifest) != nil {
			continue
		}
		if manifest.Artifact.OperationID != entry.Name() {
			return providerError("hugo.artifact_conflict", "Hugo 恢复记录与 operation 目录不一致", contracts.ErrorConflict, false, nil)
		}
		target := manifest.Artifact.TargetPath
		contentRoot := contentRoot(p.config.Root, p.config.ContentDir)
		if !withinOrEqual(target, contentRoot) {
			return providerError("hugo.artifact_unauthorized", "Hugo 恢复目标路径越界", contracts.ErrorUnauthorizedResource, false, nil)
		}
		backup := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".inkhub-"+entry.Name()+".bak")
		if err := recoverBundle(target, backup); err != nil {
			return fmt.Errorf("恢复 Hugo operation %s: %w", entry.Name(), err)
		}
		previousTarget := manifest.Artifact.PreviousTargetPath
		if previousTarget == "" {
			continue
		}
		if previousTarget == target || !withinOrEqual(previousTarget, contentRoot) {
			return providerError("hugo.artifact_unauthorized", "Hugo 旧路径恢复目标越界或与新目标相同", contracts.ErrorUnauthorizedResource, false, nil)
		}
		previousBackup := filepath.Join(filepath.Dir(previousTarget), "."+filepath.Base(previousTarget)+".inkhub-"+entry.Name()+".bak")
		if err := recoverBundle(previousTarget, previousBackup); err != nil {
			return fmt.Errorf("恢复 Hugo operation %s 的旧路径: %w", entry.Name(), err)
		}
	}
	return nil
}
