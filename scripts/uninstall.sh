#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Multigravity (mgy) 一键卸载脚本 (macOS / Linux / Windows)
# 用法:
#   ./scripts/uninstall.sh          # 卸载二进制，保留配置与配对数据
#   ./scripts/uninstall.sh --all    # 彻底卸载二进制并清除 ~/.multigravity
# ==============================================================================

INSTALL_DIR="${HOME}/.local/bin"
CONF_DIR="${HOME}/.multigravity"

echo "=================================================="
echo "🗑️  正在卸载 Multigravity (mgy)..."
echo "=================================================="

# 1. 停止运行中的进程
if pgrep -x "mgy" >/dev/null 2>&1; then
    echo "⏹️  正在停止运行中的 mgy 进程..."
    pkill -x "mgy" || true
fi
if command -v taskkill >/dev/null 2>&1; then
    taskkill //F //IM mgy.exe >/dev/null 2>&1 || true
fi

# 2. 移除二进制执行文件
TARGETS=(
    "${INSTALL_DIR}/mgy"
    "${INSTALL_DIR}/mgy.exe"
    "${LOCALAPPDATA:-}/Microsoft/WindowsApps/mgy.exe"
)

for target in "${TARGETS[@]}"; do
    if [ -n "${target}" ] && [ -f "${target}" ]; then
        rm -f "${target}"
        echo "✅ 已移除二进制: ${target}"
    fi
done

# 3. 检查并清理数据目录
if [ -d "${CONF_DIR}" ]; then
    if [ "${1:-}" = "--all" ] || [ "${1:-}" = "-a" ]; then
        rm -rf "${CONF_DIR}"
        echo "✅ 已彻底清除数据与配置目录: ${CONF_DIR}"
    else
        echo "💡 保留了数据与配置目录: ${CONF_DIR}"
        echo "   (若需彻底删除配置与配对缓存，可运行: rm -rf ~/.multigravity)"
    fi
fi

echo "=================================================="
echo "🎉 Multigravity 卸载完成！"
echo "=================================================="
