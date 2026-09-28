# 🚀 Multigravity v1.0.4

### ✨ 核心更新 (Highlights)

- 🧩 **多题交互确认（Multi-Question Interaction）全面支持**：
  - 服务端网关 `mgy` 深度适配 Antigravity 原生 `askQuestion` 多题请求，完整解析全部问题列表，彻底解决以往仅提取首题导致后续题目丢失的问题。
  - 交互提交聚合：支持全端将所有题目的选择项与自定义补充文本（`questionResponses`）打包一次性提交至上游 ConnectRPC，支持多题跳过与取消。
- 📜 **长选项横向滑动查看（防文本截断）**：
  - **iOS 端**：交互卡片移除文本强制省略截断，支持在选项内容区域横向顺畅滑动查看完整文本，完美区分点击选中与水平滑动手势。
  - **Android 端**：重构交互卡片选项容器，支持单选项水平滑动（`Modifier.horizontalScroll`），长选项与代码段清晰完整呈现。
  - **Web 端**：控制台同步支持多问题列表及长选项自适应横滚，全端体验保持一致。
- 🛡️ **Android 健壮性增强与交互卡片重构**：
  - 修复 `PendingInteraction` 因反序列化字段缺失默认值导致的致命异常（`SerializationException`），保障复杂交互会话稳定加载。
  - Android 交互卡片全面支持 Q1、Q2... 题号徽标、单选选项切换与用户自定义补充输入。
- 📦 **全平台版本与构建同步**：
  - 服务端网关 `mgy`、Android (v1.0.4)、iOS (v1.0.4)、跨平台一键安装脚本及 CI/CD 发布流程全面对齐 1.0.4。

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
- **Android**：下载下方 Assets 中的 `Multigravity-v1.0.4.apk` 安装。
- **iOS**：TestFlight 或本地签名构建。
