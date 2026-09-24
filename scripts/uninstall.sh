#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Multigravity (mgy) 一键彻底卸载脚本 (macOS / Linux / Windows Git Bash)
# 包含: 停止运行进程、卸载 mgy、卸载 cloudflared 穿透引擎、彻底清除配置与配对数据 (~/.multigravity)
#
# 用法:
#   ./scripts/uninstall.sh               # 默认彻底卸载 (包含 mgy、cloudflared、所有配置文件与配对数据)
#   ./scripts/uninstall.sh --keep-config # 仅卸载二进制与 cloudflared，保留 ~/.multigravity 配置文件
#   ./scripts/uninstall.sh --keep-cf     # 卸载 mgy 与配置，保留 cloudflared 穿透引擎二进制
# ==============================================================================

INSTALL_DIR="${HOME}/.local/bin"
CONF_DIR="${HOME}/.multigravity"

KEEP_CONFIG=false
KEEP_CF=false

for arg in "$@"; do
    case "${arg}" in
        --keep-config|-k)
            KEEP_CONFIG=true
            ;;
        --keep-cf)
            KEEP_CF=true
            ;;
        --help|-h)
            echo "Multigravity (mgy) 卸载工具"
            echo "用法: $0 [选项]"
            echo "选项:"
            echo "  --keep-config, -k   保留 ~/.multigravity 配置目录（包含配对数据与 Token）"
            echo "  --keep-cf           保留已下载的 cloudflared 穿透引擎二进制"
            echo "  --help, -h          显示此帮助信息"
            exit 0
            ;;
    esac
done

echo "=================================================="
echo "🗑️  正在彻底卸载 Multigravity (mgy) 及关联组件..."
echo "=================================================="

# 1. 停止运行中的 mgy 进程
if pgrep -f "mgy" >/dev/null 2>&1; then
    echo "⏹️  正在停止运行中的 mgy 进程..."
    pkill -x "mgy" 2>/dev/null || pkill -f "/mgy" 2>/dev/null || true
fi
if command -v taskkill >/dev/null 2>&1; then
    taskkill //F //IM mgy.exe >/dev/null 2>&1 || true
fi

# 2. 停止运行中的 cloudflared 进程（仅当未指定 --keep-cf 时）
if [ "${KEEP_CF}" = false ]; then
    if pgrep -f "cloudflared" >/dev/null 2>&1; then
        echo "⏹️  正在停止运行中的 cloudflared 穿透进程..."
        pkill -x "cloudflared" 2>/dev/null || pkill -f "cloudflared.*tunnel" 2>/dev/null || true
    fi
    if command -v taskkill >/dev/null 2>&1; then
        taskkill //F //IM cloudflared.exe >/dev/null 2>&1 || true
    fi
fi

# 3. 移除 mgy 执行文件与全局软链接
MGY_TARGETS=(
    "${INSTALL_DIR}/mgy"
    "${INSTALL_DIR}/mgy.exe"
    "/opt/homebrew/bin/mgy"
    "/usr/local/bin/mgy"
    "${LOCALAPPDATA:-}/Microsoft/WindowsApps/mgy.exe"
)

for target in "${MGY_TARGETS[@]}"; do
    if [ -n "${target}" ] && ([ -L "${target}" ] || [ -f "${target}" ]); then
        if [ -w "${target}" ] || [ -w "$(dirname "${target}")" ]; then
            rm -f "${target}" 2>/dev/null || true
            echo "✅ 已移除 mgy 执行文件/软链接: ${target}"
        elif sudo -n true 2>/dev/null; then
            sudo rm -f "${target}" 2>/dev/null || true
            echo "✅ 已通过 sudo 移除系统快捷方式: ${target}"
        fi
    fi
done

# 4. 移除 cloudflared 穿透引擎（由 Multigravity 部署的路径）
if [ "${KEEP_CF}" = false ]; then
    CF_TARGETS=(
        "${CONF_DIR}/bin/cloudflared"
        "${CONF_DIR}/bin/cloudflared.exe"
        "${INSTALL_DIR}/cloudflared"
        "${INSTALL_DIR}/cloudflared.exe"
        "${LOCALAPPDATA:-}/Microsoft/WindowsApps/cloudflared.exe"
    )

    for cf_target in "${CF_TARGETS[@]}"; do
        if [ -n "${cf_target}" ] && [ -f "${cf_target}" ]; then
            rm -f "${cf_target}" 2>/dev/null || true
            echo "✅ 已移除 Cloudflare 穿透引擎: ${cf_target}"
        fi
    done
else
    echo "💡 跳过 Cloudflare 引擎移除 (--keep-cf)"
fi

# 5. 清理配置与数据目录 ~/.multigravity
if [ -d "${CONF_DIR}" ]; then
    if [ "${KEEP_CONFIG}" = false ]; then
        rm -rf "${CONF_DIR}"
        echo "✅ 已彻底清除配置与数据目录: ${CONF_DIR} (包含配对数据、授权令牌与缓存)"
    else
        echo "💡 已保留配置与数据目录: ${CONF_DIR} (--keep-config)"
    fi
fi

echo "=================================================="
echo "🎉 Multigravity (mgy) 及 Cloudflare 组件已彻底卸载完成！"
echo "=================================================="
