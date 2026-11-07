// Package execx 统一命令执行层：调用点固定二进制与参数（即白名单）、超时控制、审计日志。
// 各模块禁止直接使用 os/exec。
package execx

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"time"
)

const maxOutput = 64 * 1024

func truncate(s string) string {
	if len(s) > maxOutput {
		return s[:maxOutput] + "\n...[输出已截断]"
	}
	return s
}

// RunContext 执行单个命令，返回合并输出、退出码与错误；ctx 控制超时/取消。
func RunContext(ctx context.Context, caller string, name string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	out := truncate(buf.String())
	cmdline := name + " " + fmt.Sprint(args)

	switch {
	case ctx.Err() != nil:
		slog.Warn("命令被终止/超时", "caller", caller, "cmd", cmdline, "reason", ctx.Err())
		return out, code, ctx.Err()
	case err != nil:
		slog.Warn("命令执行失败", "caller", caller, "cmd", cmdline, "exit", code, "output", out)
	default:
		slog.Info("命令执行", "caller", caller, "cmd", cmdline)
	}
	return out, code, err
}

// Run 带超时的命令执行。
func Run(caller string, timeout time.Duration, name string, args ...string) (string, int, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return RunContext(ctx, caller, name, args...)
}

// RunShellContext 以 shell 执行脚本（仅限计划任务等受控内部调用方）。
func RunShellContext(ctx context.Context, caller, script string) (string, int, error) {
	if runtime.GOOS == "windows" {
		return RunContext(ctx, caller, "cmd", "/c", script)
	}
	return RunContext(ctx, caller, "/bin/bash", "-c", script)
}

// RunShell 带超时的 shell 执行。
func RunShell(caller string, timeout time.Duration, script string) (string, int, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return RunShellContext(ctx, caller, script)
}
