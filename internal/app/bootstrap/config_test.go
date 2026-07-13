package bootstrap

import "testing"

func TestMergeConfigUsesDocumentedPrecedenceAndTracksOrigin(t *testing.T) {
	t.Parallel()

	got := MergeConfig(
		Layer{Name: "默认值", Values: Config{Host: "127.0.0.1", Port: 8080}},
		Layer{Name: "SQLite", Values: Config{Port: 8081}, Set: Fields("port")},
		Layer{Name: "工作区", Values: Config{Port: 8082}, Set: Fields("port")},
		Layer{Name: "环境变量", Values: Config{Port: 8083}, Set: Fields("port")},
		Layer{Name: "CLI", Values: Config{Port: 8084}, Set: Fields("port")},
	)
	if got.Config.Port != 8084 {
		t.Fatalf("Port = %d, want 8084", got.Config.Port)
	}
	if got.Origins["port"] != "CLI" {
		t.Fatalf("port origin = %q, want CLI", got.Origins["port"])
	}
	if got.Config.Host != "127.0.0.1" {
		t.Fatalf("Host = %q", got.Config.Host)
	}
}
