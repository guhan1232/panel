#!/bin/bash
# 系统基础优化：时区 / Swap / 句柄数 / 常用内核参数（幂等）
set -e

TZ_NAME="{{timezone}}"
SWAP_MB="{{swap_size}}"

echo "==> 设置时区: $TZ_NAME"
if command -v timedatectl >/dev/null 2>&1; then
  timedatectl set-timezone "$TZ_NAME" 2>/dev/null || echo "  timedatectl 不可用，跳过"
else
  ln -sf "/usr/share/zoneinfo/$TZ_NAME" /etc/localtime 2>/dev/null || echo "  时区文件不存在，跳过"
fi

echo "==> Swap 配置（目标 ${SWAP_MB}MB）"
CUR_SWAP=$(swapon --noheadings 2>/dev/null | awk '{print $3}' | grep -m1 -E '^[0-9]+$' || echo 0)
if [ "$SWAP_MB" = "0" ]; then
  echo "  跳过 swap 创建"
elif [ "${CUR_SWAP:-0}" -ge "$SWAP_MB" ] 2>/dev/null; then
  echo "  当前 swap(${CUR_SWAP}MB) 已满足，跳过"
elif [ ! -f /swapfile ]; then
  fallocate -l "${SWAP_MB}M" /swapfile && chmod 600 /swapfile && mkswap /swapfile >/dev/null && swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
  echo "  已创建并启用 ${SWAP_MB}MB swap"
else
  echo "  /swapfile 已存在，跳过（如需扩容请手动处理）"
fi

echo "==> 内核参数 / 句柄数"
CONF=/etc/sysctl.d/99-hanyuxin-tune.conf
cat > "$CONF" <<'EOF'
vm.swappiness = 10
net.core.somaxconn = 4096
net.ipv4.tcp_fastopen = 3
fs.file-max = 1000000
EOF
sysctl -p "$CONF" >/dev/null 2>&1 || true
LIMITS=/etc/security/limits.d/99-hanyuxin.conf
cat > "$LIMITS" <<'EOF'
* soft nofile 65535
* hard nofile 65535
root soft nofile 65535
root hard nofile 65535
EOF

echo "==> 完成（参数文件: $CONF, $LIMITS）"
