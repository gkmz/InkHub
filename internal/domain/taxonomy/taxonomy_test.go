package taxonomy

import "testing"

func TestNormalizeTagsUsesCanonicalNamesAndKeepsNewTerms(t *testing.T) {
	t.Parallel()

	got := NormalizeTags([]string{" go ", "GO", "", "InkHub 2"}, map[string]string{"go": "Go"})
	if len(got) != 2 || got[0] != "Go" || got[1] != "InkHub 2" {
		t.Fatalf("NormalizeTags() = %#v, want [Go InkHub 2]", got)
	}
}

func TestValidateTagsAllowsNewTermsAndLimitsCount(t *testing.T) {
	t.Parallel()

	got, err := ValidateTags([]string{"GoLang", "SQLite", "New Topic"}, Rules{
		Aliases: map[string]string{"golang": "Go"}, MinTags: 3, MaxTags: 6,
	})
	if err != nil || len(got) != 3 || got[0] != "Go" || got[2] != "New Topic" {
		t.Fatalf("ValidateTags() = %#v, %v", got, err)
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
