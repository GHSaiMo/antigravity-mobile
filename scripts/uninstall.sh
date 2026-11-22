#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Multigravity (mgy) Complete Uninstaller (macOS / Linux / Windows Git Bash)
# Stops running processes, removes mgy, cloudflared, and ~/.multigravity data
# Supports native zh / en / ja auto-detection (override via MGY_LANG or --lang)
#
# Usage:
#   ./scripts/uninstall.sh               # Full uninstall (mgy, cloudflared, configs & pairing data)
#   ./scripts/uninstall.sh --keep-config # Keep ~/.multigravity config & pairing data
#   ./scripts/uninstall.sh --keep-cf     # Keep cloudflared tunnel engine binary
#   ./scripts/uninstall.sh --lang en     # Override UI language (zh | en | ja)
# ==============================================================================

INSTALL_DIR="${HOME}/.local/bin"
CONF_DIR="${HOME}/.multigravity"

KEEP_CONFIG=false
KEEP_CF=false
CLI_LANG=""
SHOW_HELP=false

while [ $# -gt 0 ]; do
    case "$1" in
        --keep-config|-k)
            KEEP_CONFIG=true
            ;;
        --keep-cf)
            KEEP_CF=true
            ;;
        --lang=*)
            CLI_LANG="${1#*=}"
            ;;
        --lang|-l)
            if [ $# -ge 2 ]; then
                CLI_LANG="$2"
                shift
            fi
            ;;
        --help|-h)
            SHOW_HELP=true
            ;;
        *)
            ;;
    esac
    shift
done

normalize_lang() {
    local raw
    raw="$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
    case "${raw}" in
        zh|zh_*|zh-*|cn|chinese)
            printf 'zh'
            ;;
        ja|ja_*|ja-*|jp|jpn|japanese)
            printf 'ja'
            ;;
        en|en_*|en-*|english)
            printf 'en'
            ;;
        *)
            printf ''
            ;;
    esac
}

detect_ui_lang() {
    local candidate=""
    local normalized=""
    local lower_cand=""
    local mac_lang=""
    local win_lang=""

    if [ -n "${CLI_LANG}" ]; then
        normalized="$(normalize_lang "${CLI_LANG}")"
        if [ -n "${normalized}" ]; then
            printf '%s' "${normalized}"
            return 0
        fi
    fi

    for candidate in "${MGY_LANG:-}" "${MULTIGRAVITY_LANG:-}"; do
        if [ -n "${candidate}" ]; then
            normalized="$(normalize_lang "${candidate}")"
            if [ -n "${normalized}" ]; then
                printf '%s' "${normalized}"
                return 0
            fi
        fi
    done

    for candidate in "${LC_ALL:-}" "${LC_MESSAGES:-}" "${LANG:-}" "${LANGUAGE:-}"; do
        candidate="${candidate%%:*}"
        if [ -z "${candidate}" ]; then
            continue
        fi
        lower_cand="$(printf '%s' "${candidate}" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"
        case "${lower_cand}" in
            utf-8|utf8)
                continue
                ;;
            c|posix|c.*)
                printf 'en'
                return 0
                ;;
        esac
        normalized="$(normalize_lang "${lower_cand}")"
        if [ -n "${normalized}" ]; then
            printf '%s' "${normalized}"
            return 0
        fi
        printf 'en'
        return 0
    done

    if [ "$(uname -s 2>/dev/null || true)" = "Darwin" ] && command -v defaults >/dev/null 2>&1; then
        mac_lang="$(defaults read -g AppleLanguages 2>/dev/null | sed -n '2p' | tr -d ' ",' || true)"
        if [ -z "${mac_lang}" ]; then
            mac_lang="$(defaults read -g AppleLocale 2>/dev/null || true)"
        fi
        normalized="$(normalize_lang "${mac_lang}")"
        if [ -n "${normalized}" ]; then
            printf '%s' "${normalized}"
            return 0
        fi
    fi

    case "$(uname -s 2>/dev/null || true)" in
        MINGW*|MSYS*|CYGWIN*|Windows_NT)
            if command -v powershell.exe >/dev/null 2>&1; then
                win_lang="$(powershell.exe -NoProfile -Command '[System.Globalization.CultureInfo]::CurrentUICulture.Name' 2>/dev/null | tr -d '\r\n' || true)"
                normalized="$(normalize_lang "${win_lang}")"
                if [ -n "${normalized}" ]; then
                    printf '%s' "${normalized}"
                    return 0
                fi
            fi
            ;;
    esac

    printf 'en'
}

UI_LANG="$(detect_ui_lang)"

msg() {
    case "${UI_LANG}" in
        zh) printf '%s\n' "$1" ;;
        ja) printf '%s\n' "$3" ;;
        *)  printf '%s\n' "$2" ;;
    esac
}

if [ "${SHOW_HELP}" = "true" ]; then
    msg \
        "Multigravity (mgy) 卸载工具" \
        "Multigravity (mgy) Uninstaller" \
        "Multigravity (mgy) アンインストーラー"
    msg \
        "用法: $0 [选项]" \
        "Usage: $0 [options]" \
        "使い方: $0 [オプション]"
    msg \
        "选项:" \
        "Options:" \
        "オプション:"
    msg \
        "  --keep-config, -k   保留 ~/.multigravity 配置目录（包含配对数据与 Token）" \
        "  --keep-config, -k   Keep ~/.multigravity config directory (pairing data & tokens)" \
        "  --keep-config, -k   ~/.multigravity 設定ディレクトリ（ペアリング情報・トークン）を保持"
    msg \
        "  --keep-cf           保留已下载的 cloudflared 穿透引擎二进制" \
        "  --keep-cf           Keep downloaded cloudflared tunnel engine binary" \
        "  --keep-cf           ダウンロード済みの cloudflared トンネルエンジンを保持"
    msg \
        "  --lang <zh|en|ja>   指定输出语言（默认自动识别系统语言）" \
        "  --lang <zh|en|ja>   Override output language (default: auto-detect locale)" \
        "  --lang <zh|en|ja>   表示言語を指定（デフォルト: システム言語を自動判別）"
    msg \
        "  --help, -h          显示此帮助信息" \
        "  --help, -h          Show this help message" \
        "  --help, -h          このヘルプを表示"
    exit 0
fi

echo "=================================================="
msg \
    "🗑️  正在彻底卸载 Multigravity (mgy) 及关联组件..." \
    "🗑️  Uninstalling Multigravity (mgy) and related components..." \
    "🗑️  Multigravity (mgy) および関連コンポーネントをアンインストールしています..."
echo "=================================================="

# 0. 先卸载后台自启守护服务（防止 KeepAlive 在卸载中途抢跑复活幽灵进程）
if [ "$(uname -s 2>/dev/null || true)" = "Darwin" ]; then
    UID_NUM="$(id -u)"
    for plist in "${HOME}/Library/LaunchAgents/"*multigravity*.plist "${HOME}/Library/LaunchAgents/"*mgy*.plist; do
        [ -f "${plist}" ] || continue
        msg \
            "⏹️  正在停止并移除 macOS 自启服务: ${plist}" \
            "⏹️  Stopping and removing macOS LaunchAgent: ${plist}" \
            "⏹️  macOS 自動起動サービスを停止・削除しています: ${plist}"
        launchctl bootout "gui/${UID_NUM}" "${plist}" >/dev/null 2>&1 \
            || launchctl bootout "user/${UID_NUM}" "${plist}" >/dev/null 2>&1 \
            || launchctl unload -w "${plist}" >/dev/null 2>&1 \
            || true
        rm -f "${plist}" 2>/dev/null || true
    done
elif command -v systemctl >/dev/null 2>&1; then
    for unit in mgy.service multigravity.service; do
        if systemctl --user list-unit-files "${unit}" >/dev/null 2>&1 || [ -f "${HOME}/.config/systemd/user/${unit}" ]; then
            systemctl --user disable --now "${unit}" >/dev/null 2>&1 || true
            rm -f "${HOME}/.config/systemd/user/${unit}" 2>/dev/null || true
            systemctl --user daemon-reload >/dev/null 2>&1 || true
        fi
    done
fi

# 1. 停止运行中的 mgy 进程并等待彻底退出
if pgrep -x "mgy" >/dev/null 2>&1 || pgrep -f "/mgy([[:space:]]|$)" >/dev/null 2>&1; then
    msg \
        "⏹️  正在停止运行中的 mgy 进程..." \
        "⏹️  Stopping running mgy processes..." \
        "⏹️  実行中の mgy プロセスを停止しています..."
    pkill -x "mgy" 2>/dev/null || pkill -f "/mgy([[:space:]]|$)" 2>/dev/null || true
    for _ in 1 2 3 4 5 6; do
        if ! pgrep -x "mgy" >/dev/null 2>&1 && ! pgrep -f "/mgy([[:space:]]|$)" >/dev/null 2>&1; then
            break
        fi
        sleep 0.5
    done
    if pgrep -x "mgy" >/dev/null 2>&1 || pgrep -f "/mgy([[:space:]]|$)" >/dev/null 2>&1; then
        pkill -9 -x "mgy" 2>/dev/null || pkill -9 -f "/mgy([[:space:]]|$)" 2>/dev/null || true
        sleep 0.3
    fi
fi
if command -v taskkill >/dev/null 2>&1; then
    taskkill //F //IM mgy.exe >/dev/null 2>&1 || true
fi

# 2. 停止运行中的 cloudflared 进程（仅当未指定 --keep-cf 时）
if [ "${KEEP_CF}" = false ]; then
    if pgrep -f "cloudflared" >/dev/null 2>&1; then
        msg \
            "⏹️  正在停止运行中的 cloudflared 穿透进程..." \
            "⏹️  Stopping running cloudflared tunnel processes..." \
            "⏹️  実行中の cloudflared トンネルプロセスを停止しています..."
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
            msg \
                "✅ 已移除 mgy 执行文件/软链接: ${target}" \
                "✅ Removed mgy binary/symlink: ${target}" \
                "✅ mgy バイナリ/シンボリックリンクを削除しました: ${target}"
        elif sudo -n true 2>/dev/null; then
            sudo rm -f "${target}" 2>/dev/null || true
            msg \
                "✅ 已通过 sudo 移除系统快捷方式: ${target}" \
                "✅ Removed system symlink via sudo: ${target}" \
                "✅ sudo 経由でシステムリンクを削除しました: ${target}"
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
            msg \
                "✅ 已移除 Cloudflare 穿透引擎: ${cf_target}" \
                "✅ Removed Cloudflare tunnel engine: ${cf_target}" \
                "✅ Cloudflare トンネルエンジンを削除しました: ${cf_target}"
        fi
    done
else
    msg \
        "💡 跳过 Cloudflare 引擎移除 (--keep-cf)" \
        "💡 Keeping Cloudflare tunnel engine (--keep-cf)" \
        "💡 Cloudflare トンネルエンジンの削除をスキップしました (--keep-cf)"
fi

# 5. 清理配置与数据目录 ~/.multigravity
if [ -d "${CONF_DIR}" ]; then
    if [ "${KEEP_CONFIG}" = false ]; then
        rm -rf "${CONF_DIR}"
        msg \
            "✅ 已彻底清除配置与数据目录: ${CONF_DIR} (包含配对数据、授权令牌与缓存)" \
            "✅ Removed config & data directory: ${CONF_DIR} (pairing data, tokens & cache)" \
            "✅ 設定・データディレクトリを削除しました: ${CONF_DIR}（ペアリング情報・トークン・キャッシュを含む）"
    else
        msg \
            "💡 已保留配置与数据目录: ${CONF_DIR} (--keep-config)" \
            "💡 Keeping config & data directory: ${CONF_DIR} (--keep-config)" \
            "💡 設定・データディレクトリを保持しました: ${CONF_DIR} (--keep-config)"
    fi
fi

echo "=================================================="
msg \
    "🎉 Multigravity (mgy) 及 Cloudflare 组件已彻底卸载完成！" \
    "🎉 Multigravity (mgy) and Cloudflare components completely uninstalled!" \
    "🎉 Multigravity (mgy) および Cloudflare コンポーネントのアンインストールが完了しました！"
echo "=================================================="
