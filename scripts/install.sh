#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Multigravity (mgy) Cross-Platform One-Line Installer (zh / en / ja)
# Supports: macOS (Apple Silicon / Intel), Linux (x86_64 / arm64), Windows (Git Bash / MSYS)
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
#   MGY_LANG=en curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash -s -- --lang ja
# ==============================================================================

REPO="${MULTIGRAVITY_REPO:-GHSaiMo/antigravity-mobile}"
INSTALL_DIR="${HOME}/.local/bin"
CONF_DIR="${HOME}/.multigravity"
APP_STORE_URL="https://apps.apple.com/app/id6835508934"

# ------------------------------------------------------------------------------
# 0. Native i18n (zh / en / ja) - Compatible with macOS Bash 3.2+
# ------------------------------------------------------------------------------
CLI_LANG=""
SHOW_HELP=false

while [ $# -gt 0 ]; do
    case "$1" in
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

    # 1. Explicit CLI flag (--lang)
    if [ -n "${CLI_LANG}" ]; then
        normalized="$(normalize_lang "${CLI_LANG}")"
        if [ -n "${normalized}" ]; then
            printf '%s' "${normalized}"
            return 0
        fi
    fi

    # 2. Explicit environment variables (aligned with mgy CLI)
    for candidate in "${MGY_LANG:-}" "${MULTIGRAVITY_LANG:-}"; do
        if [ -n "${candidate}" ]; then
            normalized="$(normalize_lang "${candidate}")"
            if [ -n "${normalized}" ]; then
                printf '%s' "${normalized}"
                return 0
            fi
        fi
    done

    # 3. Standard POSIX locale variables
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

    # 4. macOS system preferred language fallback (when terminal doesn't export LANG)
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

    # 5. Windows Git Bash fallback via .NET CurrentUICulture
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

    # 6. International default
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

tr_str() {
    case "${UI_LANG}" in
        zh) printf '%s' "$1" ;;
        ja) printf '%s' "$3" ;;
        *)  printf '%s' "$2" ;;
    esac
}

if [ "${SHOW_HELP}" = "true" ]; then
    msg \
        "Multigravity (mgy) 全平台一键安装脚本" \
        "Multigravity (mgy) One-Line Installer" \
        "Multigravity (mgy) ワンラインインストーラー"
    msg \
        "用法: install.sh [--lang zh|en|ja] [--help]" \
        "Usage: install.sh [--lang zh|en|ja] [--help]" \
        "使い方: install.sh [--lang zh|en|ja] [--help]"
    msg \
        "环境变量: MGY_LANG=zh|en|ja, MGY_USE_MIRROR=0|1, FORCE_CONFIG=1" \
        "Env vars: MGY_LANG=zh|en|ja, MGY_USE_MIRROR=0|1, FORCE_CONFIG=1" \
        "環境変数: MGY_LANG=zh|en|ja, MGY_USE_MIRROR=0|1, FORCE_CONFIG=1"
    exit 0
fi

# Download source preference:
# Chinese locale defaults to mirror-first (official fallback); English/Japanese defaults to official GitHub first (mirror fallback).
# Override anytime with MGY_USE_MIRROR=1 or MGY_USE_MIRROR=0.
PREFER_MIRROR="false"
case "${MGY_USE_MIRROR:-}" in
    1|true|TRUE|yes|YES)
        PREFER_MIRROR="true"
        ;;
    0|false|FALSE|no|NO)
        PREFER_MIRROR="false"
        ;;
    *)
        if [ "${UI_LANG}" = "zh" ]; then
            PREFER_MIRROR="true"
        fi
        ;;
esac

# 1. 检查操作系统
OS="$(uname -s)"
case "${OS}" in
    Darwin)
        OS_TYPE="darwin"
        OS_DESC="macOS"
        BIN_NAME="mgy"
        PKG_EXT="tar.gz"
        ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
        OS_TYPE="windows"
        OS_DESC="Windows"
        BIN_NAME="mgy.exe"
        PKG_EXT="zip"
        ;;
    Linux)
        OS_TYPE="linux"
        OS_DESC="Linux"
        BIN_NAME="mgy"
        PKG_EXT="tar.gz"
        ;;
    *)
        msg \
            "❌ 暂不支持的操作系统: ${OS} (目前官方正式版支持 macOS, Linux 及 Windows)" \
            "❌ Unsupported operating system: ${OS} (officially supports macOS, Linux, and Windows)" \
            "❌ 未対応の OS です: ${OS}（公式対応: macOS / Linux / Windows）"
        exit 1
        ;;
esac

echo "=================================================="
msg \
    "🚀 正在安装 Multigravity (mgy) for ${OS_DESC}..." \
    "🚀 Installing Multigravity (mgy) for ${OS_DESC}..." \
    "🚀 Multigravity (mgy) (${OS_DESC}) をインストールしています..."
echo "=================================================="

# 2. 探测芯片架构
ARCH="$(uname -m)"
case "${ARCH}" in
    arm64|aarch64)
        if [ "${OS_TYPE}" = "darwin" ]; then
            ARCH_DESC="Apple Silicon"
            PKG_ARCH="arm64"
        elif [ "${OS_TYPE}" = "windows" ]; then
            ARCH_DESC="$(tr_str "Windows ARM64 (amd64 仿真)" "Windows ARM64 (amd64 emulation)" "Windows ARM64 (amd64 エミュレーション)")"
            PKG_ARCH="amd64"
        else
            ARCH_DESC="Linux ARM64"
            PKG_ARCH="arm64"
        fi
        ;;
    x86_64|amd64)
        ARCH_DESC="${OS_DESC} x86_64"
        PKG_ARCH="amd64"
        ;;
    *)
        msg \
            "❌ 暂不支持的系统架构: ${ARCH}" \
            "❌ Unsupported CPU architecture: ${ARCH}" \
            "❌ 未対応の CPU アーキテクチャです: ${ARCH}"
        exit 1
        ;;
esac
if [[ "${ARCH_DESC}" == *"("* ]]; then
    msg \
        "🖥️  检测到系统架构: ${ARCH_DESC}" \
        "🖥️  Detected architecture: ${ARCH_DESC}" \
        "🖥️  システムアーキテクチャを検出: ${ARCH_DESC}"
else
    msg \
        "🖥️  检测到系统架构: ${ARCH_DESC} (${PKG_ARCH})" \
        "🖥️  Detected architecture: ${ARCH_DESC} (${PKG_ARCH})" \
        "🖥️  システムアーキテクチャを検出: ${ARCH_DESC} (${PKG_ARCH})"
fi
if [ "${OS_TYPE}" = "linux" ]; then
    msg \
        "🐧 已识别 Linux 平台环境，原生支持 Antigravity Daemon Discovery 协议与实例自愈" \
        "🐧 Linux detected — native Antigravity Daemon Discovery and instance self-healing supported" \
        "🐧 Linux 環境を検出しました（Antigravity Daemon Discovery とインスタンス自動修復に対応）"
fi

# 3. 准备安装与配置目录 (配置目录严格限制 0700 仅当前用户可访问)
mkdir -p "${INSTALL_DIR}"
mkdir -p -m 700 "${CONF_DIR}" "${CONF_DIR}/logs"
chmod 700 "${CONF_DIR}" "${CONF_DIR}/logs" 2>/dev/null || true

# 4. 初始化全局默认配置文件 ~/.multigravity/.env (若不存在)
if [ ! -f "${CONF_DIR}/.env" ]; then
    if [ "${UI_LANG}" = "zh" ]; then
        cat << 'ENVEOF' > "${CONF_DIR}/.env"
# Multigravity 全局环境变量配置文件
# 保存路径: ~/.multigravity/.env

# 网关监听端口 (默认 58900)
MULTIGRAVITY_PORT=58900

# 🛡️ 局域网访问安全策略 (0: 需配对码; 1: 信任局域网免配对直达工作台)
# 适合家庭纯净内网开启 (1)；办公/公共 Wi-Fi 网络保持标准配对 (0)
MULTIGRAVITY_TRUST_LAN=0

# ☁️ Cloudflare Tunnel 专属公网穿透配置 (开箱即用)
# CF_WORKER_URL=https://dispatcher.multigravity.app
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1

# 🔔 iOS 原生 APNs 推送与灵动岛远程更新 (开箱即用，默认开启)
# APNS_ENABLED=1
# LIVE_ACTIVITY_PUSH_ENABLED=1
# APNS_RELAY_URL=https://push.multigravity.app
ENVEOF
    elif [ "${UI_LANG}" = "ja" ]; then
        cat << 'ENVEOF' > "${CONF_DIR}/.env"
# Multigravity グローバル環境設定ファイル
# 保存先: ~/.multigravity/.env

# ゲートウェイの待受ポート (デフォルト: 58900)
MULTIGRAVITY_PORT=58900

# 🛡️ LAN アクセスセキュリティ設定 (0: ペアリングコード必須 / 1: LAN を信頼してペアリング不要)
# 自宅などの信頼できるプライベートネットワーク向け (1)。オフィスや公衆 Wi-Fi では標準 (0) を推奨
MULTIGRAVITY_TRUST_LAN=0

# ☁️ Cloudflare Tunnel リモート公開設定 (そのまま利用可能)
# CF_WORKER_URL=https://dispatcher.multigravity.app
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1

# 🔔 iOS ネイティブ APNs プッシュ通知と Dynamic Island 更新 (デフォルト有効)
# APNS_ENABLED=1
# LIVE_ACTIVITY_PUSH_ENABLED=1
# APNS_RELAY_URL=https://push.multigravity.app
ENVEOF
    else
        cat << 'ENVEOF' > "${CONF_DIR}/.env"
# Multigravity Global Environment Configuration
# Path: ~/.multigravity/.env

# Gateway listen port (default: 58900)
MULTIGRAVITY_PORT=58900

# 🛡️ LAN access security policy (0: pairing code required; 1: trust LAN without pairing)
# Recommended: 1 for trusted home networks, 0 for office or public Wi-Fi
MULTIGRAVITY_TRUST_LAN=0

# ☁️ Cloudflare Tunnel remote access configuration (ready out of the box)
# CF_WORKER_URL=https://dispatcher.multigravity.app
# CF_INVITE_CODE=
# CF_TUNNEL_TOKEN=
# CF_TUNNEL_ENABLED=1

# 🔔 Native iOS APNs push & Live Activity Dynamic Island updates (enabled by default)
# APNS_ENABLED=1
# LIVE_ACTIVITY_PUSH_ENABLED=1
# APNS_RELAY_URL=https://push.multigravity.app
ENVEOF
    fi
    chmod 600 "${CONF_DIR}/.env" 2>/dev/null || true
    msg \
        "📝 已生成全局默认配置: ${CONF_DIR}/.env" \
        "📝 Created default configuration: ${CONF_DIR}/.env" \
        "📝 デフォルト設定ファイルを作成しました: ${CONF_DIR}/.env"
fi

# 5. 下载并安装对应架构二进制
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT
PKG_FILE="${TMP_DIR}/multigravity.${PKG_EXT}"

# 自动探测本机常用代理端口 (Clash / V2Ray / Surge 等)
PROXY_PORT=""
if [ -z "${https_proxy:-}" ] && [ -z "${http_proxy:-}" ] && [ -z "${all_proxy:-}" ]; then
    for test_port in 7890 10808 1080 6152; do
        if nc -z -w 1 127.0.0.1 "${test_port}" 2>/dev/null; then
            if [ "${PREFER_MIRROR}" = "true" ]; then
                msg \
                    "⚡ 检测到本机代理环境 (127.0.0.1:${test_port})，将优先从加速镜像站直连下载（官方源备用加速）" \
                    "⚡ Detected local proxy (127.0.0.1:${test_port}); trying mirrors first (official source via proxy as fallback)" \
                    "⚡ ローカルプロキシ (127.0.0.1:${test_port}) を検出しました。加速ミラー優先でダウンロードします（公式ソースはプロキシ経由で予備利用）"
            else
                msg \
                    "⚡ 检测到本机代理环境 (127.0.0.1:${test_port})，将通过本机代理加速访问 GitHub 官方源" \
                    "⚡ Detected local proxy (127.0.0.1:${test_port}); GitHub official downloads will use it" \
                    "⚡ ローカルプロキシ (127.0.0.1:${test_port}) を検出しました。GitHub 公式ソースのダウンロードに使用します"
            fi
            PROXY_PORT="${test_port}"
            break
        fi
    done
fi

# SHA-256 计算辅助函数 (兼容 macOS / Linux / Git Bash)
compute_sha256() {
    local target_file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${target_file}" | awk '{print tolower($1)}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${target_file}" | awk '{print tolower($1)}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "${target_file}" | awk '{print tolower($NF)}'
    else
        echo ""
        return 1
    fi
}

lookup_checksum_for_asset() {
    local checksums_file="$1"
    local asset_name="$2"
    awk -v asset="${asset_name}" '
        NF >= 2 && $1 ~ /^[0-9a-fA-F]{64}$/ {
            name = $2
            sub(/^\*/, "", name)
            sub(/^\.\//, "", name)
            sub(/^.*\//, "", name)
            if (name == asset) {
                print tolower($1)
                exit 0
            }
        }
    ' "${checksums_file}"
}

ARCH_PKG="multigravity-${OS_TYPE}-${PKG_ARCH}.${PKG_EXT}"
UNIV_PKG="multigravity-darwin-universal.tar.gz"
CHECKSUMS_FILE="${TMP_DIR}/checksums.txt"
OFFICIAL_CHECKSUMS_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"

# HIGH-02: 仅从 GitHub 官方 HTTPS 源获取 checksums.txt (严禁回退第三方镜像获取校验和，Fail-Closed)
msg \
    "🔒 正在从 GitHub 官方源获取 SHA-256 校验清单..." \
    "🔒 Fetching official SHA-256 checksums from GitHub..." \
    "🔒 GitHub 公式ソースから SHA-256 チェックサムを取得しています..."
CHECKSUM_CURL=(curl -fsSL --connect-timeout 10 --max-time 25)
if [ -n "${PROXY_PORT}" ]; then
    CHECKSUM_CURL+=(--proxy "http://127.0.0.1:${PROXY_PORT}")
fi
if ! "${CHECKSUM_CURL[@]}" -o "${CHECKSUMS_FILE}" "${OFFICIAL_CHECKSUMS_URL}"; then
    msg \
        "❌ 安全校验失败: 无法从 GitHub 官方 HTTPS 源获取 checksums.txt (${OFFICIAL_CHECKSUMS_URL})。" \
        "❌ Security check failed: unable to fetch checksums.txt from GitHub (${OFFICIAL_CHECKSUMS_URL})." \
        "❌ セキュリティ検証に失敗しました: GitHub 公式 HTTPS から checksums.txt を取得できませんでした (${OFFICIAL_CHECKSUMS_URL})。"
    msg \
        "   为防止第三方镜像供应链投毒，安装已安全中止（请检查网络/代理后重试）。" \
        "   Installation aborted to prevent untrusted downloads. Please check your network or proxy and try again." \
        "   サードパーティミラーの改ざん防止のためインストールを中止しました。ネットワークまたはプロキシを確認して再試行してください。"
    exit 1
fi

# 所有下载包均须通过官方 SHA-256 强校验
CANDIDATE_PKGS=("${ARCH_PKG}")
if [ "${OS_TYPE}" = "darwin" ]; then
    CANDIDATE_PKGS+=("${UNIV_PKG}")
fi

DOWNLOAD_SUCCESS=false
SELECTED_PKG=""
msg \
    "📥 正在获取 Multigravity (${OS_DESC} ${PKG_ARCH}) 最新发行版..." \
    "📥 Downloading latest Multigravity release (${OS_DESC} ${PKG_ARCH})..." \
    "📥 Multigravity (${OS_DESC} ${PKG_ARCH}) の最新リリースをダウンロードしています..."

for pkg_name in "${CANDIDATE_PKGS[@]}"; do
    EXPECTED_SHA="$(lookup_checksum_for_asset "${CHECKSUMS_FILE}" "${pkg_name}")"
    if [ -z "${EXPECTED_SHA}" ]; then
        continue
    fi

    if [ "${PREFER_MIRROR}" = "true" ]; then
        PKG_URLS=(
            "https://ghfast.top/https://github.com/${REPO}/releases/latest/download/${pkg_name}"
            "https://ghproxy.net/https://github.com/${REPO}/releases/latest/download/${pkg_name}"
            "https://github.com/${REPO}/releases/latest/download/${pkg_name}"
        )
    else
        PKG_URLS=(
            "https://github.com/${REPO}/releases/latest/download/${pkg_name}"
            "https://ghfast.top/https://github.com/${REPO}/releases/latest/download/${pkg_name}"
            "https://ghproxy.net/https://github.com/${REPO}/releases/latest/download/${pkg_name}"
        )
    fi

    for d_url in "${PKG_URLS[@]}"; do
        CURL_EXTRA_ARGS=()
        if [[ "${d_url}" == https://github.com/* ]]; then
            if [ -n "${PROXY_PORT}" ]; then
                msg \
                    "🔗 尝试从 GitHub 官方源（走本机代理加速）下载: ${d_url}" \
                    "🔗 Downloading from GitHub official source (via local proxy): ${d_url}" \
                    "🔗 GitHub 公式ソース（ローカルプロキシ経由）からダウンロード中: ${d_url}"
                CURL_EXTRA_ARGS=(--proxy "http://127.0.0.1:${PROXY_PORT}")
            else
                msg \
                    "🔗 尝试从 GitHub 官方源下载: ${d_url}" \
                    "🔗 Downloading from GitHub official source: ${d_url}" \
                    "🔗 GitHub 公式ソースからダウンロード中: ${d_url}"
            fi
        else
            msg \
                "🔗 尝试从加速镜像站直连下载: ${d_url}" \
                "🔗 Downloading from mirror: ${d_url}" \
                "🔗 加速ミラーからダウンロード中: ${d_url}"
            CURL_EXTRA_ARGS=(--noproxy "*")
        fi

        CURL_CMD=(curl -fL)
        if [ ${#CURL_EXTRA_ARGS[@]} -gt 0 ]; then
            CURL_CMD+=("${CURL_EXTRA_ARGS[@]}")
        fi
        CURL_CMD+=(--connect-timeout 8 --speed-limit 10240 --speed-time 8 -# -o "${PKG_FILE}" "${d_url}")

        if "${CURL_CMD[@]}"; then
            GOT_SHA="$(compute_sha256 "${PKG_FILE}" || true)"
            if [ -z "${GOT_SHA}" ] || [ "${GOT_SHA}" != "${EXPECTED_SHA}" ]; then
                msg \
                    "⚠️  SHA-256 校验未通过 (expected ${EXPECTED_SHA}, got ${GOT_SHA:-empty})，已丢弃并切换下一个源..." \
                    "⚠️  SHA-256 mismatch (expected ${EXPECTED_SHA}, got ${GOT_SHA:-empty}); discarding and trying next source..." \
                    "⚠️  SHA-256 チェックサムが一致しません (expected ${EXPECTED_SHA}, got ${GOT_SHA:-empty})。破棄して次のソースを試行します..."
                rm -f "${PKG_FILE}"
                continue
            fi
            msg \
                "🔒 SHA-256 校验通过 (${GOT_SHA:0:12}...)" \
                "🔒 SHA-256 verified (${GOT_SHA:0:12}...)" \
                "🔒 SHA-256 検証に成功しました (${GOT_SHA:0:12}...)"
            DOWNLOAD_SUCCESS=true
            SELECTED_PKG="${pkg_name}"
            break
        else
            msg \
                "⚠️  连接超时或速度较慢，正在自动切换备用下载源..." \
                "⚠️  Download failed or timed out; trying fallback source..." \
                "⚠️  接続タイムアウトまたは低速のため、予備ソースへ切り替えます..."
            rm -f "${PKG_FILE}"
        fi
    done

    if [ "${DOWNLOAD_SUCCESS}" = "true" ]; then
        break
    fi
done

if [ "${DOWNLOAD_SUCCESS}" = "true" ]; then
    msg \
        "📦 校验通过，正在定向提取可执行文件..." \
        "📦 Verified. Extracting binary..." \
        "📦 検証完了。実行バイナリを展開しています..."
    STAGED_BIN="${INSTALL_DIR}/.${BIN_NAME}.staged.$$"
    rm -f "${STAGED_BIN}"

    # HIGH-02: 仅定向流式提取单一目标二进制文件，绝不全量解压归档（防止路径穿越与软链接逃逸）
    if [ "${PKG_EXT}" = "zip" ]; then
        if command -v unzip >/dev/null 2>&1; then
            unzip -p "${PKG_FILE}" "${BIN_NAME}" > "${STAGED_BIN}"
        elif command -v tar.exe >/dev/null 2>&1; then
            tar.exe -xOf "${PKG_FILE}" "${BIN_NAME}" > "${STAGED_BIN}"
        elif command -v tar >/dev/null 2>&1; then
            tar -xOf "${PKG_FILE}" "${BIN_NAME}" > "${STAGED_BIN}"
        else
            msg \
                "❌ 未找到安全的 zip 单文件提取工具 (unzip / tar)。" \
                "❌ No safe single-file zip extractor found (unzip or tar required)." \
                "❌ 安全な zip 単一ファイル展開ツール (unzip / tar) が見つかりません。"
            exit 1
        fi
    else
        TAR_MEMBER="$(tar -tzf "${PKG_FILE}" | grep -E '(^|/)(mgy|gateway)$' | head -n 1 || true)"
        if [ -z "${TAR_MEMBER}" ]; then
            msg \
                "❌ 归档中未找到合法的 mgy 可执行文件条目。" \
                "❌ Valid mgy binary entry not found in archive." \
                "❌ アーカイブ内に有効な mgy 実行ファイルが見つかりませんでした。"
            exit 1
        fi
        tar -xOzf "${PKG_FILE}" "${TAR_MEMBER}" > "${STAGED_BIN}"
    fi

    if [ ! -s "${STAGED_BIN}" ]; then
        rm -f "${STAGED_BIN}"
        msg \
            "❌ 提取的可执行文件为空，安装中止。" \
            "❌ Extracted binary is empty; installation aborted." \
            "❌ 展開された実行ファイルが空のため、インストールを中止しました。"
        exit 1
    fi

    # 停止正在运行的旧版本进程以防文件写入锁
    if [ "${OS_TYPE}" = "windows" ]; then
        taskkill //F //IM mgy.exe >/dev/null 2>&1 || true
    fi

    chmod 755 "${STAGED_BIN}" 2>/dev/null || true
    mv -f "${STAGED_BIN}" "${INSTALL_DIR}/${BIN_NAME}"
else
    msg \
        "❌ 未能下载并通过官方 SHA-256 校验获取预编译包 (${ARCH_PKG})。" \
        "❌ Failed to download and verify prebuilt package (${ARCH_PKG})." \
        "❌ ビルド済みパッケージ (${ARCH_PKG}) のダウンロードおよび SHA-256 検証に失敗しました。"
    msg \
        "   请检查网络连接或手动访问官方 Release 下载校验安装:" \
        "   Please check your network or download manually from GitHub Releases:" \
        "   ネットワーク接続を確認するか、公式 Releases から手動でダウンロードしてください:"
    echo "   https://github.com/${REPO}/releases/latest"
    exit 1
fi

chmod +x "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || true

# 6. Windows / macOS / Linux 平台特性与全局软链接处理
GLOBAL_LINKED=false

if [ "${OS_TYPE}" = "darwin" ]; then
    xattr -d com.apple.quarantine "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || true
    codesign -s - -f "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || true

    # 尝试将可执行文件软链接至已在默认 PATH 中的全局目录 (当前终端即可直接运行，免 source)
    CANDIDATE_DIRS=("/opt/homebrew/bin" "/usr/local/bin")
    for c_dir in "${CANDIDATE_DIRS[@]}"; do
        if [ -d "${c_dir}" ] && [ -w "${c_dir}" ]; then
            ln -sf "${INSTALL_DIR}/${BIN_NAME}" "${c_dir}/${BIN_NAME}" 2>/dev/null && {
                msg \
                    "🔗 已自动创建全局快捷方式: ${c_dir}/${BIN_NAME} (当前终端即刻可用)" \
                    "🔗 Linked globally: ${c_dir}/${BIN_NAME} (ready in this terminal)" \
                    "🔗 グローバルシンボリックリンクを作成しました: ${c_dir}/${BIN_NAME}（このターミナルですぐ実行できます）"
                GLOBAL_LINKED=true
                break
            }
        fi
    done

    # 若常规目录不可写，尝试从当前有效 PATH 中匹配用户可写的系统 bin 目录
    if [ "${GLOBAL_LINKED}" = "false" ]; then
        IFS=':' read -ra CURRENT_PATHS <<< "${PATH:-}"
        for p_dir in "${CURRENT_PATHS[@]}"; do
            if [ -n "${p_dir}" ] && [ -d "${p_dir}" ] && [ -w "${p_dir}" ] && [ "${p_dir}" != "${INSTALL_DIR}" ]; then
                if [[ "${p_dir}" == *"/bin"* ]] && [[ "${p_dir}" != *"/tmp"* ]] && [[ "${p_dir}" != *".gemini"* ]]; then
                    ln -sf "${INSTALL_DIR}/${BIN_NAME}" "${p_dir}/${BIN_NAME}" 2>/dev/null && {
                        msg \
                            "🔗 已自动链接至现有 PATH 目录: ${p_dir}/${BIN_NAME} (当前终端即刻可用)" \
                            "🔗 Linked into PATH directory: ${p_dir}/${BIN_NAME} (ready in this terminal)" \
                            "🔗 既存の PATH ディレクトリへリンクしました: ${p_dir}/${BIN_NAME}（このターミナルですぐ実行できます）"
                        GLOBAL_LINKED=true
                        break
                    }
                fi
            fi
        done
    fi

    # 若仍未成功且存在 /usr/local/bin，尝试免密 sudo 创建软链接
    if [ "${GLOBAL_LINKED}" = "false" ] && [ -d "/usr/local/bin" ]; then
        if sudo -n true 2>/dev/null; then
            sudo ln -sf "${INSTALL_DIR}/${BIN_NAME}" "/usr/local/bin/${BIN_NAME}" 2>/dev/null && {
                msg \
                    "🔗 已通过免密授权创建全局快捷方式: /usr/local/bin/${BIN_NAME} (当前终端即刻可用)" \
                    "🔗 Linked globally via sudo: /usr/local/bin/${BIN_NAME} (ready in this terminal)" \
                    "🔗 sudo 経由でグローバルリンクを作成しました: /usr/local/bin/${BIN_NAME}（このターミナルですぐ実行できます）"
                GLOBAL_LINKED=true
            }
        fi
    fi

elif [ "${OS_TYPE}" = "linux" ]; then
    # 尝试将可执行文件软链接至已在默认 PATH 中的全局目录
    CANDIDATE_DIRS=("/usr/local/bin" "/usr/bin")
    for c_dir in "${CANDIDATE_DIRS[@]}"; do
        if [ -d "${c_dir}" ] && [ -w "${c_dir}" ]; then
            ln -sf "${INSTALL_DIR}/${BIN_NAME}" "${c_dir}/${BIN_NAME}" 2>/dev/null && {
                msg \
                    "🔗 已自动创建全局快捷方式: ${c_dir}/${BIN_NAME} (当前终端即刻可用)" \
                    "🔗 Linked globally: ${c_dir}/${BIN_NAME} (ready in this terminal)" \
                    "🔗 グローバルシンボリックリンクを作成しました: ${c_dir}/${BIN_NAME}（このターミナルですぐ実行できます）"
                GLOBAL_LINKED=true
                break
            }
        fi
    done

    # 若常规目录不可写，尝试免密 sudo 创建软链接
    if [ "${GLOBAL_LINKED}" = "false" ] && [ -d "/usr/local/bin" ]; then
        if sudo -n true 2>/dev/null; then
            sudo ln -sf "${INSTALL_DIR}/${BIN_NAME}" "/usr/local/bin/${BIN_NAME}" 2>/dev/null && {
                msg \
                    "🔗 已通过免密授权创建全局快捷方式: /usr/local/bin/${BIN_NAME} (当前终端即刻可用)" \
                    "🔗 Linked globally via sudo: /usr/local/bin/${BIN_NAME} (ready in this terminal)" \
                    "🔗 sudo 経由でグローバルリンクを作成しました: /usr/local/bin/${BIN_NAME}（このターミナルですぐ実行できます）"
                GLOBAL_LINKED=true
            }
        fi
    fi

elif [ "${OS_TYPE}" = "windows" ]; then
    # 拷贝一份至 WindowsApps (Windows 默认系统级用户 PATH，开箱即用免重启)
    if [ -n "${LOCALAPPDATA:-}" ] && [ -d "${LOCALAPPDATA}/Microsoft/WindowsApps" ]; then
        cp -f "${INSTALL_DIR}/${BIN_NAME}" "${LOCALAPPDATA}/Microsoft/WindowsApps/${BIN_NAME}" 2>/dev/null || true
        GLOBAL_LINKED=true
    fi
    # 将 ~/.local/bin 追加到 Windows User PATH
    if command -v powershell.exe >/dev/null 2>&1; then
        powershell.exe -NoProfile -ExecutionPolicy Bypass -Command '
            try {
                $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
                $dir = [System.IO.Path]::Combine($env:USERPROFILE, ".local", "bin")
                if ($userPath -notlike "*$dir*") {
                    [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
                }
            } catch {}
        ' 2>/dev/null || true
    fi
fi

# 7. 检查并持久化配置 Shell PATH 环境变量 (针对 macOS / Linux)
SHELL_NAME="$(basename "${SHELL:-bash}")"
RC_FILES=()
SHELL_RC_HINT="~/.bashrc"
if [ "${OS_TYPE}" = "darwin" ]; then
    RC_FILES+=("${HOME}/.zshrc" "${HOME}/.zprofile")
    SHELL_RC_HINT="~/.zshrc"
    if [ "${SHELL_NAME}" = "bash" ]; then
        RC_FILES+=("${HOME}/.bash_profile" "${HOME}/.bashrc")
        SHELL_RC_HINT="~/.bash_profile"
    fi
elif [ -f "${HOME}/.zshrc" ] && [ "${SHELL_NAME}" = "zsh" ]; then
    RC_FILES+=("${HOME}/.zshrc")
    SHELL_RC_HINT="~/.zshrc"
else
    RC_FILES+=("${HOME}/.bashrc" "${HOME}/.profile")
    SHELL_RC_HINT="~/.bashrc"
fi

PATH_CONFIGURED=true
if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
    if [ "${GLOBAL_LINKED}" = "false" ]; then
        PATH_CONFIGURED=false
    fi
    for rc_file in "${RC_FILES[@]}"; do
        if [ -f "${rc_file}" ] || [ "${rc_file}" = "${HOME}/.zshrc" ]; then
            touch "${rc_file}" 2>/dev/null || true
            if ! grep -q '\.local/bin' "${rc_file}" 2>/dev/null; then
                echo '' >> "${rc_file}"
                echo '# Multigravity CLI PATH' >> "${rc_file}"
                echo 'export PATH="$HOME/.local/bin:$PATH"' >> "${rc_file}"
                msg \
                    "🔧 已自动将 ~/.local/bin 追加到 ${rc_file}" \
                    "🔧 Added ~/.local/bin to PATH in ${rc_file}" \
                    "🔧 ~/.local/bin を ${rc_file} の PATH に追加しました"
            fi
        fi
    done
fi

# 8. 预先检测并安装 Cloudflare 穿透引擎 (用于远程外网安全直连)
CF_BIN_DIR="${CONF_DIR}/bin"
mkdir -p "${CF_BIN_DIR}"
CF_TARGET="${CF_BIN_DIR}/cloudflared"
if [ "${OS_TYPE}" = "windows" ]; then
    CF_TARGET="${CF_BIN_DIR}/cloudflared.exe"
fi

CF_EXISTING=""
if command -v cloudflared >/dev/null 2>&1; then
    CF_CMD="$(command -v cloudflared)"
    CF_SIZE="$(wc -c < "${CF_CMD}" 2>/dev/null || stat -f%z "${CF_CMD}" 2>/dev/null || stat -c%s "${CF_CMD}" 2>/dev/null || echo 0)"
    if [ "${CF_SIZE}" -gt 10000000 ] && "${CF_CMD}" --version >/dev/null 2>&1; then
        CF_EXISTING="${CF_CMD}"
    fi
fi
if [ -z "${CF_EXISTING}" ] && [ -f "${CF_TARGET}" ]; then
    CF_SIZE="$(wc -c < "${CF_TARGET}" 2>/dev/null || stat -f%z "${CF_TARGET}" 2>/dev/null || stat -c%s "${CF_TARGET}" 2>/dev/null || echo 0)"
    if [ "${CF_SIZE}" -gt 10000000 ] && "${CF_TARGET}" --version >/dev/null 2>&1; then
        CF_EXISTING="${CF_TARGET}"
    fi
fi

if [ -n "${CF_EXISTING}" ]; then
    msg \
        "✅ 检测到 Cloudflare 穿透引擎已就绪: ${CF_EXISTING}" \
        "✅ Cloudflare tunnel engine already installed: ${CF_EXISTING}" \
        "✅ Cloudflare トンネルエンジンを検出済み: ${CF_EXISTING}"
    if [ ! -f "${INSTALL_DIR}/cloudflared" ] && [ -f "${CF_TARGET}" ]; then
        cp -f "${CF_TARGET}" "${INSTALL_DIR}/cloudflared" 2>/dev/null || true
        chmod +x "${INSTALL_DIR}/cloudflared" 2>/dev/null || true
    fi
else
    CF_VERSION="2026.11.1"
    CF_PKG=""
    CF_EXPECTED_SHA=""
    case "${OS_TYPE}" in
        darwin)
            if [ "${PKG_ARCH}" = "arm64" ]; then
                CF_PKG="cloudflared-darwin-arm64.tgz"
                CF_EXPECTED_SHA="02fe4bcf08f2c09699cff30658228a739a0c47fb75f3653d1b8772a27ade4462"
            else
                CF_PKG="cloudflared-darwin-amd64.tgz"
                CF_EXPECTED_SHA="5e0403b188dfee71fbd9b995e7dc9910d61b661c52371935ab6475d41185632f"
            fi
            ;;
        windows)
            CF_PKG="cloudflared-windows-amd64.exe"
            CF_EXPECTED_SHA="341deda3db5ec498a0482254d6c38c0b7cf01a30c37bb5dd95d2216d08955e35"
            ;;
        linux)
            if [ "${PKG_ARCH}" = "arm64" ]; then
                CF_PKG="cloudflared-linux-arm64"
                CF_EXPECTED_SHA="b155cec3e48e489cfb8e5c602769fb8c771083018d8881d68b4d18a791567c5a"
            else
                CF_PKG="cloudflared-linux-amd64"
                CF_EXPECTED_SHA="97bb3b02d23a5980fc9c15a1a270e692da1923f103f920d5b16d4931d5ab55e7"
            fi
            ;;
    esac

    if [ -n "${CF_PKG}" ] && [ -n "${CF_EXPECTED_SHA}" ]; then
        if [ "${PREFER_MIRROR}" = "true" ]; then
            CF_URLS=(
                "https://ghfast.top/https://github.com/cloudflare/cloudflared/releases/download/${CF_VERSION}/${CF_PKG}"
                "https://ghproxy.net/https://github.com/cloudflare/cloudflared/releases/download/${CF_VERSION}/${CF_PKG}"
                "https://github.com/cloudflare/cloudflared/releases/download/${CF_VERSION}/${CF_PKG}"
            )
        else
            CF_URLS=(
                "https://github.com/cloudflare/cloudflared/releases/download/${CF_VERSION}/${CF_PKG}"
                "https://ghfast.top/https://github.com/cloudflare/cloudflared/releases/download/${CF_VERSION}/${CF_PKG}"
                "https://ghproxy.net/https://github.com/cloudflare/cloudflared/releases/download/${CF_VERSION}/${CF_PKG}"
            )
        fi

        CF_DONE=false
        KEEP_TRYING=true

        while [ "${KEEP_TRYING}" = "true" ] && [ "${CF_DONE}" = "false" ]; do
            echo ""
            msg \
                "⏬ 正在预先获取 Cloudflare 穿透引擎 (${CF_PKG}, 约 65MB)..." \
                "⏬ Pre-downloading Cloudflare tunnel engine (${CF_PKG}, ~65MB)..." \
                "⏬ Cloudflare トンネルエンジン (${CF_PKG}, 約 65MB) を事前ダウンロードしています..."

            # 重新探测本地代理端口
            CURR_PROXY_PORT="${PROXY_PORT:-}"
            if [ -z "${https_proxy:-}" ] && [ -z "${http_proxy:-}" ] && [ -z "${all_proxy:-}" ]; then
                for test_port in 7890 10808 1080 6152; do
                    if nc -z -w 1 127.0.0.1 "${test_port}" 2>/dev/null; then
                        CURR_PROXY_PORT="${test_port}"
                        break
                    fi
                done
            fi

            for cf_url in "${CF_URLS[@]}"; do
                TMP_CF="${CF_TARGET}.tmp"
                rm -f "${TMP_CF}" 2>/dev/null || true

                CURL_EXTRA=()
                if [[ "${cf_url}" == https://github.com/* ]]; then
                    if [ -n "${CURR_PROXY_PORT}" ]; then
                        msg \
                            "🔗 尝试从 Cloudflare 官方源（走本机代理 127.0.0.1:${CURR_PROXY_PORT}）下载..." \
                            "🔗 Downloading from Cloudflare official source (via local proxy 127.0.0.1:${CURR_PROXY_PORT})..." \
                            "🔗 Cloudflare 公式ソース（ローカルプロキシ 127.0.0.1:${CURR_PROXY_PORT} 経由）からダウンロード中..."
                        CURL_EXTRA=(--proxy "http://127.0.0.1:${CURR_PROXY_PORT}")
                    else
                        msg \
                            "🔗 尝试从 Cloudflare 官方源下载: ${cf_url}" \
                            "🔗 Downloading from Cloudflare official source: ${cf_url}" \
                            "🔗 Cloudflare 公式ソースからダウンロード中: ${cf_url}"
                    fi
                else
                    msg \
                        "🔗 尝试从加速镜像站直连下载: ${cf_url}" \
                        "🔗 Downloading from mirror: ${cf_url}" \
                        "🔗 加速ミラーからダウンロード中: ${cf_url}"
                    CURL_EXTRA=(--noproxy "*")
                fi

                CURL_CMD=(curl -fL)
                if [ ${#CURL_EXTRA[@]} -gt 0 ]; then
                    CURL_CMD+=("${CURL_EXTRA[@]}")
                fi
                # 超时放宽至 300 秒 (5分钟)，连接超时 15 秒，显示进度条
                CURL_CMD+=(--connect-timeout 15 --max-time 300 -# -o "${TMP_CF}" "${cf_url}")

                if "${CURL_CMD[@]}"; then
                    GOT_CF_SHA="$(compute_sha256 "${TMP_CF}" || true)"
                    if [ -z "${GOT_CF_SHA}" ] || [ "${GOT_CF_SHA}" != "${CF_EXPECTED_SHA}" ]; then
                        msg \
                            "⚠️  cloudflared SHA-256 校验不匹配 (got ${GOT_CF_SHA:-empty})，已丢弃并切换备用源..." \
                            "⚠️  cloudflared SHA-256 mismatch (got ${GOT_CF_SHA:-empty}); discarding and trying next source..." \
                            "⚠️  cloudflared の SHA-256 が一致しません (got ${GOT_CF_SHA:-empty})。破棄して次のソースを試行します..."
                        rm -f "${TMP_CF}" 2>/dev/null || true
                        continue
                    fi
                    if [[ "${CF_PKG}" == *.tgz ]] || [[ "${CF_PKG}" == *.tar.gz ]]; then
                        CF_MEMBER="$(tar -tzf "${TMP_CF}" | grep -E '(^|/)cloudflared$' | head -n 1 || true)"
                        if [ -z "${CF_MEMBER}" ]; then
                            rm -f "${TMP_CF}" 2>/dev/null || true
                            continue
                        fi
                        tar -xOzf "${TMP_CF}" "${CF_MEMBER}" > "${CF_TARGET}.bin"
                        rm -f "${TMP_CF}" 2>/dev/null || true
                        mv -f "${CF_TARGET}.bin" "${CF_TARGET}"
                    else
                        mv -f "${TMP_CF}" "${CF_TARGET}"
                    fi

                    if [ -f "${CF_TARGET}" ]; then
                        chmod +x "${CF_TARGET}" 2>/dev/null || true
                        cp -f "${CF_TARGET}" "${INSTALL_DIR}/cloudflared" 2>/dev/null || true
                        chmod +x "${INSTALL_DIR}/cloudflared" 2>/dev/null || true
                        CF_DONE=true
                        msg \
                            "✅ Cloudflare 穿透引擎安装成功: ${CF_TARGET}" \
                            "✅ Cloudflare tunnel engine installed: ${CF_TARGET}" \
                            "✅ Cloudflare トンネルエンジンをインストールしました: ${CF_TARGET}"
                        break
                    fi
                else
                    msg \
                        "⚠️  当前源下载异常或超时，尝试下一个备用源..." \
                        "⚠️  Download failed or timed out; trying fallback source..." \
                        "⚠️  ダウンロードに失敗またはタイムアウトしました。予備ソースを試行します..."
                    rm -f "${TMP_CF}" 2>/dev/null || true
                fi
            done

            if [ "${CF_DONE}" = "false" ]; then
                echo ""
                msg \
                    "⚠️  Cloudflare 穿透引擎下载失败（所有镜像与官方源均连接超时或受阻）。" \
                    "⚠️  Failed to download Cloudflare tunnel engine (all sources timed out or failed)." \
                    "⚠️  Cloudflare トンネルエンジンのダウンロードに失敗しました（全ソースでタイムアウトまたは接続失敗）。"
                msg \
                    "💡 提示: cloudflared 体积约 65MB，国内部分网络访问可能受限。可切换代理节点或开启代理客户端后重试。" \
                    "💡 Tip: cloudflared is ~65MB and may fail on restricted networks. Switch networks or enable a proxy and retry." \
                    "💡 ヒント: cloudflared は約 65MB あり、ネットワーク制限により失敗することがあります。プロキシや回線を切り替えて再試行してください。"

                if [ -t 0 ]; then
                    echo ""
                    RETRY_PROMPT="$(tr_str \
                        "❓ 是否尝试切换代理节点/网络后重试下载？(Y: 切换后重试 / N: 跳过稍后) " \
                        "❓ Retry download after switching network/proxy? (Y: retry / N: skip for now) " \
                        "❓ ネットワークやプロキシを切り替えて再試行しますか？ (Y: 再試行 / N: スキップ) ")"
                    read -r -p "${RETRY_PROMPT}" USER_CHOICE
                    case "${USER_CHOICE}" in
                        [Nn]*)
                            KEEP_TRYING=false
                            msg \
                                "⚠️  已跳过 Cloudflare 引擎预装。后续运行 mgy 时若开启穿透，引擎仍会自动尝试拉取；亦可手动放置 cloudflared 至 ${CF_TARGET}" \
                                "⚠️  Skipped cloudflared pre-install. mgy will retry automatically when tunneling is enabled, or you can place cloudflared manually at ${CF_TARGET}" \
                                "⚠️  cloudflared の事前インストールをスキップしました。次回 mgy 起動時に自動で再取得されます（手動配置先: ${CF_TARGET}）。"
                            ;;
                        *)
                            msg \
                                "🔄 正在准备重试下载，请确保网络或代理已切换就绪..." \
                                "🔄 Preparing to retry download. Please make sure your network/proxy is ready..." \
                                "🔄 ダウンロードを再試行します。ネットワークまたはプロキシの準備を確認してください..."
                            sleep 1
                            ;;
                    esac
                else
                    KEEP_TRYING=false
                    msg \
                        "⚠️  非交互式终端，已跳过预下载。后续运行 mgy 时仍会自动尝试拉取。" \
                        "⚠️  Non-interactive terminal; skipping pre-download. mgy will retry automatically on launch." \
                        "⚠️  非対話型ターミナルのため事前ダウンロードをスキップしました。mgy 起動時に自動で再取得されます。"
                fi
            fi
        done
    fi
fi

# 8.5 首次安装交互式安全向导：局域网配对策略配置
LAN_PREF_CONFIGURED=false
if grep -q "^MULTIGRAVITY_TRUST_LAN=" "${CONF_DIR}/.env" 2>/dev/null && [ "${FORCE_CONFIG:-0}" != "1" ]; then
    LAN_PREF_CONFIGURED=true
fi

# 检查是否支持交互式终端输入
CAN_PROMPT=false
if [ -t 0 ] || [ -c /dev/tty ]; then
    CAN_PROMPT=true
fi

if [ "${LAN_PREF_CONFIGURED}" = "false" ] || [ "${FORCE_CONFIG:-0}" = "1" ]; then
    if [ "${CAN_PROMPT}" = "true" ]; then
        echo ""
        echo "=================================================="
        msg \
            "⚙️  Multigravity 局域网访问安全偏好配置向导" \
            "⚙️  Multigravity LAN Access Security Setup" \
            "⚙️  Multigravity LAN アクセスセキュリティ設定"
        echo "=================================================="
        msg \
            "当您在手机、iPad 或同局域网其他电脑访问本网关时，是否需要配对码？" \
            "Require a pairing code when phones, iPads, or other devices on your LAN connect?" \
            "スマートフォン・iPad・同一 LAN 内の他端末から接続する際、ペアリングコードを必須にしますか？"
        echo ""
        msg \
            "  [1] 🔐 开启局域网配对校验（默认推荐）" \
            "  [1] 🔐 Require LAN pairing code (Recommended)" \
            "  [1] 🔐 LAN ペアリング認証を有効にする（推奨・デフォルト）"
        msg \
            "      适合办公网络、公共 Wi-Fi 或有多人/访客环境。" \
            "      Best for office networks, public Wi-Fi, or shared environments." \
            "      オフィス・公衆 Wi-Fi・共有ネットワーク環境に最適です。"
        msg \
            "      新设备首次接入必须输入配对码授权。" \
            "      New devices must authorize with a pairing code on first connection." \
            "      新しいデバイスは初回接続時にペアリングコードでの認可が必要です。"
        echo ""
        msg \
            "  [2] 🚀 信任局域网免配对（极简直连）" \
            "  [2] 🚀 Trust LAN without pairing (Instant access)" \
            "  [2] 🚀 LAN を信頼してペアリング不要にする（ダイレクト接続）"
        msg \
            "      适合家庭纯净内网、私人可控网络。" \
            "      Best for private, trusted home networks." \
            "      自宅など完全に管理されたプライベートネットワーク向けです。"
        msg \
            "      同局域网设备打开网页即可直接进入桌面工作台，公网穿透仍强制验证。" \
            "      LAN devices connect directly; public Cloudflare Tunnel always requires pairing." \
            "      同一 LAN からは即座に接続できます（パブリックトンネル経由は常に認証必須）。"
        echo ""
        LAN_CHOICE="1"
        LAN_PROMPT="$(tr_str \
            "请选择 [1-2] (默认: 1): " \
            "Select [1-2] (default: 1): " \
            "[1-2] を選択してください (デフォルト: 1): ")"
        if [ -c /dev/tty ]; then
            read -r -p "${LAN_PROMPT}" LAN_CHOICE </dev/tty || LAN_CHOICE="1"
        elif [ -t 0 ]; then
            read -r -p "${LAN_PROMPT}" LAN_CHOICE || LAN_CHOICE="1"
        fi

        case "${LAN_CHOICE}" in
            2|yes|true)
                sed -i.bak '/^MULTIGRAVITY_TRUST_LAN=/d' "${CONF_DIR}/.env" 2>/dev/null || true
                rm -f "${CONF_DIR}/.env.bak" 2>/dev/null || true
                echo "MULTIGRAVITY_TRUST_LAN=1" >> "${CONF_DIR}/.env"
                msg \
                    "✅ 已启用局域网信任免配对直连模式 (MULTIGRAVITY_TRUST_LAN=1)" \
                    "✅ Enabled trusted LAN direct mode (MULTIGRAVITY_TRUST_LAN=1)" \
                    "✅ 信頼済み LAN ダイレクト接続を有効にしました (MULTIGRAVITY_TRUST_LAN=1)"
                ;;
            *)
                sed -i.bak '/^MULTIGRAVITY_TRUST_LAN=/d' "${CONF_DIR}/.env" 2>/dev/null || true
                rm -f "${CONF_DIR}/.env.bak" 2>/dev/null || true
                echo "MULTIGRAVITY_TRUST_LAN=0" >> "${CONF_DIR}/.env"
                msg \
                    "✅ 已保持标准安全配对模式 (MULTIGRAVITY_TRUST_LAN=0)" \
                    "✅ Kept standard LAN pairing verification (MULTIGRAVITY_TRUST_LAN=0)" \
                    "✅ 標準の LAN ペアリング認証を維持しました (MULTIGRAVITY_TRUST_LAN=0)"
                ;;
        esac
        msg \
            "💡 提示: 之后可随时在 ~/.multigravity/.env 中修改 MULTIGRAVITY_TRUST_LAN 的值" \
            "💡 Tip: You can change MULTIGRAVITY_TRUST_LAN anytime in ~/.multigravity/.env" \
            "💡 ヒント: MULTIGRAVITY_TRUST_LAN はいつでも ~/.multigravity/.env で変更できます"
    else
        # 非交互环境保持默认安全策略
        if ! grep -q "^MULTIGRAVITY_TRUST_LAN=" "${CONF_DIR}/.env" 2>/dev/null; then
            echo "MULTIGRAVITY_TRUST_LAN=0" >> "${CONF_DIR}/.env"
        fi
    fi
fi

# 9. 验证安装
INSTALLED_VER="$("${INSTALL_DIR}/${BIN_NAME}" version 2>/dev/null || echo "Multigravity (mgy)")"

echo ""
echo "=================================================="
msg \
    "🎉 安装完成！${INSTALLED_VER}" \
    "🎉 Installation complete! ${INSTALLED_VER}" \
    "🎉 インストールが完了しました！ ${INSTALLED_VER}"
echo "=================================================="
msg \
    "📍 二进制安装位置:   ${INSTALL_DIR}/${BIN_NAME}" \
    "📍 Binary path:      ${INSTALL_DIR}/${BIN_NAME}" \
    "📍 バイナリ配置先:   ${INSTALL_DIR}/${BIN_NAME}"
msg \
    "📁 全局配置与数据:   ${CONF_DIR}/" \
    "📁 Config & data:    ${CONF_DIR}/" \
    "📁 設定・データ:     ${CONF_DIR}/"
msg \
    "📄 配置文件路径:     ${CONF_DIR}/.env" \
    "📄 Config file:      ${CONF_DIR}/.env" \
    "📄 設定ファイル:     ${CONF_DIR}/.env"
echo ""
msg \
    "🚀 常用指令:" \
    "🚀 Common commands:" \
    "🚀 主なコマンド:"
msg \
    "   • 启动网关主服务:   mgy" \
    "   • Start gateway server:     mgy" \
    "   • ゲートウェイを起動:       mgy"
msg \
    "   • 开机后台自动接管: mgy service install" \
    "   • Auto-start on boot:       mgy service install" \
    "   • 自動起動サービス登録:     mgy service install"
msg \
    "   • 终端打印配对码:   mgy pair" \
    "   • Show pairing QR code:     mgy pair" \
    "   • ペアリング QR を表示:     mgy pair"
msg \
    "   • 配置座舱报表:     mgy cockpit" \
    "   • Configure Cockpit sync:   mgy cockpit" \
    "   • Cockpit レポート設定:     mgy cockpit"
msg \
    "   • 配置公网穿透:     mgy cloudflare" \
    "   • Configure public tunnel:  mgy cloudflare" \
    "   • パブリックトンネル設定:   mgy cloudflare"
msg \
    "   • 查看已连接设备:   mgy list" \
    "   • List paired devices:      mgy list" \
    "   • ペアリング済み端末一覧:   mgy list"
msg \
    "   • 清空已配对设备:   mgy clear all" \
    "   • Revoke all devices:       mgy clear all" \
    "   • 全ペアリング端末を解除:   mgy clear all"
msg \
    "   • 查看命令帮助:     mgy help" \
    "   • Show CLI help:            mgy help" \
    "   • コマンドヘルプを表示:     mgy help"
echo ""
if [ "${GLOBAL_LINKED}" = "true" ]; then
    msg \
        "💡 提示: 已配置全局快捷方式，您可以在当前终端及新终端直接运行: mgy" \
        "💡 Tip: Globally linked — run 'mgy' directly in this or any new terminal." \
        "💡 ヒント: グローバルリンクを設定済みです。このターミナルまたは新しいターミナルでそのまま 'mgy' を実行できます。"
elif [ "${PATH_CONFIGURED}" = "false" ] && [ "${OS_TYPE}" != "windows" ]; then
    msg \
        "💡 提示: 请先在新打开的终端运行，或执行生效环境: source ${SHELL_RC_HINT}" \
        "💡 Tip: Open a new terminal or run: source ${SHELL_RC_HINT}" \
        "💡 ヒント: 新しいターミナルを開くか、次を実行して PATH を反映してください: source ${SHELL_RC_HINT}"
fi
msg \
    "📱 手机端使用:" \
    "📱 Mobile app:" \
    "📱 モバイルアプリ:"
msg \
    "   • iOS (App Store): ${APP_STORE_URL}" \
    "   • iOS (App Store): ${APP_STORE_URL}" \
    "   • iOS (App Store): ${APP_STORE_URL}"
msg \
    "   • Android (APK):   https://github.com/${REPO}/releases/latest" \
    "   • Android (APK):   https://github.com/${REPO}/releases/latest" \
    "   • Android (APK):   https://github.com/${REPO}/releases/latest"
msg \
    "   打开 App 扫描终端打印的二维码即可完成配对！" \
    "   Open the app and scan the QR code printed in your terminal to pair!" \
    "   アプリを開き、ターミナルに表示される QR コードを読み取ってペアリングしてください！"
echo "=================================================="
