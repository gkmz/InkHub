package disposition

import (
	"testing"
	"time"
)

func TestRecordEffectiveUsesVersionOnlyForPublished(t *testing.T) {
	t.Parallel()
	clearedAt := time.Now().UTC()
	tests := []struct {
		name    string
		record  Record
		current string
		want    bool
	}{
		{name: "当前版本已发表", record: Record{Kind: KindPublished, ContentHash: "v1"}, current: "v1", want: true},
		{name: "已发表版本过期", record: Record{Kind: KindPublished, ContentHash: "v1"}, current: "v2", want: false},
		{name: "忽略跨版本有效", record: Record{Kind: KindIgnored, ContentHash: "v1"}, current: "v2", want: true},
		{name: "已恢复不再有效", record: Record{Kind: KindIgnored, ClearedAt: &clearedAt}, current: "v1", want: false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.record.Effective(test.current); got != test.want {
				t.Fatalf("Effective() = %v, want %v", got, test.want)
			}
		})
	}
}
