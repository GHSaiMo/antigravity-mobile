#!/usr/bin/env bash
set -e

SESSION_NAME="agy-gateway"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
LOG_FILE="${PROJECT_DIR}/logs/gateway.log"

# Check if session already exists
if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
    echo "⚠️  tmux session '${SESSION_NAME}' is already running."
    echo "   View logs: tail -f ${LOG_FILE}"
    echo "   Attach:    tmux attach -t ${SESSION_NAME}"
    exit 0
fi

echo "🚀 Starting Antigravity Mobile Gateway in tmux session '${SESSION_NAME}'..."
tmux new-session -d -s "${SESSION_NAME}" -c "${PROJECT_DIR}"

# Start binary and tee output to logs/gateway.log
tmux send-keys -t "${SESSION_NAME}" "./bin/gateway -port 58900 2>&1 | tee -a ${LOG_FILE}" Enter

sleep 1

if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
    echo "✅ Gateway started successfully!"
    echo "   Local URL: http://127.0.0.1:58900"
    echo "   Log file:  ${LOG_FILE}"
    echo "   Attach:    tmux attach -t ${SESSION_NAME}"
    echo "   Stop:      ./scripts/tmux-stop.sh"
else
    echo "❌ Failed to start gateway in tmux."
    cat "${LOG_FILE}"
    exit 1
fi
