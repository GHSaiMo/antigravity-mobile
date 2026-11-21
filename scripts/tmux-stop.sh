#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

for sess in "${MGY_TMUX_SESSION:-mgy}" "agy-gateway"; do
    if tmux has-session -t "${sess}" 2>/dev/null; then
        echo "🛑 Stopping tmux session '${sess}'..."
        tmux send-keys -t "${sess}" C-c
        sleep 1
        tmux kill-session -t "${sess}" 2>/dev/null || true
        echo "✅ Tmux session '${sess}' stopped."
    fi
done

for bin_name in "mgy" "gateway"; do
    PROJECT_BINARY="${PROJECT_DIR}/bin/${bin_name}"
    if [ -f "${PROJECT_BINARY}" ]; then
        pids="$(pgrep -f "${PROJECT_BINARY}" 2>/dev/null || true)"
        if [ -n "${pids}" ]; then
            echo "🛑 Terminating gateway process (PID: ${pids})..."
            kill ${pids} 2>/dev/null || true
        fi
    fi
done
echo "✅ Stopped."
