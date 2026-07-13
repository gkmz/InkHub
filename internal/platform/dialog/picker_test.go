package dialog

import (
	"context"
	"testing"
)

func TestNativePickerHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (NativePicker{}).Pick(ctx, "选择目录"); err == nil {
		t.Fatal("Pick() must fail for a cancelled context")
	}
}
