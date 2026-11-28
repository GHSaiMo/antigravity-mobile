# Multigravity (`mgy`) 自建中继节点（BYO-Relay）配置指南

> 🌐 **简体中文** | [English](self_hosted_relay_guide_en.md) | [日本語](self_hosted_relay_guide_ja.md)

如果你想让 Multigravity 的公网中继流量**完全走自己的服务器（VPS / 云主机）**，可以通过本指南启用内置的 **BYO-Relay（Bring Your Own Relay）自托管中继模式**。

- **无需修改代码、无需重新打包手机 App**：直接使用官方 App Store / TestFlight / GitHub Release 安装包即可；
- **无需部署 Cloudflare Worker 调度器**：中继程序 `mgy-relay` 内置离线 Token 签发命令；
- **彻底脱离官方中继**：配置后电脑端 `mgy` 不再连接官方调度器，也不下载 `cloudflared`，手机端断连时也不会回拨官方节点池；
- **强制零知识端到端加密（`mgy-e2ee-v1`）**：即使走公网中继，你的 VPS 也仅转发 AES-256-GCM 密文，任何明文请求会被电脑端网关直接拒绝（`HTTP 426`）。

> **版本要求**：电脑端网关 `mgy >= v1.2.8`、手机端 iOS / Android App `>= v1.2.8`。

---

## 目录

1. [整体架构与准备工作](#1-整体架构与准备工作)
2. [第一部分：配置自己的中继服务器（VPS）](#2-第一部分配置自己的中继服务器vps)
3. [第二部分：配置电脑端网关（`mgy`）](#3-第二部分配置电脑端网关mgy)
4. [第三部分：配置手机端与 Web 端](#4-第三部分配置手机端与-web-端)
5. [第四部分：多台电脑、APNs 推送与隐私进阶](#5-第四部分多台电脑apns-推送与隐私进阶)
6. [参数与环境变量速查表](#6-参数与环境变量速查表)
7. [常见问题排查（FAQ）](#7-常见问题排查faq)

---

## 1. 整体架构与准备工作

### 1.1 连接原理

```text
你的手机 (iOS / Android / Web)
   │  HTTPS / WSS（应用层 AES-256-GCM 端到端密文）
   ▼  访问隧道域名：https://dev.example.com
你的中继服务器 (mgy-relay)
   ▲  控制域名反向长连接：wss://ctl.example.com/_mgy_tunnel（HMAC Token 鉴权 + yamux 多路复用）
   │
你的电脑 (mgy 网关) ──▶ 本机 Antigravity IDE / CLI
```

### 1.2 准备清单

1. **一台拥有公网 IP 的 Linux 服务器（VPS）**：最低 1 核 512MB 内存即可（`mgy-relay` 空闲内存仅约 10 MB）。
2. **一个自有域名，并配置两条 DNS 解析记录**（下例以 `example.com` 演示，请替换为你的真实域名）：

| 域名角色 | 示例域名 | DNS 记录 | 用途 |
| :--- | :--- | :--- | :--- |
| **控制域名** | `ctl.example.com` | `A` 记录 → 你的 VPS IP | 给电脑端 `mgy` 建立反向隧道与健康检查 `/healthz` |
| **隧道域名** | `dev.example.com` | `A` 记录 → 你的 VPS IP | 给手机端连接与扫码配对；如有多台电脑，可直接配泛解析 `*.mgy.example.com` |

> ⚠️ **必须准备两个不同的域名**：`mgy-relay` 会自动把控制域名（`MGY_RELAY_CONTROL_HOSTS`）设为保留主机，**禁止控制域名与隧道公网域名同名**。

3. **服务器防火墙放行**：放行 TCP **`443`**（HTTPS/WSS 业务流量）；如使用 Caddy 自动申请证书，同时临时或常驻放行 TCP **`80`**。

---

## 2. 第一部分：配置自己的中继服务器（VPS）

### 2.1 编译 `mgy-relay` 单文件二进制

目前 GitHub Release 仅预打包了电脑端 `mgy`，中继服务端 `mgy-relay` 需要从源码编译（Go 静态编译，产物约 7 MB，零外部依赖）。你可以在**自己电脑上交叉编译后上传到 VPS**，也可以直接在 VPS 上编译：

```bash
git clone https://github.com/GHSaiMo/antigravity-mobile.git
cd antigravity-mobile

# Linux x86_64 服务器：
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o mgy-relay ./cmd/relay

# 如果是 ARM64 服务器（如 Oracle ARM / 树莓派 / AWS Graviton），改用：
# CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o mgy-relay ./cmd/relay
```

将编译好的 `mgy-relay` 放到 VPS 的 `/opt/mgy-relay/mgy-relay` 并赋予执行权限：

```bash
sudo mkdir -p /opt/mgy-relay /etc/mgy-relay
sudo mv mgy-relay /opt/mgy-relay/mgy-relay
sudo chmod +x /opt/mgy-relay/mgy-relay
```

---

### 2.2 生成签名密钥并创建配置

在 VPS 上生成一把 256-bit 随机密钥（用于中继服务端与离线 Token 双向鉴权，请妥善保管、切勿公开）：

```bash
openssl rand -hex 32
```

新建配置文件 `/etc/mgy-relay/relay.env`（仅 root 可读）：

```bash
sudo tee /etc/mgy-relay/relay.env > /dev/null <<'EOF'
# 替换为上面生成的 64 位随机十六进制密钥
MGY_RELAY_SIGNING_SECRET=替换成你的64位随机密钥

# 控制域名（与隧道域名必须不同）
MGY_RELAY_CONTROL_HOSTS=ctl.example.com

# 允许接入的隧道域名后缀（务必覆盖默认值 .jiuge.space；也可留空允许任意非保留域名）
MGY_RELAY_ALLOWED_SUFFIXES=.example.com

# 默认监听配置（按 2.3 节选择的方案调整）
MGY_RELAY_LISTEN=127.0.0.1:8443
MGY_RELAY_TLS_CERT=
MGY_RELAY_TLS_KEY=
EOF

sudo chmod 600 /etc/mgy-relay/relay.env
```

> ⚠️ **常见坑点**：`mgy-relay` 的默认 `MGY_RELAY_ALLOWED_SUFFIXES` 为 `.jiuge.space`，默认 `MGY_RELAY_CONTROL_HOSTS` 为 `relay.jiuge.space`。自建时**必须显式改写为自己的域名**，否则电脑连接时会报错 `domain_not_allowed`。

---

### 2.3 配置 HTTPS（二选一）

#### 方案 A（推荐，最省心）：放在 Caddy / Nginx 反向代理后面

保持上面 `/etc/mgy-relay/relay.env` 中的纯 HTTP 本机监听设置不变：

```env
MGY_RELAY_LISTEN=127.0.0.1:8443
MGY_RELAY_TLS_CERT=
MGY_RELAY_TLS_KEY=
```

- **使用 Caddy（强烈推荐，全自动签发与续期 HTTPS 证书）**：

  在 `/etc/caddy/Caddyfile` 中添加：
  ```caddyfile
  ctl.example.com, dev.example.com {
      reverse_proxy 127.0.0.1:8443
  }
  ```
  重载生效：`sudo systemctl reload caddy`

- **使用 Nginx（已有 Nginx 环境）**：

  > 必须保留原始 `Host` 头（`mgy-relay` 依赖 `Host` 路由流量）、开启 WebSocket Upgrade、关闭响应缓冲并调大超时。

  ```nginx
  map $http_upgrade $connection_upgrade {
      default upgrade;
      ''      close;
  }

  server {
      listen 443 ssl http2;
      server_name ctl.example.com dev.example.com;

      ssl_certificate     /path/to/fullchain.pem;
      ssl_certificate_key /path/to/privkey.pem;
      ssl_protocols       TLSv1.2 TLSv1.3;

      client_max_body_size 64m;

      location / {
          proxy_pass http://127.0.0.1:8443;
          proxy_http_version 1.1;

          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;

          # 反向隧道 WSS 与流式对话必需
          proxy_set_header Upgrade $http_upgrade;
          proxy_set_header Connection $connection_upgrade;
          proxy_buffering off;
          proxy_read_timeout 3600s;
          proxy_send_timeout 3600s;
      }
  }
  ```
  检查并重载：`sudo nginx -t && sudo systemctl reload nginx`

---

#### 方案 B（极致精简，免装反代）：`mgy-relay` 直接监听 443

如果 VPS 上没有其他 Web 服务占着 443 端口，且你已用 `acme.sh` 或 `certbot` 签好了同时覆盖 `ctl.example.com` 和 `dev.example.com`（或通配符 `*.example.com`）的证书，只需修改 `/etc/mgy-relay/relay.env`：

```env
MGY_RELAY_LISTEN=:443
MGY_RELAY_TLS_CERT=/etc/mgy-relay/fullchain.pem
MGY_RELAY_TLS_KEY=/etc/mgy-relay/privkey.pem
```

> 提示：使用 `acme.sh` 续期时，设置 `--reloadcmd "systemctl restart mgy-relay"` 即可自动刷新证书。

---

### 2.4 配置 `systemd` 后台常驻并启动

创建服务文件 `/etc/systemd/system/mgy-relay.service`：

```bash
sudo tee /etc/systemd/system/mgy-relay.service > /dev/null <<'EOF'
[Unit]
Description=Multigravity Zero-Knowledge E2EE Reverse Relay
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/mgy-relay/relay.env
ExecStart=/opt/mgy-relay/mgy-relay
Restart=always
RestartSec=2
LimitNOFILE=65536
Environment=GOMEMLIMIT=450MiB

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now mgy-relay
sudo systemctl status mgy-relay --no-pager
```

### 2.5 验证中继服务端已就绪

在任意机器上请求控制域名的 `/healthz`：

```bash
curl -s https://ctl.example.com/healthz
```

看到类似如下 JSON 即表示服务端与 HTTPS 均配置成功：

```json
{"active_conns":0,"active_streams":0,"active_tunnels":0,"service":"mgy-relay","status":"ok","total_streams":0,"uptime_sec":12,"version":"1.1.0"}
```

---

### 2.6 离线签发接入 Token（免 Worker）

在 VPS 上运行内置的 `token` 子命令，为你的电脑签发一张永久有效的离线签名令牌：

```bash
sudo bash -c '
  set -a
  source /etc/mgy-relay/relay.env
  set +a
  /opt/mgy-relay/mgy-relay token \
    --domain dev.example.com \
    --endpoint wss://ctl.example.com/_mgy_tunnel
'
```

终端会直接输出你的 Token 以及可直接复制的 3 行电脑端环境变量：

```env
# Add to your gateway ~/.multigravity/.env (BYO-Relay self-hosted mode):
MGY_CUSTOM_RELAY_ENDPOINT=wss://ctl.example.com/_mgy_tunnel
MGY_CUSTOM_RELAY_TOKEN=mgyr1.eyJ2IjoxLCJzaWQiOi....xxxxxxxx
MGY_CUSTOM_RELAY_PUBLIC_URL=https://dev.example.com
```

把这 3 行复制下来，下一步填进电脑端即可。

---

## 3. 第二部分：配置电脑端网关（`mgy`）

### 3.1 确认网关版本 `>= v1.2.8`

在运行 Antigravity 的电脑终端执行：

```bash
mgy version
```

如果版本低于 `v1.2.8`，先一键升级：

```bash
mgy update
```

### 3.2 写入自建中继配置

打开（或新建）电脑端的网关配置文件：
- **macOS / Linux**：`~/.multigravity/.env`
- **Windows**：`%USERPROFILE%\.multigravity\.env`

将第 2.6 节生成的 3 行粘贴进去并保存：

```env
# 自建零知识 E2EE 中继（BYO-Relay）
MGY_CUSTOM_RELAY_ENDPOINT=wss://ctl.example.com/_mgy_tunnel
MGY_CUSTOM_RELAY_TOKEN=mgyr1.eyJ2IjoxLCJzaWQiOi....xxxxxxxx
MGY_CUSTOM_RELAY_PUBLIC_URL=https://dev.example.com
```

> 说明：
> - 只要同时配置了 `MGY_CUSTOM_RELAY_ENDPOINT` 和 `MGY_CUSTOM_RELAY_TOKEN`，`mgy` 会**自动进入 BYO-Relay 模式**，不再访问官方 `dispatcher.multigravity.app`，也不会下载 `cloudflared`（不需要额外设置 `CF_TUNNEL_ENABLED=0`）。
> - `MGY_CUSTOM_RELAY_PUBLIC_URL` 也可以省略，网关会自动从 Token 内部解析出 `https://dev.example.com`；显式写上便于自己核对。

### 3.3 重启网关并检查日志

- **前台终端运行模式**：在原终端按 `Ctrl + C` 停止旧进程，再重新运行：
  ```bash
  mgy
  ```
- **后台服务模式（`mgy service`）**：
  ```bash
  mgy service restart
  mgy service logs -f
  ```

看到启动横幅或日志出现以下提示，说明电脑端已成功挂接到你的自建中继：

```text
  ➜  云端隧道: 自建 E2EE 中继已就绪 (https://dev.example.com | BYO-Relay)
```
```text
INFO ✅ [BYO-Relay] 自建 E2EE 中继隧道已连接 endpoint=wss://ctl.example.com/_mgy_tunnel url=https://dev.example.com
```

### 3.4 端到端自测（强烈推荐）

在任意一台机器上执行两条命令检查链路和加密防降级机制：

```bash
# 1. 检查服务端 active_tunnels 应变为 1，active_conns 应 >= 1
curl -s https://ctl.example.com/healthz

# 2. 用明文直接访问你的隧道业务接口，预期必须返回 HTTP 426（证明隧道已通且强制 E2EE 生效）
curl -i https://dev.example.com/api/v1/version
```

第 2 条命令返回 **`HTTP/2 426`** 及 `{"error":"e2ee_required", ...}` 即代表「公网 → 自建 VPS → 电脑端网关」整条隧道完全打通且明文防降级保护生效！

---

## 4. 第三部分：配置手机端与 Web 端

手机端**无需重新编译、无需改任何源码**，直接使用 App Store / TestFlight / GitHub Release 发布的官方 App（确认版本号 `>= v1.2.8`）即可。

### 4.1 场景 A：全新安装 / 尚未配对过的手机

1. 在电脑终端运行 `mgy pair`（或直接看刚启动 `mgy` 打印出的二维码），此时二维码中已自动嵌入你的自建域名 `https://dev.example.com`、局域网地址以及 E2EE 公钥。
2. 打开手机端 Multigravity App，点击扫描二维码并扫描终端里的二维码。
3. 等待 1~2 秒配对完成即可使用，**不需要手动填写任何中继地址**。

### 4.2 场景 B：之前已经配对过官方中继的手机（迁移到自建中继）

请任选以下两种方式之一重新绑定到自建域名：

- **方式一（推荐，保留其他主机记录）**：
  1. 电脑端运行 `mgy pair` 生成最新配对二维码；
  2. 打开手机端首页，点击顶部主机名称展开主机切换面板 → 点击 **「+ 添加主机」**；
  3. 扫描电脑端的新二维码，同一台电脑会自动原地更新为你的自建中继地址与 E2EE 域名记录。
- **方式二（彻底重置旧绑定）**：
  1. 在手机端进入 **设置 → 设备配对 → 解除设备配对**；
  2. 电脑端运行 `mgy pair`；
  3. 手机端重新扫码配对即可。

### 4.3 验证手机确实走的是你的自建服务器

1. **关闭手机 Wi-Fi，切到 5G / 4G 移动流量**（确保不走局域网直连）；
2. 在手机 App 里打开任意对话或发送一条消息，确认收发与流式输出流畅；
3. 在你的 VPS 上查看中继统计或实时日志：
   ```bash
   curl -s https://ctl.example.com/healthz
   ```
   可以看到 `total_streams` 随手机操作实时递增，即证明所有远程流量 100% 经由你的服务器中转。

### 4.4 手机端自动路由优先级

配对完成后，手机端依然保持无感多链路智能选路：

```text
① 局域网直连（手机和电脑在同一 Wi-Fi 下，毫秒级直连，不消耗 VPS 流量）
   ↓ 离开家 / 切换至 5G 流量
② 你的自建 E2EE 中继（https://dev.example.com，全程 AES-256-GCM 端到端加密）
```

> 手机端已针对自定义域名做过隔离：当你使用自己的域名时，即使 VPS 暂时离线，手机也**绝不会去请求官方 `dispatcher.multigravity.app` 调度器**。

### 4.5 Web / PWA 端

如果没有关闭中继 Web 页面，直接在手机或外部电脑的浏览器打开：
```text
https://dev.example.com
```
按页面提示输入电脑端 `mgy pair` 给出的 6 位配对码即可使用（同样全程走浏览器 Web Crypto API 端到端加密）。

---

## 5. 第四部分：多台电脑、APNs 推送与隐私进阶

### 5.1 一台 VPS 同时给多台电脑做中继

无需部署多个 `mgy-relay` 实例，也无需搭建 Worker：
1. 在域名 DNS 添加泛解析记录：`*.mgy.example.com` → 你的 VPS IP，并配置好通配符证书（如用 `acme.sh` DNS API 签发 `*.mgy.example.com`）；
2. `/etc/mgy-relay/relay.env` 中设置 `MGY_RELAY_ALLOWED_SUFFIXES=.mgy.example.com`；
3. 为每台电脑分别用不同的 `--domain` 签发一张独立 Token：
   ```bash
   # 公司 Mac
   /opt/mgy-relay/mgy-relay token --domain mac.mgy.example.com --endpoint wss://ctl.example.com/_mgy_tunnel
   # 家里 Windows
   /opt/mgy-relay/mgy-relay token --domain win.mgy.example.com --endpoint wss://ctl.example.com/_mgy_tunnel
   ```
4. 将对应 3 行分别写入两台电脑的 `~/.multigravity/.env`，重启并各自用手机「+ 添加主机」扫码即可在一台手机上自由切换两台电脑。

### 5.2 关于 iOS 远程推送（APNs）与「彻底零官方请求」

启用 BYO-Relay 后，**所有对话、代码、文件和终端流量均已 100% 仅走你的服务器**。唯一仍可能触及官方域名的，是 iOS App Store 版的远程锁屏推送与灵动岛刷新（因 Apple APNs `.p8` 推送密钥与官方 App Bundle ID 绑定，默认走无状态转发节点 `https://push.multigravity.app`）。

你可以按自己的隐私偏好在电脑端 `~/.multigravity/.env` 中三选一：

| 策略 | 配置项 | 效果 |
| :--- | :--- | :--- |
| **保留推送（默认）** | 无需配置 | 支持锁屏审批与灵动岛实时更新；推送中继无数据库、不落盘 |
| **保留推送 + 强脱敏（推荐）** | `MGY_PUSH_REDACT=1` | 擦除会话标题、命令原文、路径与机器名，只发送通知类别与纯数字进度（如 `3/5`） |
| **彻底关闭官方推送** | `APNS_ENABLED=0` | 电脑端**零字节**请求任何官方域名（若需自建推送，可用个人 Apple 开发者账号重签 iOS App 并部署仓库内 [scripts/cloudflare-apns-worker/](../scripts/cloudflare-apns-worker/README.md)） |

### 5.3 其他可选安全加固（电脑端 `.env`）

```env
# 纯原生 App 隐私模式：在中继域名上关闭 Web/PWA 页面，并隐藏 /healthz 中的公钥与主机信息
MGY_RELAY_DISABLE_WEB=1

# 严格防重放模式：确认全部已配对手机都升级到 v1.2.8+ 后可开启
MGY_E2EE_STRICT_REPLAY=1
```

### 5.4 进阶：多台自建 VPS 组成测速容灾池

如果你手里有**多台不同地区的 VPS**，希望电脑自动测速选最快节点并在单机宕机时自动故障转移，可将仓库内的 [scripts/cloudflare-worker/worker.js](../scripts/cloudflare-worker/worker.js) 部署到你自己的 Cloudflare 免费账号（配置 `RELAY_POLICY=prefer` 与 `RELAY_POOL`），然后在电脑端 `.env` 中**去掉** `MGY_CUSTOM_RELAY_*` 并改配：

```env
CF_WORKER_URL=https://你的自定义worker域名
```

---

## 6. 参数与环境变量速查表

### 6.1 服务端 `mgy-relay`（VPS）

| 环境变量 | 命令行参数 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `MGY_RELAY_SIGNING_SECRET` | — | *(必填)* | HMAC-SHA256 签名密钥，与 `mgy-relay token` 使用同一把 |
| `MGY_RELAY_LISTEN` | `-listen` | `:443` | 监听地址；放在 Caddy/Nginx 后填 `127.0.0.1:8443` |
| `MGY_RELAY_TLS_CERT` | `-tls-cert` | `/etc/mgy-relay/origin.crt` | TLS 完整证书链路径；**留空即启用纯 HTTP 模式**（供反代使用） |
| `MGY_RELAY_TLS_KEY` | `-tls-key` | `/etc/mgy-relay/origin.key` | TLS 私钥路径 |
| `MGY_RELAY_CONTROL_HOSTS` | `-control-hosts` | `relay.jiuge.space` | 控制域名列表（逗号分隔），负责 `/_mgy_tunnel` 与 `/healthz`；**自建必改** |
| `MGY_RELAY_ALLOWED_SUFFIXES` | `-allowed-suffixes` | `.jiuge.space` | 允许注册的隧道域名后缀（逗号分隔）；**自建必改**，显式留空则允许任意非保留域名 |
| `MGY_RELAY_RESERVED_HOSTS` | `-reserved-hosts` | `""` | 额外禁止作为隧道域名使用的主机名（逗号分隔） |
| `MGY_RELAY_MAX_TUNNELS` | `-max-tunnels` | `10000` | 最大并发网关隧道数 |
| `MGY_RELAY_MAX_STREAMS_PER_TUNNEL` | `-max-streams` | `64` | 单条隧道最大并发 HTTP/WS 流数量 |

### 6.2 电脑端 `mgy` 网关（`~/.multigravity/.env`）

| 环境变量 | 必填 | 示例 | 说明 |
| :--- | :---: | :--- | :--- |
| `MGY_CUSTOM_RELAY_ENDPOINT` | 是 | `wss://ctl.example.com/_mgy_tunnel` | 控制域名反连地址（也可只写 `https://ctl.example.com`，网关会自动补全 `wss://` 与 `/_mgy_tunnel`） |
| `MGY_CUSTOM_RELAY_TOKEN` | 是 | `mgyr1.xxxx.yyyy` | 由 `mgy-relay token` 签发的离线令牌 |
| `MGY_CUSTOM_RELAY_PUBLIC_URL` | 否 | `https://dev.example.com` | 手机端访问的公网地址（留空时自动从 Token 的 `dom` 字段推导） |
| `MGY_PUSH_REDACT` | 否 | `1` | 推送强脱敏（可选） |
| `APNS_ENABLED` | 否 | `0` | 关闭官方 APNs 推送转发（可选） |
| `MGY_RELAY_DISABLE_WEB` | 否 | `1` | 在中继通道上关闭 Web/PWA 入口，仅允许原生 App（可选） |

---

## 7. 常见问题排查（FAQ）

1. **电脑端日志报 `403 domain_not_allowed`？**
   - 检查 VPS 上 `/etc/mgy-relay/relay.env` 中的 `MGY_RELAY_ALLOWED_SUFFIXES` 是否仍是默认的 `.jiuge.space`，或没有覆盖你的隧道域名后缀（例如隧道域名是 `dev.example.com`，后缀应写 `.example.com`）；
   - 检查是否不小心把**控制域名**和**隧道域名**写成了同一个域名（控制域名会自动被禁止作为隧道域名）。

2. **电脑端日志报 `401 invalid_token`？**
   - 签发 Token 时用的 `MGY_RELAY_SIGNING_SECRET` 与 `mgy-relay` 进程读取的密钥不一致，或复制 `.env` 时多带了换行符/空格。请确认两边密钥逐字节一致后重新签发并重启。

3. **浏览器或手机访问 `https://dev.example.com` 返回 `503 {"error":"gateway_offline"}`？**
   - 说明 DNS、HTTPS 和反代本身都已打通，但电脑端 `mgy` 还没连上你的 VPS（或电脑关机/休眠了）；
   - 检查电脑端 `mgy` 日志（`mgy service logs -f`）和服务器日志（`sudo journalctl -u mgy-relay -n 50 --no-pager`），以及 `curl https://ctl.example.com/healthz` 中 `active_tunnels` 是否为 `1`。

4. **通过 Nginx 反代后，隧道每 60 秒断连一次，或 AI 流式回复卡顿？**
   - 检查 Nginx `location /` 中是否漏了：
     - `proxy_http_version 1.1;`
     - `proxy_set_header Upgrade $http_upgrade;` 与 `proxy_set_header Connection $connection_upgrade;`
     - `proxy_read_timeout 3600s;`
     - `proxy_buffering off;`
   - 如果 Nginx 漏了 `proxy_set_header Host $host;`，会导致 `mgy-relay` 收到错误的 `Host` 头，从而返回 `unknown_host` 或误命中 `/healthz`。

5. **直接用 `curl` 访问 `https://dev.example.com/api/v1/version` 返回 `HTTP 426 e2ee_required` 是报错了吗？**
   - **不是报错，而是正常的安全防护！** 这说明你的自建中继已经完美连通电脑端网关，且电脑端正在强制拦截一切明文请求。使用手机 App 扫码配对后会自动走加密通道正常通信。

6. **以后想切回官方免费中继怎么操作？**
   - 删除或注释掉电脑 `~/.multigravity/.env` 里的 `MGY_CUSTOM_RELAY_*` 三行配置，重启 `mgy`，再用手机重新扫一次 `mgy pair` 二维码即可无损切回官方多地域中继池。
