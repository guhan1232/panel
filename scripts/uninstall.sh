#!/bin/bash
# 帝弓面板卸载脚本（可选保留数据）
# 用法: sudo bash uninstall.sh [--keep-data]
set -e

INSTALL_DIR=/opt/digong
KEEP=0
[ "${1:-}" = "--keep-data" ] && KEEP=1

[ "$(id -u)" -ne 0 ] && { echo "请使用 root 运行"; exit 1; }

systemctl disable --now digong 2>/dev/null || true
rm -f /etc/systemd/system/digong.service
systemctl daemon-reload

if [ "$KEEP" -eq 1 ]; then
  echo "已停止并移除服务，数据保留在 $INSTALL_DIR"
else
  rm -rf "$INSTALL_DIR"
  echo "已完全卸载"
fi
