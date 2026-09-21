# 🚀 Multigravity v1.0.2

### ✨ 核心更新与优化 (Highlights & Improvements)

- **统一 Cloudflare 公网通道与双通道极简架构**：
  - 彻底废弃 IPv6、DDNS 及 FRP Relay 等复杂配置与外部依赖，全面采用 Cloudflare 穿透隧道作为核心公网直连通道；
  - 扫码配对及服务端 API 端点统一对齐纯双通道模式（局域网 IPv4 + Cloudflare 安全公网域名），开箱即用；
  - 优化移动端设置界面，移除多余调试域名展示，恢复极简视觉体验。

- **移动端智能选路与无感容灾切换 (iOS / Android)**：
  - **Wi-Fi / 蜂窝网络自适应无缝切换**：局域网内优先低延迟直连；断开 Wi-Fi 或切换至蜂窝网络时，底层秒级静默切换至 Cloudflare 公网隧道，无需手动重新配对；
  - **修复 iOS 局域网配对断网后无法连接公网的缺陷**：
    - 修复 URL 协议头解析缺陷与 `primaryCloudURL` 被局域网内网 IP 反向污染的问题；
    - 引入服务端 `X-Antigravity-Cloud-URL` 响应头与 `/api/v1/auth/endpoints` 动态自愈机制，彻底杜绝端点丢失；
    - 增强 WebSocket 实时流与会话断线自动重测与重连机制。

- **Android 沉浸式体验优化与工程升级**：
  - 修复 Android 端所有底部抽屉（Bottom Sheet）在系统手势导航栏底部的空白缝隙，带来更沉浸的全面屏交互；
  - 升级 Android Gradle Plugin (AGP) 至 9.4.1，优化构建稳定性与执行性能。

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
- **Android**：下载下方 Assets 列表中的 `Multigravity-v1.0.2.apk` 直接安装。
- **iOS**：TestFlight 或项目工程本地签名构建。
