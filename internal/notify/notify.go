// Package notify 通知渠道：邮件 SMTP + IM Webhook（钉钉/企业微信/飞书/通用）。
package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

// EmailConfig 邮件通知配置。
type EmailConfig struct {
	Enabled bool     `json:"enabled"`
	Host    string   `json:"host"`
	Port    int      `json:"port"`
	User    string   `json:"user"`
	Pass    string   `json:"pass"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	SSL     bool     `json:"ssl"` // 465 隐式 TLS；否则 STARTTLS/明文
}

// WebhookConfig IM Webhook 配置。
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"` // generic | dingtalk | wecom | feishu
	URL     string `json:"url"`
}

func GetEmail(db *gorm.DB) EmailConfig {
	var c EmailConfig
	_ = json.Unmarshal([]byte(core.GetSetting(db, core.KNotifyEmail)), &c)
	if c.Port == 0 {
		c.Port = 465
	}
	return c
}

func SaveEmail(db *gorm.DB, c EmailConfig) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return core.SetSetting(db, core.KNotifyEmail, string(data))
}

func GetWebhook(db *gorm.DB) WebhookConfig {
	var c WebhookConfig
	_ = json.Unmarshal([]byte(core.GetSetting(db, core.KNotifyWebhook)), &c)
	return c
}

func SaveWebhook(db *gorm.DB, c WebhookConfig) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return core.SetSetting(db, core.KNotifyWebhook, string(data))
}

// Send 向所有启用渠道推送通知（失败仅记日志，不阻塞调用方）。
func Send(db *gorm.DB, title, content string) {
	if e := GetEmail(db); e.Enabled {
		if err := sendMail(e, title, content); err != nil {
			slog.Warn("邮件通知发送失败", "err", err)
		} else {
			slog.Info("邮件通知已发送", "to", strings.Join(e.To, ","))
		}
	}
	if w := GetWebhook(db); w.Enabled {
		if err := sendWebhook(w, title, content); err != nil {
			slog.Warn("Webhook 通知发送失败", "err", err)
		} else {
			slog.Info("Webhook 通知已发送", "type", w.Type)
		}
	}
}

func sendMail(c EmailConfig, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	from := c.From
	if from == "" {
		from = c.User
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(c.To, ","))
	fmt.Fprintf(&buf, "Subject: =?UTF-8?B?%s?=\r\n", base64.StdEncoding.EncodeToString([]byte(subject)))
	buf.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(body)

	auth := smtp.PlainAuth("", c.User, c.Pass, c.Host)
	if c.SSL {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: c.Host})
		if err != nil {
			return err
		}
		defer conn.Close()
		cl, err := smtp.NewClient(conn, c.Host)
		if err != nil {
			return err
		}
		defer cl.Close()
		if err := cl.Auth(auth); err != nil {
			return err
		}
		if err := cl.Mail(from); err != nil {
			return err
		}
		for _, to := range c.To {
			if err := cl.Rcpt(to); err != nil {
				return err
			}
		}
		w, err := cl.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return cl.Quit()
	}
	return smtp.SendMail(addr, auth, from, c.To, buf.Bytes())
}

func sendWebhook(c WebhookConfig, title, content string) error {
	text := title + "\n" + content
	var payload any
	switch c.Type {
	case "dingtalk", "wecom":
		payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
	case "feishu":
		payload = map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
	default:
		payload = map[string]string{"title": title, "content": content}
	}
	data, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(c.URL, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b := make([]byte, 256)
		n, _ := resp.Body.Read(b)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b[:n]))
	}
	return nil
}

// ---- 路由 ----

type Module struct{ db *gorm.DB }

func New(db *gorm.DB) *Module { return &Module{db: db} }

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/notify")
	g.GET("/config", m.getConfig)
	g.POST("/email", m.saveEmail)
	g.POST("/webhook", m.saveWebhook)
	g.POST("/test", m.test)
}

func (m *Module) getConfig(c *gin.Context) {
	e := GetEmail(m.db)
	if e.Pass != "" {
		e.Pass = "******"
	}
	core.OK(c, gin.H{"email": e, "webhook": GetWebhook(m.db)})
}

func (m *Module) saveEmail(c *gin.Context) {
	var req EmailConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if req.Pass == "******" { // 未修改密码则保留原值
		req.Pass = GetEmail(m.db).Pass
	}
	if err := SaveEmail(m.db, req); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) saveWebhook(c *gin.Context) {
	var req WebhookConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	if err := SaveWebhook(m.db, req); err != nil {
		core.Err(c, 500, err.Error())
		return
	}
	core.OK(c, nil)
}

func (m *Module) test(c *gin.Context) {
	var req struct {
		Channel string `binding:"required,oneof=email webhook"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	title := "webmblx 测试通知"
	content := "这是一条测试通知，发送时间 " + time.Now().Format("2006-01-02 15:04:05")
	var err error
	if req.Channel == "email" {
		e := GetEmail(m.db)
		err = sendMail(e, title, content)
	} else {
		w := GetWebhook(m.db)
		err = sendWebhook(w, title, content)
	}
	if err != nil {
		core.Err(c, 500, "发送失败: "+err.Error())
		return
	}
	core.OK(c, nil)
}
