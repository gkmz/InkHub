package article

// ContentStage 描述作者控制的文章内容阶段。
type ContentStage string

const (
	// ContentStageDraft 表示文章仍在创作，不进入审核和发布流程。
	ContentStageDraft ContentStage = "draft"
	// ContentStageReady 表示作者确认文章可以进入审核和发布流程。
	ContentStageReady ContentStage = "ready"
)

// ResolveContentStage 将 frontmatter 中的 publish.status 解析为安全的内容阶段。
// 缺少字段默认是草稿；非法值也按草稿处理，但返回可展示的修复提示。
func ResolveContentStage(value string, present, scalar bool) (ContentStage, string) {
	if !present {
		return ContentStageDraft, ""
	}
	if !scalar {
		return ContentStageDraft, "publish.status 必须是字符串"
	}
	switch value {
	case string(ContentStageDraft):
		return ContentStageDraft, ""
	case string(ContentStageReady):
		return ContentStageReady, ""
	default:
		return ContentStageDraft, "publish.status 仅支持 draft 或 ready"
	}
}
