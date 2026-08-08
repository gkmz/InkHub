package httptransport

import (
	"context"
	"fmt"

	workspaceapp "github.com/gkmz/InkHub/internal/app/workspace"
	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

// workspaceIdentityWriter 以最小文本改动为文章建立 Stable ID。
type workspaceIdentityWriter interface {
	WriteInitializationIdentity(ctx context.Context, command contracts.MetadataWriteCommand) (contracts.SourceDocument, error)
}

// workspaceInitializationIssue 描述阻止内容范围完成初始化的单篇文章问题。
type workspaceInitializationIssue struct {
	ArticlePath string `json:"article_path"`
	Code        string `json:"code"`
	Message     string `json:"message"`
}

// workspaceInitializationReport 汇总扫描、身份补全和最终索引结果。
type workspaceInitializationReport struct {
	Indexed     int                            `json:"indexed"`
	AssignedIDs int                            `json:"assigned_ids"`
	Failed      int                            `json:"failed"`
	Issues      []workspaceInitializationIssue `json:"issues"`
}

// workspaceInitializationError 保留可供界面审查的文件级失败信息。
type workspaceInitializationError struct {
	Issues []workspaceInitializationIssue
}

func (e *workspaceInitializationError) Error() string {
	return fmt.Sprintf("有 %d 篇文章无法完成 frontmatter 与 Stable ID 初始化", len(e.Issues))
}

// initializeWorkspaceSource 先完整预检，再补齐缺失身份，最后建立可重建索引。
func (h *runtimeHandler) initializeWorkspaceSource(ctx context.Context, sourceID, workspaceID string, overrideConfig []byte) (workspaceInitializationReport, error) {
	h.metadataWriteMu.Lock()
	defer h.metadataWriteMu.Unlock()

	source, err := h.buildSource(ctx, sourceID, overrideConfig)
	if err != nil {
		return workspaceInitializationReport{}, err
	}
	identityWriter, ok := source.(workspaceIdentityWriter)
	if !ok {
		return workspaceInitializationReport{}, fmt.Errorf("当前内容源不支持初始化文章身份")
	}
	scan, err := source.Scan(ctx, contracts.ScanCursor{})
	if err != nil {
		return workspaceInitializationReport{}, err
	}

	type pendingIdentity struct {
		ref         contracts.SourceRef
		fingerprint string
	}
	pending := make([]pendingIdentity, 0)
	issues := make([]workspaceInitializationIssue, 0)
	for _, reference := range scan.Documents {
		if diagnostic, blocked := firstBlockingDiagnostic(reference.Diagnostics); blocked {
			issues = append(issues, workspaceInitializationIssue{ArticlePath: reference.Ref.RelativePath, Code: diagnostic.Code, Message: diagnostic.Message})
			continue
		}
		document, readErr := source.Read(ctx, reference.Ref)
		if readErr != nil {
			issues = append(issues, workspaceInitializationIssue{ArticlePath: reference.Ref.RelativePath, Code: "workspace.article_read_failed", Message: readErr.Error()})
			continue
		}
		stableID := string(document.Article.StableID)
		if stableID == "" {
			pending = append(pending, pendingIdentity{ref: reference.Ref, fingerprint: document.Fingerprint})
			continue
		}
		if validateErr := article.StableID(stableID).Validate(); validateErr != nil {
			issues = append(issues, workspaceInitializationIssue{ArticlePath: reference.Ref.RelativePath, Code: "workspace.stable_id_invalid", Message: validateErr.Error()})
		}
	}
	if len(issues) > 0 {
		return workspaceInitializationReport{Failed: len(issues), Issues: issues}, &workspaceInitializationError{Issues: issues}
	}

	assigned := 0
	for _, value := range pending {
		stableID, idErr := newArticleStableID()
		if idErr != nil {
			return workspaceInitializationReport{AssignedIDs: assigned}, idErr
		}
		_, writeErr := identityWriter.WriteInitializationIdentity(ctx, contracts.MetadataWriteCommand{
			Ref: value.ref, ExpectedFingerprint: value.fingerprint, Patch: contracts.MetadataPatch{StableID: &stableID},
		})
		if writeErr != nil {
			issue := workspaceInitializationIssue{ArticlePath: value.ref.RelativePath, Code: "workspace.identity_write_failed", Message: writeErr.Error()}
			return workspaceInitializationReport{AssignedIDs: assigned, Failed: 1, Issues: []workspaceInitializationIssue{issue}}, &workspaceInitializationError{Issues: []workspaceInitializationIssue{issue}}
		}
		assigned++
	}

	indexed, err := workspaceapp.ScanWorkspace(ctx, source, repository.NewArticleRepository(h.db), workspaceapp.ScanOptions{WorkspaceID: workspaceID, SourceID: sourceID}, contracts.ScanCursor{})
	if err != nil {
		return workspaceInitializationReport{AssignedIDs: assigned}, err
	}
	result := workspaceInitializationReport{Indexed: indexed.Indexed, AssignedIDs: assigned, Failed: indexed.Failed, Issues: []workspaceInitializationIssue{}}
	if indexed.Failed > 0 {
		return result, fmt.Errorf("最终索引仍有 %d 篇文章失败", indexed.Failed)
	}
	return result, nil
}

func firstBlockingDiagnostic(diagnostics []contracts.Diagnostic) (contracts.Diagnostic, bool) {
	for _, diagnostic := range diagnostics {
		if diagnostic.Blocking {
			return diagnostic, true
		}
	}
	return contracts.Diagnostic{}, false
}
