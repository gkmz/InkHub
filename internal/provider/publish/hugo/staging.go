package hugo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gkmz/InkHub/internal/platform/filesystem"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

type artifactManifest struct {
	Artifact contracts.PreparedArtifact `json:"artifact"`
}

// Prepare 在 operation 级 staging 站点生成并构建候选 bundle。
func (p *Provider) Prepare(ctx context.Context, input contracts.PublishInput) (contracts.PreparedArtifact, error) {
	preflight, err := p.Preflight(ctx, input)
	if err != nil {
		return contracts.PreparedArtifact{}, err
	}
	if !preflight.Ready {
		return contracts.PreparedArtifact{}, providerError("hugo.preflight_failed", "Hugo 发布前检查未通过", contracts.ErrorValidation, false, nil)
	}
	operationRoot := filepath.Join(p.config.StagingRoot, input.OperationID)
	manifestPath := filepath.Join(operationRoot, "artifact.json")
	if content, readErr := os.ReadFile(manifestPath); readErr == nil {
		var existing artifactManifest
		if json.Unmarshal(content, &existing) != nil || existing.Artifact.OperationID != input.OperationID {
			return contracts.PreparedArtifact{}, providerError("hugo.artifact_invalid", "Hugo staging artifact 记录损坏", contracts.ErrorValidation, false, nil)
		}
		if existing.Artifact.ContentHash != input.ContentHash {
			return contracts.PreparedArtifact{}, providerError("hugo.operation_conflict", "Hugo OperationID 已绑定其他内容版本", contracts.ErrorConflict, false, nil)
		}
		if info, statErr := os.Stat(filepath.Join(existing.Artifact.Location, "index.md")); statErr == nil && !info.IsDir() {
			return existing.Artifact, nil
		}
	} else if !os.IsNotExist(readErr) {
		return contracts.PreparedArtifact{}, fmt.Errorf("读取 Hugo artifact: %w", readErr)
	}
	if err := os.RemoveAll(operationRoot); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("清理 Hugo staging: %w", err)
	}
	stagedSite := filepath.Join(operationRoot, "site")
	if err := copyTree(p.config.Root, stagedSite); err != nil {
		return contracts.PreparedArtifact{}, providerError("hugo.staging_failed", "创建 Hugo staging 失败", contracts.ErrorInternal, false, err)
	}

	target, found, err := findBundle(p.config.Root, p.config.Section, string(input.Article.StableID))
	if err != nil {
		return contracts.PreparedArtifact{}, providerError("hugo.bundle_invalid", "定位 Hugo bundle 失败", contracts.ErrorValidation, false, err)
	}
	if !found {
		segment := bundleSegment(input)
		target = filepath.Join(p.config.Root, "content", p.config.Section, segment)
		if _, statErr := os.Stat(target); statErr == nil {
			return contracts.PreparedArtifact{}, providerError("hugo.bundle_conflict", "Hugo bundle 路径已被其他文章占用", contracts.ErrorConflict, false, nil)
		} else if !os.IsNotExist(statErr) {
			return contracts.PreparedArtifact{}, providerError("hugo.bundle_invalid", "检查 Hugo bundle 目标失败", contracts.ErrorInternal, false, statErr)
		}
	}
	relativeTarget, err := filepath.Rel(p.config.Root, target)
	if err != nil || strings.HasPrefix(relativeTarget, "..") {
		return contracts.PreparedArtifact{}, providerError("hugo.target_invalid", "Hugo bundle 目标越界", contracts.ErrorUnauthorizedResource, false, err)
	}
	stagedBundle := filepath.Join(stagedSite, relativeTarget)
	if err := os.RemoveAll(stagedBundle); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("清理 staging bundle: %w", err)
	}
	if err := os.MkdirAll(stagedBundle, 0o755); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("创建 staging bundle: %w", err)
	}

	resources, err := planResources(input.ResourceRefs)
	if err != nil {
		return contracts.PreparedArtifact{}, providerError("hugo.resource_conflict", "Hugo 文章资源存在冲突", contracts.ErrorConflict, false, err)
	}
	input = rewriteResourceReferences(input, resources)
	articleContent, err := convertArticle(input)
	if err != nil {
		return contracts.PreparedArtifact{}, providerError("hugo.convert_failed", "转换 Hugo 文章失败", contracts.ErrorValidation, false, err)
	}
	if err := filesystem.AtomicWrite(filepath.Join(stagedBundle, "index.md"), articleContent, nil); err != nil {
		return contracts.PreparedArtifact{}, providerError("hugo.staging_failed", "写入 staging bundle 失败", contracts.ErrorInternal, false, err)
	}
	for _, resource := range resources {
		if err := copyFile(resource.Source, filepath.Join(stagedBundle, resource.Name)); err != nil {
			return contracts.PreparedArtifact{}, providerError("hugo.resource_copy_failed", "复制 Hugo 资源失败", contracts.ErrorInternal, false, err)
		}
	}
	revision, err := p.builder.Build(ctx, stagedSite)
	if err != nil {
		return contracts.PreparedArtifact{}, providerError("hugo.build_failed", "Hugo staging 构建失败", contracts.ErrorPermanent, false, err)
	}
	expiresAt := time.Now().UTC().Add(p.config.ArtifactTTL)
	artifact := contracts.PreparedArtifact{
		OperationID: input.OperationID, ProviderRevision: revision, ContentHash: input.ContentHash,
		Location: stagedBundle, TargetPath: target, PreviewURL: p.previewURL(filepath.Base(target)), ExpiresAt: &expiresAt,
	}
	manifest, _ := json.Marshal(artifactManifest{Artifact: artifact})
	if err := filesystem.AtomicWrite(manifestPath, manifest, nil); err != nil {
		return contracts.PreparedArtifact{}, fmt.Errorf("保存 Hugo artifact: %w", err)
	}
	return artifact, nil
}

func (p *Provider) previewURL(bundle string) string {
	if p.config.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(p.config.BaseURL, "/") + "/" + p.config.Section + "/" + bundle + "/"
}

var unsafeBundleChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func bundleSegment(input contracts.PublishInput) string {
	value := strings.TrimSpace(input.Article.Slug)
	if value == "" {
		value = string(input.Article.StableID)
	}
	value = strings.Trim(unsafeBundleChars.ReplaceAllString(value, "-"), "-.")
	if !safeSegment(value) {
		return "article"
	}
	return value
}

func rewriteResourceReferences(input contracts.PublishInput, resources []resourcePlan) contracts.PublishInput {
	for _, resource := range resources {
		for _, ref := range input.ResourceRefs {
			if ref.Resolved != resource.Source {
				continue
			}
			input.Body = strings.ReplaceAll(input.Body, ref.Original, resource.Name)
			if input.Article.Cover == ref.Original {
				input.Article.Cover = resource.Name
			}
		}
	}
	return input
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "public" || strings.HasPrefix(relative, "public"+string(filepath.Separator)) || relative == "resources" || strings.HasPrefix(relative, "resources"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Hugo 站点包含符号链接: %s", relative)
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		return copyFile(path, destination)
	})
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
