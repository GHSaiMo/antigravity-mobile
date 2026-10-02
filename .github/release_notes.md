# 🚀 Multigravity v1.0.5

### ✨ 核心更新与优化 (Highlights)

- 🐧 **首发 Linux x86_64 官方支持与真机/NAS 生产验证**：
  - **原生 Linux x86_64 (amd64) 纯静态构建**：脱离任何 glibc 动态库依赖，开箱即用于各类 Linux 发行版（Debian / Ubuntu / Arch / CentOS / 群晖 DSM / 极空间 / 自建 NAS / 无头服务器等）；并在真实 Linux 环境中完成全链路端到端闭环验证；
  - **Antigravity Daemon Discovery 协议支持**：引入对 `~/.gemini/antigravity/daemon/ls_*.json` 发现文件的原生动态解析，实现 0 子进程开销的毫秒级即时发现与反向代理；
  - **Headless / Standalone 免 CSRF 令牌连通**：放宽对 `--csrf_token` 命令行参数的硬编码限制，全面兼容无头模式或常驻服务模式运行的 Linux 版 Language Server；
  - **双模端口探测 (lsof + ss)**：在 Linux 平台针对非特权用户环境，增加基于 `ss -H -tlnp` 的端口探测降级机制，确保在容器或受限 Linux 系统上依然稳健捕获监听端口；
  - **原生守护与后台自愈**：支持配置为 systemd 用户服务（如跟随 `antigravity-ls.service` 开机自启），进程守护与健康探活平滑一体化。

- 📦 **全平台一键安装脚本体验升级 (`scripts/install.sh`)**：
  - 自动识别 Linux 平台环境与 `x86_64` 架构，下载解压对应包并输出专属提示；
  - 自动创建全局软链接至 `/usr/local/bin/mgy`，在当前终端及新终端即刻生效，无需手动配置环境变量。

- 🛡️ **智能网络边界与局域网免配对直通 (Trust LAN / Localhost Bypass)**：
  - **本机访问 100% 免配对**：真正本机发起访问（`127.0.0.1` / `::1`）自动授权并派发 Admin 会话，直达全功能桌面工作台；
  - **局域网安全策略可配置**：首次安装引导用户按环境选择偏好（`MULTIGRAVITY_TRUST_LAN=1` 信任局域网免配对直通；默认 `0` 保持配对码校验）；
  - **Cloudflare 穿透边界严格隔离**：精准校验请求指纹（`CF-Connecting-IP` / `CF-Ray`），严禁穿透流量免密绕过；
  - **自适应环境感知与友好看板**：Linux 无头环境自动屏蔽浏览器弹窗并在终端输出清晰看板，桌面环境自动唤起默认浏览器。

- 🖥️ **网页桌面版汉化与视图自适应优化**：
  - 深度汉化桌面工作台新版菜单与操作按钮；
  - 修复平板 / iPad 设备视图模式判定，平板与桌面统一直接渲染全功能桌面工作台，消除手机版配对浮层误弹。

- ⚡ **智能 Anycast 优选与穿透常驻自愈**：
  - 内置国内 Anycast 节点池并发测速（实测 60~70ms），告别晚高峰跨洋绕路卡顿；
  - 规范 Cloudflare 穿透参数并引入常驻守护（Supervisor），休眠或断线自动毫秒级拉起，保障外网 100% 可用。

- 🧩 **多题交互与审批确认体验重构**：
  - 完整支持原生 `askQuestion` 多题批量确认与统一提交，弹出选项时软键盘智能避让；
  - 优化卡片自适应高度与半屏/全屏切换，精准对齐方案 Proceed 执行状态。

- 🔑 **永久固定 Android 发布签名**：
  - 保持永久固定的 Release 签名密钥库，升级直接无缝覆盖安装，杜绝 `INSTALL_FAILED_UPDATE_INCOMPATIBLE`。

---

### 📦 服务端安装 / 更新 (`mgy`)

- **macOS (Apple Silicon & Intel) / 🐧 Linux (x86_64 / NAS)**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

---

### 📱 客户端与全平台资产下载
- **Linux 服务端**：`multigravity-linux-amd64.tar.gz` (全新加入 ⭐)
- **macOS 服务端**：`multigravity-darwin-universal.tar.gz` / `multigravity-darwin-arm64.tar.gz` / `multigravity-darwin-amd64.tar.gz`
- **Windows 服务端**：`multigravity-windows-amd64.zip`
- **Android 客户端**：`Multigravity-v1.0.5.apk` (支持直接覆盖安装)
- **iOS 客户端**：TestFlight 或通过 Xcode 本地构建安装。

