# 🚀 Multigravity v1.0.4

### ✨ 核心更新 (Highlights)

- 🧩 **多题交互确认完整支持**：网关全面解析原生 `askQuestion` 多题列表，支持全端聚合打包全部题目选择与补充说明（`questionResponses`）一次性提交。
- 🎨 **选择题交互卡片重构与深度优化**：
  - **高度自适应无留白**：根据题目和选项内容自适应卡片高度，杜绝多余空白；超出半屏自动限高并支持内滚动，右上角提供一键全屏/半屏平滑切换。
  - **问题与选项全内容多行呈现**：问题文本与选项均支持多行完整换行，告别省略截断与横向滚动。
  - **活力橙主题色统一**：边框、问号图标与题号角标统一采用活力橙主题色，对齐锁屏与灵动岛问题通知风格。
  - **输入法智能避让**：出现选项时自动收起软键盘，避免遮挡用户查看与操作。
- 🛡️ **Android 健壮性与卡片重构**：修复 `PendingInteraction` 反序列化异常；重构交互卡片，支持分题徽标（Q1/Q2）、单选切换与自由补充输入。
- ⚡ **Cockpit 切号自愈与全流程自动化**：网关自动校准 Cockpit 启动配置、自动补全缺失的 state.vscdb / Antigravity IDE 软链接与存储表、热重载配置并自动完成平滑热重启与账号注入，彻底根治切号 APP_PATH_NOT_FOUND 与 state.vscdb 丢失问题。
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
