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
	Ignored    bool            `json:"ignored,omitempty"`
}

// Status 返回建议项当前可审计的处理状态。
func (item SuggestionItem) Status() string {
	if item.Accepted {
		return "accepted"
	}
	if item.Ignored {
		return "ignored"
	}
	return "pending"
}

// DeriveSuggestionState 根据建议项处理结果计算建议版本状态。
func DeriveSuggestionState(items []SuggestionItem) SuggestionState {
	if len(items) == 0 {
		return SuggestionPending
	}
	accepted, ignored := 0, 0
	for _, item := range items {
		if item.Accepted {
			accepted++
		}
		if item.Ignored {
			ignored++
		}
	}
	if accepted == len(items) {
		return SuggestionAccepted
	}
	if ignored == len(items) {
		return SuggestionRejected
	}
	if accepted+ignored > 0 {
		return SuggestionPartiallyAccepted
	}
	return SuggestionPending
}

// SuggestionSet 保存一次 AI 分析产生的可审计建议集合。
type SuggestionSet struct {
	ID                 string
	ArticleID          string
	WorkspaceID        string
	ProviderInstanceID string
	InputContentHash   string
	Model              string
	CreatedAt          string
	UpdatedAt          string
	Items              []SuggestionItem
	State              SuggestionState
}
