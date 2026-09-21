# 🚀 Multigravity v1.0.1

### 🐛 修复与优化 (Bug Fixes & Improvements)

- **Windows 项目区路径与最近项目展示修复**：
  - **多源项目聚合检索**：重构项目扫描机制，全面聚合 Antigravity 官方工作区、`workspaceStorage`（活跃与最近打开的项目）、`state.vscdb` 以及 Cascade 对话历史记录，彻底解决部分项目（尤其是最近常用项目）未能展示在移动端的问题。
  - **中文及特殊字符路径兼容**：支持包含中文（URL 编码解码）、空格及深层目录的项目路径，修复 Windows 盘符与 `file://` 规范化匹配异常。
  - **远程工作区支持**：兼容识别 `vscode-remote://` (SSH/WSL) 项目，避免被误判为无效本地路径。
  - **文件安全沙箱加固**：工作区安全白名单同步对齐 URL 解码路径，确保项目内文件查看与上下文传输安全稳定。

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

