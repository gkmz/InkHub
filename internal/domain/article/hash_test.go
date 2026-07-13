package article

import "testing"

func TestNormalizeAndHashIsDeterministic(t *testing.T) {
	t.Parallel()

	base := HashInput{Body: "\ufeff第一行\r\n第二行\r\n", Title: "标题", Keywords: []string{"Go"}}
	want, err := NormalizeAndHash(base)
	if err != nil {
		t.Fatalf("NormalizeAndHash() error = %v", err)
	}

	base.Body = "第一行\n第二行\n"
	got, err := NormalizeAndHash(base)
	if err != nil {
		t.Fatalf("NormalizeAndHash() error = %v", err)
	}
	if got != want {
		t.Fatalf("NormalizeAndHash() = %q, want %q", got, want)
	}
}

func TestNormalizeAndHashIncludesKeywords(t *testing.T) {
	t.Parallel()

	first, _ := NormalizeAndHash(HashInput{Body: "正文", Keywords: []string{"go"}})
	second, _ := NormalizeAndHash(HashInput{Body: "正文", Keywords: []string{"sqlite"}})
	if first == second {
		t.Fatal("changing keywords must change the content hash")
	}
}

func TestNormalizeAndHashTreatsNilAndEmptyListsEqually(t *testing.T) {
	t.Parallel()

	withNil, _ := NormalizeAndHash(HashInput{Body: "正文"})
	withEmpty, _ := NormalizeAndHash(HashInput{Body: "正文", Tags: []string{}, Keywords: []string{}})
	if withNil != withEmpty {
		t.Fatal("nil and empty metadata lists must produce the same content hash")
	}
}
