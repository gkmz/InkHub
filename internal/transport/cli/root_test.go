package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpListsAllMVPCommands(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Run(context.Background(), []string{"inkhub", "--help"}, nil, &output); err != nil {
		t.Fatalf("帮助命令: %v", err)
	}
	for _, command := range []string{"init", "doctor", "scan", "db backup", "template init", "template validate"} {
		if !strings.Contains(output.String(), command) {
			t.Fatalf("帮助缺少 %q:\n%s", command, output.String())
		}
	}
}

func TestScanCallsApplicationCommand(t *testing.T) {
	t.Parallel()

	commands := &fakeCommands{}
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"inkhub", "scan", "--workspace", "w1"}, commands, &output); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if commands.scanWorkspace != "w1" || !strings.Contains(output.String(), "扫描任务") {
		t.Fatalf("scan 未调用 Application: commands=%+v output=%s", commands, output.String())
	}
}

type fakeCommands struct{ scanWorkspace string }

func (*fakeCommands) Initialize(context.Context, InitOptions) error { return nil }
func (*fakeCommands) Doctor(context.Context) ([]Diagnostic, error) {
	return []Diagnostic{{Name: "database", OK: true}}, nil
}
func (f *fakeCommands) Scan(_ context.Context, workspace string) (string, error) {
	f.scanWorkspace = workspace
	return "job_scan", nil
}
func (*fakeCommands) Backup(context.Context, string) (string, error) { return "/backup.db", nil }
func (*fakeCommands) InitTemplate(context.Context, string) error     { return nil }
func (*fakeCommands) ValidateTemplate(context.Context, string) error { return nil }
