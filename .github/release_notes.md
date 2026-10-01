# 🚀 Multigravity v1.0.4

### ✨ 核心更新与优化 (Highlights)

- ⚡ **智能 Anycast 优选与穿透常驻自愈**：
  - 双端内置国内 Anycast 节点池并发测速（实测 60~70ms），告别晚高峰跨洋绕路卡顿；
  - 严格规范 Cloudflare 穿透参数并引入常驻守护（Supervisor），休眠或断线自动毫秒级拉起，保障外网 100% 可用。

- 🧩 **多题交互与审批确认体验重构**：
  - 完整支持原生 `askQuestion` 多题批量确认与统一提交，弹出选项时软键盘智能避让；
  - 优化卡片自适应高度与半屏/全屏切换，精准对齐方案 Proceed 执行状态。

- 🔤 **双端 Markdown & LaTeX 排版对齐**：
  - 支持带参可伸缩箭头（`\xrightarrow` 等）、结论框及上下标注，兼容 AI 流程图与图表展示；
  - Android 端补全 150+ 数学符号单趟扫描与代码隔离保护，杜绝符号遗留与美元金额冲突。

- 🌐 **纯净 IPv4 架构与极简短配对**：
  - 全栈收敛为纯 IPv4 架构，终端配对看板精简为 8 位短主机名，大幅降低二维码密度；
  - 智能过滤 Cloudflare 握手重复警告，终端日志更清爽。

- 🔑 **永久固定 Android 发布签名**：
  - 配置永久固定的 Release 签名密钥库，后续所有版本均支持直接无缝覆盖安装（注：从旧版升级至此版本需最后卸载一次旧版）。

---

### 📦 服务端安装 / 更新 (`mgy`)

- **macOS (Apple Silicon & Intel)**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

---

### 📱 客户端下载
- **Android**：下载下方 Assets 中的 `Multigravity-v1.0.4.apk` 安装。
- **iOS**：TestFlight 或通过 Xcode 本地签名安装。
