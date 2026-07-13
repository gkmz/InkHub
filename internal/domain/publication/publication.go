// Package publication 定义渠道处理状态和过期判定。
package publication

// State 是渠道处理记录的状态。
type State string

const (
	StateNever     State = "never"
	StatePrepared  State = "prepared"
	StateCopied    State = "copied"
	StateConfirmed State = "confirmed"
	StatePublished State = "published"
	StateFailed    State = "failed"
	StateOutdated  State = "outdated"
)

// DisplayState 根据当前文章版本派生用户可见状态。
func DisplayState(stored State, currentHash, processedHash string) State {
	if stored == StateFailed {
		return StateFailed
	}
	if stored != StateNever && currentHash != processedHash {
		return StateOutdated
	}
	return stored
}
