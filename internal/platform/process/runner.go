// Package process 提供不经过 shell 的受控外部进程执行。
package process

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

const defaultMaxOutputBytes = 1024 * 1024

// Request 描述一次受控进程调用。
type Request struct {
	Executable     string
	Arguments      []string
	WorkingDir     string
	MaxOutputBytes int
}

// Result 保存进程的标准输出和标准错误。
type Result struct {
	Stdout string
	Stderr string
}

// Runner 使用参数数组执行外部进程。
type Runner struct{}

// Run 执行进程并限制输出大小。
func (Runner) Run(ctx context.Context, request Request) (Result, error) {
	if request.Executable == "" {
		return Result{}, fmt.Errorf("可执行文件不能为空")
	}
	limit := request.MaxOutputBytes
	if limit <= 0 {
		limit = defaultMaxOutputBytes
	}
	stdout := &limitedBuffer{remaining: limit}
	stderr := &limitedBuffer{remaining: limit}
	command := exec.CommandContext(ctx, request.Executable, request.Arguments...)
	command.Dir = request.WorkingDir
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if stdout.exceeded || stderr.exceeded {
		return result, fmt.Errorf("进程输出超过 %d 字节限制", limit)
	}
	if err != nil {
		return result, fmt.Errorf("执行 %s: %w", request.Executable, err)
	}
	return result, nil
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	remaining int
	exceeded  bool
}

// Write 实现 io.Writer，并在达到限制后继续报告原始消费长度。
func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	if len(data) > b.remaining {
		data = data[:b.remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(data)
	b.remaining -= len(data)
	return original, nil
}

// String 返回限制范围内已收集的输出。
func (b *limitedBuffer) String() string {
	return b.buffer.String()
}
