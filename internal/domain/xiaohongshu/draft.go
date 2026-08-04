// Package xiaohongshu 定义小红书内容草稿和图片渲染的领域对象。
package xiaohongshu

// DraftState 是小红书草稿的生命周期状态。
type DraftState string

// BlockKind 表示小红书卡片中的内容块类型。
type BlockKind string

const (
	BlockKindParagraph BlockKind = "paragraph"
	BlockKindHeading   BlockKind = "heading"
	BlockKindImage     BlockKind = "image"
	BlockKindCode      BlockKind = "code"
	BlockKindTable     BlockKind = "table"
	BlockKindText      BlockKind = "text"
)

// Block 是页面中的可编辑内容块。
type Block struct {
	ID         string    `json:"id"`
	Kind       BlockKind `json:"kind"`
	HTML       string    `json:"html"`
	Splittable bool      `json:"splittable"`
}

// Page 是一张独立的小红书发布卡片。
type Page struct {
	ID             string  `json:"id"`
	Blocks         []Block `json:"blocks"`
	MeasuredHeight int     `json:"measured_height"`
}

const (
	// DraftStateDraft 表示草稿可编辑，尚未人工确认发布。
	DraftStateDraft DraftState = "draft"
	// DraftStatePublished 表示用户已确认图片已经手动发布。
	DraftStatePublished DraftState = "published"
	// DraftStateStale 表示文章源内容已变化，草稿仅保留用于审计。
	DraftStateStale DraftState = "stale"
)

// Draft 是一次完整的小红书内容草稿版本。
type Draft struct {
	ID                string
	WorkspaceID       string
	ArticleID         string
	SourceContentHash string
	Title             string
	BodyHTML          string
	Pages             []Page
	Topics            []string
	SourceNote        string
	CommentCopy       string
	AIModel           string
	PromptVersion     string
	State             DraftState
	CreatedAt         string
	UpdatedAt         string
}

// Render 是基于草稿和手机模板生成的一次图片渲染版本。
type Render struct {
	ID              string
	DraftID         string
	ArticleID       string
	TemplateID      string
	TemplateVersion string
	ViewportWidth   int
	PageHeight      int
	HTMLHash        string
	PageCount       int
	State           string
	CreatedAt       string
	UpdatedAt       string
}

// Event 是小红书工作流的追加审计事件。
type Event struct {
	ID        string
	DraftID   string
	RenderID  string
	EventType string
	Payload   string
	CreatedAt string
}
