#!/usr/bin/env bash
# 对本机正在运行的 language_server 发一个 ConnectRPC(JSON) 请求，用于逆向探测接口形状。
#
# 用法：scripts/ls-rpc-probe.sh <Method> ['{"json":"body"}']
#   scripts/ls-rpc-probe.sh GetAvailableModels
#   scripts/ls-rpc-probe.sh SearchConversations '{"query":"gateway"}' | jq .
#
# 约束：
# - 仅限探测只读方法。SendUserCascadeMessage / ManageSidecar / SendAllQueuedMessages /
#   Git* 等有副作用的方法不要随手调用；探测其必填字段时请用 '{}'，看校验报错即可。
# - 从进程参数读取 CSRF token 与监听端口，不会打印 token。
# - 请求/响应字段名多为 camelCase（如 conversationId / cascadeId / stepIndex），
#   报错信息里的 snake_case（如 project_path is required）只是提示缺字段。
set -euo pipefail
[[ $# -ge 1 ]] || { sed -n 2,12p "$0"; exit 2; }
PID="$(pgrep -f 'language_server( |$)' | while read -r p; do
  ps -o args= -p "$p" | grep -q -- '--csrf_token' && { echo "$p"; break; }; done)"
[[ -n "$PID" ]] || { echo "未找到运行中的 language_server" >&2; exit 3; }
CSRF="$(ps -o args= -p "$PID" | grep -oE -- '--csrf_token +[0-9a-fA-F-]+' | awk '{print $2}')"
BODY="${2:-"{}"}"
for P in $(lsof -nP -a -p "$PID" -iTCP -sTCP:LISTEN 2>/dev/null | awk 'NR>1{n=split($9,a,":"); print a[n]}'); do
  R="$(curl -sk -m 10 -X POST "https://127.0.0.1:$P/exa.language_server_pb.LanguageServerService/$1" \
        -H 'Content-Type: application/json' -H 'Connect-Protocol-Version: 1' \
        -H "x-codeium-csrf-token: $CSRF" -d "$BODY" || true)"
  [[ -n "$R" ]] && { echo "$R"; exit 0; }
done
echo "无响应（language_server 可能刚重启，或使用了 http 端口）" >&2; exit 4
