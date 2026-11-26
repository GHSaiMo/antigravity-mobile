# Multigravity Hub (`mgy hub`) 多机聚合接入指南

> 🌐 [English Quick Guide →](#english-guide)

**Multigravity Hub（`mgy hub`）** 让你把同一局域网内的多台电脑（如主力 MacBook、Mac Mini、Windows 台式机、Linux 开发机）聚合为**一个统一网关入口**——手机只需扫码配对 Hub 一台，即可同屏管理所有机器上的 Antigravity 会话、项目、审批与灵动岛进度，并共享 Hub 的单一 E2EE 公网隧道与推送通道。

同时，被接入的普通 `mgy` 主机**依然完整保留独立服务能力**：可以照常单独配对某台手机并直连，互不冲突。

---

## 目录

1. [架构与模式选择：`mgy hub` vs 手机端多主机切换](#1-架构与模式选择mgy-hub-vs-手机端多主机切换)
2. [推荐接入方式：普通 `mgy` + 6 位确认码热接入](#2-推荐接入方式普通-mgy--6-位确认码热接入)
3. [进阶接入方式：`mgy node` 纯子节点与共享密钥自动发现](#3-进阶接入方式mgy-node-纯子节点与共享密钥自动发现)
4. [日常管理命令速查](#4-日常管理命令速查)
5. [环境变量与数据文件](#5-环境变量与数据文件)
6. [安全模型与网络边界](#6-安全模型与网络边界)
7. [常见问题排查（FAQ）](#7-常见问题排查faq)
8. [English Quick Guide](#english-guide)

---

## 1. 架构与模式选择：`mgy hub` vs 手机端多主机切换

```text
                       ┌──[LAN 认证令牌]──▶ 主机 A (普通 mgy，仍可单独连手机)
手机 (iOS/Android/PWA) ─┼─[统一 E2EE 隧道]──▶ mgy hub (主力机或 NAS -headless)
                       └──[LAN 认证令牌]──▶ 主机 B (普通 mgy 或 mgy node)
```

Multigravity 提供两种多机方案，可以按需选择，也支持**叠加组合**：

| 对比维度 | `mgy hub`（服务端局域网聚合） | 手机端多网关切换（App 端左右扫 / 长按切机） |
| :--- | :--- | :--- |
| **网络拓扑** | 同一局域网内的多台机器 | 跨网络、完全独立的机器（家里 / 公司 / 云服务器） |
| **手机配对次数** | **仅配对 Hub 一台**（单一二维码与推送通道） | 每台电脑各配对一次，数据与 E2EE 密钥完全隔离 |
| **会话展示方式** | **同屏聚合**，自动标注 `[主机名]` 前缀，可用 `filter` 切换视角 | 每次展示当前激活的一台主机，左右轻扫标题瞬间切换 |
| **组合用法** | 家里局域网用 `mgy hub` 聚合多台机器；手机端再单独绑定公司电脑，实现「家庭集群 ↔ 公司主机」一键切换 |

---

## 2. 推荐接入方式：普通 `mgy` + 6 位确认码热接入

这是**配置成本最低、且保留单机直连能力**的标准方式：所有电脑只需要照常跑 `mgy`，不需要记特殊子节点命令，也不需要查局域网 IP。

### 第一步：各台电脑正常启动普通 `mgy`

在需要被聚合的电脑（如 `MacMini`、`WinPC`）上直接运行：

```bash
mgy
```

> 普通 `mgy` 默认开启**被动局域网发现**（仅回应局域网 Hub 探针，不主动往外刷屏广播，也不会未经确认自动放行任何连接）。

### 第二步：在主控机或 NAS 上启动 `mgy hub`

- **主工作机兼做 Hub**（同时代理本机 Antigravity + 局域网其他机器）：
  ```bash
  mgy hub -name HomeHub
  ```
- **NAS / 常驻小主机无头 Hub**（本机不装 Antigravity，只做聚合、E2EE 隧道与推送出口）：
  ```bash
  mgy hub -headless -name HomeHub
  ```

`mgy hub` 启动后会自动扫描局域网（约 1.2 秒），并在启动看板直接列出发现到的普通 `mgy` 设备：

```text
  ➜  Hub 模式:   无头聚合 (Headless) (已存节点: 0 | 加入码: 9K4M2P)
  ➜  局域网发现: 扫到 2 台待配对 mgy 设备（需 6 位确认码）:
       • MacMini (http://192.168.50.99:58900)  [mgy hub pair MacMini]
       • WinPC   (http://192.168.50.18:58900)  [mgy hub pair WinPC]
```

如果其他电脑是稍后才启动 `mgy` 的，随时在 Hub 端运行一次主动扫描即可：

```bash
mgy hub scan
```

### 第三步：发起二次确认码配对（仅首次）

#### 场景 A：目标机前台终端可见（默认交互模式，类似 AirPlay 弹码）

在 Hub 端直接按名称或局域网 IP 发起配对：

```bash
mgy hub pair MacMini
# 或直接指定 IP / URL（适合跨网段或未触发广播时）：
mgy hub pair 192.168.50.99
mgy hub pair http://192.168.50.99:58910 -name 客厅MacMini
```

1. `MacMini` 的 `mgy` 终端会立刻醒目打印一个 **6 位一次性确认码**（如 `A7K 9P2`，有效期 2 分钟，限试 3 次）；
2. 在 Hub 端提示处输入该确认码（支持空格和大小写，如 `a7k9p2`）；
3. 校验通过后，**目标机无需重启**即刻热生效：Hub 自动保存节点与专属随机令牌，目标机持久化授权并热挂载推送/灵动岛转发。此后双方任意重启均自动重连，无需再输第二次码。

#### 场景 B：目标机跑在后台 / `tmux` / 系统服务中看不到实时日志

1. 在目标机上执行：
   ```bash
   mgy pair -hub
   ```
   终端会输出 6 位确认码与可直接复制的配对命令（支持 `-copy` 写入剪贴板、`-uri` 仅输出裸码）。
2. 在 Hub 端一步完成非交互配对：
   ```bash
   mgy hub pair http://192.168.50.99:58900 -code A7K9P2
   ```

### 第四步：手机扫码连接 Hub

用手机 App 扫描 **Hub 终端**上的配对二维码（或执行 `mgy pair`）即可。手机端所有会话与项目都会自动带上 `[MacMini]`、`[WinPC]` 前缀。

> **独立直连兼容**：已接入 Hub 的普通 `mgy` 依然可以随时单独与另一台手机配对（在目标机执行 `mgy pair` 扫码），与 Hub 并发运行、互不影响。

---

## 3. 进阶接入方式：`mgy node` 纯子节点与共享密钥自动发现

如果某些机器只作为内网算力节点、**不需要自己开公网隧道和打印二维码**，可以使用专门的 `mgy node` 模式：

### 方式 A：使用 Hub 的 6 位加入码

复制 `mgy hub` 启动看板中的接入命令到子节点执行（首次加入后凭证自动保存，后续重启自动重连）：

```bash
mgy node -hub http://192.168.50.10:58900 -code <6位加入码> -name MacMini
```

### 方式 B：共享集群密钥（局域网零 IP 自动发现免密加入）

适合多台常驻内网主机自动化部署（例如配合 `systemd` / `launchd` / `tmux`）：

1. 在 Hub 端查看集群密钥和一行加入命令：
   ```bash
   mgy hub secret
   ```
2. 在子节点使用相同密钥启动（同网段可省略 `-hub`，通过 UDP `58901` 广播交换不可逆指纹自动互认，明文密钥不走网络）：
   ```bash
   mgy node -secret <HUB_CLUSTER_SECRET> -name MacMini
   # 或通过环境变量注入：
   MGY_HUB_SECRET=<HUB_CLUSTER_SECRET> mgy node -name MacMini
   ```

### 方式 C：静态节点清单（无广播环境）

在关闭 UDP 广播或跨 VLAN 场景下，可直接通过 `-nodes` 或 `mgy hub add` 挂载静态节点：

```bash
mgy hub -headless -no-discovery \
  -secret <SECRET> \
  -nodes M1=http://192.168.50.12:58900,M2=http://192.168.50.13:58900
```

> **提示**：`mgy node` 默认关闭自身公网穿透隧道和启动二维码。若某个 `mgy node` 同时需要让外部手机单独直连，启动时附加环境变量 `MGY_NODE_ENABLE_TUNNEL=1` 并执行 `mgy pair` 即可。

---

## 4. 日常管理命令速查

### Hub 端管理

| 命令 | 说明 |
| :--- | :--- |
| `mgy hub [-headless] [-name 名称]` | 启动统一 Hub 网关（自动扫描局域网 `mgy` 设备） |
| `mgy hub scan` | 主动扫描局域网内运行中的普通 `mgy` 与节点设备 |
| `mgy hub pair <名称\|IP\|URL> [-code 码] [-name 别名]` | 向局域网普通 `mgy` 发起 6 位确认码配对接入 |
| `mgy hub status` / `mgy hub nodes` | 查看 Hub 状态、已接入节点延迟/在线状态及待配对设备 |
| `mgy hub filter <all\|local\|节点名>` | 切换手机端看到的聚合视图（实时通知客户端刷新） |
| `mgy hub remove <id\|名称>` | 从 Hub 移除指定节点（支持离线编辑配置） |
| `mgy hub add <url> [-name 名称] [-token 令牌]` | 手动添加静态节点 |
| `mgy hub secret` | 查看共享集群密钥与免交互加入命令 |

### 被接入普通 `mgy` 主机端管理

| 命令 | 说明 |
| :--- | :--- |
| `mgy pair -hub [-copy]` | 签发 6 位局域网 Hub 确认码（适合后台/`tmux` 场景） |
| `mgy hub links` | 查看当前已授权接入本机的所有 Hub（支持离线查看） |
| `mgy hub unlink <hub_id\|名称>` | 立即吊销指定 Hub 对本机的接入授权（不影响已配对手机） |

*(若网关未运行在默认 `58900` 端口，上述所有子命令均支持追加 `-port <端口号>`)*

---

## 5. 环境变量与数据文件

### 常用环境变量（可写入 `~/.multigravity/.env`）

| 环境变量 | 适用角色 | 说明 |
| :--- | :--- | :--- |
| `MGY_HUB_HEADLESS` | Hub | 设为 `1` 启用无头聚合模式（不检查本机 Antigravity，适合 NAS） |
| `MGY_HUB_NAME` / `MGY_NODE_NAME` | Hub / Node | 显示名称（用于会话/项目 `[名称]` 前缀，默认取主机名） |
| `MGY_HUB_SECRET` | Hub / Node | 集群共享密钥（仅用于同密钥 `mgy node` 免密自动发现） |
| `MGY_HUB_NODES` | Hub | 逗号分隔静态节点列表，如 `M1=http://192.168.50.12:58900` |
| `MGY_HUB_URL` / `MGY_HUB_CODE` | Node | `mgy node` 目标 Hub 地址与 6 位加入码 |
| `MGY_HUB_DISCOVERY_PORT` | 全部 | 局域网 UDP 发现端口（默认 `58901`） |
| `MGY_DISABLE_DISCOVERY` | 全部 | 设为 `1` 彻底关闭局域网 UDP 发现与被动应答（等价 `-no-discovery`） |
| `MGY_NODE_ENABLE_TUNNEL` | Node | 设为 `1` 允许 `mgy node` 节点同时保留自己的公网穿透隧道 |

### 持久化配置文件（均位于 `~/.multigravity/`，权限 `0600`）

- `hub_nodes.json`（Hub 侧）：保存已接入节点清单、专属令牌、集群密钥与当前 `active_filter`。
- `hub_links.json`（被接入普通 `mgy` 侧）：保存已授权接入本机的所有 Hub 身份、URL 与独立 `nodeToken`。
- `hub_node.json`（节点身份）：保存本机稳定的 `NodeID` 与 `mgy node` 连接信息。

---

## 6. 安全模型与网络边界

1. **普通 `mgy` 零默认越权**：被动发现仅返回非敏感元信息（`NodeID`、主机名、OS、端口、引擎健康态），**不会开放任何 API 权限**；若指定 `-host 127.0.0.1` 或 `-no-discovery`，连发现应答也会完全关闭。
2. **仅限私有局域网直连**：配对接口（`/api/v1/hub/pair-request`、`/api/v1/hub/pair-confirm`、`/api/v1/hub/verify`）硬校验源地址必须为私网 IP 或本机回环，且拒绝一切经反向代理、Cloudflare Tunnel 或 E2EE 中继转发的请求（直接返回 `403`）。
3. **防暴力破解与防刷屏**：6 位确认码采用 32 字符去混淆字母数字集（$32^6 \approx 10.7\text{ 亿}$ 组合），有效期 120 秒，**输错 3 次立即销毁**；同一来源 IP 发起配对请求限流为每分钟最多 2 次，且确认码与发起方 `HubID` 绑定防止其他 Hub 截胡。
4. **防 DHCP 换 IP 时的令牌窃取**：当已配对节点更换局域网 IP 后，Hub **绝不直接向新 IP 发送 `nodeToken`**，而是先发送随机 `nonce` 要求目标机返回 `HMAC-SHA256(nodeToken, nonce)` 证明持有密钥，验明正身后才迁移 `BaseURL`。
5. **随时可审计与单向吊销**：目标机可随时通过 `mgy hub links` / `mgy hub unlink` 查看或切断任意 Hub 授权，令牌即时失效，且完全不影响该机上直连的手机设备。

---

## 7. 常见问题排查（FAQ）

- **`mgy hub` 启动或 `mgy hub scan` 扫不到局域网里的其他 `mgy`？**
  1. 确认目标机运行的是包含 Hub 发现能力的最新版 `mgy`，且未加 `-host 127.0.0.1` 或 `-no-discovery`；
  2. 检查两台机器是否在同一二层网段，路由器/办公 Wi-Fi 是否开启了 **AP 隔离（客户端隔离）**；
  3. 检查 macOS「本地网络权限」或 Windows 防火墙是否放行了 `mgy`（TCP `58900` 与 UDP `58901`），以及 Clash/Surge 等 TUN 代理是否劫持了局域网路由；
  4. 即使 UDP 广播受阻，仍可直接按 IP 配对：`mgy hub pair 192.168.50.99`（无需依赖广播即可完成 6 位码接入）。

- **同一台手机既配对了 Hub，又单独配对了其中某台普通 `mgy`，会怎样？**
  两套连接都能正常工作，但该普通 `mgy` 触发审批/完成推送时，手机会收到**两份推送**（一份来自该机直连，一份经 Hub 转发）。建议同一台手机只绑定 Hub；不同手机（例如同事或备用机）各自直连单台主机则互不干扰。

- **在公共 Wi-Fi 或公司内网下，不想被局域网扫到本机 `mgy`？**
  启动时加 `-no-discovery`，或在 `~/.multigravity/.env` 中设置 `MGY_DISABLE_DISCOVERY=1` 即可完全关闭局域网发现应答。

---

<a id="english-guide"></a>
## 8. English Quick Guide

**Multigravity Hub (`mgy hub`)** aggregates multiple LAN machines running `mgy` behind a single unified gateway—so your phone only pairs once with the Hub, sees all sessions and projects prefixed with `[HostName]`, and shares one E2EE tunnel and push channel. Each ordinary `mgy` host still remains a full standalone gateway that can serve directly paired phones at the same time.

### Quickstart (Recommended: Ordinary `mgy` + 6-Char Confirmation Code)

1. **Run ordinary `mgy` on each workstation** (zero extra flags required):
   ```bash
   mgy
   ```
   Ordinary `mgy` enables **passive LAN discovery** by default (replies only to LAN Hub probes; never broadcasts unsolicited and never auto-grants access).
2. **Start the Hub** on your primary machine or an always-on NAS/server:
   ```bash
   mgy hub -name HomeHub            # Local Antigravity + LAN nodes
   mgy hub -headless -name HomeHub  # Headless aggregator (ideal for NAS / Linux server)
   ```
   On startup, `mgy hub` automatically scans the LAN (~1.2s) and lists discovered `mgy` hosts right in the banner. You can re-scan anytime with `mgy hub scan`.
3. **Pair a discovered host using a one-time 6-char confirmation code**:
   - **Interactive (target terminal visible)**:
     ```bash
     mgy hub pair MacMini          # or: mgy hub pair 192.168.50.99
     ```
     The target `mgy` terminal prints a 6-char confirmation code (valid 2 mins, max 3 tries). Enter it on the Hub to bind permanently—no restart required on the target host.
   - **Background / `tmux` host**:
     Run `mgy pair -hub` on the target host to generate a 6-char code, then run on the Hub:
     ```bash
     mgy hub pair http://192.168.50.99:58900 -code A7K9P2
     ```
4. **Pair your phone with the Hub** (`mgy pair` on the Hub).

### Dedicated LAN Node Mode (`mgy node`)

For dedicated LAN workers that do not need their own public tunnel or mobile QR code:
```bash
# Join via 6-char Hub join code:
mgy node -hub http://192.168.50.10:58900 -code <JOIN_CODE> -name MacMini

# Or zero-touch LAN auto-join via shared cluster secret (inspect via `mgy hub secret`):
mgy node -secret <HUB_CLUSTER_SECRET> -name MacMini
```
*(Set `MGY_NODE_ENABLE_TUNNEL=1` if a `mgy node` also needs its own independent public tunnel.)*

### Essential Commands & Security Guarantees

- **Hub side**: `mgy hub scan` · `mgy hub pair <name|IP|URL> [-code CODE]` · `mgy hub status` · `mgy hub filter <all|local|node>` · `mgy hub remove <id|name>`
- **Target `mgy` host side**: `mgy pair -hub` · `mgy hub links` · `mgy hub unlink <hub_id|name>`
- **Security**:
  - Pairing & verification endpoints strictly enforce direct private LAN / loopback origin (`403` for proxied, public, Cloudflare, or E2EE relay requests).
  - 6-char unambiguous code ($32^6 \approx 1.07\text{B}$ space), 120s TTL, burned after 3 wrong tries, per-IP rate-limited, and bound to requester `HubID`.
  - DHCP IP migrations are verified via `HMAC-SHA256(nodeToken, nonce)` (`/api/v1/hub/verify`) before updating `BaseURL`—`nodeToken` is never sent to unverified LAN IPs.
  - Disable LAN discovery completely anytime via `-no-discovery` or `MGY_DISABLE_DISCOVERY=1`.
