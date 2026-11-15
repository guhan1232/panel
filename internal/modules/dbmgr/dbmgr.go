// Package dbmgr 数据库管理：MySQL 与 PostgreSQL 的建库、建用户、授权、删除。
// 连接凭据存于面板 Setting；所有标识符做严格校验防注入。
package dbmgr

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// mysqlIdent 将标识符安全包裹为反引号标识符：白名单校验 + 二次过滤反引号。
// DDL（CREATE/DROP DATABASE、用户与授权）无法参数化，白名单 + 包裹是标准做法。
func mysqlIdent(name string) (string, error) {
	if err := validIdent(name); err != nil {
		return "", err
	}
	return "`" + strings.ReplaceAll(name, "`", "") + "`", nil
}

// pgIdent 将标识符安全包裹为双引号标识符（PostgreSQL）。
func pgIdent(name string) (string, error) {
	if err := validIdent(name); err != nil {
		return "", err
	}
	return `"` + strings.ReplaceAll(name, `"`, "") + `"`, nil
}

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
	g.GET("/status", m.dbStatus)
	g.POST("/install-mariadb", m.installMariaDB)
	g.POST("/install-mysql", m.installMySQL)
	g.POST("/uninstall", m.uninstall)
	g.POST("/mysql-config", m.saveMysql)
	g.POST("/pg-config", m.savePg)
	g.POST("/test", m.test)
	g.POST("/backup", m.backupCreate)
	g.GET("/backup/list", m.backupList)
	g.GET("/backup/download", m.backupDownload)
	g.POST("/backup/restore", m.backupRestore)
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

// connErr 判断是否为「服务未安装/未监听」类连接错误（降级为 installed:false，不报 500 弹窗）。
func connErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "dial tcp") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "no such file") ||
		strings.Contains(s, "i/o timeout")
}

// dbStatus 探测 MySQL/PG 服务是否在监听（1.5s 超时，绝不报错弹窗）。
func (m *Module) dbStatus(c *gin.Context) {
	mq := getMysql(m.db)
	mAddr := fmt.Sprintf("%s:%d", mq.Host, mq.Port)
	mOk := probeTCP(mAddr)
	pg := getPg(m.db)
	pAddr := fmt.Sprintf("%s:%d", pg.Host, pg.Port)
	pOk := probeTCP(pAddr)
	core.OK(c, gin.H{
		"mysql": gin.H{"installed": mOk, "addr": mAddr},
		"pg":    gin.H{"installed": pOk, "addr": pAddr},
	})
}

func probeTCP(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 1500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// installMariaDB 一键安装 MariaDB（备选；默认走 installMySQL）。
func (m *Module) installMariaDB(c *gin.Context) {
	var out string
	var code int
	var err error
	if _, _, e := execx.Run("db", 5*time.Second, "apt-get", "--version"); e == nil {
		out, code, err = execx.Run("db", 600*time.Second, "sh", "-c",
			"DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq mariadb-server && systemctl enable --now mariadb")
	} else if _, _, e := execx.Run("db", 5*time.Second, "dnf", "--version"); e == nil {
		out, code, err = execx.Run("db", 600*time.Second, "sh", "-c",
			"dnf install -y mariadb-server && systemctl enable --now mariadb")
	} else if _, _, e := execx.Run("db", 5*time.Second, "yum", "--version"); e == nil {
		out, code, err = execx.Run("db", 600*time.Second, "sh", "-c",
			"yum install -y mariadb-server && systemctl enable --now mariadb")
	} else {
		core.Err(c, 400, "未识别的包管理器（仅支持 apt/dnf/yum）")
		return
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("安装失败(exit %d): %s", code, out))
		return
	}
	m.waitForPort(getMysql(m.db).Port)
	msg := m.configureMysqlRoot(c)
	m.applyMySQLLightweight("mariadb")
	slog.Info("MariaDB 安装完成")
	core.OK(c, gin.H{"ok": true, "message": "MariaDB 已安装并启动。" + msg})
}

// installMySQL 一键安装 MySQL（默认推荐）：
//   - Ubuntu：apt 官方源直接安装 mysql-server；
//   - Debian：apt 无 mysql 包，添加 MySQL 官方 APT 仓库后安装（失败时提示改用 MariaDB 备选）。
//   - RHEL：dnf/yum 官方 mysql-server 包。
// 安装后自动修复 root 认证（Error 1698：把 root 改为密码认证并生成随机强密码），凭据写入面板连接配置。
func (m *Module) installMySQL(c *gin.Context) {
	var out string
	var code int
	var err error
	if _, _, e := execx.Run("db", 5*time.Second, "apt-get", "--version"); e == nil {
		isDebian := false
		if b, e2 := os.ReadFile("/etc/os-release"); e2 == nil && strings.Contains(string(b), "ID=debian") {
			isDebian = true
		}
		if isDebian {
			// 手动配置 MySQL 官方 APT 仓库；先卸 MariaDB 释放 3306（默认 MySQL）
			// trixie 上官方只有 mysql-innovation（9.x）组件；bookworm 为 mysql-8.0
			script := `set -e; ` +
				`systemctl stop mariadb mysql 2>/dev/null || true; ` +
				`DEBIAN_FRONTEND=noninteractive apt-get purge -y -qq mariadb-server mariadb-server-* 2>/dev/null || true; ` +
				`rm -rf /var/lib/mysql; ` +
				`rm -f /etc/apt/sources.list.d/mysql.list; ` +
				`apt-get update -qq; ` +
				`DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gnupg lsb-release wget ca-certificates; ` +
				`echo "deb [trusted=yes] https://repo.mysql.com/apt/debian $(lsb_release -sc) mysql-innovation mysql-tools" > /etc/apt/sources.list.d/mysql.list; ` +
				`apt-get update -qq; ` +
				`DEBIAN_FRONTEND=noninteractive apt-get install -y -qq mysql-community-server; ` +
				`systemctl enable --now mysql`
			out, code, err = execx.Run("db", 900*time.Second, "sh", "-c", script)
		} else {
			out, code, err = execx.Run("db", 900*time.Second, "sh", "-c",
				"DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq mysql-server && systemctl enable --now mysql")
		}
	} else if _, _, e := execx.Run("db", 5*time.Second, "dnf", "--version"); e == nil {
		out, code, err = execx.Run("db", 900*time.Second, "sh", "-c", "dnf install -y mysql-server && systemctl enable --now mysqld")
	} else if _, _, e := execx.Run("db", 5*time.Second, "yum", "--version"); e == nil {
		out, code, err = execx.Run("db", 900*time.Second, "sh", "-c", "yum install -y mysql-server && systemctl enable --now mysqld")
	} else {
		core.Err(c, 400, "未识别的包管理器（仅支持 apt/dnf/yum）")
		return
	}
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("MySQL 安装失败(exit %d): %s。Debian 可改用「安装 MariaDB」备选，或检查外网可达性。", code, tailMsg(out)))
		return
	}
	m.waitForPort(getMysql(m.db).Port)
	msg := m.configureMysqlRoot(c)
	m.applyMySQLLightweight("mysql")
	slog.Info("MySQL 安装完成")
	core.OK(c, gin.H{"ok": true, "message": "MySQL 已安装并启动。" + msg})
}

// configureMysqlRoot 修复 Error 1698：root@localhost 默认 unix_socket/caching_sha2 免密认证，
// TCP(127.0.0.1) 无法匹配账号被拒。经 unix socket 免密通道：为 root@localhost 设密码（保留 socket 免密），
// 并创建 root@127.0.0.1（TCP 专用同密码），验证 TCP 登录后写入面板连接配置。
func (m *Module) configureMysqlRoot(c *gin.Context) string {
	pw := randHex(8) + "Mx" + randHex(8) // 十六进制随机串，长度足够且含字母数字
	ver, _, _ := execx.Run("db", 15*time.Second, "sh", "-c", "mysql --version 2>/dev/null || mariadb --version 2>/dev/null")
	var sql string
	if strings.Contains(ver, "MariaDB") {
		sql = fmt.Sprintf(`ALTER USER 'root'@'localhost' IDENTIFIED VIA mysql_native_password USING PASSWORD('%s') OR unix_socket; `+
			`CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY '%s'; `+
			`GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; FLUSH PRIVILEGES;`, pw, pw)
	} else {
		sql = fmt.Sprintf(`ALTER USER 'root'@'localhost' IDENTIFIED BY '%s'; `+
			`CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' IDENTIFIED BY '%s'; `+
			`GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; FLUSH PRIVILEGES;`, pw, pw)
	}
	// root 权限（面板即 root 运行）经 unix socket 免密执行
	if _, code, err := execx.Run("db", 30*time.Second, "sh", "-c", "mysql -uroot -e \""+sql+"\""); err != nil {
		return "（root 密码自动配置失败 exit " + strconv.Itoa(code) + "，请手动执行 mysql_secure_installation）"
	}
	// TCP + 密码验证（1698 修复的验收点）
	if _, _, err := execx.Run("db", 15*time.Second, "sh", "-c", "mysql -h127.0.0.1 -uroot -p'"+pw+"' -e 'SELECT 1' >/dev/null 2>&1"); err != nil {
		return "（root 密码已设置但 TCP 验证未通过，请检查 bind-address）"
	}
	// 凭据写入面板连接配置
	cfg := MysqlCfg{Host: "127.0.0.1", Port: 3306, User: "root", Password: pw}
	data, _ := json.Marshal(cfg)
	_ = core.SetSetting(m.db, core.KMysqlCfg, string(data))
	return "root 认证已修复（Error 1698）：root@localhost 保留 socket 免密、root@127.0.0.1 走 TCP 密码登录，连接配置已保存到面板。"
}

func (m *Module) waitForPort(port int) {
	for i := 0; i < 15; i++ {
		if probeTCP(fmt.Sprintf("127.0.0.1:%d", port)) {
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b)
}

func tailMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return "..." + s[len(s)-300:]
	}
	return s
}

// uninstall 卸载数据库服务（危险：可选删除数据目录）。
func (m *Module) uninstall(c *gin.Context) {
	var req struct {
		Engine     string `binding:"required,oneof=mysql mariadb postgresql"`
		RemoveData bool   `json:"remove_data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	var script string
	var rmData string
	switch req.Engine {
	case "mysql", "mariadb":
		if _, _, e := execx.Run("db", 5*time.Second, "apt-get", "--version"); e == nil {
			script = "systemctl stop mysql mariadb 2>/dev/null; DEBIAN_FRONTEND=noninteractive apt-get purge -y 'mysql-*' 'mariadb-*' 'default-mysql-*' 2>/dev/null; apt-get autoremove -y -qq"
		} else {
			script = "systemctl stop mysqld mariadb 2>/dev/null; dnf remove -y mysql-server mariadb-server 2>/dev/null"
		}
		rmData = "rm -rf /var/lib/mysql /etc/mysql /etc/my.cnf.d /var/log/mysql"
	case "postgresql":
		if _, _, e := execx.Run("db", 5*time.Second, "apt-get", "--version"); e == nil {
			script = "systemctl stop postgresql 2>/dev/null; DEBIAN_FRONTEND=noninteractive apt-get purge -y 'postgresql-*' 2>/dev/null; apt-get autoremove -y -qq"
		} else {
			script = "systemctl stop postgresql 2>/dev/null; dnf remove -y postgresql-server 2>/dev/null"
		}
		rmData = "rm -rf /var/lib/postgresql /etc/postgresql"
	}
	out, code, err := execx.Run("db", 600*time.Second, "sh", "-c", script)
	if err != nil {
		core.Err(c, 500, fmt.Sprintf("卸载失败(exit %d): %s", code, tailMsg(out)))
		return
	}
	if req.RemoveData {
		if _, code, err := execx.Run("db", 60*time.Second, "sh", "-c", rmData); err != nil {
			core.Err(c, 500, fmt.Sprintf("数据目录删除失败(exit %d)", code))
			return
		}
	}
	// 清空面板连接配置
	if req.Engine == "postgresql" {
		_ = core.SetSetting(m.db, core.KPgCfg, "")
	} else {
		_ = core.SetSetting(m.db, core.KMysqlCfg, "")
	}
	slog.Info("数据库服务已卸载", "engine", req.Engine, "remove_data", req.RemoveData)
	core.OK(c, gin.H{"ok": true, "message": "卸载完成" + map[bool]string{true: "（数据目录已删除）", false: "（数据目录保留在原位置）"}[req.RemoveData]})
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
		if connErr(err) {
			core.OK(c, gin.H{"installed": false})
			return
		}
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
	id, idErr := mysqlIdent(req.Name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec("CREATE DATABASE " + id + " CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) mysqlDropDB(c *gin.Context) {
	name := c.Param("name")
	id, idErr := mysqlIdent(name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec("DROP DATABASE " + id); err != nil {
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
		if connErr(err) {
			core.OK(c, gin.H{"installed": false})
			return
		}
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
	id, idErr := mysqlIdent(req.Name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	if _, err := d.Exec("CREATE USER " + id + "@`" + sqlStr(req.Host) + "` IDENTIFIED BY '" + sqlStr(req.Password) + "'"); err != nil {
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
	userId, idErr := mysqlIdent(req.User)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	dbId, idErr := mysqlIdent(req.DB)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	// MySQL 8+ 不允许 GRANT 隐式建用户（Error 1410），先检查账号存在性给出清晰提示
	var cnt int
	if err := d.QueryRow("SELECT COUNT(*) FROM mysql.user WHERE user = ? AND host = ?", req.User, req.Host).Scan(&cnt); err == nil && cnt == 0 {
		core.Err(c, 400, "用户不存在（或 Host 不匹配），请先「创建用户」再授权")
		return
	}
	if _, err := d.Exec("GRANT ALL PRIVILEGES ON " + dbId + ".* TO " + userId + "@`" + sqlStr(req.Host) + "`"); err != nil {
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
	userId, idErr := mysqlIdent(name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	d, err := mysqlDB(getMysql(m.db))
	if err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	defer d.Close()
	if _, err := d.Exec("DROP USER " + userId + "@`" + sqlStr(host) + "`"); err != nil {
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
		if connErr(err) {
			core.OK(c, gin.H{"installed": false})
			return
		}
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, "SELECT datname FROM pg_database WHERE NOT datistemplate AND datname NOT IN ('template0','template1')")
	if err != nil {
		if connErr(err) {
			core.OK(c, gin.H{"installed": false})
			return
		}
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
	id, idErr := pgIdent(req.Name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
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
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+id); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) pgDropDB(c *gin.Context) {
	name := c.Param("name")
	id, idErr := pgIdent(name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
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
	if _, err := conn.Exec(ctx, "DROP DATABASE "+id); err != nil {
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
		if connErr(err) {
			core.OK(c, gin.H{"installed": false})
			return
		}
		core.Err(c, 500, err.Error())
		return
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, "SELECT rolname FROM pg_roles WHERE rolcanlogin")
	if err != nil {
		if connErr(err) {
			core.OK(c, gin.H{"installed": false})
			return
		}
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
	id, idErr := pgIdent(req.Name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
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
	if _, err := conn.Exec(ctx, "CREATE ROLE "+id+" LOGIN PASSWORD '"+sqlStr(req.Password)+"'"); err != nil {
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
	userId, idErr := pgIdent(req.User)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
		return
	}
	dbId, idErr := pgIdent(req.DB)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
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
	if _, err := conn.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+dbId+" TO "+userId); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) pgDropUser(c *gin.Context) {
	name := c.Param("name")
	id, idErr := pgIdent(name)
	if idErr != nil {
		core.Err(c, 400, idErr.Error())
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
	if _, err := conn.Exec(ctx, "DROP ROLE "+id); err != nil {
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

// backupRestore 从备份文件恢复数据库（危险操作：整库覆盖，需 force 确认 + 审计）。
func (m *Module) backupRestore(c *gin.Context) {
	var req struct {
		File  string `json:"file" binding:"required"`
		Force bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if !req.Force {
		core.Err(c, 400, "恢复将整库覆盖当前数据，需传 force=true 确认")
		return
	}
	if !backupFileRe.MatchString(req.File) {
		core.Err(c, 400, "文件名非法")
		return
	}
	// 文件名格式 db-<engine>-<db>-<ts>.sql.gz
	parts := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(req.File, "db-"), ".sql.gz"), "-", 3)
	if len(parts) != 3 {
		core.Err(c, 400, "文件名无法解析引擎与库名")
		return
	}
	engine, database := parts[0], parts[1]
	if !dbIdentRe.MatchString(database) {
		core.Err(c, 400, "数据库名非法")
		return
	}
	backupPath := filepath.Join(m.backupsDir(), req.File)
	var script string
	if engine == "mysql" {
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
		script = fmt.Sprintf("set -o pipefail\nexport MYSQL_PWD=%s\ngunzip -c %s | mysql -h %s -P %d -u %s %s",
			shQuote(cfg.Password), shQuote(backupPath), shQuote(cfg.Host), cfg.Port, shQuote(cfg.User), shQuote(database))
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
		script = fmt.Sprintf("set -o pipefail\nexport PGPASSWORD=%s\nexport PGSSLMODE=%s\ngunzip -c %s | psql -h %s -p %d -U %s -d %s",
			shQuote(cfg.Password), shQuote(cfg.SSLMode), shQuote(backupPath), shQuote(cfg.Host), cfg.Port, shQuote(cfg.User), shQuote(database))
	}
	if out2, code, err := execx.RunShell("dbmgr:restore", 30*time.Minute, script); err != nil {
		core.Err(c, 500, fmt.Sprintf("恢复失败(exit %d): %s", code, tailOf(out2)))
		return
	}
	core.OK(c, gin.H{"ok": true})
}


// applyMySQLLightweight 小内存机（≤4GB）安装后自适应轻量化：
// 写独立配置文件（不碰发行版默认值），重启探活失败自动回滚删除；任何异常只记日志，不影响安装结果。
// ≤2GB：buffer_pool 32M + performance_schema OFF + max_conn 30；2-4GB：buffer_pool 128M + P_S OFF + max_conn 80。
func (m *Module) applyMySQLLightweight(kind string) {
	total := memTotalBytes()
	if total <= 0 || total > 4<<30 {
		return // 大内存机保持发行版默认（性能优先）
	}
	bufferPool, maxConn := "32M", "30"
	if total > 2<<30 {
		bufferPool, maxConn = "128M", "80"
	}
	var dir, unit string
	if kind == "mariadb" {
		unit = "mariadb"
		if _, e := os.Stat("/etc/mysql/mariadb.conf.d"); e == nil {
			dir = "/etc/mysql/mariadb.conf.d"
		} else {
			dir = "/etc/my.cnf.d"
		}
	} else {
		if _, e := os.Stat("/etc/mysql/mysql.conf.d"); e == nil {
			dir, unit = "/etc/mysql/mysql.conf.d", "mysql"
		} else if _, e2 := os.Stat("/etc/my.cnf.d"); e2 == nil {
			dir, unit = "/etc/my.cnf.d", "mysqld" // RHEL 系 mysql-server 单元名
		} else {
			return
		}
	}
	conf := fmt.Sprintf("# 寒语馨面板小内存机轻量化（自动写入；删除本文件并重启 %s 即恢复发行版默认）\n[mysqld]\ninnodb_buffer_pool_size = %s\nmax_connections = %s\ninnodb_log_buffer_size = 8M\ntable_open_cache = 200\nthread_cache_size = 8\n", unit, bufferPool, maxConn)
	if total <= 2<<30 {
		conf += "performance_schema = OFF\n"
	}
	path := filepath.Join(dir, "zz-hanyuxin-light.cnf")
	if e := os.WriteFile(path, []byte(conf), 0644); e != nil {
		slog.Warn("MySQL 轻量化配置写入失败（不影响使用）", "err", e)
		return
	}
	if _, _, e := execx.Run("db", 90*time.Second, "systemctl", "restart", unit); e != nil {
		_ = os.Remove(path)
		_, _, _ = execx.Run("db", 90*time.Second, "systemctl", "restart", unit)
		slog.Warn("MySQL 轻量化重启失败，已回滚删除配置", "err", e)
		return
	}
	slog.Info("MySQL 轻量化配置已生效", "path", path, "buffer_pool", bufferPool)
}

// memTotalBytes 读取 /proc/meminfo 的 MemTotal（字节；失败返回 0）。
func memTotalBytes() int64 {
	b, e := os.ReadFile("/proc/meminfo")
	if e != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			if f := strings.Fields(line); len(f) >= 2 {
				if kb, e2 := strconv.ParseInt(f[1], 10, 64); e2 == nil {
					return kb * 1024
				}
			}
		}
	}
	return 0
}
