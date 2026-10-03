# 🚀 Multigravity v1.0.5

### ✨ 核心更新 (Highlights)

- 🐧 **Linux 官方支持 (x86_64 / NAS)**
  - 原生提供纯静态构建，兼容各类 Linux 发行版与 NAS 环境（Debian / Ubuntu / 群晖 DSM / 极空间 / 自建 NAS 等）；
  - 支持 Antigravity Daemon 动态发现、Headless 免令牌连通与双模端口探测，可作为 systemd 用户服务平滑常驻。

- ⚡ **会话体验与性能优化**
  - **秒开防卡死**：重构会话加载流程与流心跳机制，彻底消除切换会话时的无限转圈与卡顿；
  - **三端自适应**：根据屏幕宽度自动适配桌面端（≥1024px）、平板端（768~1024px）与手机端（<768px），切换时精准保留当前会话；
  - **停止按钮修复**：彻底修复 Web 桌面版停止按钮 (Stop/Cancel) 无响应问题，取消状态秒级同步。

- 🛡️ **网络边界与安全策略**
  - **本机免配对直连**：`127.0.0.1` 本机直通全功能桌面工作台；
  - **LAN 策略交互切换**：新增 `mgy lan` 控制台指令，随时查看与一键切换局域网放行/配对策略；
  - **穿透守护与日志降噪**：优化 Cloudflare 专属隧道与 Anycast 优选加速；公网地址脱敏显示，过滤无效 context 报错。

- 🌐 **深度汉化与交互重构**
  - 桌面工作台全场景汉化（侧边栏、主题预设、系统设置、代码变更与独立终端）；
  - 重构 `askQuestion` 多题批量确认与方案审批交互体验。

- 🔑 **Android 统一签名**
  - 永久固定 Release 签名密钥库，升级直接无缝覆盖安装。

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
