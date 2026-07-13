package publication

import "testing"

func TestDisplayStateDerivesOutdated(t *testing.T) {
	t.Parallel()

	got := DisplayState(StatePublished, "current", "published")
	if got != StateOutdated {
		t.Fatalf("DisplayState() = %q, want %q", got, StateOutdated)
	}

	got = DisplayState(StateFailed, "current", "published")
	if got != StateFailed {
		t.Fatalf("failed state must take precedence, got %q", got)
	}
}
