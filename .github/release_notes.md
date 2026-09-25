# 🚀 Multigravity v1.0.3

### ✨ 核心更新 (Highlights)

- 💻 **工作区与会话协同**：深度集成 Antigravity 2.0 原生项目配置，新建会话自动绑定对应工作区；支持用户上传原图无损预览。
- 🔔 **全端远程推送**：服务端全面支持 iOS (Bark) 与 Android (FCM) 双通道并发推送，适配 Android 13+ 通知权限与深层跳转。
- ⚡ **离线缓存与性能调优**：移动端（iOS / Android）全面支持会话列表与对话历史本地秒开；优化内存占用与长列表滑动流畅度。
- 📱 **执行态与中断响应加固**：优化多端（iOS / Android / Web）活跃执行与后台任务运行态保持，支持即时停止中断。
- 🍏 **iOS Swift 6 全面对齐**：工程更名为 Multigravity，全面对齐 Swift 6 并发安全规范。
- 🏝️ **iOS 体验重塑**：灵动岛与实时活动支持深浅色模式自适应，状态徽章与图标比例全面优化。
- 🛠️ **CLI 管理向导**：新增 `mgy cockpit`、`mgy bark` 与 `mgy cf` 交互式诊断与配置工具；一键安装脚本自动配置全局 PATH。
- 🛡️ **架构收敛与安全加固**：彻底下线冗余穿透模块，收敛为“内网直连 + Cloudflare 专属隧道”纯净双通道；增强敏感凭证隔离与接口限流。
- 🔐 **长连接鉴权安全规范**：全端（Android / iOS）WebSocket 长连接与媒体资源加载全面切入标准 Header 鉴权规范与 `/ws-ticket` 短效凭据机制，消除 URL Token 暴露风险。
- 🔗 **配对 URI 规范化与域名脱敏**：配对 URI 参数顺序统一为 `code -> host -> lan -> port -> ssl -> platform`，剔除冗余 `os` 参数；公网主域名实现子域脱敏与客户端智能补全，显著降低二维码点阵密度并增强防窥保护。
- 🖼️ **Android 消息附件与后台通知加固**：修复发送图片附件在气泡中重复渲染的问题；优化 Android 通知栏后台执行状态同步与孤儿通知自动清理。
- 📐 **Android 输入框设计对齐 iOS**：优化输入框内边距与字体度量，将默认单行高度精准约束为 44dp，与右侧圆形发送/停止按钮（44dp）顶底严格对齐，并保持完美的 22dp 半圆胶囊弧度。

---

### 📦 服务端安装 / 更新 (`mgy`)

- **macOS**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

---

### 📱 客户端下载
- **Android**：下载下方 Assets 中的 `Multigravity-v1.0.3.apk` 安装。
- **iOS**：TestFlight 或本地签名构建。
