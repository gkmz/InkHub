package editorial

import "encoding/json"

// SuggestionState 是一组 AI 建议的处理状态。
type SuggestionState string

const (
	SuggestionPending           SuggestionState = "pending"
	SuggestionPartiallyAccepted SuggestionState = "partially_accepted"
	SuggestionAccepted          SuggestionState = "accepted"
	SuggestionRejected          SuggestionState = "rejected"
	SuggestionExpired           SuggestionState = "expired"
	SuggestionInvalid           SuggestionState = "invalid"
)

// SuggestionItem 是可被单独采纳的类型化字段建议。
type SuggestionItem struct {
	ID         string          `json:"id"`
	Field      string          `json:"field"`
	Value      json.RawMessage `json:"value"`
	Rationale  string          `json:"rationale,omitempty"`
	Confidence float64         `json:"confidence,omitempty"`
	NewTerm    bool            `json:"new_term,omitempty"`
	UsageCount int             `json:"usage_count,omitempty"`
	Accepted   bool            `json:"accepted,omitempty"`
}

// SuggestionSet 保存一次 AI 分析产生的可审计建议集合。
type SuggestionSet struct {
	ID                 string
	ArticleID          string
	WorkspaceID        string
	ProviderInstanceID string
	InputContentHash   string
	Model              string
	Items              []SuggestionItem
	State              SuggestionState
}
