package httptransport

import (
	"testing"

	"github.com/gkmz/InkHub/internal/provider/publish/hugo"
)

func TestMatchTakeoverBundleKeepsOutOfScopeSourceUnmatched(t *testing.T) {
	bundle := hugo.TakeoverBundle{
		BundlePath: "posts/essay/history",
		Title:      "范围外历史文章",
		SourceID:   "article_OUT_OF_SCOPE",
	}

	candidate := matchTakeoverBundle(bundle, []takeoverArticle{{
		ID:       "article-row-1",
		StableID: "article_MANAGED",
		Path:     "managed/article.md",
		Title:    "范围内文章",
	}})

	if candidate.Status != "unmatched" {
		t.Fatalf("范围外 Hugo Bundle 状态 = %q，期望 unmatched", candidate.Status)
	}
	if candidate.MatchReason != "Hugo source_id 未在当前内容库中找到，保持原样" {
		t.Fatalf("范围外 Hugo Bundle 原因 = %q", candidate.MatchReason)
	}
}

func TestMatchTakeoverBundleStillMatchesManagedSourceID(t *testing.T) {
	bundle := hugo.TakeoverBundle{
		BundlePath: "posts/managed",
		SourceID:   "article_MANAGED",
	}

	candidate := matchTakeoverBundle(bundle, []takeoverArticle{{
		ID:       "article-row-1",
		StableID: "article_MANAGED",
		Path:     "managed/article.md",
		Title:    "范围内文章",
	}})

	if candidate.Status != "matched" || candidate.MatchReason != "source_id" {
		t.Fatalf("范围内 Hugo Bundle 匹配结果 = status %q, reason %q", candidate.Status, candidate.MatchReason)
	}
}

func TestResolveDuplicateTakeoverMatchesStillBlocksAmbiguousBundles(t *testing.T) {
	candidates := []hugoTakeoverCandidate{
		{ArticleID: "article-row-1", BundlePath: "posts/first", Status: "matched", MatchReason: "url"},
		{ArticleID: "article-row-1", BundlePath: "posts/second", Status: "matched", MatchReason: "path_title_date"},
	}

	resolveDuplicateTakeoverMatches(candidates)

	for _, candidate := range candidates {
		if candidate.Status != "conflict" {
			t.Fatalf("歧义 Bundle %s 状态 = %q，期望 conflict", candidate.BundlePath, candidate.Status)
		}
	}
}
