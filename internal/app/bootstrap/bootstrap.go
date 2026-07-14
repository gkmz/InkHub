// Package bootstrap 负责解析启动参数并装配 InkHub 应用。
package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gkmz/InkHub/internal/buildinfo"
	transportcli "github.com/gkmz/InkHub/internal/transport/cli"
)

// Run 解析命令行参数并运行 InkHub 应用。
func Run(ctx context.Context, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		return transportcli.Run(ctx, args, nil, os.Stdout)
	}

	flags := flag.NewFlagSet("inkhub", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	showVersion := flags.Bool("version", false, "显示版本")
	_ = flags.String("data-dir", "", "覆盖数据目录")
	_ = flags.String("workspace", "", "选择工作区")
	_ = flags.String("host", "127.0.0.1", "监听地址")
	_ = flags.Int("port", 8080, "监听端口")

	flagArgs := args
	if len(flagArgs) > 0 {
		flagArgs = flagArgs[1:]
	}
	if err := flags.Parse(flagArgs); err != nil {
		return fmt.Errorf("解析启动参数: %w", err)
	}
	if *showVersion {
		_, err := fmt.Fprintln(os.Stdout, buildinfo.Version)
		return err
	}

	return nil
}
