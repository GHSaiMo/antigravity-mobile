# Multigravity APNs 无状态推送中继 Worker 部署指南

本服务托管在 Cloudflare Workers（**免费版即可**，每日 100,000 次免费请求），用于在**不暴露 `.p8` 私钥**的前提下，将本地 `mgy` 网关的推送事件转发至 Apple APNs，驱动 iOS 原生通知与灵动岛（Live Activity）后台远程刷新。

- **零数据库 / 无状态**：仅在内存中缓存 45 分钟 ES256 JWT，不落盘、不记录日志、不保存任何用户数据。
- **独立隔离**：与隧道调度 Worker（`dispatcher.jiuge.space`）完全隔离，互不影响密钥安全与配额。

---

## 极速部署步骤

### 1. 创建 Worker
1. 登录 [Cloudflare 控制台](https://dash.cloudflare.com)；
2. 左侧进入 **Compute (Workers) → Workers & Pages** → **Create Worker**；
3. 名称填写 `mgy-apns-relay` → 点击 **Deploy**；
4. 点击 **Edit code**，粘贴本目录下 [`worker.js`](worker.js) 完整代码 → 点击 **Deploy**。

### 2. 绑定自定义域名（强烈推荐）
1. 进入 `mgy-apns-relay` → **Settings → Domains & Routes → Add → Custom Domain**；
2. 填入 **`push.jiuge.space`**（或你的自有域名），Cloudflare 会自动签发 SSL 证书与 DNS 路由，避免 `*.workers.dev` 在国内网络环境波动。

### 3. 配置环境变量与 Secrets
在 **Settings → Variables and Secrets** 中添加以下 4 项：

| 变量名 | 类型 | 示例 / 说明 |
| :--- | :--- | :--- |
| `APNS_PRIVATE_KEY` | **Secret**（加密） | 粘贴从 Apple Developer 下载的 `AuthKey_XXXXXXXXXX.p8` 完整内容（含 `-----BEGIN/END PRIVATE KEY-----`） |
| `APNS_KEY_ID` | Text | 苹果后台生成的 10 位 Key ID |
| `APNS_TEAM_ID` | Text | Apple Developer Team ID（如 `7ZW5X4CFNP`） |
| `APNS_BUNDLE_ID` | Text | 主 App Bundle ID（如 `com.ghsaimo.antigravity`） |

### 4. 配置防刷限流（推荐，免费）
在 Cloudflare 域名管理 → **Security → WAF → Rate limiting rules** 添加 1 条免费规则：
- 条件：`Hostname equals push.jiuge.space`
- 阈值：单 IP 每 10 秒超过 60 次请求 → Action: `Block`（返回 `429`）。

### 5. 验证部署
```bash
curl -s https://push.jiuge.space/healthz
# 预期返回：{"status":"ok","service":"Multigravity APNs Stateless Push Relay","version":"1.0.0","configured":true}
```
