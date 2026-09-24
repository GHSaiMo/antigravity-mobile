# 🚀 Multigravity v1.0.3

### ✨ 核心更新与优化 (Highlights & Improvements)

- 🔔 **Android 远程推送与系统通知全量对齐 (FCM & DeepLink)**：
  - **多通道并发通知引擎**：网关服务端升级支持 Bark (iOS) 与 Firebase Cloud Messaging (Android) 双通道并发推送，并配备退避重试与 Token 注册机制；
  - **动态通知渠道与强提醒**：Android 客户端划分“实时会话活动”与“高优先级任务提醒”双通知渠道，支持直接点击通知深层唤起并跳转至目标会话；
  - **系统级权限与视觉规范**：完整适配 Android 13+ 运行时通知权限申请与引导，引入符合 Material 标准的单色 24dp 状态栏通知图标。

- 🏝️ **iOS 灵动岛 (Dynamic Island) 与实时活动 (Live Activity) 视觉重塑**：
  - **双模式外观自适应**：深度适配系统深色/浅色模式，升级为醒目 Prominent 布局；
  - **原生透明通道与大比例图标**：灵动岛 Logo 采用原生透明通道渲染消除生硬色块，全面放大状态徽章、Logo 与状态标识比例，关键任务信息一眼即得。

- ⚡ **本地离线会话缓存与秒级即开 (Conversation Cache)**：
  - 双端（iOS / Android）全面引入会话列表与对话历史本地持久化缓存机制；
  - 启动应用无需等待网络请求与网关探测，瞬间呈现最新会话内容，后台静默拉取增量对齐，体验丝滑流畅。

- 💻 **服务端宿主平台智能识别与零延迟引擎预装**：
  - **宿主平台感知**：网关与移动端联动展示服务运行平台（macOS / Windows），配对二维码与设备设置信息更直观；
  - **内建 Cloudflared 引擎**：一键安装脚本默认预载并验证 Cloudflare 穿透引擎，首次启动零等待、零外部依赖下载，穿透就绪快人一步。

- 🛡️ **纯双通道架构收敛与智能选路容灾**：
  - 彻底移除废弃的本地 TLS/自签证书冗余逻辑与启动告警，精炼为“内网 HTTP 直连 + 外网 Cloudflare 隧道”纯净双通道架构；
  - Android 客户端增强非网关 Wi-Fi 智能选路逻辑，在外部陌生 Wi-Fi 环境下秒级无缝回退至公网隧道直连，杜绝连接僵死。

- 🎯 **Cockpit 端口自动探测与动态自愈引擎 (Port Auto-Discovery)**：
  - **端口漂移智能感知**：自动解决部分电脑或特定配置下 Cockpit 端口被占用换端口（如 18081 / 19528 冲突漂移）导致配额刷新失败的问题；
  - **多级嗅探与指纹校验**：支持 OS 级进程监听端口实时嗅探与候选端口池快速协议验证，毫秒级锁定实际活动端口并自动记忆，确保配额同步与无感切号稳固不掉线。

- ⚡ **一键安装体验升级与全局免配置**：
  - macOS 一键安装脚本在写入 PATH 后自动建立 `/usr/local/bin` 或 `~/.local/bin` 全局软链接；
  - 用户安装完毕后无需手动执行 `source ~/.zshrc`，即可直接在任意新旧终端执行 `mgy`。

- 🪟 **桌面 Cockpit 工具静默启动与防焦点抢占**：
  - 优化 Cockpit Tools 启动机制，增加已运行进程锁与防重复启动保护；
  - macOS 采用后台隐式拉起结合异步窗口焦点保护，Windows 启用窗口隐藏，消除研发过程中应用弹窗与输入焦点抢占干扰。

---

### 📦 服务端快速更新 / 安装 (`mgy`)

已安装用户在终端再次执行一键命令即可自动升级至最新版，**已有配对授权和配置会自动保留**：

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```
  *(备用直连：`irm https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex`)*

- **macOS (Apple Silicon & Intel)**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

---

### 📱 客户端配套下载
- **Android**：下载下方 Assets 列表中的 `Multigravity-v1.0.3.apk` 直接安装。
- **iOS**：TestFlight 或项目工程本地签名构建。
