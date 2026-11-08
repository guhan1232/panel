// webmblx —— Linux Web 管理面板（单二进制）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"cnb.cool/hanyuxin/panel/internal/audit"
	"cnb.cool/hanyuxin/panel/internal/auth"
	"cnb.cool/hanyuxin/panel/internal/config"
	"cnb.cool/hanyuxin/panel/internal/core"
	"cnb.cool/hanyuxin/panel/internal/modules/alert"
	"cnb.cool/hanyuxin/panel/internal/modules/appstore"
	"cnb.cool/hanyuxin/panel/internal/modules/cert"
	"cnb.cool/hanyuxin/panel/internal/modules/cron"
	"cnb.cool/hanyuxin/panel/internal/modules/dashboard"
	"cnb.cool/hanyuxin/panel/internal/modules/dbmgr"
	"cnb.cool/hanyuxin/panel/internal/modules/filem"
	"cnb.cool/hanyuxin/panel/internal/modules/firewall"
	"cnb.cool/hanyuxin/panel/internal/modules/node"
	"cnb.cool/hanyuxin/panel/internal/modules/proc"
	rtm "cnb.cool/hanyuxin/panel/internal/modules/runtime"
	"cnb.cool/hanyuxin/panel/internal/modules/setting"
	"cnb.cool/hanyuxin/panel/internal/modules/term"
	"cnb.cool/hanyuxin/panel/internal/modules/website"
	"cnb.cool/hanyuxin/panel/internal/notify"
	"cnb.cool/hanyuxin/panel/internal/objectstore"
	"cnb.cool/hanyuxin/panel/internal/selfcert"
	webui "cnb.cool/hanyuxin/panel"
)

func main() {
	cfgPath := flag.String("c", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}
	core.Setup(cfg.LogDir, cfg.LogLevel)
	slog.Info("webmblx 启动", "version", core.Version, "listen", cfg.Server.Listen)
	scheme := "http"
	if cfg.Server.TLS.Enabled {
		scheme = "https"
	}
	slog.Info("面板访问地址", "url", fmt.Sprintf("%s://<服务器IP>%s/s/%s", scheme, cfg.Server.Listen, cfg.Server.Entrance))

	db, err := core.OpenDB(cfg.DataDir)
	if err != nil {
		slog.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}
	if err := core.BootstrapAdmin(db, cfg.DataDir); err != nil {
		slog.Error("初始化管理员失败", "err", err)
		os.Exit(1)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	authSvc := auth.New(db, cfg)

	// 安全入口
	r.GET("/s/:code", authSvc.HandleEntrance)

	// API：全部经过入口防护；登录之外的接口需要会话
	api := r.Group("/api", authSvc.EntranceRequired(), audit.Middleware(db))
	authSvc.Register(api)
	protected := api.Group("", authSvc.Middleware())
	authSvc.RegisterAdmin(protected) // 用户管理（仅 admin）

	cronMod := cron.New(db, cfg.DataDir)
	if err := cronMod.Start(); err != nil {
		slog.Error("计划任务模块启动失败", "err", err)
		os.Exit(1)
	}
	cronMod.Register(protected)
	filem.New(cfg.DataDir).Register(protected)
	// 终端为系统 shell，viewer 角色禁止
	term.Register(protected.Group("", authSvc.RequireRole(auth.RoleAdmin, auth.RoleOperator)), db)
	proc.Register(protected)
	setting.New(db, cfg).Register(protected)

	// P1 模块
	auditMod := audit.New(db)
	auditMod.Start()
	auditMod.Register(protected)
	notify.New(db).Register(protected)
	objectstore.New(db).Register(protected)
	firewall.Register(protected)
	websiteMod := website.New(db)
	if err := websiteMod.Start(); err != nil {
		slog.Error("网站模块启动失败", "err", err)
	}
	websiteMod.Register(protected)
	certMod := cert.New(db, cfg.DataDir)
	certMod.Register(protected)
	certMod.Start()
	dbmgr.New(db).Register(protected)

	// 多节点（Agent 反连走独立端点，token 鉴权；管理面走会话权限）
	protectedRW := protected.Group("", authSvc.RequireRole(auth.RoleAdmin, auth.RoleOperator))
	protectedAdmin := protected.Group("", authSvc.RequireAdmin())
	nodeMod := node.New(db)
	if err := nodeMod.Start(); err != nil {
		slog.Error("节点模块启动失败", "err", err)
		os.Exit(1)
	}
	r.GET("/api/ws/agent", nodeMod.HandleAgent)
	nodeMod.Register(protected, protectedRW, protectedAdmin)

	// 运行时版本管理器（宝塔式：多版本共存/设默认/卸载）
	runtimeMod := rtm.New(db, cfg.DataDir)
	if err := runtimeMod.Start(); err != nil {
		slog.Error("运行时模块启动失败", "err", err)
	}
	runtimeMod.Register(protected)

	// P2 模块
	dash := dashboard.New()
	dash.Register(protected)
	alertMod := alert.New(db, dash)
	if err := alertMod.Start(); err != nil {
		slog.Error("告警模块启动失败", "err", err)
	}
	alertMod.Register(protected)
	appstoreMod := appstore.New(db, cfg.DataDir)
	if err := appstoreMod.Start(); err != nil {
		slog.Error("应用商店模块启动失败", "err", err)
	}
	appstoreMod.Register(protected, protectedAdmin)

	// SPA 静态资源（嵌入前端）
	spaFS := webui.FS()
	fileServer := http.FileServer(http.FS(spaFS))
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if p == "/api" || strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "not found"})
			return
		}
		if !authSvc.SpaAllowed(c) {
			c.String(http.StatusNotFound, "404 page not found")
			return
		}
		if f, err := spaFS.Open(strings.TrimPrefix(p, "/")); err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		webui.ServeIndex(c.Writer, spaFS)
	})

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: r}
	go serve(srv, cfg, cfg.DataDir)

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("正在停止面板...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("面板已停止")
}

func serve(srv *http.Server, cfg *config.Config, dataDir string) {
	var err error
	if cfg.Server.TLS.Enabled {
		cert, key := cfg.Server.TLS.Cert, cfg.Server.TLS.Key
		if cert == "" || key == "" {
			cert, key, err = selfcert.Ensure(filepath.Join(dataDir, "certs"))
			if err != nil {
				slog.Error("生成自签证书失败", "err", err)
				os.Exit(1)
			}
			slog.Info("已生成自签证书", "cert", cert)
		}
		slog.Info("HTTPS 服务已启动", "addr", cfg.Server.Listen)
		err = srv.ListenAndServeTLS(cert, key)
	} else {
		slog.Warn("HTTPS 未启用（可在 config.yaml 中开启 server.tls.enabled）", "addr", cfg.Server.Listen)
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		slog.Error("服务异常退出", "err", err)
		os.Exit(1)
	}
}