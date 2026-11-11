#!/bin/bash
# 寒语馨面板全功能冒烟测试 v2（34 项核心 API 打点）
# 用法：PANEL_BASE=https://IP:端口 PANEL_ENTRANCE=<入口> PANEL_USER=<用户> PANEL_PASS=<密码> bash smoke.sh
# 判定：HTTP 200 且业务 code==0；空数据也算通过
  # 登录拿会话
  LOGIN_BODY="{\"username\":\"$PANEL_USER\",\"password\":\"$PANEL_PASS\"}"
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

echo ""
echo "================ 冒烟结果: PASS=$PASS FAIL=$FAIL ================"
[ -n "$FAILED" ] && echo -e "$FAILED"
