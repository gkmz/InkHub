package taxonomy

import "testing"

func TestValidateTagsNormalizesAliasesAndLimitsCount(t *testing.T) {
	t.Parallel()

	got, err := ValidateTags([]string{"GoLang", "SQLite", "AI"}, Rules{
		Aliases: map[string]string{"golang": "go"},
		Allowed: map[string]bool{"go": true, "sqlite": true, "ai": true},
		MinTags: 3,
		MaxTags: 6,
	})
	if err != nil {
		t.Fatalf("ValidateTags() error = %v", err)
	}
	if got[0] != "go" {
		t.Fatalf("ValidateTags()[0] = %q, want go", got[0])
	}

	if _, err := ValidateTags([]string{"go"}, Rules{MinTags: 3, MaxTags: 6}); err == nil {
		t.Fatal("ValidateTags() must reject too few tags")
	}
}

func TestValidateTagsChecksCountAfterNormalization(t *testing.T) {
	t.Parallel()

	_, err := ValidateTags([]string{"Go", "go", "GoLang"}, Rules{
		Aliases: map[string]string{"golang": "go"},
		MinTags: 3,
		MaxTags: 6,
	})
	if err == nil {
		t.Fatal("ValidateTags() must reject too few unique normalized tags")
	}
}
