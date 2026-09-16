#!/usr/bin/env bash
# Issue / renew Let's Encrypt cert for agy.jiuge.space via Cloudflare DNS-01.
# Prerequisites:
#   1. Cloudflare A record agy.jiuge.space → 124.222.226.143, proxy OFF (grey cloud)
#   2. export CF_Token='...'  (zone DNS edit token; do not commit)
# See docs/https_cloud_relay_guide.md
set -euo pipefail

DOMAIN="${AGY_TLS_DOMAIN:-agy.jiuge.space}"
CERT_DIR="${AGY_TLS_DIR:-${HOME}/.antigravity-mobile/certs}"
ACME_HOME="${HOME}/.acme.sh"
ACME="${ACME_HOME}/acme.sh"
EMAIL="${AGY_TLS_EMAIL:-taojiuzhen@gmail.com}"

if [[ -z "${CF_Token:-}" && -z "${CF_Token_File:-}" ]]; then
	echo "❌ 未设置 CF_Token。"
	echo "   在 Cloudflare 中文控制台：右上角头像 → 我的个人资料 → API 令牌 → 创建令牌 → 使用模板「编辑区域 DNS」→ 区域选 jiuge.space"
	echo "   然后: export CF_Token='你的令牌'"
	exit 1
fi

if [[ -n "${CF_Token_File:-}" && -z "${CF_Token:-}" ]]; then
	CF_Token="$(tr -d ' \r\n' < "${CF_Token_File}")"
	export CF_Token
fi

if [[ ! -x "${ACME}" ]]; then
	echo "➡️  安装 acme.sh ..."
	curl https://get.acme.sh | sh -s email="${EMAIL}"
fi

if [[ ! -x "${ACME}" ]]; then
	echo "❌ acme.sh 未找到：${ACME}"
	exit 1
fi

mkdir -p "${CERT_DIR}"
chmod 700 "${CERT_DIR}"

echo "➡️  向 Let's Encrypt 申请 ${DOMAIN}（Cloudflare DNS-01）..."
"${ACME}" --issue --dns dns_cf -d "${DOMAIN}" --server letsencrypt --home "${ACME_HOME}"

FULLCHAIN="${CERT_DIR}/${DOMAIN}.fullchain.cer"
KEYFILE="${CERT_DIR}/${DOMAIN}.key"

echo "➡️  安装证书到 ${CERT_DIR} ..."
"${ACME}" --install-cert -d "${DOMAIN}" --home "${ACME_HOME}" \
	--fullchain-file "${FULLCHAIN}" \
	--key-file "${KEYFILE}" \
	--reloadcmd "chmod 600 '${FULLCHAIN}' '${KEYFILE}' && echo 'certs refreshed; restart gateway (tmux: C-c then make run) to load them'"

chmod 600 "${FULLCHAIN}" "${KEYFILE}"
echo "✅ 证书已写入："
echo "   TLS_CERT_FILE=${FULLCHAIN}"
echo "   TLS_KEY_FILE=${KEYFILE}"
echo
echo "下一步：把这三项写入项目 .env 后重启网关（见 docs/https_cloud_relay_guide.md）"
echo "   DDNS_HOST=${DOMAIN}"
echo "   GATEWAY_SSL=1"
echo "   TLS_CERT_FILE=${FULLCHAIN}"
echo "   TLS_KEY_FILE=${KEYFILE}"
