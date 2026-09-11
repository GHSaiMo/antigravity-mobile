#!/usr/bin/env bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Read GATEWAY_PORT from .env if present, otherwise default to 58900
PORT="58900"
if [ -f "${PROJECT_DIR}/.env" ]; then
    ENV_PORT=$(grep -E '^GATEWAY_PORT=' "${PROJECT_DIR}/.env" | cut -d'=' -f2 | tr -d ' "\r\n')
    if [ -n "$ENV_PORT" ]; then
        PORT="$ENV_PORT"
    fi
fi
if [ -n "$GATEWAY_PORT" ]; then
    PORT="$GATEWAY_PORT"
fi

# Request new pairing session from running gateway
RESP=$(curl -s -f -X POST "http://127.0.0.1:${PORT}/api/v1/auth/session" 2>/dev/null || true)

if [ -z "$RESP" ]; then
    echo "❌ 无法连接到网关 (http://127.0.0.1:${PORT})，请确认网关是否已启动。"
    echo "   启动网关: make run 或 make tmux-start"
    exit 1
fi

# Parse JSON and render QR Code
python3 -c '
import json, sys, os

try:
    resp_raw = sys.argv[1]
    data = json.loads(resp_raw)
except Exception as e:
    print(f"❌ 解析网关返回失败: {e}", file=sys.stderr)
    sys.exit(1)

code = data.get("code", "")
uri = data.get("uri", "")

if not code or not uri:
    print("❌ 返回数据中缺少 code 或 uri", file=sys.stderr)
    sys.exit(1)

print("==================================================")
print("📱 Antigravity Mobile 客户端扫码一键配对")
print("==================================================")
print(f"🔑 配对码 (5分钟有效):\n   {code}\n")
print(f"🔗 配对链接 (URI):\n   {uri}")
print("==================================================\n")

try:
    import qrcode
    qr = qrcode.QRCode(border=1)
    qr.add_data(uri)
    qr.print_ascii(invert=True)
    print("\n💡 请使用 Antigravity 手机客户端扫描上方二维码完成配对。")
    print("💡 也可以直接复制上述完整链接 (agy://pair...) 粘贴到 App/Web 端。")
except ImportError:
    print("💡 提示: 可直接复制上方的完整配对链接 (agy://pair...) 粘贴到手机客户端完成配对。")
' "$RESP"
