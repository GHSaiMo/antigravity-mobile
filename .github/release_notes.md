# 🚀 Multigravity v1.0.4

### ⚠️ 老用户重要升级提示 (Breaking Change / Migration)
- **老用户升级后必须执行一次 `mgy cf reset`**：
  本次版本已将 Cloudflare 穿透体系全面升级为**方案 A：客户端智能 Anycast 极速加速体系（`.jiuge.space`）**。
  **此前已生成旧域名的老用户**，在升级网关后请务必在终端执行：
  ```bash
  mgy cf reset
  ```
  该命令将自动重置并接入新的原生专属隧道与极速短域名通道。完成后重新在手机 App 扫码配对即可。

---

### ✨ 核心更新 (Highlights)

- ⚡ **客户端智能 Anycast 优选加速架构全面落地（方案 A）**：
  - **原生极速通道**：网关专属域名精简升级为单层根域通配符覆盖的 `.jiuge.space`（如 `825a5a50.jiuge.space`），享受 Cloudflare Universal SSL 毫秒级免等自动签发。
  - **移动端智能多线路 Anycast 并发优选加速**：Android（OkHttp `CloudflareOptimizedDns`）与 iOS（`CloudflareAnycastAccelerator`）网络层内置覆盖电信、联通、移动全运营商的高概率亚太直连 Anycast 候选节点池，并在后台启动高并发 TCP 赛马测速（毫秒级动态锁定当前真实网络环境下延迟最低的节点，实测 60~70ms），免去国内运营商劣质/污染的递归 DNS 解析，直通 Cloudflare 骨干内网；同时保留系统默认解析作为终极兜底，彻底根除 Error 1000 错误与晚高峰连接卡顿！
  - **多租户/多用户原生隔离**：每个用户的 `mgy` 均对应独立的专属 Cloudflare Tunnel 与原生 DNS CNAME 记录，天然支持多用户自由分发。
- 🔗 **极简配对协议与短域名自动补全**：
  - 配对看板与二维码彻底收敛为单条全局 `🔗 配对 URI`，移除了原先分开输出的公网/局域网冗余链接。
  - 二维码仅携带短主机名（如 `825a5a50`）与局域网 IP，省略默认 `port=443` 与 `ssl` 冗余参数，大幅降低二维码密度提高扫码成功率，同时保护根域名隐私。
  - Android & iOS 客户端对齐新的短域名解析逻辑，扫码及手动输入自动拼接 `.jiuge.space`，并智能维持局域网 Wi-Fi 直连与公网穿透双路由。
- 🧩 **多题交互确认完整支持**：网关全面解析原生 `askQuestion` 多题列表，支持全端聚合打包全部题目选择与补充说明（`questionResponses`）一次性提交。
- 🎨 **选择题交互卡片重构与深度优化**：
  - **高度自适应无留白**：根据题目和选项内容自适应卡片高度，杜绝多余空白；超出半屏自动限高并支持内滚动，右上角提供一键全屏/半屏平滑切换。
  - **问题与选项全内容多行呈现**：问题文本与选项均支持多行完整换行，告别省略截断与横向滚动。
  - **活力橙主题色统一**：边框、问号图标与题号角标统一采用活力橙主题色，对齐锁屏与灵动岛问题通知风格。
  - **输入法智能避让**：出现选项时自动收起软键盘，避免遮挡用户查看与操作。
- 🛡️ **Android 健壮性与卡片重构**：修复 `PendingInteraction` 反序列化异常；重构交互卡片，支持分题徽标（Q1/Q2）、单选切换与自由补充输入。
- ⚡ **Cockpit 切号自愈与全流程自动化**：网关自动校准 Cockpit 启动配置、自动补全缺失的 state.vscdb / Antigravity IDE 软链接与存储表、热重载配置并自动完成平滑热重启与账号注入，彻底根治切号 APP_PATH_NOT_FOUND 与 state.vscdb 丢失问题。
- 🎯 **方案 Proceed 状态精准对齐**：彻底修复历史已审批文档在后续局部编辑时（`replace_file_content`）因磁盘过期元数据回退而误触发 Proceed 按钮的缺陷，保持手机双端与桌面端 Antigravity IDE 状态 100% 对齐。
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
