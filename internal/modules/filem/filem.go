// Package filem 文件管理模块：浏览、上传下载、编辑、压缩解压、回收站删除。
package filem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/archive"
	"cnb.cool/hanyuxin/panel/internal/core"
)

const maxReadSize = 2 << 20 // 2MB

type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	Owner   string    `json:"owner"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mtime"`
}

type Module struct {
	dataDir string
}

func New(dataDir string) *Module {
	return &Module{dataDir: dataDir}
}

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/files")
	g.GET("/list", m.list)
	g.POST("/mkdir", m.mkdir)
	g.POST("/rename", m.rename)
	g.POST("/move", m.move)
	g.POST("/delete", m.delete)
	g.POST("/save", m.save)
	g.GET("/read", m.read)
	g.GET("/download", m.download)
	g.POST("/upload", m.upload)
	g.POST("/upload-chunk", m.uploadChunk)
	g.POST("/upload-finish", m.uploadFinish)
	g.POST("/chmod", m.chmod)
	g.POST("/compress", m.compress)
	g.POST("/decompress", m.decompress)
}

func clean(p string) string {
	if p == "" {
		return string(filepath.Separator)
	}
	return filepath.Clean(p)
}

// guard 拒绝根目录与空路径的删除/改名类操作。
func guard(p string) error {
	p = clean(p)
	if p == "" || p == string(filepath.Separator) {
		return fmt.Errorf("拒绝操作根目录")
	}
	return nil
}

func (m *Module) list(c *gin.Context) {
	dir := clean(c.Query("path"))
	info, err := os.Stat(dir)
	if err != nil {
		core.Err(c, 404, "路径不存在: "+err.Error())
		return
	}
	if !info.IsDir() {
		core.Err(c, 400, "不是目录")
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	items := make([]Entry, 0, len(ents))
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, Entry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			Size:    fi.Size(),
			Mode:    fi.Mode().String(),
			Owner:   ownerOf(fi),
			IsDir:   e.IsDir(),
			ModTime: fi.ModTime(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	core.OK(c, gin.H{"path": dir, "items": items})
}

func (m *Module) mkdir(c *gin.Context) {
	var req struct {
		Path string `binding:"required"`
		Name string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	target := filepath.Join(clean(req.Path), req.Name)
	if err := os.MkdirAll(target, 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) rename(c *gin.Context) {
	var req struct {
		Path string `binding:"required"`
		Name string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := guard(req.Path); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	target := filepath.Join(filepath.Dir(clean(req.Path)), req.Name)
	if err := os.Rename(clean(req.Path), target); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) move(c *gin.Context) {
	var req struct {
		Src string `binding:"required"`
		Dst string `binding:"required"` // 目标目录
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := guard(req.Src); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	dst := filepath.Join(clean(req.Dst), filepath.Base(clean(req.Src)))
	if err := os.Rename(clean(req.Src), dst); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

// delete 移入面板回收站；跨磁盘无法移动时直接删除并提示。
func (m *Module) delete(c *gin.Context) {
	var req struct {
		Paths []string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	recycle := filepath.Join(m.dataDir, "recycle")
	_ = os.MkdirAll(recycle, 0o755)
	var msgs []string
	for i, p := range req.Paths {
		if err := guard(p); err != nil {
			msgs = append(msgs, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		p = clean(p)
		name := fmt.Sprintf("%d-%s", time.Now().UnixNano()+int64(i), filepath.Base(p))
		if err := os.Rename(p, filepath.Join(recycle, name)); err != nil {
			// 跨磁盘等场景回退为直接删除
			if err2 := os.RemoveAll(p); err2 != nil {
				msgs = append(msgs, fmt.Sprintf("%s: %v", p, err2))
			} else {
				msgs = append(msgs, fmt.Sprintf("%s: 已直接删除（无法移入回收站）", p))
			}
		}
	}
	if len(msgs) > 0 {
		core.Err(c, 500, strings.Join(msgs, "; "))
		return
	}
	core.OK(c, nil)
}

func (m *Module) read(c *gin.Context) {
	p := clean(c.Query("path"))
	fi, err := os.Stat(p)
	if err != nil {
		core.Err(c, 404, err.Error())
		return
	}
	if fi.IsDir() {
		core.Err(c, 400, "不能读取目录")
		return
	}
	if fi.Size() > maxReadSize {
		core.Err(c, 400, "文件超过 2MB，请在终端中编辑")
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"content": string(data)})
}

func (m *Module) save(c *gin.Context) {
	var req struct {
		Path    string `binding:"required"`
		Content string
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := guard(req.Path); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if err := os.WriteFile(clean(req.Path), []byte(req.Content), 0o644); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) download(c *gin.Context) {
	p := clean(c.Query("path"))
	fi, err := os.Stat(p)
	if err != nil {
		core.Err(c, 404, err.Error())
		return
	}
	if fi.IsDir() {
		core.Err(c, 400, "请先压缩目录后再下载")
		return
	}
	c.FileAttachment(p, filepath.Base(p))
}

func (m *Module) upload(c *gin.Context) {
	dir := clean(c.PostForm("path"))
	fh, err := c.FormFile("file")
	if err != nil {
		core.BindErr(c, err)
		return
	}
	target := filepath.Join(dir, filepath.Base(fh.Filename))
	if err := c.SaveUploadedFile(fh, target); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"path": target})
}

func (m *Module) chmod(c *gin.Context) {
	var req struct {
		Path string `binding:"required"`
		Mode string `binding:"required"` // 八进制，如 0644
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	mode, err := strconv.ParseUint(strings.TrimPrefix(req.Mode, "0"), 8, 32)
	if err != nil {
		core.Err(c, 400, "权限格式错误（应为八进制，如 0644）")
		return
	}
	if err := os.Chmod(clean(req.Path), os.FileMode(mode)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) compress(c *gin.Context) {
	var req struct {
		Paths  []string `binding:"required"`
		Format string   `binding:"required,oneof=zip targz"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if len(req.Paths) == 0 {
		core.Err(c, 400, "未选择文件")
		return
	}
	first := clean(req.Paths[0])
	base := filepath.Dir(first)
	name := strings.TrimSuffix(filepath.Base(first), filepath.Ext(first))
	if name == "" || name == "." {
		name = "archive"
	}
	ext := ".tar.gz"
	if req.Format == "zip" {
		ext = ".zip"
	}
	out := filepath.Join(base, name+ext)
	var err error
	if req.Format == "zip" {
		err = archive.ZipAt(c.Request.Context(), base, req.Paths, out)
	} else {
		err = archive.TarGzAt(c.Request.Context(), base, req.Paths, out)
	}
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"path": out})
}

func (m *Module) decompress(c *gin.Context) {
	var req struct {
		Path string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	p := clean(req.Path)
	lower := strings.ToLower(p)
	var dest string
	var err error
	switch {
	case strings.HasSuffix(lower, ".zip"):
		dest = strings.TrimSuffix(p, filepath.Ext(p))
		err = archive.UnZip(p, dest)
	case strings.HasSuffix(lower, ".tar.gz"):
		dest = strings.TrimSuffix(p, ".tar.gz")
		err = archive.UnTarGz(p, dest)
	case strings.HasSuffix(lower, ".tgz"):
		dest = strings.TrimSuffix(p, ".tgz")
		err = archive.UnTarGz(p, dest)
	default:
		core.Err(c, 400, "仅支持 .zip / .tar.gz / .tgz")
		return
	}
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"path": dest})
}

// chunksDir 分片临时目录。
func (m *Module) chunksDir() string {
	dir := filepath.Join(m.dataDir, "chunks")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// uploadChunk 大文件分片上传：body 为原始分片字节，query: path/upload_id/index。
func (m *Module) uploadChunk(c *gin.Context) {
	uploadID := c.Query("upload_id")
	idx := c.Query("index")
	if uploadID == "" || idx == "" || !uploadIDRe.MatchString(uploadID) {
		core.Err(c, 400, "分片参数非法")
		return
	}
	cdir := filepath.Join(m.chunksDir(), uploadID)
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(c.Request.Body, 0)); err != nil {
		// no-op：保持连接语义
	}
	out, err := os.Create(filepath.Join(cdir, fmt.Sprintf("%06s.part", idx)))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(c.Request.Body, 64<<20))
	if err != nil {
		os.Remove(out.Name())
		core.Err(c, 500, "分片写入失败: "+err.Error())
		return
	}
	core.OK(c, gin.H{"index": idx, "bytes": written})
}

// uploadFinish 合并分片到目标文件（按 index 数字序），并清理临时目录。
func (m *Module) uploadFinish(c *gin.Context) {
	var req struct {
		Path     string `binding:"required"`
		UploadID string `binding:"required"`
		Total    int    `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !uploadIDRe.MatchString(req.UploadID) {
		core.Err(c, 400, "分片参数非法")
		return
	}
	cdir := filepath.Join(m.chunksDir(), req.UploadID)
	target := clean(req.Path)
	if target == "" {
		core.Err(c, 400, "目标路径非法")
		return
	}
	out, err := os.Create(target)
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer out.Close()
	for i := 0; i < req.Total; i++ {
		part, err := os.ReadFile(filepath.Join(cdir, fmt.Sprintf("%06d.part", i)))
		if err != nil {
			core.Err(c, 500, fmt.Sprintf("分片 %d 缺失: %v", i, err))
			return
		}
		if _, err := out.Write(part); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
	}
	_ = os.RemoveAll(cdir)
	fi, _ := os.Stat(target)
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	core.OK(c, gin.H{"path": target, "size": size})
}

// uploadIDRe 分片上传会话 ID 白名单。
var uploadIDRe = regexp.MustCompile(`^[a-f0-9]{32}$`)
