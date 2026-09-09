#!/usr/bin/env bash
SESSION_NAME="agy-gateway"

if tmux has-session -t "${SESSION_NAME}" 2>/dev/null; then
    echo "🛑 Stopping tmux session '${SESSION_NAME}'..."
    tmux send-keys -t "${SESSION_NAME}" C-c
    sleep 1
    tmux kill-session -t "${SESSION_NAME}" 2>/dev/null || true
fi

# Ensure any gateway process is terminated
pkill -f "bin/gateway" 2>/dev/null || true
echo "✅ Stopped."
