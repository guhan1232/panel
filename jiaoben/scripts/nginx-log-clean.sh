#!/bin/bash
# Nginx 日志轮转清理：按大小轮转 /var/log/nginx 日志并清理过期归档（幂等）
set -e

MAX_MB="{{max_mb}}"
KEEP_DAYS="{{keep_days}}"
LOG_DIR=/var/log/nginx
ARCHIVE=$LOG_DIR/archive

[ -d "$LOG_DIR" ] || { echo "未找到 $LOG_DIR（未安装 nginx 或日志目录不同），跳过"; exit 0; }
mkdir -p "$ARCHIVE"
TS=$(date '+%Y%m%d-%H%M%S')

echo "==> 按大小轮转（阈值 ${MAX_MB}MB）"
for f in "$LOG_DIR"/*.log; do
  [ -f "$f" ] || continue
  SIZE_MB=$(( $(stat -c%s "$f") / 1024 / 1024 ))
  if [ "$SIZE_MB" -ge "$MAX_MB" ]; then
    base=$(basename "$f")
    mv "$f" "$ARCHIVE/${base%.log}.$TS.log"
    kill -USR1 $(cat /run/nginx.pid 2>/dev/null || pgrep -o nginx) 2>/dev/null || true
    echo "  轮转: $base (${SIZE_MB}MB)"
  else
    echo "  跳过: $base (${SIZE_MB}MB)"
  fi
done

echo "==> 清理 ${KEEP_DAYS} 天前归档"
find "$ARCHIVE" -name "*.log" -mtime +"$KEEP_DAYS" -delete 2>/dev/null || true

echo "==> 完成"
