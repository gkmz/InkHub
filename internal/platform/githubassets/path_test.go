package githubassets

import (
	"strings"
	"testing"
)

func TestAssetPathAndRawURLAreDeterministic(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("a", 64)
	config := Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "inkhub/articles", Token: "secret"}
	path, err := AssetPath(config.Prefix, digest, ".png")
	if err != nil {
		t.Fatalf("生成资源路径: %v", err)
	}
	if path != "inkhub/articles/aa/"+digest+".png" {
		t.Fatalf("资源路径=%q", path)
	}
	value, err := RawURL(config, path)
	if err != nil {
		t.Fatalf("生成 Raw URL: %v", err)
	}
	if value != "https://raw.githubusercontent.com/gkmz/images/main/inkhub/articles/aa/"+digest+".png" {
		t.Fatalf("Raw URL=%q", value)
	}
}

func TestConfigRejectsPathAndURLInjection(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("b", 64)
	for _, test := range []struct {
		name   string
		config Config
	}{
		{name: "private api host in owner", config: Config{Owner: "evil.com/x", Repository: "images", Branch: "main", Prefix: "inkhub", Token: "secret"}},
		{name: "parent prefix", config: Config{Owner: "gkmz", Repository: "images", Branch: "main", Prefix: "../secret", Token: "secret"}},
		{name: "backslash branch", config: Config{Owner: "gkmz", Repository: "images", Branch: `main\evil`, Prefix: "inkhub", Token: "secret"}},
		{name: "control character", config: Config{Owner: "gkmz", Repository: "images", Branch: "main\nnext", Prefix: "inkhub", Token: "secret"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.config, nil, nil); err == nil {
				t.Fatal("非法配置应被拒绝")
			}
		})
	}
	if _, err := AssetPath("inkhub", digest, ".svg"); err == nil {
		t.Fatal("非图片规范扩展应被拒绝")
	}
}
