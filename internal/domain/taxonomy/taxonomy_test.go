package taxonomy

import "testing"

func TestNormalizeTagsUsesStableNamesAndKeepsNewTerms(t *testing.T) {
	t.Parallel()

	got := NormalizeTags([]string{" go ", "GO", "", "InkHub 2"}, map[string]string{"go": "Go"})
	if len(got) != 2 || got[0] != "go" || got[1] != "inkhub-2" {
		t.Fatalf("NormalizeTags() = %#v, want [go inkhub-2]", got)
	}
}

func TestNormalizeTagHandlesEnglishChineseAndSeparators(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"Coding Agent": "coding-agent",
		"Superpowers":  "superpowers",
		"AI 编程":        "ai-编程",
		"开发效率":         "开发效率",
		"go__agent":    "go-agent",
	}
	for input, expected := range cases {
		if got := NormalizeTag(input); got != expected {
			t.Errorf("NormalizeTag(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeTagsStrictRejectsTagWithoutUsableCharacters(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeTagsStrict([]string{"---"}, nil); err == nil {
		t.Fatal("NormalizeTagsStrict() must reject an unusable tag")
	}
}

func TestValidateTagsAllowsNewTermsAndLimitsCount(t *testing.T) {
	t.Parallel()

	got, err := ValidateTags([]string{"GoLang", "SQLite", "New Topic"}, Rules{
		Aliases: map[string]string{"golang": "Go"}, MinTags: 3, MaxTags: 6,
	})
	if err != nil || len(got) != 3 || got[0] != "go" || got[2] != "new-topic" {
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
