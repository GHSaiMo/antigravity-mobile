# Antigravity Mobile Gateway (反重力移动网关方案 A)

## 1. 业务目标与核心痛点

用户希望在手机端随时随地直连 Mac mini 上的反重力（Antigravity）服务，实现随时对话、下发指令与监控 Agent 任务。

### 现状排查发现的关键技术约束：
1. **端口动态分配**：Antigravity 启动参数为 `--https_server_port 0`，后台的 `language_server` 每次重启后监听的本地端口都是随机的（例如当前是 62226）。
2. **鉴权 Token 动态生成**：启动参数带有动态 `--csrf_token <UUID>`，每次重启会刷新。
3. **公网暴露的高危性**：反重力拥有本地 Terminal 执行和文件读写权限，如果直接通过 Cloudflare 穿透暴露原生端口，一旦泄露等同于机器失陷。
4. **官方 Web 界面未对移动端做适配**：官方前端为桌面宽屏设计，在手机浏览器中字体小、排版挤压、输入法遮挡。

---

## 2. 总体架构设计（方案 A：PWA + 本地自适应网关）

```
[ 手机端 Safari / PWA 全屏应用 ]
              │ (HTTPS / WSS)
              ▼
    [ Cloudflare Tunnel / 域名 ]
              │
              ▼
[ Mac mini 本地固定端口网关 (例如: 127.0.0.1:58900) ]  <-- 本项目核心
  ├── 1. 访问鉴权层 (独立移动端访问密码 / Auth Token)
  ├── 2. 实例探测器 (自动 ps/lsof 发现 Antigravity 实时端口与 csrf_token)
  ├── 3. 动态反向代理 (HTTP & WebSocket 透明双向转发并注入 CSRF)
  └── 4. 移动端专属前端 (轻量 Vue/React 或专用移动端 H5 聊天 UI)
              │ (动态本地端口 + CSRF Token)
              ▼
[ Antigravity language_server (127.0.0.1:动态端口) ]
```

---

## 3. 核心功能模块划分

### 模块一：Antigravity 实例动态发现器 (Inspector)
- 定时/按需探测系统进程：读取 `ps aux | grep language_server`。
- 自动提取：
  - 动态监听端口（通过 `lsof -p <pid> -iTCP -sTCP:LISTEN` 或启动参数关联）
  - 当前有效 `csrf_token`
- 当 Antigravity 重启时，网关平滑重连，无需人工修改 Cloudflare 配置。

### 模块二：固定端口中间件网关 (Gateway / Reverse Proxy)
- **固定监听端口**：默认固定为本地 `127.0.0.1:58900`（供 Cloudflare Tunnel 对接）。
- **安全与鉴权**：
  - **网络层门禁**：通过 Cloudflare Access / Zero Trust 统一做外网身份鉴权（邮箱验证码/PIN码），网关本身无需设计繁琐的独立密码体系，保持极简高效。
- **协议透明转发**：
  - 支持 HTTP API 与 WebSocket 实时双向流（Agent 打字机与工具执行流）。
  - 自动向 Antigravity 注入 `X-CSRF-Token` 和 `Host` 伪装头。

### 模块三：移动端适配前端 (Mobile Web / PWA)
- **形态**：定制专属轻量移动 H5 / PWA，支持 iOS Safari “添加到主屏幕” 形成全屏 App，无浏览器地址栏干扰。
- **界面优化**：
  - 专为手机屏幕调优的卡片与流式打字机效果。
  - 底部自适应吸顶输入框，规避 iOS 软键盘遮挡。
  - 会话列表、历史记录切换、折叠详细工具调用过程，聚焦核心结论与代码交付。

---

## 4. 后续演进（方案 A -> 方案 B）
在方案 A 跑通后，网关层的所有接口和鉴权协议保持不变：
- 可以直接用 Flutter / Swift 原生 App 替换移动端 Web 页面；
- 原生 App 直接挂载该网关的 WebSocket，拓展 **iOS 动态岛进度展示** 与 **Agent 任务完成推送**。
