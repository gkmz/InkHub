// Package job 定义持久化后台任务状态。
package job

import "fmt"

// State 是后台任务状态。
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

var transitions = map[State]map[State]bool{
	StateQueued:  {StateRunning: true, StateCancelled: true},
	StateRunning: {StateQueued: true, StateSucceeded: true, StateFailed: true, StateCancelled: true},
	StateFailed:  {StateQueued: true},
}

// Transition 校验一次任务状态转换是否合法。
func Transition(from, to State) error {
	if from == to {
		return nil
	}
	if !transitions[from][to] {
		return fmt.Errorf("不允许任务状态从 %q 转换到 %q", from, to)
	}
	return nil
}

// Job 是持久化后台任务的领域模型。
type Job struct {
	ID          string
	WorkspaceID string
	Kind        string
	DedupeKey   string
	State       State
	Progress    int
	Attempts    int
}
