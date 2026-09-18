#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Multigravity (mgy) macOS 一键极简安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
# ==============================================================================

REPO="${MULTIGRAVITY_REPO:-GHSaiMo/antigravity-mobile}"
BIN_NAME="mgy"
INSTALL_DIR="${HOME}/.local/bin"
CONF_DIR="${HOME}/.multigravity"

echo "=================================================="
echo "🚀 正在安装 Multigravity (mgy) for macOS..."
echo "=================================================="

# 1. 检查操作系统 (仅支持 macOS)
OS="$(uname -s)"
if [ "${OS}" != "Darwin" ]; then
    echo "❌ 错误: 本一键脚本仅适用于 macOS。当前操作系统: ${OS}"
    echo "   Linux 或 Windows 用户请参考项目文档手动安装。"
    exit 1
fi

# 2. 探测芯片架构
ARCH="$(uname -m)"
case "${ARCH}" in
    arm64|aarch64)
        ARCH_DESC="Apple Silicon (M系列)"
        PKG_ARCH="arm64"
        ;;
    x86_64|amd64)
        ARCH_DESC="Intel x86_64"
        PKG_ARCH="amd64"
        ;;
    *)
        echo "❌ 暂不支持的 Mac 架构: ${ARCH}"
        exit 1
        ;;
esac
echo "🖥️  检测到系统架构: ${ARCH_DESC} (${PKG_ARCH})"

# 3. 准备安装与配置目录
mkdir -p "${INSTALL_DIR}"
mkdir -p "${CONF_DIR}" "${CONF_DIR}/logs"

# 4. 初始化全局默认配置文件 ~/.multigravity/.env (若不存在)
if [ ! -f "${CONF_DIR}/.env" ]; then
    cat << 'ENVEOF' > "${CONF_DIR}/.env"
# Multigravity 全局环境变量配置文件
# 保存路径: ~/.multigravity/.env

# 网关监听端口 (默认 58900)
MULTIGRAVITY_PORT=58900

# 网关监听主机/IP (默认留空双栈绑定所有网卡，设为 127.0.0.1 仅限本机)
# MULTIGRAVITY_HOST=127.0.0.1

# 公网 DDNS 域名或固定 IPv6 地址 (若需要外网直连)
# DDNS_HOST=agy.example.com

# 公网 IPv6 自动广播 (默认 1：检测到公网 IPv6 时自动打入复合配对二维码与链接；设为 0 关闭)
INCLUDE_PUBLIC_IPV6=1

# 是否默认优先使用纯 IPv6 作为二维码 (默认 0 生成双栈复合码；设为 1 纯 IPv6 码)
# MULTIGRAVITY_PREFER_IPV6=0

# HTTPS / SSL 加密访问 (启用需设为 1 并指定证书和私钥文件)
# MULTIGRAVITY_SSL=0
# MULTIGRAVITY_TLS_CERT=~/.multigravity/certs/fullchain.cer
# MULTIGRAVITY_TLS_KEY=~/.multigravity/certs/private.key

# iOS Bark 实时推送通知 (填入 Device Key 或 Bark 完整 URL)
# BARK_URL=
# BARK_ICON=https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/web/icons/icon-192.png
# BARK_GROUP=Antigravity
# BARK_SOUND_ACTION=alarm
# BARK_SOUND_COMPLETE=glass

# FRP 内网穿透云中继配置 (在外网无公网 IP 时使用)
# FRP_ENABLED=false
# FRP_SERVER_ADDR=frp.example.com
# FRP_SERVER_PORT=7000
# FRP_TOKEN=your-strong-token
# FRP_REMOTE_PORT=58900

# 管理员特权密钥 (外网访问或开启 FRP 时用于鉴权，留空则首次运行自动生成)
# MULTIGRAVITY_ADMIN_TOKEN=
ENVEOF
    chmod 600 "${CONF_DIR}/.env"
    echo "📝 已生成全局默认配置: ${CONF_DIR}/.env"
fi

# 6. 下载并安装对应架构二进制 (优先单架构轻量包 ~7MB，降级回退 Universal 包 ~15MB)
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT
TAR_FILE="${TMP_DIR}/multigravity.tar.gz"

# 自动探测本机常用代理端口 (Clash / V2Ray / Surge 等)
PROXY_FLAG=""
if [ -z "${https_proxy:-}" ] && [ -z "${http_proxy:-}" ] && [ -z "${all_proxy:-}" ]; then
    for test_port in 7890 10808 1080 6152; do
        if nc -z -w 1 127.0.0.1 "${test_port}" 2>/dev/null; then
            echo "⚡ 检测到本机代理环境 (127.0.0.1:${test_port})，已自动接入加速"
            PROXY_FLAG="--proxy http://127.0.0.1:${test_port}"
            break
        fi
    done
fi

ARCH_TAR="multigravity-darwin-${PKG_ARCH}.tar.gz"
UNIV_TAR="multigravity-darwin-universal.tar.gz"

DOWNLOAD_URLS=(
    "https://github.com/${REPO}/releases/latest/download/${ARCH_TAR}"
    "https://ghfast.top/https://github.com/${REPO}/releases/latest/download/${ARCH_TAR}"
    "https://ghproxy.net/https://github.com/${REPO}/releases/latest/download/${ARCH_TAR}"
    "https://github.com/${REPO}/releases/latest/download/${UNIV_TAR}"
    "https://ghfast.top/https://github.com/${REPO}/releases/latest/download/${UNIV_TAR}"
    "https://ghproxy.net/https://github.com/${REPO}/releases/latest/download/${UNIV_TAR}"
)

DOWNLOAD_SUCCESS=false
echo "📥 正在获取 Multigravity (${PKG_ARCH}) 最新发行版..."

for d_url in "${DOWNLOAD_URLS[@]}"; do
    echo "🔗 尝试下载: ${d_url}"
    # shellcheck disable=SC2086
    if curl -fL ${PROXY_FLAG} --connect-timeout 6 --speed-limit 10240 --speed-time 8 -# -o "${TAR_FILE}" "${d_url}"; then
        if tar -tzf "${TAR_FILE}" >/dev/null 2>&1; then
            DOWNLOAD_SUCCESS=true
            break
        else
            echo "⚠️  下载的文件损坏或非标准 tar.gz，正在尝试下一个源..."
            rm -f "${TAR_FILE}"
        fi
    else
        echo "⚠️  连接超时或速度较慢，正在自动切换加速镜像源..."
    fi
done

if [ "${DOWNLOAD_SUCCESS}" = "true" ]; then
    echo "📦 下载完成，正在解压安装..."
    tar -xzf "${TAR_FILE}" -C "${TMP_DIR}"
    if [ -f "${TMP_DIR}/${BIN_NAME}" ]; then
        mv "${TMP_DIR}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
    elif [ -f "${TMP_DIR}/gateway" ]; then
        mv "${TMP_DIR}/gateway" "${INSTALL_DIR}/${BIN_NAME}"
    else
        echo "❌ 解压归档中未找到可执行文件。"
        exit 1
    fi
else
    # 所有下载源均失败时的回退检查
    echo "⚠️  未能从网络镜像获取预编译包。"
    if [ -f "./bin/${BIN_NAME}" ]; then
        echo "💡 检测到当前目录存在编译好的 ./bin/${BIN_NAME}，正在直接复制安装..."
        cp "./bin/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
    elif command -v go >/dev/null 2>&1 && [ -f "go.mod" ]; then
        echo "🔨 检测到本地 Go 编译环境，正在就地编译..."
        go build -ldflags="-s -w -X 'main.Version=1.0.0'" -o "${INSTALL_DIR}/${BIN_NAME}" ./cmd/gateway
    else
        echo "❌ 无法下载 Release 预编译包且无可用本地环境。"
        echo "   您可以尝试开启代理或手动访问以下地址下载解压:"
        echo "   https://github.com/${REPO}/releases/latest"
        exit 1
    fi
fi

chmod +x "${INSTALL_DIR}/${BIN_NAME}"

# 7. 绕过 macOS Gatekeeper / Quarantine 并完成 Ad-hoc 签名
xattr -d com.apple.quarantine "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || true
codesign -s - -f "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || true

# 8. 检查 PATH 环境变量
SHELL_NAME="$(basename "${SHELL:-zsh}")"
RC_FILE="${HOME}/.zshrc"
if [ "${SHELL_NAME}" = "bash" ]; then
    RC_FILE="${HOME}/.bash_profile"
fi

PATH_CONFIGURED=true
if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
    PATH_CONFIGURED=false
    if [ -f "${RC_FILE}" ]; then
        if ! grep -q '\.local/bin' "${RC_FILE}" 2>/dev/null; then
            echo '' >> "${RC_FILE}"
            echo '# Multigravity CLI PATH' >> "${RC_FILE}"
            echo 'export PATH="$HOME/.local/bin:$PATH"' >> "${RC_FILE}"
            echo "🔧 已自动将 ~/.local/bin 追加到 ${RC_FILE}"
        fi
    fi
fi

# 9. 验证安装
INSTALLED_VER="$("${INSTALL_DIR}/${BIN_NAME}" version 2>/dev/null || echo "1.0.0")"

echo ""
echo "=================================================="
echo "🎉 安装完成！${INSTALLED_VER}"
echo "=================================================="
echo "📍 二进制安装位置:   ${INSTALL_DIR}/${BIN_NAME}"
echo "📁 全局配置与数据:   ${CONF_DIR}/"
echo "📄 配置文件路径:     ${CONF_DIR}/.env"
echo ""
echo "🚀 常用指令:"
echo "   • 启动网关主服务:   mgy"
echo "   • 终端打印配对码:   mgy pair"
echo "   • 查看已连接设备:   mgy list"
echo "   • 清空已配对设备:   mgy clear all"
echo "   • 查看命令帮助:     mgy help"
echo ""
if [ "${PATH_CONFIGURED}" = "false" ]; then
    echo "💡 提示: 请先在新打开的终端运行，或执行生效环境: source ${RC_FILE}"
fi
echo "📱 手机端使用:"
echo "   请在 GitHub Releases 下载安装 Multigravity-*.apk，"
echo "   打开 App 扫描终端打印的二维码即可完成配对！"
echo "=================================================="
