# 🚀 Multigravity v1.0.1

### ✨ 优化与修复 (Improvements & Fixes)

- **一键安装脚本镜像站优先与代理直连策略优化**：
  - 针对国内用户和代理环境（如 Clash / V2Ray / Surge 等 7890 端口），调整一键脚本默认优先通过高速镜像源（`ghfast.top` / `ghproxy.net`）直连下载，避免强行走海外代理导致的限速或网络中断。
  - 仅在镜像站不可用时才自动回退至 GitHub 官方源并接入本机代理加速。
  - 修复镜像直连参数中 `--noproxy "*"` 因变量未加引号触发的 Shell 通配符目录展开（Globbing）漏洞。
  - 优化文档代码块格式，避免在 macOS 原生 zsh 下粘贴执行时误触发 `command not found: #`。
  - 规范 shell 脚本换行符为 LF，防止在 macOS / Linux / Git Bash 下出现 CRLF 解析异常。

- **IPv6 终端配置与排错指引多平台自适应**：
  - 终端配对信息及 IPv6 状态提示自动感知操作系统（Windows / macOS / Linux）。
  - Windows 系统下准确提示前往「设置 -> 网络和 Internet」开启「Internet 协议版本 6 (TCP/IPv6)」，并补充 Windows Defender 防火墙放行指引，不再显示 macOS 专属设置文案。

- **Windows 项目区路径与最近项目展示修复**：
  - 聚合 Antigravity 官方工作区、`workspaceStorage`（活跃与最近打开的项目）、`state.vscdb` 以及 Cascade 对话历史记录。
  - 支持中文（URL 编码解码）、空格路径及 `vscode-remote://` 远程项目。

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
- **Android**：下载下方 Assets 列表中的 `Multigravity-v1.0.1.apk` 直接安装。
- **iOS**：TestFlight 或项目内工程自行签名构建。

