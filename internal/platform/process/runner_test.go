package process

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestRunnerPassesArgumentsWithoutShellExpansion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("printf fixture is Unix-specific")
	}

	result, err := (Runner{}).Run(context.Background(), Request{
		Executable: "printf",
		Arguments:  []string{"%s", "$(echo unsafe);`whoami`"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "$(echo unsafe);`whoami`" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
}
