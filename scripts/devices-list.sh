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

BASE_URL="http://127.0.0.1:${PORT}"
CURL_OPTS=(-sS)

TMP_RESP="$(mktemp)"
trap 'rm -f "${TMP_RESP}"' EXIT

MODE="offline"
JSON_DATA="[]"

# 1. Probe if gateway is running
HTTP_CODE="$(curl "${CURL_OPTS[@]}" -m 2 -o "${TMP_RESP}" -w '%{http_code}' "${BASE_URL}/healthz" 2>/dev/null || true)"

if [ "${HTTP_CODE}" = "200" ]; then
    # Gateway is online, fetch devices from API
    DEV_CODE="$(curl "${CURL_OPTS[@]}" -m 3 -o "${TMP_RESP}" -w '%{http_code}' "${AUTH_ARGS[@]}" "${BASE_URL}/api/v1/devices" 2>/dev/null || true)"
    if [ "${DEV_CODE}" = "200" ]; then
        MODE="online"
        JSON_DATA="$(cat "${TMP_RESP}")"
    else
        echo "⚠️  网关已在线但获取设备接口返回 HTTP ${DEV_CODE}，回退至本地存储文件读取。" >&2
        if [ -f "${STORE_FILE}" ]; then
            JSON_DATA="$(cat "${STORE_FILE}")"
        fi
    fi
else
    # Gateway is offline, read directly from local store file
    if [ -f "${STORE_FILE}" ]; then
        JSON_DATA="$(cat "${STORE_FILE}")"
    fi
fi

python3 -c '
import json, sys, unicodedata
from datetime import datetime

raw = sys.argv[1].strip()
mode = sys.argv[2]

def str_width(s):
    w = 0
    for ch in str(s):
        if unicodedata.east_asian_width(ch) in ("F", "W"):
            w += 2
        else:
            w += 1
    return w

def pad_str(s, width, align="left"):
    s = str(s)
    cur = str_width(s)
    if cur >= width:
        return s + " "
    pad = " " * (width - cur)
    if align == "right":
        return pad + s + " "
    return s + pad + " "

try:
    devices = json.loads(raw) if raw else []
    if not isinstance(devices, list):
        devices = []
except Exception as e:
    print(f"❌ 解析设备数据失败: {e}", file=sys.stderr)
    sys.exit(1)

status_desc = "网关运行中 (在线实时数据)" if mode == "online" else "网关未运行 (读取本地存储)"

if not devices:
    print("========================================================================================================")
    print(f"📱 Antigravity Mobile 已配对设备列表 (共 0 台 | {status_desc})")
    print("========================================================================================================")
    print("ℹ️  当前暂无已配对设备。")
    print("💡 提示: 执行 make pair 可生成配对二维码与扫码链接。")
    print("========================================================================================================")
    sys.exit(0)

print("========================================================================================================")
print(f"📱 Antigravity Mobile 已配对设备列表 (共 {len(devices)} 台 | {status_desc})")
print("========================================================================================================")

col_id = 18
col_name = 30
col_plat = 8
col_time1 = 20
col_time2 = 20
col_ip = 15

header = (
    pad_str("设备 ID", col_id) +
    pad_str("设备名称", col_name) +
    pad_str("平台", col_plat) +
    pad_str("首次配对时间", col_time1) +
    pad_str("最后活跃时间", col_time2) +
    pad_str("最后 IP", col_ip)
)
print(header)
print("-" * 115)

def format_time(t_str):
    if not t_str:
        return "-"
    try:
        dt = datetime.fromisoformat(t_str)
        return dt.strftime("%Y-%m-%d %H:%M:%S")
    except Exception:
        return t_str[:19].replace("T", " ")

for dev in devices:
    d_id = dev.get("device_id", "-")
    d_name = dev.get("device_name", "未知设备")
    if str_width(d_name) > col_name - 2:
        while str_width(d_name) > col_name - 5:
            d_name = d_name[:-1]
        d_name += "..."
    plat = dev.get("platform", "unknown")
    created = format_time(dev.get("created_at", ""))
    last_seen = format_time(dev.get("last_seen_at", ""))
    last_ip = dev.get("last_seen_ip", "-")

    row = (
        pad_str(d_id, col_id) +
        pad_str(d_name, col_name) +
        pad_str(plat, col_plat) +
        pad_str(created, col_time1) +
        pad_str(last_seen, col_time2) +
        pad_str(last_ip, col_ip)
    )
    print(row)

print("========================================================================================================")
print("💡 提示: 执行 make clear all 可清空所有已配对设备；执行 make pair 可生成新配对二维码。")
' "${JSON_DATA}" "${MODE}"
