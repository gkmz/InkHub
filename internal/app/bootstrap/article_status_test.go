package bootstrap

import (
	"testing"

	"github.com/gkmz/InkHub/internal/domain/article"
)

func TestDeriveArticleWorkflowSkipsDrafts(t *testing.T) {
	got := deriveArticleWorkflow(articleWorkflowInput{
		ContentStage: article.ContentStageDraft,
		ReviewState:  "pending_review",
		ContentHash:  "v1",
	})
	if got.State != "draft" || got.NextAction != "" || got.Bucket != "" {
		t.Fatalf("draft workflow = %+v", got)
	}
}

func TestDeriveArticleWorkflowUsesConfirmedWeChatAsTerminal(t *testing.T) {
	got := deriveArticleWorkflow(articleWorkflowInput{
		ContentStage: article.ContentStageReady,
		ReviewState:  "approved",
		ApprovedHash: "v1",
		ContentHash:  "v2",
		HugoState:    "published",
		HugoHash:     "v1",
		WeChatState:  "confirmed",
		WeChatHash:   "v1",
	})
	if got.State != "changed" || got.WeChatLabel != "已确认草稿" || got.Bucket != "changed" {
		t.Fatalf("confirmed WeChat workflow = %+v", got)
	}
}

func TestDeriveArticleWorkflowPlacesFullyHandledReadyArticleInLatestReady(t *testing.T) {
	got := deriveArticleWorkflow(articleWorkflowInput{
		ContentStage:      article.ContentStageReady,
		ReviewState:       "approved",
		ApprovedHash:      "v2",
		ContentHash:       "v2",
		WeChatState:       "confirmed",
		WeChatHash:        "v1",
		AvailableChannels: []string{"wechat"},
	})
	if got.Bucket != "latest_ready" || got.NextAction != "view" || got.WeChatLabel != "已确认草稿" {
		t.Fatalf("fully handled ready workflow = %+v", got)
	}
}

func TestDeriveArticleWorkflowPrioritizesFailureChangedAndReview(t *testing.T) {
	tests := []struct {
		name, reviewState, approvedHash, hugoState, wantState, wantBucket, wantAction string
	}{
		{name: "failed", reviewState: "blocked", hugoState: "published", wantState: "blocked", wantBucket: "failed", wantAction: "retry"},
		{name: "changed", reviewState: "approved", approvedHash: "old", hugoState: "published", wantState: "changed", wantBucket: "changed", wantAction: "review"},
		{name: "review", reviewState: "pending_review", wantState: "pending_review", wantBucket: "needs_review", wantAction: "review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveArticleWorkflow(articleWorkflowInput{
				ContentStage: article.ContentStageReady,
				ReviewState:  tt.reviewState,
				ApprovedHash: tt.approvedHash,
				ContentHash:  "current",
				HugoState:    tt.hugoState,
			})
			if got.State != tt.wantState || got.Bucket != tt.wantBucket || got.NextAction != tt.wantAction {
				t.Fatalf("workflow = %+v, want state=%q bucket=%q action=%q", got, tt.wantState, tt.wantBucket, tt.wantAction)
			}
		})
	}
}
