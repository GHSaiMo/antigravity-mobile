#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ADMIN_TOKEN_FILE="${HOME}/.antigravity-mobile/admin_token"
DEFAULT_STORE_FILE="${HOME}/.antigravity-mobile/auth_store.json"

read_env_val() {
    local key="$1"
    local file="${PROJECT_DIR}/.env"
    if [ -f "${file}" ]; then
        grep -E "^${key}=" "${file}" 2>/dev/null | tail -n 1 | cut -d'=' -f2- | tr -d '\r' | sed -e 's/^[[:space:]]*["'"'"']//' -e 's/["'"'"'][[:space:]]*$//' || true
    fi
}

# Read configuration safely without sourcing (M-5)
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

ENV_STORE_FILE="$(read_env_val "MULTIGRAVITY_AUTH_STORE_PATH")"
if [ -z "${ENV_STORE_FILE}" ]; then
    ENV_STORE_FILE="$(read_env_val "AUTH_STORE_PATH")"
fi
STORE_FILE="${AUTH_STORE_PATH:-${ENV_STORE_FILE:-${DEFAULT_STORE_FILE}}}"
STORE_FILE="${STORE_FILE/#\~/$HOME}"

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

SSL_VAL="${GATEWAY_SSL:-$(read_env_val "MULTIGRAVITY_SSL")}"
if [ -z "${SSL_VAL}" ]; then
    SSL_VAL="$(read_env_val "GATEWAY_SSL")"
fi
ssl_on=false
case "${SSL_VAL}" in
    1|true|TRUE|yes|YES) ssl_on=true ;;
esac

DDNS_VAL="${DDNS_HOST:-$(read_env_val "DDNS_HOST")}"
TLS_CERT_VAL="${TLS_CERT_FILE:-$(read_env_val "MULTIGRAVITY_TLS_CERT")}"
TLS_KEY_VAL="${TLS_KEY_FILE:-$(read_env_val "MULTIGRAVITY_TLS_KEY")}"

CURL_OPTS=(-sS)
if $ssl_on || { [ -n "${TLS_CERT_VAL:-}" ] && [ -n "${TLS_KEY_VAL:-}" ]; }; then
    if [ -n "${DDNS_VAL:-}" ]; then
        CURL_OPTS=(--resolve "${DDNS_VAL}:${PORT}:127.0.0.1" -k -sS)
        BASE_URL="https://${DDNS_VAL}:${PORT}"
    else
        CURL_OPTS=(-k -sS)
        BASE_URL="https://127.0.0.1:${PORT}"
    fi
else
    BASE_URL="http://127.0.0.1:${PORT}"
fi

# Parse CLI arguments
FORCE_FLAG=false
case "${FORCE:-0}" in
    1|true|TRUE|yes|YES)
        FORCE_FLAG=true
        ;;
esac

TARGET="all"
for arg in "$@"; do
    case "$arg" in
        -f|--force)
            FORCE_FLAG=true
            ;;
        all|"")
            TARGET="all"
            ;;
        *)
            TARGET="$arg"
            ;;
    esac
done

TMP_RESP="$(mktemp)"
trap 'rm -f "${TMP_RESP}"' EXIT

MODE="offline"
JSON_DATA="[]"

# 1. Probe if gateway is running
HTTP_CODE="$(curl "${CURL_OPTS[@]}" -m 2 -o "${TMP_RESP}" -w '%{http_code}' "${BASE_URL}/healthz" 2>/dev/null || true)"

if [ "${HTTP_CODE}" = "200" ]; then
    DEV_CODE="$(curl "${CURL_OPTS[@]}" -m 3 -o "${TMP_RESP}" -w '%{http_code}' "${AUTH_ARGS[@]}" "${BASE_URL}/api/v1/devices" 2>/dev/null || true)"
    if [ "${DEV_CODE}" = "200" ]; then
        MODE="online"
        JSON_DATA="$(cat "${TMP_RESP}")"
    else
        echo "⚠️  网关已在线但获取设备接口返回 HTTP ${DEV_CODE}，回退至本地存储文件检查。" >&2
        if [ -f "${STORE_FILE}" ]; then
            JSON_DATA="$(cat "${STORE_FILE}")"
        fi
    fi
else
    if [ -f "${STORE_FILE}" ]; then
        JSON_DATA="$(cat "${STORE_FILE}")"
    fi
fi

# Check device count using python3
DEV_COUNT=$(python3 -c '
import json, sys
raw = sys.argv[1].strip()
try:
    data = json.loads(raw) if raw else []
    print(len(data) if isinstance(data, list) else 0)
except Exception:
    print(0)
' "${JSON_DATA}")

if [ "${DEV_COUNT}" -eq 0 ]; then
    echo "ℹ️  当前暂无已配对设备，无需清理。"
    exit 0
fi

# If target is a specific device, verify it exists
if [ "${TARGET}" != "all" ]; then
    FOUND=$(python3 -c '
import json, sys
raw = sys.argv[1].strip()
target = sys.argv[2].strip()
try:
    data = json.loads(raw) if raw else []
    match = any(d.get("device_id") == target for d in data)
    print("yes" if match else "no")
except Exception:
    print("no")
' "${JSON_DATA}" "${TARGET}")

    if [ "${FOUND}" != "yes" ]; then
        echo "❌ 未找到 ID 为 '${TARGET}' 的已配对设备。"
        echo "   可执行 make list 查看当前所有已配对设备 ID。"
        exit 1
    fi
fi

# Print pre-clear warning and summary
python3 -c '
import json, sys
raw = sys.argv[1].strip()
target = sys.argv[2].strip()
try:
    data = json.loads(raw)
except Exception:
    data = []

if target == "all":
    print("==================================================")
    print(f"⚠️  准备清除所有已配对设备 (共 {len(data)} 台):")
    for d in data:
        d_id = d.get("device_id", "-")
        d_name = d.get("device_name", "未知设备")
        d_plat = d.get("platform", "未知平台")
        print(f"   • {d_id}: {d_name} ({d_plat})")
    print("⚠️  清除后所有手机/客户端将立即断开连接，必须重新扫码配对。")
    print("==================================================")
else:
    match = [d for d in data if d.get("device_id") == target]
    if match:
        d = match[0]
        d_id = d.get("device_id", "-")
        d_name = d.get("device_name", "未知设备")
        d_plat = d.get("platform", "未知平台")
        print("==================================================")
        print(f"⚠️  准备清除指定设备:")
        print(f"   • {d_id}: {d_name} ({d_plat})")
        print("⚠️  清除后该设备将失去访问权限。")
        print("==================================================")
' "${JSON_DATA}" "${TARGET}"

# Prompt for confirmation if not forced
if [ "${FORCE_FLAG}" = false ]; then
    confirm=""
    prompt_msg="确认清除？[y/N]: "
    if [ "${TARGET}" = "all" ]; then
        prompt_msg="确认清除所有已配对设备？[y/N]: "
    fi

    if [ -t 0 ]; then
        read -r -p "${prompt_msg}" confirm
    else
        read -r confirm || confirm="n"
    fi

    case "${confirm:-}" in
        [yY][eE][sS]|[yY]) ;;
        *)
            echo "❌ 操作已取消，未清除任何设备。"
            exit 0
            ;;
    esac
fi

# Execute clear
CLEARED_COUNT=0

if [ "${MODE}" = "online" ]; then
    # Perform deletion via gateway API
    DEL_URL="${BASE_URL}/api/v1/devices/${TARGET}"
    HTTP_CODE="$(curl "${CURL_OPTS[@]}" -o "${TMP_RESP}" -w '%{http_code}' -X DELETE "${AUTH_ARGS[@]}" "${DEL_URL}" || true)"
    if [ "${HTTP_CODE}" != "200" ]; then
        echo "❌ 网关接口返回错误 (HTTP ${HTTP_CODE}):"
        cat "${TMP_RESP}"
        exit 1
    fi
    CLEARED_COUNT=$(python3 -c '
import json, sys
raw = sys.argv[1].strip()
target = sys.argv[2].strip()
try:
    data = json.loads(raw)
    if "cleared" in data:
        print(data["cleared"])
    else:
        print(1)
except Exception:
    print(1)
' "$(cat "${TMP_RESP}")" "${TARGET}")
else
    # Offline mode: perform file modification
    if [ -f "${STORE_FILE}" ]; then
        cp -f "${STORE_FILE}" "${STORE_FILE}.bak"
    fi
    python3 -c '
import json, sys, os
store_file = sys.argv[1]
target = sys.argv[2]

data = []
if os.path.exists(store_file):
    try:
        with open(store_file, "r") as f:
            data = json.load(f)
    except Exception:
        data = []

if target == "all":
    cleared = len(data)
    with open(store_file, "w") as f:
        json.dump([], f, indent=2)
    print(cleared)
else:
    new_data = [d for d in data if d.get("device_id") != target]
    cleared = len(data) - len(new_data)
    with open(store_file, "w") as f:
        json.dump(new_data, f, indent=2)
    print(cleared)
' "${STORE_FILE}" "${TARGET}" > "${TMP_RESP}"
    CLEARED_COUNT="$(cat "${TMP_RESP}")"
fi

echo
if [ "${TARGET}" = "all" ]; then
    echo "✅ 已成功清除所有已配对设备 (共 ${CLEARED_COUNT} 台)！"
    echo "💡 如需重新配对手机客户端，请执行 \`make pair\` 重新生成二维码扫码。"
else
    echo "✅ 已成功清除设备 '${TARGET}'！"
fi
