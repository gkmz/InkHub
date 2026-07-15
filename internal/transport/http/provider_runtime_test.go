package httptransport

import (
	"testing"

	"github.com/gkmz/InkHub/internal/provider/registry"
	"github.com/gkmz/InkHub/internal/provider/source/obsidian"
	taxonomyhugo "github.com/gkmz/InkHub/internal/provider/taxonomy/hugo"
)

func testProviderRuntime(t *testing.T) *registry.Registry {
	t.Helper()
	runtime := registry.New(nil)
	if err := runtime.RegisterSource(obsidian.NewFactory()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.RegisterTaxonomy(taxonomyhugo.NewFactory()); err != nil {
		t.Fatal(err)
	}
	return runtime
}
