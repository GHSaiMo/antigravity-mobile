# Multigravity Cloudflare Tunnel 自动分发 Worker 部署指南

本服务托管在 Cloudflare Workers（免费版），用于为所有运行 `mgy` 网关的电脑**全自动申请、配置并绑定专属永久子域名与 Cloudflare Tunnel**，并支持多域名池自动溢流与未来自建中继（`relay_v1`）灰度调度。

---

## ⚡ 极速部署步骤 (2 分钟完成)

### 1. 登录并创建 Worker
1. 访问 [Cloudflare 控制台](https://dash.cloudflare.com)；
2. 左侧导航栏点击 **Compute (Workers) -> Workers & Pages**；
3. 点击 **Create application** -> **Create Worker**；
4. 名称自定义（如 `mgy-tunnel-dispatcher`），点击 **Deploy**。

### 2. 粘贴代码
1. 部署完成后，点击 **Edit code** 进入在线编辑器；
2. 清空默认代码，将本项目中的 [`worker.js`](worker.js) 内容全部复制并粘贴进去；
3. 点击右上角 **Deploy** 保存。

### 3. 配置环境变量与密钥 (Settings -> Variables)
返回该 Worker 的详情页，进入 **Settings** -> **Variables and Secrets**，点击 **Add** 配置以下变量：

| 变量名称 | 类型 | 必填 | 说明与获取方式 |
| :--- | :--- | :---: | :--- |
| `CF_ACCOUNT_ID` | Variable | 是 | **你的 Cloudflare 账户 ID**。在 Cloudflare 任意域名主页右下角或控制台 URL 中复制。 |
| `CF_ZONE_ID` | Variable | 是* | **主域名的 Zone ID**（与 `BASE_DOMAIN` 搭配使用；若配置了 `CF_ZONES` 则会自动作为首选主域名）。 |
| `BASE_DOMAIN` | Variable | 是* | **主域名**，例如 `jiuge.space`（需确保根域名已托管在 Cloudflare）。 |
| `CF_API_TOKEN` | **Secret** | 是 | **Cloudflare API 令牌**，需拥有：<br>1. **Account** -> **Cloudflare Tunnel** -> **Edit**<br>2. **Zone** -> **DNS** -> **Edit**（多域名时选 **All zones** 或勾选池内全部域名） |
| `INVITE_CODE` | Secret | 否 | 私有准入暗号；**留空则全公开无感接入**。 |
| `STALE_DAYS` | Variable | 否 | 离线僵尸隧道回收天数（默认 `14`，配额紧张时建议设为 `7`，客户端重启后会自动按原机器密钥无缝续领原域名）。 |
| `NEVER_USED_DAYS` | Variable | 否 | 创建后从未连接过的隧道回收天数（默认 `3`）。 |
| `CF_ZONES` | Variable | 否 | **多域名扩容池**（突破免费版单域名 200 条 DNS 配额上限，吃满单账号 1,000 Tunnel）。支持简洁写法 `zoneId1:jiuge.space,zoneId2:mgy1.xyz` 或 JSON 数组 `[{"zone_id":"...","base_domain":"..."}]`。 |
| `ZONE_STRATEGY` | Variable | 否 | 多域名分配策略：`spillover`（默认，优先填满主域名，配额满且 GC 后自动溢流到备用域名）或 `hash`（按机器哈希均匀分散）。 |

### 4. 配置定时垃圾回收 Cron Trigger（推荐）
在 Worker 详情页进入 **Settings -> Triggers -> Cron Triggers**，添加一条定时任务（例如 `0 */6 * * *`，每 6 小时执行一次），即可定时回收长期失联的僵尸隧道与孤儿 DNS 记录。

### 5. 未来自建中继（`relay_v1`）预埋变量（默认关闭）
以下变量仅在未来上线自建 Go 反向中继（`mgy-relay`）时配置；`RELAY_POLICY` 缺省为 `off`，不会影响现有 Cloudflare 流量：

| 变量名称 | 说明 |
| :--- | :--- |
| `RELAY_POLICY` | `off`（默认）/ `spillover`（CF 配额满时自动溢流给支持 `relay_v1` 的客户端）/ `prefer`（支持 `relay_v1` 的客户端优先走中继） |
| `RELAY_ENDPOINTS` | 按区域映射的中继接入点 JSON，如 `{"ap":"wss://ap.relay.jiuge.space/_mgy_tunnel","us":"wss://us.relay.jiuge.space/_mgy_tunnel","default":"wss://ap.relay.jiuge.space/_mgy_tunnel"}` |
| `RELAY_BASE_DOMAINS` | 按区域映射的用户泛解析基域 JSON，如 `{"ap":"ap.jiuge.space","us":"us.jiuge.space","default":"r.jiuge.space"}` |
| `RELAY_SIGNING_SECRET` | 用于签发无状态 HMAC-SHA256 中继令牌（`mgyr1.<payload>.<sig>`）的共享密钥 |

### 6. 获取 Worker 调度入口
配置完成后，在 Worker 概览页复制分配的公网 URL（或绑定自定义域名如 `https://dispatcher.jiuge.space`），写入服务端网关的 `.env`：
```bash
CF_WORKER_URL=https://dispatcher.jiuge.space
```
客户端 `mgy` 首次启动即会自动向该 Worker 注册并获取专属永久域名。
