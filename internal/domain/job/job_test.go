package job

import "testing"

func TestTransition(t *testing.T) {
	t.Parallel()

	if err := Transition(StateQueued, StateRunning); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if err := Transition(StateSucceeded, StateRunning); err == nil {
		t.Fatal("completed jobs must not return to running")
	}
}
