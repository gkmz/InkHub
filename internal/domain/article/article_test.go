package article

import "testing"

func TestStableIDValidation(t *testing.T) {
	t.Parallel()

	if err := StableID("article_01J2ABCDEF").Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := StableID("../bad").Validate(); err == nil {
		t.Fatal("Validate() must reject invalid stable IDs")
	}
}
