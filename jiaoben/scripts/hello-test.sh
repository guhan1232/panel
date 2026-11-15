#!/bin/bash
# 连通性测试脚本：验证脚本应用通道（无副作用）
set -e

echo "=== 寒语馨面板 · 脚本应用连通性测试 ==="
echo "时间: $(date '+%F %T')"
echo "主机: $(hostname)"
echo "内核: $(uname -r)"
echo "实例名(环境变量): ${PANEL_APP_NAME:-未注入}"
echo "参数 note: {{note}}"

# 落一个结果文件便于面板日志审计
echo "hello-test OK $(date '+%s')" >> /tmp/hanyuxin-hello-test.log
echo "=== 完成 ==="
