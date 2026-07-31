package article

import "testing"

func TestResolveContentStageDefaultsToDraft(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		present   bool
		scalar    bool
		wantStage ContentStage
		wantIssue string
	}{
		{name: "missing", wantStage: ContentStageDraft},
		{name: "draft", value: "draft", present: true, scalar: true, wantStage: ContentStageDraft},
		{name: "ready", value: "ready", present: true, scalar: true, wantStage: ContentStageReady},
		{name: "empty", present: true, scalar: true, wantStage: ContentStageDraft, wantIssue: "publish.status 仅支持 draft 或 ready"},
		{name: "unknown", value: "published", present: true, scalar: true, wantStage: ContentStageDraft, wantIssue: "publish.status 仅支持 draft 或 ready"},
		{name: "non-scalar", present: true, wantStage: ContentStageDraft, wantIssue: "publish.status 必须是字符串"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStage, gotIssue := ResolveContentStage(tt.value, tt.present, tt.scalar)
			if gotStage != tt.wantStage || gotIssue != tt.wantIssue {
				t.Fatalf("ResolveContentStage() = %q, %q; want %q, %q", gotStage, gotIssue, tt.wantStage, tt.wantIssue)
			}
		})
	}
}
