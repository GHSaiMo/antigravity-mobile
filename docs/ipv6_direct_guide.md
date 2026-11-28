# Multigravity (`mgy`) IPv6 直连配置指南

> 🌐 [English Quick Guide →](#english-quick-guide)

如果你所在的家庭宽带、校园网、公司内网或云主机具备 **IPv6**，可以通过本指南为 Multigravity 网关开启 **IPv4/IPv6 双栈监听**（以及可选的**免反代原生 HTTPS 端口**），让手机在 5G/4G 或其他支持 IPv6 的网络下直接连接你的电脑，获得更低延迟和更高带宽；当手机处于无 IPv6 的 Wi-Fi 环境时，仍会自动回退到官方 E2EE 加密中继，无缝不断联。

> **适用版本**：网关 `mgy >= v1.2.9-hotfix.1`（及后续 `v1.2.9+` 正式版），**默认保持纯 IPv4 (`tcp4`) 关闭状态**，按需手动开启，对现有用户零影响。  
> **手机端**：iOS / Android 无需更新即可直接使用 **DDNS + HTTPS** 方式直连。

---

## 目录

1. [先升级网关](#1-先升级网关)
2. [三种直连方式怎么选](#2-三种直连方式怎么选)
3. [前置检查：确认电脑与路由器 IPv6 可用](#3-前置检查确认电脑与路由器-ipv6-可用)
4. [方式一（推荐）：DDNS + 网关原生 HTTPS（免反代）](#4-方式一推荐ddns--网关原生-https免反代)
5. [方式二：DDNS + 反向代理（Lucky / Caddy / Nginx）](#5-方式二ddns--反向代理lucky--caddy--nginx)
6. [方式三：Tailscale / 局域网私网 IPv6 直连](#6-方式三tailscale--局域网私网-ipv6-直连)
7. [手机端配置与自动路由优先级](#7-手机端配置与自动路由优先级)
8. [进阶选项：关闭默认中继 / 强制 E2EE / 后台自启服务](#8-进阶选项关闭默认中继--强制-e2ee--后台自启服务)
9. [参数与环境变量速查表](#9-参数与环境变量速查表)
10. [常见问题排查（FAQ）](#10-常见问题排查faq)
11. [English Quick Guide](#english-quick-guide)

---

## 1. 先升级网关

在运行 `mgy` 的电脑终端执行：

```bash
# 首次升级到包含 IPv6 直连支持的定向热修复版
mgy update v1.2.9-hotfix.1

# 如果此前已安装过早期 v1.2.9-hotfix.1，请加 -force 强制刷新
mgy update -force v1.2.9-hotfix.1
```

验证新参数已就绪：

```bash
mgy help | grep -E "ipv6|tls-"
```

看到 `-ipv6`、`-tls-cert`、`-tls-key`、`-tls-port` 即表示版本正确。

---

## 2. 三种直连方式怎么选

| 方式 | 适用场景 | 是否需要域名与证书 | 推荐程度 |
| :--- | :--- | :--- | :--- |
| **方式一：DDNS + 网关原生 HTTPS** | 已有域名和 ACME 证书，不想额外装 Nginx/Caddy | 需要（单进程直出，自动热重载证书） | ⭐⭐⭐⭐⭐ 最简洁 |
| **方式二：DDNS + Lucky / Caddy / Nginx 反代** | 家里 NAS、软路由或本机本来就在跑反代 + 自动签证书 | 由反代自动申请和续期 | ⭐⭐⭐⭐⭐ 已有反代首选 |
| **方式三：Tailscale / 私网 IPv6** | 已组建 Tailscale 异地组网，或仅在局域网内用私网 IPv6 | 不需要（走加密隧道或局域网） | ⭐⭐⭐⭐ 零域名方案 |

> **为什么不支持「无域名无证书的公网裸 IPv6 明文 HTTP」（如 `http://[2408:...]:58900`）？**
> 1. **安全原因**：公网明文 HTTP 会直接暴露设备认证令牌（Bearer Token）与对话内容；
> 2. **系统限制**：iOS 系统级 ATS 与 Android 客户端安全拦截器均默认禁止向公网地址发送明文 HTTP；
> 3. **地址稳定性**：家庭宽带 IPv6 前缀与系统临时隐私地址会定期轮换，裸 IP 配对后过一两天就会失联，使用 DDNS 域名（AAAA 记录）才稳定省心。

---

## 3. 前置检查：确认电脑与路由器 IPv6 可用

### 3.1 检查电脑是否拿到公网 IPv6

- **macOS / Linux**：
  ```bash
  curl -6 -s https://api64.ipify.org && echo
  ```
- **Windows (PowerShell)**：
  ```powershell
  curl.exe -6 -s https://api64.ipify.org
  ```

如果输出类似 `2408:...`、`2409:...`、`240e:...` 或 `2001:...` 的地址，说明电脑已具备公网 IPv6。

### 3.2 路由器 IPv6 防火墙放行（最常见卡点）

国内家宽光猫与路由器默认开启 **IPv6 防火墙**，会拦截外网主动连入电脑的 TCP 连接。

- **安全做法**：在路由器的「IPv6 防火墙 / 通信规则 / 入站规则」中，**仅放行目标端口的 TCP 入站流量**（例如方式一的 `58943`，或方式二反代的 HTTPS 端口），**不要放行 `58900`，也不要整机裸奔关闭防火墙**。
- **避开 80 / 443**：家庭宽带运营商普遍封禁入站 `80` 和 `443` 端口，请使用高位端口（如默认的 **`58943`**）。

---

## 4. 方式一（推荐）：DDNS + 网关原生 HTTPS（免反代）

这种方式无需部署任何反向代理。`mgy` 会在独立端口（默认 `58943`）启用原生 **TLS 1.2+**，同时保留原 `58900` HTTP 端口供本机 CLI（`mgy pair` / `list` / `clear`）与 Wi-Fi 局域网使用。

### 步骤 1：准备 DDNS（AAAA 记录）与 PEM 证书

- 使用 `ddns-go`、Lucky、路由器自带 DDNS 或域名商 API，将你的域名（例如 `home.example.com`）的 **AAAA 记录** 动态指向电脑的公网 IPv6。
- 使用 `acme.sh`、`certbot`、`ddns-go` 或 Lucky 申请免费 Let's Encrypt / ZeroSSL 证书，得到两个 PEM 文件：
  - 完整证书链：`fullchain.pem`（**注意用完整链，不要只用单张 `cert.pem`**）
  - 私钥：`privkey.pem`

### 步骤 2：写入 `~/.multigravity/.env`

打开（或新建）配置文件 `~/.multigravity/.env`（Windows 位于 `%USERPROFILE%\.multigravity\.env`），添加：

```env
# 开启 IPv4 / IPv6 双栈监听
MULTIGRAVITY_ENABLE_IPV6=1

# 启用独立原生 HTTPS 端口（支持 ACME 证书自动热重载，续期无需重启 mgy）
MGY_TLS_CERT=/path/to/fullchain.pem
MGY_TLS_KEY=/path/to/privkey.pem
MGY_TLS_PORT=58943

# 可选：填写 DDNS 域名，便于启动看板直接展示完整地址
DDNS_HOST=home.example.com
```

也可以直接通过命令行参数临时体验：

```bash
mgy -ipv6 -tls-cert /path/to/fullchain.pem -tls-key /path/to/privkey.pem -tls-port 58943 -ddns home.example.com
```

### 步骤 3：重启网关并确认启动看板

重启 `mgy` 后，终端看板中会出现两行提示：

```text
  ➜  双栈监听:   IPv4 / IPv6 双栈已开启 (:58900 | 支持 Tailscale/反代/DDNS 直连)
  ➜  原生 HTTPS: https://home.example.com:58943 (TLS 1.2+，证书自动热重载，监听 :58943)
```

### 步骤 4：自测公网 IPv6 HTTPS 连通性

在路由器放行 TCP `58943` 入站后，用手机断开 Wi-Fi（切到 5G/4G 流量），或在其他支持 IPv6 的机器上执行：

```bash
curl -6 -i https://home.example.com:58943/healthz
```

返回 `HTTP/2 200`（或 `HTTP/1.1 200`）和 `{"status":"ok", ...}` 即说明公网 IPv6 HTTPS 直连已彻底打通。

> **关于证书自动续期**：`mgy` 每 30 秒自动检查证书文件的修改时间与大小，当 `acme.sh` / `certbot` 续期覆盖证书后会自动热重载，**无需重启网关**；即便写入瞬间文件短暂不完整，也会安全保留旧证书不中断连接。

---

## 5. 方式二：DDNS + 反向代理（Lucky / Caddy / Nginx）

如果你已经在用 Lucky、Caddy 或 Nginx 做域名反代与自动签证书，直接把反代转发给 `mgy` 的 `58900` 端口即可。

### 步骤 1：决定是否开启双栈监听

- **反代和 `mgy` 在同一台电脑上**：反代直接转发到 `http://127.0.0.1:58900` 即可，其实连 `MULTIGRAVITY_ENABLE_IPV6=1` 都不必开。
- **反代在路由器 / NAS 等另一台设备上，需要通过 IPv6 访问电脑上的 `mgy`**：在电脑的 `~/.multigravity/.env` 中开启：
  ```env
  MULTIGRAVITY_ENABLE_IPV6=1
  ```
  然后反代后端指向 `http://[电脑IPv6]:58900`（或同一局域网下的 `http://电脑局域网IPv4:58900`）。

### 步骤 2：常见反代配置参考

> Multigravity 使用 **WebSocket** 推送流式回复与事件，反代必须正确转发 `Upgrade` / `Connection` 头并调大读超时。

#### Caddy（最省心，内置自动 HTTPS + WebSocket 原生透传）

```caddyfile
home.example.com:58943 {
    reverse_proxy 127.0.0.1:58900
}
```

#### Nginx

```nginx
server {
    listen [::]:58943 ssl http2;
    listen 58943 ssl http2;
    server_name home.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:58900;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket / 长连接流式输出必需
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_buffering off;
    }
}
```

#### Lucky（国内软路由 / NAS 常用神器）

1. 在 Lucky「动态域名」配好公网 IPv6 DDNS（AAAA 记录）；
2. 在 Lucky「SSL/TLS 证书」申请对应域名证书；
3. 在 Lucky「Web 服务」新增规则：
   - 监听类型勾选 `tcp6`（或 `tcp4` + `tcp6`），监听端口填高位端口（如 `58943`），开启 **TLS**；
   - 新增子规则，前端域名填 `home.example.com`，后端地址填 `http://127.0.0.1:58900`（或跨设备内网地址）；
   - Lucky 默认支持 WebSocket 透传，保存即可。

---

## 6. 方式三：Tailscale / 局域网私网 IPv6 直连

如果你已经在使用 Tailscale，且希望通过 Tailscale IPv6（`fd7a:115c:a1e0::/48`）或局域网 ULA IPv6（`fc00::/7`）直连：

1. 在电脑 `~/.multigravity/.env` 中开启双栈监听（或直接仅绑定 Tailscale IPv6）：
   ```env
   MULTIGRAVITY_ENABLE_IPV6=1
   # 如希望网关仅绑定在 Tailscale IPv6 接口上，也可直接指定：
   # MULTIGRAVITY_HOST=fd7a:115c:a1e0::xxxx
   ```
2. 重启 `mgy`；
3. 手机端在「自定义地址」填入 `http://[fd7a:115c:a1e0::xxxx]:58900`（需手机端支持 Tailscale IPv6 明文放行；也可继续使用 Tailscale IPv4 `http://100.x.x.x:58900` 或 MagicDNS `http://你的机器名.tailxxxx.ts.net:58900`）。

---

## 7. 手机端配置与自动路由优先级

1. 如果尚未配对，先用手机扫描 `mgy` 启动时（或 `mgy pair`）生成的二维码完成配对；
2. 打开手机端 **设置 → 网络连接 → 自定义地址**；
3. 填入你的直连地址并保存，例如：
   ```text
   https://home.example.com:58943
   ```
4. 无需任何额外开关，手机端会自动按以下顺序智能选路：

```text
① Wi-Fi 局域网直连（同 Wi-Fi 下毫秒级延迟，优先命中）
   ↓ 离开家 / 切 5G 蜂窝网络
② 自定义地址（你的 IPv6 HTTPS 直连，高带宽低延迟）
   ↓ 遇到无 IPv6 的公司 Wi-Fi / 防火墙阻断
③ 官方 E2EE 端到端加密中继（自动无缝兜底，绝不断联）
```

---

## 8. 进阶选项：关闭默认中继 / 强制 E2EE / 后台自启服务

### 8.1 仅走自己的 IPv6 直连（关闭默认云端中继）

如果你不需要官方中继兜底，希望彻底纯自建直连，可在 `~/.multigravity/.env` 中增加：

```env
CF_TUNNEL_ENABLED=0
```

> 建议：只有在你确认日常外出的所有网络（5G、公司 Wi-Fi 等）都稳定具备 IPv6 时再关闭中继；保留开启不会影响直连优先级，还能在无 IPv6 环境下自动救场。

### 8.2 在自有反代 / CDN 后强制叠加零知识 E2EE

如果你通过第三方反代或 CDN 转发流量，希望反代节点也完全看不到聊天明文，可在 `.env` 开启：

```env
MGY_FORCE_E2EE=1
```

开启后，所有经由反代或公网来源的请求都必须走端到端加密通道（iOS / Android 会自动识别 `HTTP 426` 并无缝切换 E2EE，本机 CLI 与局域网直连不受影响）。

### 8.3 配合 `mgy service` 开机后台自启

- 如果参数都写在 `~/.multigravity/.env` 中，只需执行一次重启即可生效：
  ```bash
  mgy service restart
  ```
- 如果希望把 `-ipv6` 直接写进后台服务启动参数，可在安装服务时带上 `-ipv6`：
  ```bash
  mgy service install -ipv6
  ```

---

## 9. 参数与环境变量速查表

| 命令行参数 | 环境变量（前者优先） | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-ipv6` | `MULTIGRAVITY_ENABLE_IPV6` / `MGY_ENABLE_IPV6` | `false` (`0`) | 开启 IPv4/IPv6 双栈监听；默认仅监听 IPv4 (`tcp4`) |
| `-host <地址>` | `MULTIGRAVITY_HOST` | `""` | 指定绑定地址；直接填 IPv6（如 `::`、`::1`、`[fd7a:...]`）会自动启用 IPv6 监听 |
| `-port <端口>` | `MULTIGRAVITY_PORT` | `58900` | 主 HTTP / WebSocket 监听端口（供本机 CLI、局域网与反代使用） |
| `-tls-cert <路径>` | `MGY_TLS_CERT` / `MULTIGRAVITY_TLS_CERT` | `""` | 原生 HTTPS 完整证书链路径（PEM），需与 `-tls-key` 同时配置 |
| `-tls-key <路径>` | `MGY_TLS_KEY` / `MULTIGRAVITY_TLS_KEY` | `""` | 原生 HTTPS 私钥路径（PEM），支持 ACME 续期自动热重载 |
| `-tls-port <端口>` | `MGY_TLS_PORT` / `MULTIGRAVITY_TLS_PORT` | `58943` | 原生 HTTPS 独立监听端口（不可与 `-port` 相同） |
| `-ddns <域名>` | `DDNS_HOST` | `""` | 公网 DDNS 域名，用于看板显示与配对端点 |
| — | `CF_TUNNEL_ENABLED` | `1` | 设为 `0` 可关闭默认云端中继，仅保留本地/局域网/自建直连 |
| — | `MGY_FORCE_E2EE` | `0` | 设为 `1` 强制所有外部/反代流量走零知识 E2EE |

---

## 10. 常见问题排查（FAQ）

1. **电脑本地测试正常，但手机切 5G 连超时？**
   - 90% 是**光猫或路由器的 IPv6 防火墙**拦截了入站连接。请检查：
     - 光猫是否为桥接模式（如果是光猫拨号，光猫自身也有一层 IPv6 防火墙需要放行）；
     - 路由器 IPv6 防火墙是否放行了目标 TCP 端口（如 `58943`）；
     - DDNS 的 AAAA 记录是否已更新为电脑**当前实际生效**的公网 IPv6（注意不要解析成了已过期的临时地址或路由器自己的 IPv6）。
2. **手机提示「SSL 握手失败」或证书不受信任？**
   - 检查证书文件是否使用的是**完整证书链 `fullchain.pem`**（缺少中间证书会导致手机端 TLS 校验失败）；
   - 检查手机端填写的域名是否与证书 SAN / CN 匹配，且协议前缀是 `https://`、端口是原生 TLS 端口（如 `58943`）或反代 HTTPS 端口，而不是 HTTP 端口 `58900`。
3. **手机提示「公网地址不支持明文 HTTP」？**
   - 手机端出于安全策略（iOS ATS / Android 拦截器）禁止公网明文 HTTP，请配置域名证书或反代并使用 `https://` 地址。
4. **为什么不能把 `-tls-port` 设成和 `-port` 一样（例如都用 `58900`）？**
   - 保持独立 HTTPS 端口可以让本机 `mgy pair` / `mgy list` / `mgy clear`、`mgy service status` 健康检查以及 Wi-Fi 局域网 HTTP 零干扰正常运行，同时方便你在路由器防火墙里只暴露 HTTPS 端口、把 HTTP 端口留在内网。
5. **开启 `-ipv6` 后，原有的 IPv4 局域网和 `127.0.0.1` 还能连吗？**
   - 可以。未指定具体 IPv6 地址（`-host ""` 或 `-host ::`）时，Go 网关绑定的是标准 **IPv4/IPv6 双栈套接字**，`127.0.0.1`、`192.168.x.x` 与 IPv6 会同时正常工作。
6. **在没有 IPv6 的公司 Wi-Fi 下会断联吗？**
   - 只要没有关闭 `CF_TUNNEL_ENABLED=0`，手机端探测到 IPv6 自定义地址不可达后，会自动无缝切换到官方 E2EE 中继，切回 5G 或有 IPv6 网络后再自动恢复直连。

---

## English Quick Guide

Multigravity gateway (`mgy >= v1.2.9-hotfix.1`) provides **opt-in IPv4/IPv6 dual-stack listening** and an **optional dedicated native HTTPS port** (default remains IPv4-only `tcp4` with zero impact on existing setups). No mobile app update is required for HTTPS DDNS direct access.

### 1. Update Gateway

```bash
mgy update v1.2.9-hotfix.1
# If refreshing an earlier hotfix.1 install:
mgy update -force v1.2.9-hotfix.1
```

### 2. Configure `~/.multigravity/.env`

**Option A — Native HTTPS on a dedicated port (no reverse proxy needed, ACME hot-reload supported):**

```env
MULTIGRAVITY_ENABLE_IPV6=1
MGY_TLS_CERT=/path/to/fullchain.pem
MGY_TLS_KEY=/path/to/privkey.pem
MGY_TLS_PORT=58943
DDNS_HOST=home.example.com
```

Allow inbound TCP `58943` in your router's IPv6 firewall (keep HTTP port `58900` LAN-only).

**Option B — Behind a reverse proxy (Caddy / Nginx / Lucky):**

- If the reverse proxy runs on the **same host**, proxy directly to `http://127.0.0.1:58900` (ensure WebSocket `Upgrade`/`Connection` forwarding is enabled).
- If the reverse proxy runs on **another device** (router/NAS) reaching `mgy` over IPv6, add `MULTIGRAVITY_ENABLE_IPV6=1` to `~/.multigravity/.env`.

**Option C — Tailscale / Private IPv6:**

Add `MULTIGRAVITY_ENABLE_IPV6=1` (or bind specifically with `MULTIGRAVITY_HOST=fd7a:115c:a1e0::xxxx`), then connect via `http://[fd7a:115c:a1e0::xxxx]:58900`.

### 3. Configure Mobile App

After normal QR pairing, open **Settings → Network → Custom Server Address** and enter:

```text
https://home.example.com:58943
```

The mobile app automatically routes in order: **Wi-Fi LAN → Custom IPv6 Direct → Official E2EE Cloud Relay fallback** (unless disabled with `CF_TUNNEL_ENABLED=0`).
