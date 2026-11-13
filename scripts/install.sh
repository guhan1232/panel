#!/bin/bash
# 寒语馨面板一键安装/升级脚本（仓库直连，安装即 HTTPS）
#
# 用法：
#   1) 全新安装（自动下载最新二进制，默认自签 10 年 HTTPS 证书）：
#      curl -fsSL https://cnb.cool/hanyuxin/panel/-/raw/main/scripts/install.sh | bash
#   2) 带中间 CA（叶子证书有效期跟随 CA，多久是多久）：
#      bash install.sh --ca-cert /path/ca.crt --ca-key /path/ca.key
#   3) 升级已安装面板（自动备份旧二进制，保留全部配置与数据）：
#      bash install.sh --upgrade
#   4) 本地二进制（老用法保留）：
#      bash install.sh --bin /path/hanyuxin
#
# 可选参数：
#   --listen :27958   监听地址（默认 :27958；已有配置则尊重现状）
#   --no-firewall     跳过防火墙端口自动放行
#   --force           非升级模式下忽略「已安装」检查强制重装
#
# 说明：正常路径用 CA 签发（有效期跟随 CA）；仅当未提供 CA 时才自签兜底（10 年，含公网 IP SAN）。
set -euo pipefail

REPO="https://cnb.cool/hanyuxin/panel"
INSTALL_DIR=/opt/hanyuxin
SERVICE=hanyuxin
BIN_ARG="" ; CA_CERT_ARG="" ; CA_KEY_ARG="" ; LISTEN_ARG=""
UPGRADE=0 ; NO_FIREWALL=0 ; FORCE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin)        BIN_ARG="$2"; shift 2 ;;
    --ca-cert)    CA_CERT_ARG="$2"; shift 2 ;;
    --ca-key)     CA_KEY_ARG="$2"; shift 2 ;;
    --listen)     LISTEN_ARG="$2"; shift 2 ;;
    --upgrade)    UPGRADE=1; shift ;;
    --no-firewall) NO_FIREWALL=1; shift ;;
    --force)      FORCE=1; shift ;;
    *) echo "未知参数: $1" >&2; exit 1 ;;
  esac
done

log() { echo -e "\033[36m==> \033[0m$*"; }
ok()  { echo -e "\033[32m ✓  \033[0m$*"; }
die() { echo -e "\033[31m安装失败: \033[0m$*" >&2; exit 1; }

[[ "$(uname -s)" == "Linux" ]] || die "面板仅支持 Linux"
[[ $EUID -eq 0 ]] || die "请使用 root 运行"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARTIFACT="hanyuxin-linux-amd64" ;;
  aarch64) ARTIFACT="hanyuxin-linux-arm64" ;;
  *) die "暂不支持架构: $ARCH" ;;
esac

# 依赖预检：CA 签发与配置改写依赖 python3/openssl，提前报错避免装到一半失败
command -v curl >/dev/null || die "需要 curl"
if [[ -n "$CA_CERT_ARG" ]]; then
  command -v python3 >/dev/null || die "CA 签发模式需要 python3（用于写配置）"
  command -v openssl >/dev/null || die "CA 签发模式需要 openssl"
fi

# 升级语义：已安装且未显式 --upgrade 时提示用 --upgrade（--force 覆盖此检查）
if [[ -x "$INSTALL_DIR/hanyuxin" && "$UPGRADE" == "0" && "$FORCE" == "0" && -z "$BIN_ARG" ]]; then
  echo -e "\033[33m检测到已安装 $INSTALL_DIR/hanyuxin。\033[0m"
  echo "  升级版本请使用: bash $0 --upgrade"
  echo "  强制重装请使用: bash $0 --force"
  exit 1
fi

# ---- 1. 获取二进制 ----
mkdir -p "$INSTALL_DIR"
if [[ -n "$BIN_ARG" ]]; then
  log "使用本地二进制: $BIN_ARG"
  cp -f "$BIN_ARG" "$INSTALL_DIR/hanyuxin.tmp"
else
  log "从仓库获取面板二进制（$ARTIFACT）..."
  MANIFEST="$(curl -fsSL "$REPO/-/raw/main/upgrade.json" 2>/dev/null || true)"
  BIN_URL="$(echo "$MANIFEST" | tr ',' '\n' | grep '"binary_url"' | head -1 | sed 's/.*: *"\(.*\)".*/\1/' || true)"
  BIN_SHA="$(echo "$MANIFEST" | tr ',' '\n' | grep '"sha256"' | head -1 | sed 's/.*: *"\(.*\)".*/\1/' || true)"
  if [[ -z "$BIN_URL" ]]; then
    BIN_URL="$REPO/-/raw/main/release/$ARTIFACT"
  fi
  curl -fL --retry 3 --retry-delay 2 -o "$INSTALL_DIR/hanyuxin.tmp" "$BIN_URL" || die "二进制下载失败: $BIN_URL"
  # 校验是 ELF 而非 HTML 错误页
  head -c 4 "$INSTALL_DIR/hanyuxin.tmp" | grep -q "ELF" || die "下载内容不是有效的 ELF 二进制（检查仓库 release 是否已上传）"
  # 清单带 sha256 时校验完整性
  if [[ -n "$BIN_SHA" ]] && command -v sha256sum >/dev/null; then
    echo "$BIN_SHA  $INSTALL_DIR/hanyuxin.tmp" | sha256sum -c - >/dev/null 2>&1 || die "SHA256 校验失败（下载不完整或被篡改）"
    ok "SHA256 校验通过"
  fi
  # 升级模式：备份旧二进制（失败可一键回滚）
  if [[ "$UPGRADE" == "1" && -f "$INSTALL_DIR/hanyuxin" ]]; then
    BAK="$INSTALL_DIR/hanyuxin.bak-$(date +%Y%m%d%H%M%S)"
    cp -f "$INSTALL_DIR/hanyuxin" "$BAK"
    log "已备份旧版本 -> $BAK（回滚: cp $BAK $INSTALL_DIR/hanyuxin && systemctl restart $SERVICE）"
  fi
  mv -f "$INSTALL_DIR/hanyuxin.tmp" "$INSTALL_DIR/hanyuxin"
fi
chmod +x "$INSTALL_DIR/hanyuxin"

# ---- 2. 生成基础配置（首装才生成；已有配置尊重现状） ----
if [[ ! -f "$INSTALL_DIR/config.yaml" ]]; then
  log "生成配置（默认启用 HTTPS）..."
  ENT=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' | cut -c1-8)
  cat > "$INSTALL_DIR/config.yaml" <<EOF
server:
  listen: ${LISTEN_ARG:-:27958}
  entrance: $ENT
  tls:
    enabled: true
data_dir: data
log_dir: logs
log_level: info
EOF
fi

# ---- 3. 证书：CA 签发（跟随 CA 有效期）优先，缺省面板自启自签 10 年兜底 ----
if [[ -n "$CA_CERT_ARG" && -n "$CA_KEY_ARG" ]]; then
  log "使用中间 CA 签发面板证书（有效期跟随 CA，多久是多久）..."
  openssl x509 -in "$CA_CERT_ARG" -noout >/dev/null 2>&1 || die "CA 证书无效"
  mkdir -p "$INSTALL_DIR/certs"
  PANEL_IP=$(curl -fss --max-time 5 ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')
  PANEL_IP=${PANEL_IP:-127.0.0.1}
  # CA 剩余可签天数（多久是多久）
  END_STR=$(openssl x509 -in "$CA_CERT_ARG" -noout -enddate | cut -d= -f2)
  if date -d "$END_STR" +%s >/dev/null 2>&1; then
    CA_DAYS=$(( ($(date -d "$END_STR" +%s) - $(date +%s)) / 86400 ))
  else
    CA_DAYS=3650
  fi
  [[ "$CA_DAYS" -gt 1 ]] || CA_DAYS=3650
  cat > "$INSTALL_DIR/certs/panel.ext" <<EOF
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=IP:${PANEL_IP},IP:127.0.0.1,IP:::1,DNS:localhost
EOF
  openssl ecparam -genkey -name prime256v1 -out "$INSTALL_DIR/certs/panel.key" 2>/dev/null
  openssl req -new -key "$INSTALL_DIR/certs/panel.key" -subj "/CN=${PANEL_IP}/O=mianban" -out "$INSTALL_DIR/certs/panel.csr"
  openssl x509 -req -in "$INSTALL_DIR/certs/panel.csr" -CA "$CA_CERT_ARG" -CAkey "$CA_KEY_ARG" \
    -CAcreateserial -days "$CA_DAYS" -extfile "$INSTALL_DIR/certs/panel.ext" \
    -out "$INSTALL_DIR/certs/panel.crt" 2>/dev/null || die "CA 签发失败（检查 CA 证书与私钥是否匹配）"
  rm -f "$INSTALL_DIR/certs/panel.csr"
  cat "$INSTALL_DIR/certs/panel.crt" "$CA_CERT_ARG" > "$INSTALL_DIR/certs/panel.chain.crt"
  log "CA 签发完成（剩余 $CA_DAYS 天），证书链已拼接"
  HAVE_CA_CERT=1
else
  HAVE_CA_CERT=0
  log "未提供 CA —— 面板首次启动将自动自签证书（10 年，含公网 IP SAN）"
fi

# ---- 4. 写入证书路径到 config（CA 签发时；自签则留空由面板 Ensure 生成） ----
if [[ "$HAVE_CA_CERT" == "1" ]]; then
  python3 - "$INSTALL_DIR/config.yaml" "$INSTALL_DIR" <<'PY'
import sys, re
p, d = sys.argv[1], sys.argv[2]
s = open(p).read()
if re.search(r'^  tls:', s, re.M):
    s = re.sub(r'^  tls:\n(?:    .*\n)*', f'  tls:\n    enabled: true\n    cert: {d}/certs/panel.chain.crt\n    key: {d}/certs/panel.key\n', s, flags=re.M)
else:
    s = s.replace('server:\n', f'server:\n  tls:\n    enabled: true\n    cert: {d}/certs/panel.chain.crt\n    key: {d}/certs/panel.key\n', 1)
open(p, 'w').write(s)
PY
fi

# ---- 5. systemd 服务 ----
cat > "/etc/systemd/system/${SERVICE}.service" <<EOF
[Unit]
Description=寒语馨面板 (Hanyuxin Panel) Linux Web Panel
After=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/hanyuxin -c $INSTALL_DIR/config.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65536
User=root

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable "$SERVICE" >/dev/null 2>&1 || true
systemctl restart "$SERVICE"

# ---- 6. 轮询等待启动（最多 30 秒，比固定 sleep 更稳） ----
log "等待面板启动..."
UP=0
for i in $(seq 1 30); do
  systemctl is-active --quiet "$SERVICE" && UP=1 && break
  sleep 1
done
if [[ "$UP" != "1" ]]; then
  journalctl -u "$SERVICE" --no-pager -n 20
  die "服务启动失败，见上方日志"
fi
ok "面板服务运行中"

# ---- 7. 防火墙自动放行面板端口（--no-firewall 跳过） ----
PORT=$(grep -E '^\s*listen:' "$INSTALL_DIR/config.yaml" 2>/dev/null | awk '{print $2}' | tr -d ':')
PORT=${PORT:-27958}
if [[ "$NO_FIREWALL" == "0" ]]; then
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
    ufw allow "$PORT/tcp" >/dev/null 2>&1 && ok "ufw 已放行 $PORT/tcp" || log "ufw 放行失败，请手动放行 $PORT/tcp"
  elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state 2>/dev/null | grep -q running; then
    firewall-cmd --permanent --add-port="$PORT/tcp" >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
    ok "firewalld 已放行 $PORT/tcp"
  else
    log "未检测到启用中的防火墙（如无法访问请检查云安全组放行 $PORT/tcp）"
  fi
fi

# ---- 8. 输出访问信息 ----
ENTRANCE=$(grep -E '^\s*entrance:' "$INSTALL_DIR/config.yaml" 2>/dev/null | awk '{print $2}')
IP=$(curl -fss --max-time 5 ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')
SCHEME=https
grep -qE 'enabled:\s*false' "$INSTALL_DIR/config.yaml" 2>/dev/null && SCHEME=http
URL="$SCHEME://$IP:${PORT}/s/${ENTRANCE:-<见 config.yaml>}"

echo ""
echo "=============================================="
echo " 寒语馨面板${UPGRADE:+升级}完成"
echo " 访问地址: $URL"
[[ "$SCHEME" == "https" ]] && echo " 证书说明: $([[ $HAVE_CA_CERT == 1 ]] && echo '中间 CA 签发（有效期跟随 CA）' || echo '自签 10 年（浏览器提示不受信任属正常，可导入 CA 后消除）')"
if [[ "$UPGRADE" == "1" ]]; then
  echo " 升级模式: 配置与数据未变动；异常回滚: cp $INSTALL_DIR/hanyuxin.bak-* $INSTALL_DIR/hanyuxin"
else
  echo " 初始账号: cat $INSTALL_DIR/data/initial_admin.txt（改密后自动删除）"
fi
echo " 移动 APP: 登录面板 → 系统设置 → 移动 APP 扫码绑定"
echo " 服务管理: systemctl {status|restart|stop} $SERVICE"
echo " 配置文件: $INSTALL_DIR/config.yaml"
echo "=============================================="
