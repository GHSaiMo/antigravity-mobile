# 🚀 Multigravity v1.0.4

### ✨ 核心更新 (Highlights)

- 🧩 **多题交互确认完整支持**：网关全面解析原生 `askQuestion` 多题列表，支持全端聚合打包全部题目选择与补充说明（`questionResponses`）一次性提交。
- 📜 **长选项横向滑动防截断**：iOS、Android 与 Web 端交互卡片全面支持长文本平滑横向滚动，彻底告别选项内容被省略截断。
- 🛡️ **Android 健壮性与卡片重构**：修复 `PendingInteraction` 反序列化异常；重构交互卡片，支持分题徽标（Q1/Q2）、单选切换与自由补充输入。
- ⚡ **Cockpit 切号自愈与校验加固**：网关自动校准 Cockpit 启动配置并补全应用路径，彻底解决移动端切号 `APP_PATH_NOT_FOUND` 报错，并加入切号失败自动回滚机制。
- 📦 **全端版本同步**：网关 `mgy`、iOS、Android、一键安装脚本及 CI 发布流程全面对齐 v1.0.4。

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
- **iOS**：TestFlight 或通过 Xcode 本地签名安装。
