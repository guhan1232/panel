// Package dbmgr 数据库管理：MySQL 与 PostgreSQL 的建库、建用户、授权、删除。
// 连接凭据存于面板 Setting；所有标识符做严格校验防注入。
package dbmgr

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
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

type Module struct{ db *gorm.DB }

func New(db *gorm.DB) *Module { return &Module{db: db} }

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/dbmgr")
	g.GET("/config", m.getConfig)
	g.POST("/mysql-config", m.saveMysql)
	g.POST("/pg-config", m.savePg)
	g.POST("/test", m.test)

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
