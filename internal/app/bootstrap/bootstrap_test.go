package bootstrap

import (
	"context"
	"testing"
)

func TestRunVersionDoesNotOpenWorkspace(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), []string{"inkhub", "--version"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunAcceptsEmptyArguments(t *testing.T) {
	t.Parallel()

	if err := Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}
