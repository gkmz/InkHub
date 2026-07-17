package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appjob "github.com/gkmz/InkHub/internal/app/job"
	"github.com/gkmz/InkHub/internal/app/publication"
	domainjob "github.com/gkmz/InkHub/internal/domain/job"
	domainpublication "github.com/gkmz/InkHub/internal/domain/publication"
	"github.com/gkmz/InkHub/internal/provider/contracts"
	"github.com/gkmz/InkHub/internal/storage/sqlite/repository"
)

type hugoPreviewPayload struct {
	PreviewID   string `json:"preview_id"`
	ArticleID   string `json:"article_id"`
	ProviderID  string `json:"provider_instance_id"`
	ContentHash string `json:"content_hash"`
	Section     string `json:"section"`
}

type hugoPreviewJobHandler struct {
	publicationJobHandler
	jobs *repository.JobRepository
}

func (h hugoPreviewJobHandler) handlePreview(ctx context.Context, execution *appjob.Execution) (string, error) {
	payload, err := decodeHugoPreviewPayload(execution.Job)
	if err != nil {
		return "", err
	}
	input, providerType, config, err := h.loadInput(ctx, payload.PreviewID, publicationPayload{ArticleID: payload.ArticleID, ProviderID: payload.ProviderID, ContentHash: payload.ContentHash})
	if err != nil {
		return "", err
	}
	if providerType != string(contracts.ProviderHugo) {
		return "", fmt.Errorf("Hugo 预览任务只能使用 Hugo Provider")
	}
	input.TargetSection = payload.Section
	input.PreviewOnly = true
	view, err := providerConfigView(config)
	if err != nil {
		return "", err
	}
	provider, err := h.runtime.BuildPublish(ctx, contracts.ProviderRef{ID: payload.ProviderID, Type: contracts.ProviderHugo}, view)
	if err != nil {
		return "", err
	}
	if err := execution.ReportProgress(ctx, 20); err != nil {
		return "", err
	}
	preflight, err := provider.Preflight(ctx, input)
	if err != nil {
		return "", err
	}
	if !preflight.Ready {
		return "", fmt.Errorf("Hugo 发布前检查未通过")
	}
	if err := execution.ReportProgress(ctx, 45); err != nil {
		return "", err
	}
	artifact, err := provider.Prepare(ctx, input)
	if err != nil {
		return "", err
	}
	if err := execution.ReportProgress(ctx, 85); err != nil {
		return "", err
	}
	result := publication.HugoPreviewResult{PreviewID: payload.PreviewID, ArticleID: payload.ArticleID, WorkspaceID: execution.Job.WorkspaceID, ProviderID: payload.ProviderID, Section: payload.Section, Artifact: artifact, Diagnostics: preflight.Diagnostics}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("编码 Hugo 预览结果: %w", err)
	}
	return string(encoded), nil
}

func (h hugoPreviewJobHandler) handleDeliver(ctx context.Context, execution *appjob.Execution) (string, error) {
	payload, err := decodeHugoPreviewPayload(execution.Job)
	if err != nil {
		return "", err
	}
	previewJob, err := h.jobs.FindByID(ctx, payload.PreviewID)
	if err != nil {
		return "", err
	}
	if previewJob.State != domainjob.StateSucceeded || previewJob.WorkspaceID != execution.Job.WorkspaceID {
		return "", publication.ErrPreviewNotReady
	}
	var preview publication.HugoPreviewResult
	if err := json.Unmarshal([]byte(previewJob.ResultJSON), &preview); err != nil {
		return "", publication.ErrPreviewInvalid
	}
	// 交付只能消费任务所引用预览的同一身份、内容版本和 staging Artifact。
	if preview.PreviewID != previewJob.ID || preview.Artifact.OperationID != previewJob.ID || preview.ArticleID != payload.ArticleID || preview.ProviderID != payload.ProviderID || preview.WorkspaceID != execution.Job.WorkspaceID || preview.Artifact.ContentHash != payload.ContentHash {
		return "", publication.ErrPreviewInvalid
	}
	if preview.Artifact.ExpiresAt == nil || !preview.Artifact.ExpiresAt.After(time.Now().UTC()) {
		return "", publication.ErrPreviewExpired
	}
	_, providerType, config, err := h.loadInput(ctx, preview.PreviewID, publicationPayload{ArticleID: payload.ArticleID, ProviderID: payload.ProviderID, ContentHash: payload.ContentHash})
	if err != nil {
		return "", err
	}
	if providerType != string(contracts.ProviderHugo) {
		return "", fmt.Errorf("Hugo 交付任务只能使用 Hugo Provider")
	}
	view, err := providerConfigView(config)
	if err != nil {
		return "", err
	}
	provider, err := h.runtime.BuildPublish(ctx, contracts.ProviderRef{ID: payload.ProviderID, Type: contracts.ProviderHugo}, view)
	if err != nil {
		return "", err
	}
	if err := execution.ReportProgress(ctx, 45); err != nil {
		return "", err
	}
	delivery, err := provider.Deliver(ctx, preview.Artifact)
	if err != nil {
		return "", err
	}
	if err := execution.ReportProgress(ctx, 85); err != nil {
		return "", err
	}
	recordID := stableAPIID("publication", payload.ArticleID, payload.ProviderID)
	err = h.publications.SaveWithEvent(ctx, repository.PublicationRecord{ID: recordID, ArticleID: payload.ArticleID, ProviderInstanceID: payload.ProviderID, WorkspaceID: execution.Job.WorkspaceID, State: domainpublication.StatePublished, ContentHash: payload.ContentHash, ProviderRevision: delivery.ProviderRevision}, repository.PublicationEvent{ID: stableAPIID("event", recordID, "published", payload.ContentHash), Type: "published", ContentHash: payload.ContentHash, Payload: map[string]string{"job": execution.Job.ID, "preview": preview.PreviewID}})
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(map[string]string{"state": "published", "preview_id": preview.PreviewID})
	if err != nil {
		return "", fmt.Errorf("编码 Hugo 交付结果: %w", err)
	}
	return string(encoded), nil
}

func decodeHugoPreviewPayload(job domainjob.Job) (hugoPreviewPayload, error) {
	var payload hugoPreviewPayload
	if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
		return hugoPreviewPayload{}, fmt.Errorf("解析 Hugo 任务: %w", err)
	}
	if payload.PreviewID == "" || payload.ArticleID == "" || payload.ProviderID == "" || payload.ContentHash == "" {
		return hugoPreviewPayload{}, fmt.Errorf("Hugo 任务参数不完整")
	}
	if job.Kind == "hugo_preview" && (payload.PreviewID != job.ID || payload.Section == "") {
		return hugoPreviewPayload{}, fmt.Errorf("Hugo 预览任务身份无效")
	}
	return payload, nil
}
