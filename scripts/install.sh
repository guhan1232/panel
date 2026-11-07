#!/bin/bash
# webmblx 一键安装脚本（Debian/Ubuntu/CentOS/Rocky）
# 用法: sudo bash install.sh <webmblx二进制路径>
set -e

INSTALL_DIR=/opt/webmblx
BIN="${1:-}"

if [ -z "$BIN" ]; then
  echo "用法: sudo bash $0 <webmblx二进制路径>"
  echo "示例: sudo bash $0 ./webmblx"
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行本脚本"
  exit 1
fi

echo "==> 安装目录: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
cp -f "$BIN" "$INSTALL_DIR/webmblx"
chmod +x "$INSTALL_DIR/webmblx"

echo "==> 写入 systemd 服务"
cat > /etc/systemd/system/webmblx.service <<EOF
[Unit]
Description=webmblx Linux Web Panel
After=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/webmblx -c $INSTALL_DIR/config.yaml
Restart=on-failure
RestartSec=3
LimitNOFILE=65536
User=root

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now webmblx
sleep 2

PORT=$(grep -E '^\s*listen:' "$INSTALL_DIR/config.yaml" 2>/dev/null | awk '{print $2}')
ENTRANCE=$(grep -E '^\s*entrance:' "$INSTALL_DIR/config.yaml" 2>/dev/null | awk '{print $2}')
PORT=${PORT:-":8443"}

echo ""
echo "=============================================="
echo " webmblx 安装完成"
echo " 访问入口: http://<服务器IP>${PORT}/s/${ENTRANCE}"
echo " 初始密码: cat $INSTALL_DIR/data/initial_admin.txt"
echo " 服务管理: systemctl {status|restart|stop} webmblx"
echo " 配置文件: $INSTALL_DIR/config.yaml"
echo " 建议尽快: 启用 HTTPS、修改初始密码"
echo "=============================================="
