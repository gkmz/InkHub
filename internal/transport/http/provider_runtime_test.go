package httptransport

import (
	"testing"

	"github.com/gkmz/InkHub/internal/provider/registry"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
)

func testSourceRuntime(t *testing.T) *registry.Registry {
	t.Helper()
	runtime := registry.New(nil)
	if err := runtime.RegisterSource(obsidian.NewFactory()); err != nil {
		t.Fatal(err)
	}
	return runtime
}
