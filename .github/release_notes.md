# 🚀 Multigravity v1.0.3

### ✨ 核心更新 (Highlights)

- 💻 **工作区与会话协同**：深度集成 Antigravity 2.0 原生项目配置，新建会话自动绑定对应工作区；支持用户上传原图无损预览。
- 🔔 **全端远程推送**：服务端全面支持 iOS (Bark) 与 Android (FCM) 双通道并发推送，适配 Android 13+ 通知权限与深层跳转。
- ⚡ **离线缓存与性能调优**：移动端（iOS / Android）全面支持会话列表与对话历史本地秒开；优化内存占用与长列表滑动流畅度。
- 🏝️ **iOS 体验重塑**：灵动岛与实时活动支持深浅色模式自适应，状态徽章与图标比例全面优化。
- 🛠️ **CLI 管理向导**：新增 `mgy cockpit`、`mgy bark` 与 `mgy cf` 交互式诊断与配置工具；一键安装脚本自动配置全局 PATH。
- 🛡️ **架构收敛与安全加固**：彻底下线冗余穿透模块，收敛为“内网直连 + Cloudflare 专属隧道”纯净双通道；增强敏感凭证隔离与接口限流。

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
