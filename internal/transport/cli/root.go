// Package cli 提供调用 Application 用例的命令行 Adapter。
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

// InitOptions 描述工作区初始化参数。
type InitOptions struct {
	VaultPath string
	HugoPath  string
}

// Diagnostic 是 doctor 命令的脱敏检查结果。
type Diagnostic struct {
	Name    string
	OK      bool
	Message string
}

// Commands 是 CLI 与 Application 之间的用例端口。
type Commands interface {
	Initialize(ctx context.Context, options InitOptions) error
	Doctor(ctx context.Context) ([]Diagnostic, error)
	Scan(ctx context.Context, workspace string) (jobID string, err error)
	Backup(ctx context.Context, destination string) (path string, err error)
	InitTemplate(ctx context.Context, destination string) error
	ValidateTemplate(ctx context.Context, path string) error
}

// Run 解析 CLI 命令并调用单个 Application 用例。
func Run(ctx context.Context, args []string, commands Commands, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}
	values := args
	if len(values) > 0 && strings.HasSuffix(values[0], "inkhub") {
		values = values[1:]
	}
	if len(values) == 0 || values[0] == "--help" || values[0] == "-h" || values[0] == "help" {
		printHelp(output)
		return nil
	}
	if commands == nil {
		return fmt.Errorf("CLI Application 用例尚未装配")
	}
	switch values[0] {
	case "init":
		flags := commandFlags("init")
		vault := flags.String("vault", "", "Obsidian Vault 路径")
		hugo := flags.String("hugo", "", "Hugo 根目录")
		if err := flags.Parse(values[1:]); err != nil || *vault == "" {
			return fmt.Errorf("init 参数无效")
		}
		if err := commands.Initialize(ctx, InitOptions{VaultPath: *vault, HugoPath: *hugo}); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(output, "工作区初始化完成")
	case "doctor":
		diagnostics, err := commands.Doctor(ctx)
		if err != nil {
			return err
		}
		for _, item := range diagnostics {
			state := "OK"
			if !item.OK {
				state = "FAIL"
			}
			_, _ = fmt.Fprintf(output, "%s  %s  %s\n", state, item.Name, item.Message)
		}
	case "scan":
		flags := commandFlags("scan")
		workspace := flags.String("workspace", "", "工作区 ID")
		if err := flags.Parse(values[1:]); err != nil || *workspace == "" {
			return fmt.Errorf("scan 参数无效")
		}
		jobID, err := commands.Scan(ctx, *workspace)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(output, "扫描任务已创建: %s\n", jobID)
	case "db":
		if len(values) < 2 || values[1] != "backup" {
			return fmt.Errorf("未知 db 子命令")
		}
		flags := commandFlags("db backup")
		destination := flags.String("output", "", "备份目标路径")
		if err := flags.Parse(values[2:]); err != nil {
			return fmt.Errorf("db backup 参数无效")
		}
		path, err := commands.Backup(ctx, *destination)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(output, "数据库备份完成: %s\n", path)
	case "template":
		if len(values) < 2 {
			return fmt.Errorf("缺少 template 子命令")
		}
		switch values[1] {
		case "init":
			flags := commandFlags("template init")
			destination := flags.String("output", "", "模板目录")
			if err := flags.Parse(values[2:]); err != nil || *destination == "" {
				return fmt.Errorf("template init 参数无效")
			}
			if err := commands.InitTemplate(ctx, *destination); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(output, "模板骨架已创建")
		case "validate":
			flags := commandFlags("template validate")
			path := flags.String("path", "", "模板目录或包")
			if err := flags.Parse(values[2:]); err != nil || *path == "" {
				return fmt.Errorf("template validate 参数无效")
			}
			if err := commands.ValidateTemplate(ctx, *path); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(output, "模板校验通过")
		default:
			return fmt.Errorf("未知 template 子命令: %s", values[1])
		}
	default:
		return fmt.Errorf("未知命令: %s", values[0])
	}
	return nil
}

func commandFlags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func printHelp(output io.Writer) {
	_, _ = fmt.Fprintln(output, `InkHub 本地内容工作台

命令:
  init               初始化工作区
  doctor             检查本机依赖与配置
  scan               扫描内容源并返回任务 ID
  db backup          创建 SQLite 备份
  template init      创建模板骨架
  template validate  校验模板安全与兼容性`)
}
