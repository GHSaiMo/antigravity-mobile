# 🚀 Multigravity v1.0.2

### ✨ 核心更新与优化 (Highlights & Improvements)

- 🌐 **极简双通道与自动化公网直连**：
  - 默认启用 Cloudflare 自动化穿透隧道，无需公网 IP 或复杂端口映射，扫码开箱即用；
  - 精简双端网络设置与配对流程，隐藏冗余调试信息，对齐局域网与公网纯双通道体验。

- 🔄 **移动端智能选路与无感容灾切换**：
  - Wi-Fi 与蜂窝移动网络自适应无缝切换，优先局域网低延迟直连，离开内网秒级回退公网；
  - 彻底修复 iOS/Android 离开局域网后公网连接中断或端点丢失的缺陷，大幅提升连接稳定性。

- ⚡ **传输性能与网络体验优化**：
  - 服务端启用 WebSocket 数据压缩与流式内存池复用，有效降低带宽开销与传输延迟；
  - 客户端完善退避重连与响应缓存机制，连接建立更迅捷，降低后台能耗。

- 🛡️ **全栈安全加固与依赖升级**：
  - 收紧移动端网络通信安全策略，本地 CLI 全面规范安全回环校验；
  - 排查并升级核心依赖组件，消除潜在安全漏洞。

- 📱 **Android 沉浸式体验优化**：
  - 消除所有底部抽屉组件与系统手势导航栏之间的白边缝隙，视觉更沉浸；
  - 升级底层构建工具链，提升编译与运行稳定性。

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
