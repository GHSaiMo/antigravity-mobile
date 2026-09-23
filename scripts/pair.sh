#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ADMIN_TOKEN_FILE="${HOME}/.antigravity-mobile/admin_token"

read_env_val() {
    local key="$1"
    local file="${PROJECT_DIR}/.env"
    if [ -f "${file}" ]; then
        grep -E "^${key}=" "${file}" 2>/dev/null | tail -n 1 | cut -d'=' -f2- | tr -d '\r' | sed -e 's/^[[:space:]]*["'"'"']//' -e 's/["'"'"'][[:space:]]*$//' || true
    fi
}

# Read GATEWAY_PORT / ADMIN_TOKEN from .env safely without sourcing (M-5)
PORT="58900"
ENV_PORT="$(read_env_val "MULTIGRAVITY_PORT")"
if [ -z "${ENV_PORT}" ]; then
    ENV_PORT="$(read_env_val "GATEWAY_PORT")"
fi
if [ -n "${ENV_PORT}" ]; then
    PORT="${ENV_PORT}"
fi
if [ -n "${GATEWAY_PORT:-}" ]; then
    PORT="$GATEWAY_PORT"
fi

if [ -z "${ADMIN_TOKEN:-}" ]; then
    ADMIN_TOKEN="$(read_env_val "MULTIGRAVITY_ADMIN_TOKEN")"
    if [ -z "${ADMIN_TOKEN}" ]; then
        ADMIN_TOKEN="$(read_env_val "ADMIN_TOKEN")"
    fi
fi
if [ -z "${ADMIN_TOKEN:-}" ] && [ -f "${ADMIN_TOKEN_FILE}" ]; then
    ADMIN_TOKEN="$(tr -d ' \r\n' < "${ADMIN_TOKEN_FILE}")"
fi

AUTH_ARGS=()
if [ -n "${ADMIN_TOKEN:-}" ]; then
    AUTH_ARGS=(-H "Authorization: Bearer ${ADMIN_TOKEN}")
fi

PAIR_URL="http://127.0.0.1:${PORT}/api/v1/auth/session"
CURL_OPTS=()

TMP_BODY="$(mktemp)"
trap 'rm -f "${TMP_BODY}"' EXIT

HTTP_CODE="$(curl -sS -o "${TMP_BODY}" -w '%{http_code}' -X POST \
    "${CURL_OPTS[@]}" \
    "${AUTH_ARGS[@]}" \
    "${PAIR_URL}" || true)"

if [ -z "${HTTP_CODE}" ] || [ "${HTTP_CODE}" = "000" ]; then
    echo "❌ 无法连接到网关 (${PAIR_URL})，请确认网关是否已启动。"
    echo "   启动网关: make run 或 make tmux-start"
    exit 1
fi

RESP="$(cat "${TMP_BODY}")"

if [ "${HTTP_CODE}" != "200" ]; then
    echo "❌ 网关拒绝签发配对码 (HTTP ${HTTP_CODE})"
    if [ -n "${RESP}" ]; then
        echo "   ${RESP}"
    fi
    if [ "${HTTP_CODE}" = "401" ]; then
        echo
        echo "   提示: 管理员鉴权失败，请确认令牌已配置在环境变量或:"
        echo "     ${ADMIN_TOKEN_FILE}"
    fi
    exit 1
fi

python3 -c '
import json, sys

try:
    data = json.loads(sys.argv[1])
except Exception as e:
    print(f"❌ 解析网关返回失败: {e}", file=sys.stderr)
    sys.exit(1)

code = data.get("code", "")
uri = data.get("uri", "")

if not code or not uri:
    print("❌ 返回数据中缺少 code 或 uri", file=sys.stderr)
    print(sys.argv[1], file=sys.stderr)
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
