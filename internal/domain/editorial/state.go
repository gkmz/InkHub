// Package editorial 定义文章审核状态和合法转换。
package editorial

import "fmt"

// State 是文章当前审核状态。
type State string

const (
	StateDraft         State = "draft"
	StateIncomplete    State = "incomplete"
	StatePendingReview State = "pending_review"
	StateApproved      State = "approved"
	StateChanged       State = "changed"
	StateBlocked       State = "blocked"
)

var allowedTransitions = map[State]map[State]bool{
	StateDraft:         {StateIncomplete: true, StatePendingReview: true, StateBlocked: true},
	StateIncomplete:    {StatePendingReview: true, StateBlocked: true},
	StatePendingReview: {StateApproved: true, StateIncomplete: true, StateBlocked: true},
	StateApproved:      {StateChanged: true, StateBlocked: true},
	StateChanged:       {StatePendingReview: true, StateIncomplete: true, StateBlocked: true},
	StateBlocked:       {StateIncomplete: true},
}

// Transition 校验一次审核状态转换是否合法。
func Transition(from, to State) error {
	if from == to {
		return nil
	}
	if !allowedTransitions[from][to] {
		return fmt.Errorf("不允许审核状态从 %q 转换到 %q", from, to)
	}
	return nil
}
