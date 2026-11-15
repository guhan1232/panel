//go:build linux || darwin

// Package platform 封装平台相关能力（PTY 终端），Windows 下提供占位实现以便本地编译。
package platform

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// Terminal 一个 PTY 终端会话。
type Terminal struct {
	cmd *exec.Cmd
	tty *os.File
}

// StartTerminal 启动登录 shell。
func StartTerminal(cols, rows uint16) (*Terminal, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell, "-l")
	tty, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	_ = pty.Setsize(tty, &pty.Winsize{Rows: rows, Cols: cols})
	return &Terminal{cmd: cmd, tty: tty}, nil
}

func (t *Terminal) Write(p []byte) (int, error) { return t.tty.Write(p) }
func (t *Terminal) Read(p []byte) (int, error)  { return t.tty.Read(p) }

func (t *Terminal) Resize(cols, rows uint16) error {
	return pty.Setsize(t.tty, &pty.Winsize{Rows: rows, Cols: cols})
}

func (t *Terminal) Close() {
	if t.tty != nil {
		_ = t.tty.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}
}
