# 方案 1：Cloudflare 灰云 DNS + 网关 HTTPS（`agy.jiuge.space:58900`）

把手机访问云中继从明文 `http://124.222.226.143:58900` 换成：

```text
https://agy.jiuge.space:58900
```

Cloudflare **只做 DNS**，流量仍直连腾讯云上海 → FRP → 家里 Mac。证书在 Mac 上终止，VPS 只转发 TCP，看不到明文。

本文是完整落地方案。橙云代理（小黄云 / Origin Rules 把 443 转到 58900）是另一条路，会绕海外、延迟高，这里不采用。

---

## 1. 最终数据路径

```text
iPhone  https://agy.jiuge.space:58900  (TLS 1.3)
    →  DNS 灰云解析到 124.222.226.143（不经过 Cloudflare 节点）
    →  腾讯云 FRP :58900 原样转发 TCP（密文，VPS 解不开）
    →  Mac 网关 :58900 用 Let's Encrypt 证书解密
```

| 层 | 做什么 | 加密吗 |
|---|---|---|
| Cloudflare | 仅把 `agy.jiuge.space` 解析成 VPS IP | 不经流量 |
| FRP `VPS:7000` | Mac ↔ VPS 控制通道 | 默认已 TLS（`FRP_TLS_ENABLE`） |
| `VPS:58900` | 手机业务口 | 转发 TLS 字节流 |
| Mac `:58900` | 网关 `ListenAndServeTLS` | 证书私钥只在 Mac |

只动网关端口 **58900**（或 `GATEWAY_PORT`）。Language Server、FRP 7000、其它电脑服务不变。

打开 HTTPS 后，该端口上 **不能再走 HTTP**。Let’s Encrypt 证书签给域名，签不了 `192.168.x.x` / 公网 IPv6。因此：

- 手机请一律用 `https://agy.jiuge.space:58900`（家里 Wi-Fi 也会走 VPS 再回来，大约几十毫秒，和现在蜂窝中继同一量级）。
- 不要用 `https://192.168.50.9:58900`，证书主机名对不上，iOS 会直接拒绝。
- 配对二维码里不再下发局域网 IP / IPv6 的 `https://` 地址。

国内云对 **80/443** 未备案域名会拦截；**58900 非标端口不受这条限制**。不要把中继改到 443，除非已经备案。

---

## 2. 现状核对（落地前）

| 项 | 值 |
|---|---|
| 域名 | `agy.jiuge.space`（区域 `jiuge.space`，NS 已在 Cloudflare） |
| 中继 IPv4 | `124.222.226.143` |
| 端口 | `58900` |
| 证书目录 | `~/.antigravity-mobile/certs/`（不进 git） |
| 签发方式 | acme.sh + Cloudflare DNS-01（不开 80 端口） |

落地前用电脑查一下（必须是 VPS IP，不能是 Cloudflare 任播）：

```bash
dig +short A agy.jiuge.space
# 期望：124.222.226.143
# 若是 104.21.x.x / 172.67.x.x，说明还是橙云，必须改成灰云后再继续。
```

---

## 3. Cloudflare 中文控制台：逐步点击路径

控制台语言：右上角头像 → **「我的个人资料」** → 左侧 **「外观」/「Preferences」** → **「语言」** 选 **简体中文**。下面按简体中文写。

### 3.1 打开站点

1. 浏览器打开 [https://dash.cloudflare.com/](https://dash.cloudflare.com/) 并登录。
2. 首页账户下的站点列表里，点域名 **`jiuge.space`**（不要点其它站点）。
3. 进入该站点后，看左侧深色导航。

### 3.2 添加或改成灰云 A 记录（必须）

1. 左侧点 **「DNS」**。
2. 默认落在 **「记录」**（英文界面是 Records）。若在「设置」里，再点一次 **「记录」**。
3. 在记录表里找名称是 **`agy`**、类型 **`A`** 的那一行。

**没有 `agy` 记录时：点「添加记录」**

| 表单项（中文） | 填什么 |
|---|---|
| 类型 | `A` |
| 名称 | `agy`（右侧预览应出现 `agy.jiuge.space`） |
| IPv4 地址 | `124.222.226.143` |
| 代理状态 | **关掉**。云朵必须是 **灰色**，旁注 **「仅 DNS」**。若是橙色「已代理」，再点一下云朵，变成灰云。 |
| TTL | **自动** |

点 **「保存」**。

**已经有 `agy` 记录时：点该行右侧「编辑」**

1. IPv4 改成 `124.222.226.143`（不要填 Cloudflare 的 104/172 地址）。
2. **代理状态** 点成灰色 **「仅 DNS」**。橙云会让解析变成 Cloudflare 节点，手机打 `:58900` 打不到 FRP。
3. 点 **「保存」**。

改完等 1～2 分钟，电脑上再执行：

```bash
dig +short A agy.jiuge.space @1.1.1.1
```

只应看到 `124.222.226.143`。

### 3.3 SSL/TLS 页不用改（灰云不经流量）

灰云时手机 **不经过** Cloudflare 代理，左侧 **「SSL/TLS」→「概述」** 里的「灵活 / 完全 / 完全（严格）」**管不到** `:58900`。

仍建议不要改成 **「灵活」**，以免以后误开橙云时 Cloudflare 用明文回源。保持默认 **「完全」** 或 **「完全（严格）」** 即可。

**不要** 在 **「规则」→「源服务器规则 / Origin Rules」** 里把 443 转到 58900——那是橙云方案。

### 3.4 创建 DNS 编辑令牌（给 Mac 签证书用）

签发走 DNS-01，要能改 `jiuge.space` 的 TXT 记录。

1. 任意页面点 **右上角头像**（邮箱缩写那个圈）。
2. 下拉菜单点 **「我的个人资料」**。
3. 左侧点 **「API 令牌」**（不要用「API 密钥」里的 Global Key）。
4. 点右侧蓝色 **「创建令牌」**。
5. 模板列表找到 **「编辑区域 DNS」**，点右侧 **「使用模板」**。
6. 本页往下滚到 **「区域资源」**：
   - 第一格：**包括**
   - 第二格：**特定区域**
   - 第三格：选 **`jiuge.space`**
7. **客户端 IP 地址过滤** 可保持「未指定」（在家签发即可）。
8. 底部点 **「继续以显示摘要」**。
9. 确认权限是 `jiuge.space` 的 **DNS:编辑**，点 **「创建令牌」**。
10. **立刻复制** 整串 Token。这个页面只出现一次。不要发到聊天、不要提交 git。

令牌只用于本机 `acme.sh`。用完可以留着自动续期；泄漏了就回到 **「API 令牌」** 把这一条撤销。

---

## 4. Mac 上签发证书并打开网关 TLS

DNS 已是灰云、手里有 Token 之后，在 Mac 执行。项目里有脚本：`scripts/issue-agy-tls.sh`。

```bash
cd /Users/hal9000/Projects/antigravity-mobile

# 一次性：安装 acme.sh（已安装会跳过）
curl https://get.acme.sh | sh -s email=taojiuzhen@gmail.com
source ~/.zshrc

# 把上一步复制的 Token 先放进当前终端（不要写进 .env / 不要提交）
export CF_Token='粘贴令牌'

./scripts/issue-agy-tls.sh
```

脚本会：

1. 用 Cloudflare DNS-01 向 Let’s Encrypt 申请 `agy.jiuge.space`
2. 把 `fullchain.cer` / `agy.jiuge.space.key` 装到 `~/.antigravity-mobile/certs/`
3. 续期后仍写到同一路径（需再重启网关加载新证）

然后在项目 `.env` 里加上（`FRP_*` 保持原样）：

```env
DDNS_HOST=agy.jiuge.space
GATEWAY_SSL=1
TLS_CERT_FILE=/Users/hal9000/.antigravity-mobile/certs/agy.jiuge.space.fullchain.cer
TLS_KEY_FILE=/Users/hal9000/.antigravity-mobile/certs/agy.jiuge.space.key
```

`GATEWAY_SSL=1` **必须和两份证书一起出现**。只开开关、不放证书时，二维码会写 `https://`，进程却仍是 HTTP，手机 SSL 握手失败。

重启网关（tmux 会话 `agy-gateway` 里 Ctrl-C，再 `make run`）。日志应有：

```text
🔒 TLS enabled with cert=... key=...
☁️  Cloud Relay Tunnel ENABLED: https://agy.jiuge.space:58900
```

本机验证（不要加 `-k`，要看到系统信任链通过）：

```bash
curl -i --resolve agy.jiuge.space:58900:127.0.0.1 https://agy.jiuge.space:58900/healthz
curl -i https://agy.jiuge.space:58900/healthz
```

后者走公网 VPS，成功说明灰云 + FRP + Mac 证书全通。

最后 **重新扫码配对**。二维码里应是 `ssl=1` 且 host/relay 为 `agy.jiuge.space`。

---

## 5. 验收清单

- [ ] `dig +short A agy.jiuge.space` 只有 `124.222.226.143`
- [ ] Cloudflare 该记录云朵为灰、文案「仅 DNS」
- [ ] 网关日志 `TLS enabled` 且中继 URL 是 `https://agy.jiuge.space:58900`（不是 `https://124.222.226.143:58900`）
- [ ] `curl https://agy.jiuge.space:58900/healthz` 返回 `{"status":"ok"}`，无证书警告
- [ ] 手机扫新码后，设置里的中继是 `https://agy.jiuge.space:58900`
- [ ] 蜂窝网络能拉会话；家里 Wi-Fi 同样用该域名（不走 `192.168.*` 的 https）

---

## 6. 常见问题

**改完 DNS 仍解析到 104.21 / 172.67？**  
记录还是橙云，或电脑 DNS 有缓存。控制台把云朵点灰，再 `dig +short A agy.jiuge.space @1.1.1.1`。

**证书报 `unauthorized` / 加不上 TXT？**  
Token 权限不是「特定区域 jiuge.space」的 DNS 编辑，或复制时少了字符。

**手机报 SSL 握手失败？**  
网关没带上证书、或二维码仍是 `https://IP`。确认 `.env` 三件套并重新 `make pair`。

**家里变慢？**  
HTTPS 后局域网不再直连 IP，会经 VPS 绕一圈。这是证书对不上内网 IP 的取舍，不是 Cloudflare 又中转了一次（灰云不经流量）。

**证书到期？**  
acme.sh 会装定时任务，约每 60 天续期并覆盖 `~/.antigravity-mobile/certs/`。续期后重启一次网关。不必每年手动签。
