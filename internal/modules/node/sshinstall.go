// sshinstall.go —— SSH 一键接入：面板经 SSH 连接目标机，推送 Agent 二进制，
// 注册 systemd 服务（无 systemd 用 nohup 兜底），等待 Agent 反连注册。
// SSH 凭据即用即弃：不落库、不写日志，仅审计记 host/user/name。
package node

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gossh "golang.org/x/crypto/ssh"

	"cnb.cool/hanyuxin/panel/internal/core"
)

const (
	agentInstallDir = "/opt/hanyuxin-agent"
	agentInstallBin = agentInstallDir + "/hanyuxin-agent"
	agentUnitPath   = "/etc/systemd/system/hanyuxin-agent.service"
	// 无本机 agent 二进制时的下载地址（与在线升级同一仓库 raw 模式）
	agentURLTemplate = "https://cnb.cool/hanyuxin/panel/-/git/raw/main/release/hanyuxin-agent-linux-%s"
)

type sshInstallReq struct {
	Host       string `json:"host" binding:"required"`
	Port       int    `json:"port"`
	User       string `json:"user" binding:"required"`
	AuthType   string `json:"auth_type"` // password（默认）| key
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
	Passphrase string `json:"passphrase"`
	Name       string `json:"name"`                      // 默认取 host
	Server     string `json:"server" binding:"required"` // 面板 WS 地址，如 ws://IP:PORT
	AgentURL   string `json:"agent_url"`                 // 可选覆盖下载地址
}

var nodeNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// sshRun 在 SSH 会话执行命令，带超时（0 = 默认 60s）。
func sshRun(client *gossh.Client, timeout time.Duration, cmd string) (string, int, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", -1, err
	}
	defer sess.Close()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	var out strings.Builder
	sess.Stdout, sess.Stderr = &out, &out
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err := <-done:
		code := 0
		if ee, ok := err.(*gossh.ExitError); ok {
			code = ee.ExitStatus()
		}
		return out.String(), code, err
	case <-time.After(timeout):
		sess.Close()
		return out.String(), -1, fmt.Errorf("命令超时(%s): %s", timeout, cmd)
	}
}

// sshUpload 经会话 stdin 流式上传本地文件（cat > 目标路径），无需额外 SFTP 依赖。
func sshUpload(client *gossh.Client, timeout time.Duration, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	sess.Stdin = f
	done := make(chan error, 1)
	go func() { done <- sess.Run("cat > " + remotePath) }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		sess.Close()
		return fmt.Errorf("上传超时(%s)", timeout)
	}
}

// localAgentBinary 查找面板本机同目录的 agent 二进制（优先带架构后缀）。
func localAgentBinary(arch string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, name := range []string{"hanyuxin-agent-linux-" + arch, "hanyuxin-agent"} {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 1<<20 {
			return p
		}
	}
	return ""
}

// agentURLDefault 默认 Agent 下载地址（按架构）。
func agentURLDefault(arch string) string { return strings.ReplaceAll(agentURLTemplate, "%ARCH%", arch) }

// remoteShellSafe 校验可拼入远程 shell 的参数（禁 shell 元字符，杜绝注入）。
func remoteShellSafe(field, v string) error {
	if v == "" || strings.ContainsAny(v, " \t\r\n\"'`$;&|<>\\") {
		return fmt.Errorf("%s 含非法字符或为空", field)
	}
	return nil
}

// sshInstall SSH 一键安装 Agent 并等待其反连注册。
func (m *Module) sshInstall(c *gin.Context) {
	var req sshInstallReq
	if err := c.ShouldBindJSON(&req); err != nil {
		core.BindErr(c, err)
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	req.User = strings.TrimSpace(req.User)
	req.Name = strings.TrimSpace(req.Name)
	req.Server = strings.TrimSpace(req.Server)
	req.AgentURL = strings.TrimSpace(req.AgentURL)
	if req.Port <= 0 {
		req.Port = 22
	}
	if req.Name == "" {
		req.Name = req.Host
	}
	if !nodeNameRe.MatchString(req.Name) {
		core.Err(c, 400, "节点名仅允许字母数字点短横线下划线（1-64 位）")
		return
	}
	u, err := url.Parse(req.Server)
	if err != nil || u.Host == "" ||
		(u.Scheme != "ws" && u.Scheme != "wss" && u.Scheme != "http" && u.Scheme != "https") {
		core.Err(c, 400, "面板地址需为 ws://、wss://、http:// 或 https:// 开头且含主机")
		return
	}
	if req.AuthType == "" {
		req.AuthType = "password"
	}
	if req.AuthType != "password" && req.AuthType != "key" {
		core.Err(c, 400, "认证方式仅支持 password / key")
		return
	}
	if req.AuthType == "password" && req.Password == "" {
		core.Err(c, 400, "请填写 SSH 密码")
		return
	}
	if req.AuthType == "key" && strings.TrimSpace(req.PrivateKey) == "" {
		core.Err(c, 400, "请填写 SSH 私钥")
		return
	}
	// agent_url 可选覆盖：白名单校验防 shell 注入；默认模板为常量安全
	if req.AgentURL != "" {
		if err := remoteShellSafe("agent_url", req.AgentURL); err != nil ||
			(!strings.HasPrefix(req.AgentURL, "http://") && !strings.HasPrefix(req.AgentURL, "https://")) {
			core.Err(c, 400, "agent_url 需以 http(s):// 开头且不含特殊字符")
			return
		}
	}

	// SSH 认证（凭据即用即弃，不落库不记日志）
	var auths []gossh.AuthMethod
	if req.AuthType == "key" {
		var signer gossh.Signer
		if req.Passphrase != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase([]byte(req.PrivateKey), []byte(req.Passphrase))
		} else {
			signer, err = gossh.ParsePrivateKey([]byte(req.PrivateKey))
		}
		if err != nil {
			core.Err(c, 400, "私钥解析失败: "+err.Error())
			return
		}
		auths = []gossh.AuthMethod{gossh.PublicKeys(signer)}
	} else {
		auths = []gossh.AuthMethod{gossh.Password(req.Password)}
	}
	cfg := &gossh.ClientConfig{
		User: req.User,
		Auth: auths,
		// 首次接入无法预知目标机 hostkey；SSH 通道本身已加密，接受任意指纹
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	client, err := gossh.Dial("tcp", req.Host+":"+strconv.Itoa(req.Port), cfg)
	if err != nil {
		core.Err(c, 500, "SSH 连接失败: "+err.Error())
		return
	}
	defer client.Close()

	// 1. 架构探测
	archOut, _, err := sshRun(client, 15*time.Second, "uname -m")
	if err != nil {
		core.Err(c, 500, "探测架构失败: "+err.Error())
		return
	}
	var arch string
	switch a := strings.TrimSpace(archOut); a {
	case "x86_64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	default:
		core.Err(c, 400, "不支持的架构: "+a)
		return
	}

	// 2. 准备目录
	if _, code, err := sshRun(client, 30*time.Second, "mkdir -p "+agentInstallDir); err != nil || code != 0 {
		core.Err(c, 500, "创建目录失败: "+strings.TrimSpace(archOut))
		return
	}

	// 3. Agent 二进制：本机同目录优先（stdin 流上传），否则目标机 curl 下载
	if local := localAgentBinary(arch); local != "" {
		slog.Info("SSH 接入：上传本机 agent", "local", local, "host", req.Host, "remote", agentInstallBin)
		if err := sshUpload(client, 10*time.Minute, local, agentInstallBin); err != nil {
			core.Err(c, 500, "上传 Agent 失败: "+err.Error())
			return
		}
	} else {
		dlURL := agentURLDefault(arch)
		if req.AgentURL != "" {
			dlURL = req.AgentURL
		}
		slog.Info("SSH 接入：目标机下载 agent", "url", dlURL, "host", req.Host)
		if out, code, err := sshRun(client, 10*time.Minute, "curl -fsSL --retry 3 --connect-timeout 30 -m 600 -o "+agentInstallBin+" "+dlURL); err != nil || code != 0 {
			core.Err(c, 500, fmt.Sprintf("下载 Agent 失败(exit %d)：面板同目录未找到 hanyuxin-agent，且目标机无法从 %s 下载\n%s", code, dlURL, strings.TrimSpace(out)))
			return
		}
	}
	if _, code, err := sshRun(client, 30*time.Second, "chmod 755 "+agentInstallBin); err != nil || code != 0 {
		core.Err(c, 500, "chmod 失败")
		return
	}

	// 4. 注册服务：systemd 优先，nohup 兜底
	launch := agentInstallBin + " -s " + req.Server + " -t " + core.GetSetting(m.db, core.KNodeToken) + " -name " + req.Name
	hasSystemd := false
	if _, _, err := sshRun(client, 15*time.Second, "command -v systemctl"); err == nil {
		hasSystemd = true
	}
	if hasSystemd {
		// unit 内容含 token/地址，base64 传输避免 shell 转义问题
		unit := "[Unit]\nDescription=hanyuxin panel agent\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nExecStart=" + launch + "\nRestart=always\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n"
		b64 := base64.StdEncoding.EncodeToString([]byte(unit))
		if _, code, err := sshRun(client, 30*time.Second, "echo "+b64+" | base64 -d > "+agentUnitPath+" && chmod 600 "+agentUnitPath); err != nil || code != 0 {
			core.Err(c, 500, "写入服务单元失败")
			return
		}
		// 覆盖旧安装：先停旧服务再启用新单元
		_, _, _ = sshRun(client, 30*time.Second, "systemctl stop hanyuxin-agent 2>/dev/null; systemctl daemon-reload && systemctl enable --now hanyuxin-agent")
		if out, _, _ := sshRun(client, 15*time.Second, "systemctl is-active hanyuxin-agent"); strings.TrimSpace(out) != "active" {
			jr, _, _ := sshRun(client, 15*time.Second, "journalctl -u hanyuxin-agent -n 5 --no-pager 2>/dev/null")
			core.Err(c, 500, "Agent 服务未进入 active 状态\n"+strings.TrimSpace(jr))
			return
		}
	} else {
		_, _, _ = sshRun(client, 15*time.Second, "pkill -f "+agentInstallBin+" 2>/dev/null; true")
		if _, code, err := sshRun(client, 15*time.Second, "nohup "+launch+" >> "+agentInstallDir+"/agent.log 2>&1 < /dev/null &"); err != nil || code != 0 {
			core.Err(c, 500, "nohup 启动失败")
			return
		}
	}

	// 5. 等待 Agent 反连注册（最多 15s）
	slog.Info("SSH 接入：等待节点反连注册", "name", req.Name)
	var node *Node
	for i := 0; i < 15; i++ {
		time.Sleep(time.Second)
		var n Node
		if err := m.db.Where("name = ? AND last_seen > ?", req.Name, time.Now().Add(-30*time.Second)).First(&n).Error; err == nil {
			node = &n
			break
		}
	}
	if node == nil {
		core.Err(c, 500, fmt.Sprintf("Agent 已启动但 15 秒内未反连注册：请确认面板地址 %s 对目标机可达（防火墙放行面板端口）", req.Server))
		return
	}
	core.OK(c, gin.H{"node_id": node.ID, "name": node.Name, "arch": arch, "systemd": hasSystemd})
}
