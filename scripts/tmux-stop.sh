#!/usr/bin/env bash
SESSION_NAME="agy-gateway"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
    echo "🛑 Stopping tmux session '${SESSION_NAME}'..."
    tmux send-keys -t "${SESSION_NAME}" C-c
    sleep 1
    tmux kill-session -t "${SESSION_NAME}" 2>/dev/null || true
    echo "✅ Tmux session '${SESSION_NAME}' stopped."
else
    echo "ℹ️  No active tmux session '${SESSION_NAME}' found."
fi

# Gracefully terminate gateway process associated with this project directory if still running
PROJECT_BINARY="${PROJECT_DIR}/bin/gateway"
if [ -f "${PROJECT_BINARY}" ]; then
    pids=$(pgrep -f "${PROJECT_BINARY}" 2>/dev/null || true)
    if [ -n "${pids}" ]; then
        echo "🛑 Terminating gateway process (PID: ${pids})..."
        kill ${pids} 2>/dev/null || true
    fi
fi
echo "✅ Stopped."
