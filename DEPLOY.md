# 寒语馨面板 · 部署与验收手册

> 当前测试机：154.8.165.155（Debian 13）——面板 :27958 入口 /s/0cc66583，2026-11-10 验收通过。
>
> 适用版本：v0.34.0（commit 67a9a4e 之后构建）。产物：`bin/hanyuxin`（面板，41MB）、`bin/hanyuxin-agent`（多节点 Agent，18MB）。
> 前端已通过 go:embed 嵌入二进制，无需单独部署静态文件。

## 一、构建产物（本机已就绪）

| 产物 | 说明 | SHA256（前 16 位） |
|---|---|---|
| `bin/hanyuxin` | 面板主程序（linux/amd64，前端已嵌入） | c3394c06a02fadd7 |
| `bin/hanyuxin-agent` | 多节点 Agent（linux/amd64） | 8cb40e90e519d17d |

重新构建（本机无 make 时用等效命令）：

```bash
npm --prefix web run build          # 构建前端（产物嵌入 go:embed）
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/hanyuxin ./cmd/server
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/hanyuxin-agent ./cmd/agent
```

## 二、服务器部署（替换旧 webmblx/digong 二进制）

```bash
# 1. 上传两个二进制到服务器（scp 或面板自己的文件管理）
scp bin/hanyuxin bin/hanyuxin-agent root@<服务器IP>:/root/wmtest/

# 2. 停止旧面板、启动新面板
ssh root@<服务器IP>
cd /root/wmtest
pkill -x digong 2>/dev/null; pkill -x hanyuxin 2>/dev/null; sleep 1
chmod +x hanyuxin hanyuxin-agent
(nohup ./hanyuxin > panel.log 2>&1 < /dev/null &)
sleep 3 && head -1 panel.log        # 应显示「寒语馨面板启动 version=0.34.0」

# 3. 实例单元迁移（v0.26 前装的 native 应用）
# 旧单元名 webmblx-app-ne-121 / digong-app-ne-121 → hanyuxin-app-ne-121：
systemctl stop webmblx-app-ne-121 digong-app-ne-121 2>/dev/null
rm -f /etc/systemd/system/webmblx-app-*.service /etc/systemd/system/digong-app-*.service
systemctl daemon-reload
# 然后在面板「软件商店 → 已安装实例」里对 ne-121 点「升级」重装一次（自动生成 hanyuxin-app-ne-121 单元并启动）
```

正式环境建议改用 systemd 服务（`deploy/hanyuxin.service` + `scripts/install.sh`）而非 nohup。

## 三、验收清单（v0.26 → v0.34 新功能）

面板地址：`http://<IP>:11330/s/<入口路径>`（登录 admin + initial_admin.txt 密码）。

### 软件商店（v0.26）
- [ ] Docker 容器管理页新增「容器」「镜像」Tab：列表/启停/重启/删除/日志/资源统计、镜像清理
- [ ] 任一 Compose 应用实例「编排」按钮 → 修改 YAML → 保存并应用
- [ ] 「部署自定义 Compose」粘贴 YAML 部署
- [ ] 内置模板 13 个（新增 Uptime-Kuma / Alist / n8n / Gitea）

### 应用升级（v0.26）
- [ ] 软件商店 → 实例明细 → 「升级」按钮 → 版本下拉（GitHub Releases / 仓库版本）→ 升级后版本号记录到实例

### 备份（v0.26/v0.28）
- [ ] 数据库页「备份管理」Tab：选择库 → 立即备份 → 列表/下载/删除/恢复（force 确认）
- [ ] 设置页「面板数据备份」→ 下载 ZIP（panel.db 快照 + config.yaml + 证书）

### 证书（v0.27）
- [ ] SSL 证书页签发弹窗：校验方式 HTTP-01/DNS-01（阿里云/DNSPod/Cloudflare），泛域名走 DNS-01
- [ ] 证书「部署到网站」→ 选站点 → HTTPS 启用并强制跳转

### 多节点（v0.27/v0.31）
- [ ] 节点页每节点「文件」抽屉：浏览/上传/下载/删除
- [ ] 「终端」抽屉：xterm 交互式 shell（WS 经 /api/ws/nodeterm/:id）
- [ ] 计划任务表单「执行位置」选远程节点（shell 类经 Agent 通道）
- [ ] 告警规则「监控对象」选远程节点（最新指标判定）
- [ ] 节点指标抽屉「近 30 天」切换（小时聚合）
- [ ] 仪表盘 summary 支持 ?node=<id>

### 面板能力（v0.28~v0.34）
- [ ] 登录 2FA：设置页「两因素验证」绑定 → 退出重登需输动态码
- [ ] 在线会话列表与踢出（设置页）
- [ ] 面板「在线升级」卡片（检查更新/一键升级，自动备份 *.bak）
- [ ] 文件管理 >5MB 上传自动分片（5MB/片进度提示）
- [ ] 审计日志导出 CSV；终端命令含密码自动掩码
- [ ] 顶栏 EN/中 语言切换；≤768px 移动端抽屉/弹窗/表格适配

## 四、升级清单维护

在线自升级依赖仓库根的 `upgrade.json`（版本号 + 二进制 URL + 可选 SHA256）。每次发版需：
1. 更新 `upgrade.json` 的 `version`
2. 将新二进制上传到 `binary_url` 指向的地址（当前指向仓库 raw 的 `release/hanyuxin-linux-amd64`，需在 CNB 仓库上传该文件）
3. 填写产物 SHA256

## 五、回滚

```bash
systemctl stop hanyuxin 2>/dev/null
cp /opt/hanyuxin/hanyuxin.bak /opt/hanyuxin/hanyuxin   # 或 /root/wmtest/hanyuxin.bak
systemctl start hanyuxin || (cd /root/wmtest && nohup ./hanyuxin > panel.log 2>&1 &)
```
