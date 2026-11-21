#!/usr/bin/env bash
set -euo pipefail
umask 077

SESSION_NAME="${MGY_TMUX_SESSION:-mgy}"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DATA_DIR="${MULTIGRAVITY_DATA_DIR:-${HOME}/.multigravity}"
DATA_DIR="${DATA_DIR/#\~/$HOME}"
LOG_DIR="${PROJECT_DIR}/logs"
LOG_FILE="${LOG_DIR}/gateway.log"

mkdir -p -m 700 "${LOG_DIR}"
chmod 700 "${LOG_DIR}" 2>/dev/null || true
touch "${LOG_FILE}"
chmod 600 "${LOG_FILE}" 2>/dev/null || true

# MED-05: Parse .env safely without `source` to prevent shell command injection.
read_env_val() {
    local key="$1"
    local val=""
    for file in "${PROJECT_DIR}/.env" "${DATA_DIR}/.env"; do
        if [ -f "${file}" ]; then
            local found
            found="$(grep -E "^${key}=" "${file}" 2>/dev/null | tail -n 1 | cut -d'=' -f2- | tr -d '\r' | sed -e 's/^[[:space:]]*["'"'"']//' -e 's/["'"'"'][[:space:]]*$//' || true)"
            if [ -n "${found}" ]; then
                val="${found}"
                break
            fi
        fi
    done
    printf '%s' "${val}"
}

PORT="${MULTIGRAVITY_PORT:-${GATEWAY_PORT:-}}"
if [ -z "${PORT}" ]; then
    PORT="$(read_env_val "MULTIGRAVITY_PORT")"
fi
if [ -z "${PORT}" ]; then
    PORT="$(read_env_val "GATEWAY_PORT")"
fi
PORT="${PORT:-58900}"

if ! [[ "${PORT}" =~ ^[0-9]+$ ]] || [ "${PORT}" -lt 1 ] || [ "${PORT}" -gt 65535 ]; then
    echo "❌ Invalid gateway port: ${PORT}" >&2
    exit 1
fi

BIN_CMD=""
if [ -x "${PROJECT_DIR}/bin/mgy" ]; then
    BIN_CMD="${PROJECT_DIR}/bin/mgy"
elif [ -x "${PROJECT_DIR}/bin/gateway" ]; then
    BIN_CMD="${PROJECT_DIR}/bin/gateway"
elif command -v mgy >/dev/null 2>&1; then
    BIN_CMD="$(command -v mgy)"
else
    echo "❌ Gateway binary not found (expected ./bin/mgy or mgy in PATH)." >&2
    exit 1
fi

# Check if session already exists
if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
    echo "⚠️  tmux session '${SESSION_NAME}' is already running."
    echo "   View logs: tail -f ${LOG_FILE}"
    echo "   Attach:    tmux attach -t ${SESSION_NAME}"
    exit 0
fi

echo "🚀 Starting Multigravity Gateway on port ${PORT} in tmux session '${SESSION_NAME}'..."
tmux new-session -d -s "${SESSION_NAME}" -c "${PROJECT_DIR}"

# MED-05: Quote all paths safely and disable automatic startup QR code dumping into log files.
LAUNCH_CMD="$(printf 'umask 077 && %q -port %q -qr=false 2>&1 | tee -a %q' "${BIN_CMD}" "${PORT}" "${LOG_FILE}")"
tmux send-keys -t "${SESSION_NAME}" "${LAUNCH_CMD}" Enter

sleep 1

if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
    echo "✅ Gateway started successfully!"
    echo "   Local URL: http://127.0.0.1:${PORT}"
    echo "   Log file:  ${LOG_FILE} (mode 0600)"
    echo "   Pair CLI:  mgy pair -port ${PORT}"
    echo "   Attach:    tmux attach -t ${SESSION_NAME}"
    echo "   Stop:      ./scripts/tmux-stop.sh"
else
    echo "❌ Failed to start gateway in tmux."
    cat "${LOG_FILE}"
    exit 1
fi
