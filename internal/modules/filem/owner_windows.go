//go:build windows

package filem

import "os"

// ownerOf Windows 下不展示属主。
func ownerOf(info os.FileInfo) string { return "" }
