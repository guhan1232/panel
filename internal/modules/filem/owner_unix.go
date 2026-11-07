//go:build linux || darwin

package filem

import (
	"fmt"
	"os"
	"os/user"
	"syscall"
)

// ownerOf 返回文件属主用户名（Linux/macOS）。
func ownerOf(info os.FileInfo) string {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if u, err := user.LookupId(fmt.Sprint(st.Uid)); err == nil {
			return u.Username
		}
		return fmt.Sprint(st.Uid)
	}
	return ""
}
