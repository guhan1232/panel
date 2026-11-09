// Package dbmgr 数据库管理：MySQL 与 PostgreSQL 的建库、建用户、授权、删除。
// 连接凭据存于面板 Setting；所有标识符做严格校验防注入。
package dbmgr

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/execx"
)

var identRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_$-]{0,62}$`)

type MysqlCfg struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type PgCfg struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`
}

func getMysql(db *gorm.DB) MysqlCfg {
	var c MysqlCfg
	_ = json.Unmarshal([]byte(core.GetSetting(db, core.KMysqlCfg)), &c)
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 3306
	}
	if c.User == "" {
		c.User = "root"
	}
	return c
}

func getPg(db *gorm.DB) PgCfg {
	var c PgCfg
	_ = json.Unmarshal([]byte(core.GetSetting(db, core.KPgCfg)), &c)
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 5432
	}
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	return c
}

func validIdent(name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("标识符非法（仅字母/数字/下划线，且不以特殊字符结尾）: %s", name)
	}
	return nil
}

// sqlStr 转义 SQL 字符串字面量中的单引号（仅用于无法参数化的 DDL）。
func sqlStr(s string) string { return strings.ReplaceAll(s, "'", "''") }

// ---- 连接 ----

func mysqlDB(c MysqlCfg) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", c.Host, c.Port)
	cfg.Timeout = 5 * time.Second
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	return sql.Open("mysql", cfg.FormatDSN())
}

func pgConn(ctx context.Context, c PgCfg) (*pgx.Conn, error) {
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/postgres",
	}
	dsn.RawQuery = url.Values{"sslmode": []string{c.SSLMode}}.Encode()
	return pgx.Connect(ctx, dsn.String())
}

// ---- 路由 ----

type Module struct {
	db      *gorm.DB
	dataDir string
}

func New(db *gorm.DB, dataDir string) *Module { return &Module{db: db, dataDir: dataDir} }

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/dbmgr")
	g.GET("/config", m.getConfig)
	g.POST("/mysql-config", m.saveMysql)
	g.POST("/pg-config", m.savePg)
	g.POST("/test", m.test)
	g.POST("/backup", m.backupCreate)
	g.GET("/backup/list", m.backupList)
	g.GET("/backup/download", m.backupDownload)
	g.DELETE("/backup", m.backupDelete)

	mg := g.Group("/mysql")
	mg.GET("/databases", m.mysqlList)
	mg.POST("/databases", m.mysqlCreateDB)
	mg.DELETE("/databases/:name", m.mysqlDropDB)
	mg.GET("/users", m.mysqlUsers)
	mg.POST("/users", m.mysqlCreateUser)
	mg.POST("/grant", m.mysqlGrant)
	mg.DELETE("/users/:name/:host", m.mysqlDropUser)

	pg := g.Group("/pg")
	pg.GET("/databases", m.pgList)
	pg.POST("/databases", m.pgCreateDB)
	pg.DELETE("/databases/:name", m.pgDropDB)
	pg.GET("/users", m.pgUsers)
	pg.POST("/users", m.pgCreateUser)
	pg.POST("/grant", m.pgGrant)
	pg.DELETE("/users/:name", m.pgDropUser)
}

func (m *Module) getConfig(c *gin.Context) {
	mq := getMysql(m.db)
	if mq.Password != "" {
		mq.Password = "******"
	}
	pg := getPg(m.db)
	if pg.Password != "" {
		pg.Password = "******"
	}
	core.OK(c, gin.H{"mysql": mq, "pg": pg})
}

func (m *Module) saveMysql(c *gin.Context) {
	var req MysqlCfg
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if req.Password == "******" {
		req.Password = getMysql(m.db).Password
	}
	data, _ := json.Marshal(req)
	if err := core.SetSetting(m.db, core.KMysqlCfg, string(data)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) savePg(c *gin.Context) {
	var req PgCfg
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if req.Password == "******" {
		req.Password = getPg(m.db).Password
	}
	data, _ := json.Marshal(req)
	if err := core.SetSetting(m.db, core.KPgCfg, string(data)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) test(c *gin.Context) {
	var req struct {
		Engine string `binding:"required,oneof=mysql pg"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var err error
	if req.Engine == "mysql" {
		var d *sql.DB
		if d, err = mysqlDB(getMysql(m.db)); err == nil {
			err = d.Ping()
			d.Close()
		}
	} else {
		var conn *pgx.Conn
		if conn, err = pgConn(ctx, getPg(m.db)); err == nil {
			err = conn.Ping(ctx)
			conn.Close(ctx)
		}
	}
	if err != nil {
		core.Err(c, 500, "连接失败: "+err.Error())
		return
	}
	core.OK(c, nil)
}

// ---- MySQL ----

var systemMysqlDBs = map[string]bool{"information_schema": true, "mysql": true, "performance_schema": true, "sys": true}

func (m *Module) mysqlList(c *gin.Context) {
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	rows, err := d.Query("SHOW DATABASES")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && !systemMysqlDBs[name] {
			items = append(items, name)
		}
	}
	core.OK(c, items)
}

func (m *Module) mysqlCreateDB(c *gin.Context) {
	var req struct {
		Name string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validIdent(req.Name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec(fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci", req.Name)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) mysqlDropDB(c *gin.Context) {
	name := c.Param("name")
	if err := validIdent(name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec(fmt.Sprintf("DROP DATABASE `%s`", name)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) mysqlUsers(c *gin.Context) {
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	rows, err := d.Query("SELECT user, host FROM mysql.user")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer rows.Close()
	type u struct {
		User string `json:"user"`
		Host string `json:"host"`
	}
	items := []u{}
	for rows.Next() {
		var x u
		if rows.Scan(&x.User, &x.Host) == nil {
			items = append(items, x)
		}
	}
	core.OK(c, items)
}

func (m *Module) mysqlCreateUser(c *gin.Context) {
	var req struct {
		Name     string `binding:"required"`
		Host     string `binding:"required"`
		Password string `binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validIdent(req.Name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec(fmt.Sprintf("CREATE USER `%s`@`%s` IDENTIFIED BY '%s'",
		req.Name, sqlStr(req.Host), sqlStr(req.Password))); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) mysqlGrant(c *gin.Context) {
	var req struct {
		User string `binding:"required"`
		Host string `binding:"required"`
		DB   string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validIdent(req.User); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if err := validIdent(req.DB); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec(fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO `%s`@`%s`", req.DB, req.User, sqlStr(req.Host))); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	if _, err := d.Exec("FLUSH PRIVILEGES"); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) mysqlDropUser(c *gin.Context) {
	name, host := c.Param("name"), c.Param("host")
	if err := validIdent(name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec(fmt.Sprintf("DROP USER `%s`@`%s`", name, sqlStr(host))); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

// ---- PostgreSQL ----

func (m *Module) pgList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, "SELECT datname FROM pg_database WHERE NOT datistemplate AND datname NOT IN ('template0','template1')")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			items = append(items, name)
		}
	}
	core.OK(c, items)
}

func (m *Module) pgCreateDB(c *gin.Context) {
	var req struct {
		Name string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validIdent(req.Name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE "%s"`, req.Name)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) pgDropDB(c *gin.Context) {
	name := c.Param("name")
	if err := validIdent(name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE "%s"`, name)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) pgUsers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, "SELECT rolname FROM pg_roles WHERE rolcanlogin")
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []string{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			items = append(items, name)
		}
	}
	core.OK(c, items)
}

func (m *Module) pgCreateUser(c *gin.Context) {
	var req struct {
		Name     string `binding:"required"`
		Password string `binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validIdent(req.Name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE ROLE "%s" LOGIN PASSWORD '%s'`, req.Name, sqlStr(req.Password))); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) pgGrant(c *gin.Context) {
	var req struct {
		User string `binding:"required"`
		DB   string `binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := validIdent(req.User); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	if err := validIdent(req.DB); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s"`, req.DB, req.User)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) pgDropUser(c *gin.Context) {
	name := c.Param("name")
	if err := validIdent(name); err != nil {
		core.Err(c, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgConn(ctx, getPg(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP ROLE "%s"`, name)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

// dbIdentRe 备份目标库名校验（与计划任务 backup_db 一致）。
var dbIdentRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// backupFileRe 备份文件名白名单（防路径穿越）。
var backupFileRe = regexp.MustCompile(`^db-(mysql|pg)-[A-Za-z0-9_]+-\d{8}-\d{6}\.sql\.gz$`)

// shQuote shell 单引号安全包裹。
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tailOf 取输出尾部（报错展示用）。
func tailOf(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 600 {
		return s
	}
	return s[len(s)-600:]
}

// ---- 数据库即时备份：复用计划任务 backup_db 的 dump 逻辑 ----

func (m *Module) backupsDir() string {
	dir := filepath.Join(m.dataDir, "backups")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

type backupReq struct {
	Engine   string `json:"engine" binding:"required"`
	Database string `json:"database" binding:"required"`
}

func (m *Module) backupCreate(c *gin.Context) {
	var req backupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if req.Engine != "mysql" && req.Engine != "pg" {
		core.Err(c, 400, "engine 需为 mysql 或 pg")
		return
	}
	if !dbIdentRe.MatchString(req.Database) {
		core.Err(c, 400, "数据库名非法")
		return
	}
	out := filepath.Join(m.backupsDir(), fmt.Sprintf("db-%s-%s-%s.sql.gz", req.Engine, req.Database, time.Now().Format("20060102-150405")))
	var script string
	if req.Engine == "mysql" {
		var cfg struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
		}
		_ = json.Unmarshal([]byte(core.GetSetting(m.db, core.KMysqlCfg)), &cfg)
		if cfg.Host == "" {
			cfg.Host = "127.0.0.1"
		}
		if cfg.Port == 0 {
			cfg.Port = 3306
		}
		if cfg.User == "" {
			cfg.User = "root"
		}
		script = fmt.Sprintf("set -o pipefail\nexport MYSQL_PWD=%s\nmysqldump -h %s -P %d -u %s --single-transaction --routines --databases %s | gzip > %s",
			shQuote(cfg.Password), shQuote(cfg.Host), cfg.Port, shQuote(cfg.User), shQuote(req.Database), shQuote(out))
	} else {
		var cfg struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
			SSLMode  string `json:"ssl_mode"`
		}
		_ = json.Unmarshal([]byte(core.GetSetting(m.db, core.KPgCfg)), &cfg)
		if cfg.Host == "" {
			cfg.Host = "127.0.0.1"
		}
		if cfg.Port == 0 {
			cfg.Port = 5432
		}
		if cfg.SSLMode == "" {
			cfg.SSLMode = "disable"
		}
		script = fmt.Sprintf("set -o pipefail\nexport PGPASSWORD=%s\nexport PGSSLMODE=%s\npg_dump -h %s -p %d -U %s -d %s | gzip > %s",
			shQuote(cfg.Password), shQuote(cfg.SSLMode), shQuote(cfg.Host), cfg.Port, shQuote(cfg.User), shQuote(req.Database), shQuote(out))
	}
	if out2, code, err := execx.RunShell("dbmgr:backup", 20*time.Minute, script); err != nil {
		core.Err(c, 500, fmt.Sprintf("备份失败(exit %d): %s", code, tailOf(out2)))
		return
	}
	fi, _ := os.Stat(out)
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	core.OK(c, gin.H{"file": filepath.Base(out), "size": size})
}

func (m *Module) backupList(c *gin.Context) {
	dir := m.backupsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		core.OK(c, []gin.H{})
		return
	}
	out := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql.gz") {
			continue
		}
		fi, _ := e.Info()
		var size int64
		if fi != nil {
			size = fi.Size()
		}
		var modTime string
		if fi != nil {
			modTime = fi.ModTime().Format("2006-01-02 15:04:05")
		}
		out = append(out, gin.H{
			"file": e.Name(),
			"size": size,
			"time": modTime,
		})
	}
	core.OK(c, out)
}

func (m *Module) backupDownload(c *gin.Context) {
	name := c.Query("file")
	if !backupFileRe.MatchString(name) {
		core.Err(c, 400, "文件名非法")
		return
	}
	p := filepath.Join(m.backupsDir(), name)
	if _, err := os.Stat(p); err != nil {
		core.Err(c, 404, "备份不存在")
		return
	}
	c.FileAttachment(p, name)
}

func (m *Module) backupDelete(c *gin.Context) {
	name := c.Query("file")
	if !backupFileRe.MatchString(name) {
		core.Err(c, 400, "文件名非法")
		return
	}
	if err := os.Remove(filepath.Join(m.backupsDir(), name)); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, gin.H{"ok": true})
}
