//go:build windows

package platform

import (
	"errors"
	"os"
)

// Terminal Windows 占位实现：面板目标平台为 Linux，本地 Windows 开发时保证可编译。
type Terminal struct{}

func StartTerminal(cols, rows uint16) (*Terminal, error) {
	return nil, errors.New("Web 终端仅支持 Linux/macOS 部署环境")
}

func (t *Terminal) Write(p []byte) (int, error) { return 0, os.ErrInvalid }
func (t *Terminal) Read(p []byte) (int, error)  { return 0, os.ErrInvalid }
func (t *Terminal) Resize(cols, rows uint16) error {
	return os.ErrInvalid
}
func (t *Terminal) Close() {}
