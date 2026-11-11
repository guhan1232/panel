// 站点数据备份/恢复：root 目录 + nginx conf 打包 tar.gz 存于 data/backups/sites/，
// 支持手动备份、下载（导出）、上传导入恢复、删除；恢复前自动留存安全备份。
package website

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
)

// siteBackupDir 站点备份存储目录。
func siteBackupDir(dataDir string) string { return filepath.Join(dataDir, "backups", "sites") }

// siteBackup 手动备份：root 目录 + conf 打包 tar.gz。
func (m *Module) siteBackup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var s Site
	if err := m.db.First(&s, id).Error; err != nil {
		core.Err(c, 404, "站点不存在")
		return
	}
	if s.Root == "" || s.Root == "/" {
		core.Err(c, 400, "反代站点无根目录文件可备（配置请走导出）")
		return
	}
	if _, err := os.Stat(s.Root); err != nil {
		core.Err(c, 400, "根目录不存在: "+s.Root)
		return
	}
	_ = os.MkdirAll(siteBackupDir(m.dataDir), 0o700)
	name := s.Name + "-" + time.Now().Format("20060102-150405") + ".tar.gz"
	out := filepath.Join(siteBackupDir(m.dataDir), name)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()
	paths := []string{s.Root}
	if cp := confPath(s.Name, true); cp != "" {
		if _, e := os.Stat(cp); e == nil {
			paths = append(paths, cp)
		}
	}
	if err := archive.TarGzAt(ctx, "/", paths, out); err != nil {
		_ = os.Remove(out)
		core.Err(c, 500, "打包失败: "+err.Error())
		return
	}
	info, _ := os.Stat(out)
	slog.Info("站点备份完成", "site", s.Name, "file", name, "size", info.Size())
	core.OK(c, gin.H{"file": name, "size": info.Size()})
}

// siteBackupList 备份列表（全部站点合并）。
func (m *Module) siteBackupList(c *gin.Context) {
	dir := siteBackupDir(m.dataDir)
	items := []gin.H{}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
				continue
			}
			info, _ := e.Info()
			base := strings.TrimSuffix(e.Name(), ".tar.gz")
			site := base
			if len(base) > 15 && base[len(base)-15] == '-' {
				site = base[:len(base)-15]
			}
			items = append(items, gin.H{
				"file": e.Name(),
				"size": info.Size(),
				"time": info.ModTime().Format("2006-01-02 15:04:05"),
				"site": site,
			})
		}
	}
	core.OK(c, items)
}

// siteBackupDownload 下载备份包（导出）。
func (m *Module) siteBackupDownload(c *gin.Context) {
	file := c.Query("file")
	if !isSafeBackupName(file) {
		core.Err(c, 400, "文件名非法")
		return
	}
	path := filepath.Join(siteBackupDir(m.dataDir), file)
	if _, err := os.Stat(path); err != nil {
		core.Err(c, 404, "备份不存在")
		return
	}
	c.FileAttachment(path, file)
}

// siteBackupRestore 恢复：上传 tar.gz 导入或恢复已存备份；恢复前自动留存安全备份。
func (m *Module) siteBackupRestore(c *gin.Context) {
	id, _ := strconv.Atoi(c.PostForm("id"))
	var s Site
	if err := m.db.First(&s, id).Error; err != nil {
		core.Err(c, 404, "站点不存在")
		return
	}
	if s.Root == "" || s.Root == "/" {
		core.Err(c, 400, "该站点无根目录，不支持数据恢复")
		return
	}
	var srcPath string
	f, _ := c.FormFile("file")
	if f != nil {
		srcPath = filepath.Join(siteBackupDir(m.dataDir), "upload-"+time.Now().Format("20060102-150405")+".tar.gz")
		if err := c.SaveUploadedFile(f, srcPath); err != nil {
			core.Err(c, 500, "保存上传失败: "+err.Error())
			return
		}
	} else {
		file := c.PostForm("file")
		if !isSafeBackupName(file) {
			core.Err(c, 400, "文件名非法")
			return
		}
		srcPath = filepath.Join(siteBackupDir(m.dataDir), file)
	}
	if _, err := os.Stat(srcPath); err != nil {
		core.Err(c, 404, "备份包不存在")
		return
	}
	// 恢复前自动留存当前数据（防误恢复）
	_ = os.MkdirAll(siteBackupDir(m.dataDir), 0o700)
	preBak := filepath.Join(siteBackupDir(m.dataDir), s.Name+"-pre-restore-"+time.Now().Format("20060102-150405")+".tar.gz")
	if _, err := os.Stat(s.Root); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		_ = archive.TarGzAt(ctx, "/", []string{s.Root}, preBak)
		cancel()
	}
	tmpDir, err := os.MkdirTemp("", "site-restore-*")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)
	if err := archive.UnTarGz(srcPath, tmpDir); err != nil {
		core.Err(c, 400, "备份包解析失败: "+err.Error())
		return
	}
	relRoot := strings.TrimPrefix(s.Root, "/")
	restored := filepath.Join(tmpDir, relRoot)
	if _, err := os.Stat(restored); err != nil {
		core.Err(c, 400, "备份包内未找到站点目录 "+relRoot)
		return
	}
	_ = os.RemoveAll(s.Root)
	if err := os.MkdirAll(filepath.Dir(s.Root), 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if err := os.Rename(restored, s.Root); err != nil {
		if err := copyDir(restored, s.Root); err != nil {
			core.Err(c, 500, "恢复目录失败: "+err.Error())
			return
		}
	}
	confFile := filepath.Join(tmpDir, strings.TrimPrefix(confPath(s.Name, true), "/"))
	if _, err := os.Stat(confFile); err == nil {
		data, _ := os.ReadFile(confFile)
		if err := writeConfWithCheck(confPath(s.Name, true), string(data)); err != nil {
			core.Err(c, 400, "conf 恢复失败(已回滚): "+err.Error())
			return
		}
		_, _ = reloadNginx()
	}
	slog.Info("站点数据已恢复", "site", s.Name, "root", s.Root)
	core.OK(c, gin.H{"ok": true, "message": "恢复完成（恢复前数据已留存 " + filepath.Base(preBak) + "）"})
}

// siteBackupDelete 删除备份文件。
func (m *Module) siteBackupDelete(c *gin.Context) {
	file := c.Query("file")
	if !isSafeBackupName(file) {
		core.Err(c, 400, "文件名非法")
		return
	}
	_ = os.Remove(filepath.Join(siteBackupDir(m.dataDir), file))
	core.OK(c, nil)
}

// isSafeBackupName 防路径穿越。
func isSafeBackupName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") || strings.HasPrefix(name, ".") {
		return false
	}
	return filepath.Base(name) == name
}

// copyDir 递归拷贝目录。
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
