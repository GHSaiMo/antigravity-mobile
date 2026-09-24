# 🚀 Multigravity v1.0.3

### ✨ 核心更新与优化 (Highlights & Improvements)

- 💻 **Antigravity 2.0 工作区项目 UUID 智能发现与端协同绑定**：
  - **原生项目配置深度集成**：网关直读 `~/.gemini/config/projects/*.json` 原生项目定义，精准提取项目 UUID 并补全工作区属性；
  - **跨端会话工作区自愈绑定**：修复从移动端新建会话时未能自动进入电脑对应工作区的问题，网关支持根据工作区路径自动补齐 Project ID 上下文；
  - **用户上传原图无损预览**：从轨迹数据中解析用户原始图片 URI，并在 iOS 端与 Web 端提供高质量全屏大图预览支持。

- 🛡️ **安全审计深度加固与架构收敛 (Security Audit Hardening)**：
  - **废弃穿透模块彻底移除**：彻底下线并清理旧版 FRP 内网穿透模块及关联冗余依赖，精简服务端二进制体积；
  - **敏感凭证安全隔离**：Cloudflare 隧道 Token 改为通过环境变量安全注入，消除系统进程列表中暴露命令行参数的潜在风险；
  - **进程交互与凭证防注入加固**：加固 AppleScript 宿主应用标识符校验杜绝外部命令注入，Cockpit OAuth Token 采用静态只读 SQL 参数化读取；
  - **高频接口限流与内网拓扑脱敏**：配对与鉴权端点引入请求速率限制与并发保护，外网访问时自动脱敏内网 IP 拓扑；
  - **配置校验严谨化**：智能检测并阻断未填写的示例占位 Bark Key，防止误发无效请求。

- ⚡ **全端极致性能调优与网络韧性增强 (Performance & Resilience)**：
  - **网关服务端**：修复 WebSocket 连接回收时潜在的 Goroutine 泄露隐患，升级 Cloudflare 隧道优雅停机机制（SIGINT 结合超时强制回收）；
  - **Android 客户端**：彻底消除主线程同步 Bitmap 解码与大图持有，网络请求全面切至协程挂起 (`Call.await`)，稳定消息列表重组 Key，杜绝长列表卡顿与潜在 OOM；
  - **iOS 客户端**：会话缓存与历史记录全量异步后台装载，引入系统低内存警告响应与 LRU 缓存上限保护；
  - **Web 客户端**：输入草稿持久化防抖 (300ms) 并在发送后立即注销，WebSocket 连接升级为指数退避重连算法，强化 Service Worker 资源缓存一致性。

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

- 🛸 **Cockpit Tools 交互式配置向导与诊断套件 (`mgy cockpit`)**：
  - **全流程 CLI 交互向导**：新增 `mgy cockpit` 命令，一键交互式完成 Cockpit Tools HTTP 报表服务开启、端口确认与安全访问 Token 配置；
  - **高强度安全 Token 自动生成**：支持输入 `g` / `gen` 一键生成 32 位高强度随机 Token，告别默认占位符 `change-this-token`；
  - **一键重启生效与即时连通性验证**：配置保存后支持自动优雅重启 Cockpit Tools 桌面进程，实时探测 18081 端口上线并发起 HTTP 200 验证；
  - **网关启动自检与告警抑制**：网关启动横幅直观展示 Cockpit 服务就绪状态与操作指引；在服务未配置时抑制盲目自愈重试与 Bark 误告警。

- 🔔 **Bark 实时推送交互式配置套件 (`mgy bark`，iOS 专用)**：
  - **交互式向导与新手引导**：新增 `mgy bark` 命令，清晰指引 iPhone 用户获取 Device Key 并一键写入全局配置；
  - **个性化提示音与即时验证**：支持自定义审批警报音与任务完成音，配置完成后可一键向 iPhone 发送真实测试推送验证连通性。

- ☁️ **Cloudflare 专属穿透隧道管理工具 (`mgy cloudflare` / `mgy cf`)**：
  - **穿透状态与专属域名管理**：新增 `mgy cloudflare` 命令行向导，支持查看已分配的永久专属 HTTPS 域名与穿透引擎状态；
  - **调度器自定义与域名一键重置**：支持切换自建 Worker 调度器、配置邀请码暗号，并支持 `mgy cf reset` 一键重置更换全新域名。


- ⚡ **一键安装体验升级与全局免配置**：
  - macOS 一键安装脚本在写入 PATH 后自动建立 `/usr/local/bin` 或 `~/.local/bin` 全局软链接；
  - 用户安装完毕后无需手动执行 `source ~/.zshrc`，即可直接在任意新旧终端执行 `mgy`。

- 🪟 **桌面 Cockpit 工具静默启动与防焦点抢占**：
  - 优化 Cockpit Tools 启动机制，增加已运行进程锁与防重复启动保护；
  - macOS 采用后台隐式拉起结合异步窗口焦点保护，Windows 启用窗口隐藏，消除研发过程中应用弹窗与输入焦点抢占干扰。

---

### 📦 服务端快速更新 / 安装 (`mgy`)

已安装用户在终端再次执行一键命令即可自动升级至最新版，**已有配对授权和配置会自动保留**：

- **macOS (Apple Silicon & Intel)**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

---

### 📱 客户端配套下载
- **Android**：下载下方 Assets 列表中的 `Multigravity-v1.0.3.apk` 直接安装。
- **iOS**：TestFlight 或项目工程本地签名构建。
