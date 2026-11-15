#!/bin/bash
# 寒语馨面板全功能冒烟测试 v3（核心 API 打点 + 扫码登录/APP 会话全流程）
# 用法：PANEL_BASE=https://IP:端口 PANEL_ENTRANCE=<入口> PANEL_USER=<用户> PANEL_PASS=<密码> bash smoke.sh
# 判定：HTTP 200 且业务 code==0；空数据也算通过
  # 登录拿会话（PANEL_TOTP 可选：账号开启两因素验证时传入当前动态码）
  LOGIN_BODY="{\"username\":\"$PANEL_USER\",\"password\":\"$PANEL_PASS\""
  [ -n "$PANEL_TOTP" ] && LOGIN_BODY="$LOGIN_BODY,\"code\":\"$PANEL_TOTP\""
  LOGIN_BODY="$LOGIN_BODY}"
  J=$(curl -sk -c - -X POST "$PANEL_BASE/api/auth/login" -H 'Content-Type: application/json' -d "$LOGIN_BODY" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" | grep wm_session | awk '{print $7}')
  echo "$J" > /tmp/sid
  B="wm_entrance=$PANEL_ENTRANCE; wm_session=$J"
BASE=$PANEL_BASE/api
PASS=0; FAIL=0; FAILED=""

t() { # t <名称> <方法> <curl参数...>
  local name="$1" method="$2"; shift 2
  local code body ok=1
  code=$(curl -sk -o /tmp/smoke.json -w '%{http_code}' -b "$B" -X "$method" "$@" 2>/dev/null)
  body=$(head -c 260 /tmp/smoke.json 2>/dev/null)
  [ "$code" = "200" ] || ok=0
  echo "$body" | grep -q '"code":0' || ok=0
  if [ "$ok" = "1" ]; then
    PASS=$((PASS+1)); echo "PASS  $name"
  else
    FAIL=$((FAIL+1)); FAILED="$FAILED
  FAIL $name ($code): $body"
    echo "FAIL  $name ($code): $body"
  fi
}

echo "===== 1. 认证与系统 ====="
t 面板信息 GET $BASE/setting/info
t TLS状态 GET $BASE/setting/tls/status
t 应用源状态 GET $BASE/apps/source

echo "===== 2. 网站 ====="
t nginx状态 GET $BASE/sites/nginx/status
t 站点列表 GET $BASE/sites
t 面板反代 GET $BASE/sites/panel-proxy
t 备份列表 GET $BASE/sites/backups

echo "===== 3. 数据库 ====="
t DB状态 GET $BASE/dbmgr/status
t MySQL库列表 GET $BASE/dbmgr/mysql/databases

echo "===== 4. 证书 ====="
t 证书列表 GET $BASE/certs
t DNS服务商 GET $BASE/certs/dns-providers

echo "===== 5. 防火墙 ====="
t 防火墙状态 GET $BASE/firewall/status
t 防火墙规则 GET $BASE/firewall/rules

echo "===== 6. 运行时/进程/计划任务 ====="
t 进程列表 GET $BASE/proc/list
t 计划任务列表 GET $BASE/tasks
t 运行时列表 GET $BASE/runtimes

echo "===== 7. 文件 ====="
t 文件列表 GET "$BASE/files/list?path=/tmp"

echo "===== 8. 应用商店/Docker ====="
t 模板列表 GET $BASE/apps/templates
t 已安装应用 GET $BASE/apps
t Docker状态 GET $BASE/apps/docker/status
t Docker总览 GET $BASE/apps/docker/overview
t Docker网络 GET $BASE/apps/docker/networks
t Docker存储卷 GET $BASE/apps/docker/volumes
t Docker镜像 GET $BASE/apps/docker/images
t 脚本列表 GET $BASE/apps/scripts

echo "===== 9. 面板数据 ====="
t 节点列表 GET $BASE/nodes
t 告警规则 GET $BASE/alerts/rules
t 通知配置 GET $BASE/notify/config
t 对象存储配置 GET $BASE/s3/config
t 审计日志 GET "$BASE/audit/list?page=1&size=5"
t 会话列表 GET $BASE/auth/sessions
t 用户列表 GET $BASE/users
t 仪表盘 GET $BASE/dashboard/summary

echo "===== 10. 补充 ====="
t 升级检查 GET $BASE/setting/upgrade/check

echo "===== 11. 扫码登录与 APP 会话（v0.41.0）====="
# mark <名称> <0|1>：0 为通过（与 [] 的退出码一致）
mark() {
  if [ "$2" = "0" ]; then
    PASS=$((PASS+1)); echo "PASS  $1"
  else
    FAIL=$((FAIL+1)); FAILED="$FAILED
  FAIL $1"
    echo "FAIL  $1"
  fi
}
QR=$(curl -sk -X POST "$BASE/auth/qr/create" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" -H 'Content-Type: application/json' -d '{"ua":"smoke-web"}')
QR_KEY=$(echo "$QR" | grep -o '"key":"[a-f0-9]\{32\}"' | cut -d'"' -f4)
QR_TEXT=$(echo "$QR" | sed 's/.*"qr_text":"\([^"]*\)".*/\1/' | sed 's/\\u0026/\&/g')
[ -n "$QR_KEY" ]; mark "二维码创建返回 key" $?
echo "$QR_TEXT" | grep -q 'hanyuxin://login' && echo "$QR_TEXT" | grep -q "entrance=$PANEL_ENTRANCE" && echo "$QR_TEXT" | grep -q "key=$QR_KEY"; mark "qr_text 协议/入口码/密钥齐全" $?
curl -sk "$BASE/auth/qr/state?key=$QR_KEY" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" | grep -q '"state":"pending"'; mark "扫码轮询初始 pending" $?
CODE=$(curl -sk -o /dev/null -w '%{http_code}' -X POST "$BASE/auth/qr/confirm" -H 'Content-Type: application/json' -d "{\"key\":\"$QR_KEY\"}" -H "Cookie: wm_entrance=$PANEL_ENTRANCE")
[ "$CODE" = "401" ]; mark "未登录 confirm 被拒 401" $?
# APP 端模拟：client=app 登录（登录需入口码——APP 侧入口码来自二维码或手动录入；
# 登录后携带 Bearer 的后续请求才豁免入口 Cookie；账号开启 2FA 时需带 PANEL_TOTP 动态码）
APP_TOTP=""
[ -n "$PANEL_TOTP" ] && APP_TOTP=",\"code\":\"$PANEL_TOTP\""
APP_LOGIN=$(curl -sk -X POST "$BASE/auth/login" -H 'Content-Type: application/json' -H "Cookie: wm_entrance=$PANEL_ENTRANCE" -d "{\"username\":\"$PANEL_USER\",\"password\":\"$PANEL_PASS\",\"client\":\"app\",\"device_name\":\"smoke-test\"$APP_TOTP}")
APP_TOKEN=$(echo "$APP_LOGIN" | sed 's/.*"token":"\([a-f0-9]\{64\}\)".*/\1/')
AH="Authorization: Bearer $APP_TOKEN"
[ "${#APP_TOKEN}" = "64" ]; mark "APP 登录返回 64 位 token" $?
curl -sk "$BASE/auth/me" -H "$AH" | grep -q '"code":0'; mark "Bearer 免入口访问 me" $?
curl -sk "$BASE/auth/sessions" -H "$AH" | grep -q '"client":"app"'; mark "会话列表含 APP 客户端" $?
curl -sk -X POST "$BASE/auth/qr/confirm" -H "$AH" -H 'Content-Type: application/json' -d "{\"key\":\"$QR_KEY\"}" | grep -q '"code":0'; mark "Bearer 确认扫码 confirm" $?
curl -sk "$BASE/auth/qr/state?key=$QR_KEY" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" | grep -q '"state":"scanned"'; mark "扫码后状态 scanned" $?
curl -sk -X POST "$BASE/auth/qr/approve" -H "$AH" -H 'Content-Type: application/json' -d "{\"key\":\"$QR_KEY\"}" | grep -q '"code":0'; mark "Bearer 确认登录 approve" $?
QR_STATE=$(curl -sk "$BASE/auth/qr/state?key=$QR_KEY" -H "Cookie: wm_entrance=$PANEL_ENTRANCE")
QR_TICKET=$(echo "$QR_STATE" | sed 's/.*"ticket":"\([a-f0-9]\{64\}\)".*/\1/')
[ "${#QR_TICKET}" = "64" ]; mark "确认后下发一次性票据" $?
EX_COOKIE=$(curl -sk -c - -X POST "$BASE/auth/qr/exchange" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" -H 'Content-Type: application/json' -d "{\"key\":\"$QR_KEY\",\"ticket\":\"$QR_TICKET\"}" | grep wm_session | awk '{print $7}')
[ "${#EX_COOKIE}" = "64" ]; mark "exchange 兑换 Web 会话 Cookie" $?
curl -sk "$BASE/auth/me" -H "Cookie: wm_entrance=$PANEL_ENTRANCE; wm_session=$EX_COOKIE" | grep -q '"code":0'; mark "兑换会话可访问 me" $?
curl -sk -X POST "$BASE/auth/qr/exchange" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" -H 'Content-Type: application/json' -d "{\"key\":\"$QR_KEY\",\"ticket\":\"$QR_TICKET\"}" | grep -q '"code":400'; mark "票据一次性（二次兑换业务码 400）" $?
curl -sk "$BASE/app/check-update?platform=android&v=1.0.0" -H "Cookie: wm_entrance=$PANEL_ENTRANCE" | grep -q '"code":0'; mark "APP 更新检查" $?
curl -sk "$BASE/setting/app-config" -H "$AH" | grep -q '"code":0'; mark "APP 配置读取" $?
curl -sk -X POST "$BASE/setting/app-config" -H "$AH" -H 'Content-Type: application/json' -d '{}' | grep -q '"code":0'; mark "APP 配置写入" $?

echo ""
echo "================ 冒烟结果: PASS=$PASS FAIL=$FAIL ================"
[ -n "$FAILED" ] && echo -e "$FAILED"
