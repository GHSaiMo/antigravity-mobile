# 🚀 Multigravity v1.0.5

### ✨ 核心更新 (Highlights)

- 🐧 **Linux 官方支持 (x86_64 / NAS)**
  - 原生提供纯静态构建，兼容各类 Linux 发行版与 NAS 环境（Debian / Ubuntu / 群晖 DSM / 极空间 / 自建 NAS 等）；
  - 支持 Antigravity Daemon 动态发现、Headless 免令牌连通与双模端口探测，可作为 systemd 用户服务平滑常驻。

- ⚡ **Web 控制台架构减负与性能跃升**
  - **桌面工作台会话切换秒级响应**：精确定位并彻底根除桌面端切换会话时 `StreamAgentStateUpdates` 延迟 30 秒回收导致耗尽 Chrome HTTP/1.1 连接池引发的卡顿排队，切换时即时释放旧状态流；
  - **收敛自研原生控制台**：全端统一采用极速轻量、原生中文的 Web 控制台，支持会话实时打字机流、多选题卡片决策与方案审批；
  - **命令行静默纯粹**：移除 `mgy` 启动后自动弹出浏览器的侵入式行为，保持终端干净清爽；
  - **二进制大幅瘦身**：网关单二进制体积缩减约 28%（18MB ➔ 13MB），启动毫秒级响应。

- 📱 **Android 客户端沉浸体验优化**
  - **横屏自适应修复**：升级至 Material 3 1.3.1 并优化 `sheetMaxWidth` 约束，彻底修复平板及手机横屏下 BottomSheet 靠右显示与内容截断问题；
  - **会话列表单行对齐**：会话卡片标题统一固定按单行显示，超长文字末尾以省略号截断，保持卡片高度统一整洁；
  - **固定 Release 统一签名**：永久固定 Release 签名密钥库，升级直接无缝覆盖安装，杜绝 `INSTALL_FAILED_UPDATE_INCOMPATIBLE`。

- 🛡️ **网络边界与安全策略**
  - **LAN 策略交互切换**：新增 `mgy lan` 控制台指令，随时查看与一键切换局域网放行/配对策略；
  - **穿透守护与日志降噪**：优化 Cloudflare 专属隧道与 Anycast 优选加速；公网地址脱敏显示，过滤无效 context 报错。

---

### 📦 服务端一键安装 / 更新 (`mgy`)

- **macOS / Linux**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

---

### 📱 客户端与全平台资产

- **Linux**：`multigravity-linux-amd64.tar.gz`
- **macOS**：`multigravity-darwin-universal.tar.gz`
- **Windows**：`multigravity-windows-amd64.zip`
- **Android**：`Multigravity-v1.0.5.apk` (支持直接覆盖安装)
- **iOS**：TestFlight 或通过 Xcode 本地构建
