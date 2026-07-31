package publication

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/InkHub/internal/domain/article"
	"github.com/gkmz/InkHub/internal/provider/contracts"
)

func TestWeChatPlanIsReadOnlyAndConfirmEnqueuesBoundIntent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	provider := &staticPlanningProvider{items: []contracts.AssetPlanItem{{Reference: "images/cover.png", MediaType: "image/png", Size: 120, State: "upload"}}}
	resolver := staticWeChatPlanResolver{value: WeChatPlanArticle{
		WorkspaceID: "w1", ArticleID: "a1", ProviderID: "wechat1", ContentHash: "hash1",
		ContentStage: article.ContentStageReady,
		TemplateID:   "default", TemplateRevision: "template1", Provider: provider,
	}}
	queue := &capturingQueue{}
	service, err := NewWeChatPlanService(resolver, queue, []byte("01234567890123456789012345678901"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.Plan(context.Background(), "a1", "default")
	if err != nil || !plan.Ready || len(plan.Images) != 1 || plan.Token == "" {
		t.Fatalf("生成微信计划: plan=%+v err=%v", plan, err)
	}
	if provider.uploadCalls != 0 {
		t.Fatal("生成计划不得上传图片")
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(strings.Split(plan.Token, ".")[0])
	if strings.Contains(string(decoded), "hash1") || strings.Contains(string(decoded), "wechat1") {
		t.Fatalf("opaque token 泄露内部身份: %s", decoded)
	}
	jobID, err := service.Confirm(context.Background(), "a1", plan.Token)
	if err != nil || jobID == "" || queue.last.Kind != "wechat_prepare" || queue.last.ContentHash != "hash1" {
		t.Fatalf("确认计划未入队: id=%s intent=%+v err=%v", jobID, queue.last, err)
	}
}

func TestWeChatPlanRejectsTamperedToken(t *testing.T) {
	t.Parallel()

	service, err := NewWeChatPlanService(staticWeChatPlanResolver{value: WeChatPlanArticle{
		WorkspaceID: "w1", ArticleID: "a1", ProviderID: "wechat1", ContentHash: "hash1", TemplateID: "default", TemplateRevision: "template1", Provider: &staticPlanningProvider{},
		ContentStage: article.ContentStageReady,
	}}, &capturingQueue{}, []byte("01234567890123456789012345678901"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.Plan(context.Background(), "a1", "default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(context.Background(), "a1", plan.Token+"tampered"); err != ErrWeChatPlanInvalid {
		t.Fatalf("篡改 token 错误=%v", err)
	}
}

type staticWeChatPlanResolver struct{ value WeChatPlanArticle }

func (resolver staticWeChatPlanResolver) ResolveWeChatPlan(context.Context, string, string) (WeChatPlanArticle, error) {
	return resolver.value, nil
}

type staticPlanningProvider struct {
	items       []contracts.AssetPlanItem
	uploadCalls int
}

func (provider *staticPlanningProvider) InspectAssets(context.Context, contracts.PublishInput) ([]contracts.AssetPlanItem, []contracts.Diagnostic, error) {
	return provider.items, nil, nil
}

func (provider *staticPlanningProvider) Descriptor() contracts.PublishDescriptor {
	return contracts.PublishDescriptor{}
}
func (provider *staticPlanningProvider) Validate(context.Context) error { return nil }
func (provider *staticPlanningProvider) Preflight(context.Context, contracts.PublishInput) (contracts.PreflightResult, error) {
	return contracts.PreflightResult{Ready: true}, nil
}
func (provider *staticPlanningProvider) Prepare(context.Context, contracts.PublishInput) (contracts.PreparedArtifact, error) {
	provider.uploadCalls++
	return contracts.PreparedArtifact{}, nil
}
func (provider *staticPlanningProvider) Deliver(context.Context, contracts.PreparedArtifact) (contracts.DeliveryResult, error) {
	return contracts.DeliveryResult{}, nil
}
