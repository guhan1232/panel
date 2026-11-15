#!/bin/bash
# Cloudflare DDNS：把本机公网 IP 更新到指定 A 记录（需 API Token 与 Zone DNS 编辑权限）
set -e

TOKEN="{{cf_token}}"
ZONE="{{zone}}"
RECORD="{{record}}"

if [ -z "$TOKEN" ] || [ "$TOKEN" = "{{cf_token}}" ]; then
  echo "错误: 请填写 Cloudflare API Token"; exit 1
fi
[ "$ZONE" = "{{zone}}" ] && ZONE="example.com"

API=https://api.cloudflare.com/client/v4
H1="Authorization: Bearer $TOKEN"
H2="Content-Type: application/json"

# 当前公网 IP（IPv4 优先）
IP=$(curl -fsS4 --max-time 8 https://ifconfig.me/ip 2>/dev/null || curl -fsS4 --max-time 8 https://api.ipify.org 2>/dev/null)
[ -n "$IP" ] || { echo "错误: 获取公网 IP 失败"; exit 1; }
echo "当前公网 IPv4: $IP"

# Zone ID
ZONE_ID=$(curl -fsS --max-time 10 -H "$H1" "$API/zones?name=$ZONE" | grep -oE '"id":"[a-f0-9]{32}"' | head -1 | cut -d'"' -f4)
[ -n "$ZONE_ID" ] || { echo "错误: 找不到 Zone $ZONE（检查 Token 权限/域名）"; exit 1; }

# 记录 ID（已存在则更新，否则创建）
REC_ID=$(curl -fsS --max-time 10 -H "$H1" "$API/zones/$ZONE_ID/dns_records?type=A&name=$RECORD" | grep -oE '"id":"[a-f0-9]{32}"' | head -1 | cut -d'"' -f4)
BODY="{\"type\":\"A\",\"name\":\"$RECORD\",\"content\":\"$IP\",\"ttl\":120,\"proxied\":false}"

if [ -n "$REC_ID" ]; then
  R=$(curl -fsS -X PUT --max-time 10 -H "$H1" -H "$H2" -d "$BODY" "$API/zones/$ZONE_ID/dns_records/$REC_ID")
  echo "已更新 A 记录 $RECORD -> $IP"
else
  R=$(curl -fsS -X POST --max-time 10 -H "$H1" -H "$H2" -d "$BODY" "$API/zones/$ZONE_ID/dns_records")
  echo "已创建 A 记录 $RECORD -> $IP"
fi
echo "$R" | grep -q '"success":true' && echo "DDNS 完成" || { echo "DDNS 失败: $R"; exit 1; }
