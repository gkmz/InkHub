package editorial

import "testing"

func TestTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from State
		to   State
		ok   bool
	}{
		{name: "ready for review", from: StateIncomplete, to: StatePendingReview, ok: true},
		{name: "approve", from: StatePendingReview, to: StateApproved, ok: true},
		{name: "content changed", from: StateApproved, to: StateChanged, ok: true},
		{name: "cannot skip review", from: StateIncomplete, to: StateApproved, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Transition(tt.from, tt.to)
			if (err == nil) != tt.ok {
				t.Fatalf("Transition(%q, %q) error = %v", tt.from, tt.to, err)
			}
		})
	}
}
