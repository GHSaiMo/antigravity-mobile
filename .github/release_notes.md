# 🚀 Multigravity v1.0.4

### ✨ 核心更新 (Highlights)

- ⚡ **客户端智能 Anycast 优选加速**：
  - 双端（Android / iOS）网络层内置覆盖国内电信、联通、移动的 Anycast 优质节点池。
  - 启动及切网时后台并发赛马测速，毫秒级锁定延迟最低节点（实测 60~70ms），告别跨洋绕路与晚高峰卡顿。
  - 保留系统原生 DNS 终极兜底，兼具极速连接与 100% 可用性。
- 🔗 **极简配对协议与短域名**：
  - 终端配对看板彻底收敛为单条全局 URI，仅携带 8 位短主机名，大幅降低二维码密度。
  - 客户端扫码与手动输入自动补全并适配根域通配符 SSL，保护主域隐私。
- 🧩 **多题交互确认与卡片体验重构**：
  - 完整支持原生 `askQuestion` 多题列表批量确认与统一提交。
  - 卡片高度根据题目自适应无留白，支持半屏/全屏平滑切换与多行换行。
  - 弹出选项时软键盘智能避让，提升单手操作体验。
- ⚡ **Cockpit 切号自愈与自动化**：
  - 自动校准 IDE 路径、软链接与配置表，根除切号失败与配置文件丢失问题。
- 🎯 **方案 Proceed 状态精准对齐**：
  - 修复文档局部修改时审批状态误回退缺陷，保持手机端与桌面端状态严格一致。
- 🛡️ **健壮性优化**：
  - 修复 Android 交互模型反序列化异常，优化弱网下的连接恢复机制。

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
