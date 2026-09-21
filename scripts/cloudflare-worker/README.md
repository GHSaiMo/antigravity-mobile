# Multigravity Cloudflare Tunnel 自动分发 Worker 部署指南

本服务托管在 Cloudflare Workers（免费版），用于为所有运行 `mgy` 网关的电脑**全自动申请、配置并绑定专属永久子域名与 Cloudflare Tunnel**。

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
返回该 Worker 的详情页，进入 **Settings** -> **Variables and Secrets**，点击 **Add** 配置以下 4 个变量：

| 变量名称 | 类型 | 说明与获取方式 |
| :--- | :--- | :--- |
| `CF_ACCOUNT_ID` | Variable / Plain | **你的 Cloudflare 账户 ID**。在 Cloudflare 任意域名主页右下角，或控制台 URL 中即可直接复制。 |
| `CF_ZONE_ID` | Variable / Plain | **你的主域名的 Zone ID**。在 Cloudflare 进入你托管的域名概览页，右下角可直接复制。 |
| `BASE_DOMAIN` | Variable / Plain | **分配的基础二级域名**。例如 `mgy.yourdomain.com`（需要确保根域名已解析在 Cloudflare）。 |
| `CF_API_TOKEN` | **Secret** (加密) | **Cloudflare API 令牌**。前往 [API Tokens 页面](https://dash.cloudflare.com/profile/api-tokens) -> Create Token -> 使用模板或自定义，确保拥有：<br>1. **Account** -> **Cloudflare Tunnel** -> **Edit**<br>2. **Zone** -> **DNS** -> **Edit** |
| `INVITE_CODE` | Secret (可选) | 若想对群友加暗号，可填入字符串（如 `6688`）；**留空则全公开无感接入**。 |

### 4. 获取 Worker 调度入口
配置完成后，在 Worker 概览页复制分配的公网 URL，形如：
`https://mgy-tunnel-dispatcher.<your-subdomain>.workers.dev`

将该地址配置到服务端网关的 `.env` 中：
```bash
CF_WORKER_URL=https://mgy-tunnel-dispatcher.<your-subdomain>.workers.dev
```
客户端 `mgy` 首次启动即会自动向该 Worker 注册并获取专属永久域名！
