> 🧪 **预发布测试版 / Pre-release Build (`v1.2.8-beta.1`)**
>
> - 此版本仅供灰度内测与尝鲜体验，**不会推送给正式版用户**（`mgy update`、Android 应用内自动更新及一键安装脚本均自动跳过预发布版，`v1.2.7` 仍保持为 `Latest` 正式版）。
> - **网关更新至本测试版**：运行 `mgy update v1.2.8-beta.1`；测试完毕后运行 `mgy update` 即可切回最新正式版。
> - **Android 测试安装**：直接下载下方 `Multigravity-v1.2.8-beta.1.apk` 安装即可（与 `v1.2.7` 同私有正式签名，可直接覆盖升级，后续亦可无缝覆盖升级至 `v1.2.8` 正式版）。
>
> ---
>
> - This is a **Pre-release** build for grayscale testing and **will not be pushed to stable users** (`mgy update`, Android in-app updater, and install scripts ignore pre-releases; `v1.2.7` remains the `Latest` stable release).
> - **Gateway CLI**: run `mgy update v1.2.8-beta.1` to install this build, or `mgy update` to return to the latest stable release.
> - **Android**: download and install `Multigravity-v1.2.8-beta.1.apk` below (signed with the same private release key as `v1.2.7` for seamless in-place upgrades).

---

## 🇨🇳 简体中文更新日志

### ✨ 核心灰度新特性：一机多主机（解耦多网关切换）
面向同时管理多台电脑（如公司 Mac + 家里台式机 / 远程开发机）的高级用户，现支持**一台手机绑定多台完全解耦的桌面网关**，单主机用户全程零视觉打扰：
- **隐蔽触发，零心智负担**：在首页顶部 **长按 `Multigravity` 标题（约 0.22 秒）并向下滑动** 即可呼出多主机切换浮层（与模型栏「长按上滑」对称）；单主机模式下首页与设置页**不新增任何多余按钮或角标**。
- **实时预览各主机状态 & 一键添加**：浮层内并发拉取所有已配对主机的实时状态（运行中会话数 / 未读完成数 / 离线状态），松手即可无感秒切网关并彻底隔离本地缓存、草稿与连接；底部内置 **`+ 添加主机`** 扫码入口，松手停留可点击 `⋯` 重命名或解绑任意主机。
- **多端并发推送 & 点哪条切哪台**：已绑定的多台网关同时独立推送通知（iOS APNs / 灵动岛与 Android FCM），绑定 2 台及以上主机时自动展示主机名副标题区分来源；点击任意通知或在锁屏/灵动岛直接点选「批准 / 拒绝 / 回复」，均会自动路由至对应网关与目标会话。
- **全向后兼容**：线上 APNs Relay Worker 已升级至 `v1.2.1` 并完全兼容旧版网关与客户端，老用户无感升级。

### 🛠️ 自 `v1.2.7` 以来的其他功能与修复
- **`/btw` 侧边分支折叠条（iOS / Android / Web）**：将 `/btw` 支线问答收拢为附着在主消息下方的可展开折叠条，保持主对话流清爽连贯。
- **Swarm 看板体验增强（iOS / Android / Web）**：完整支持 Markdown 与多级嵌套列表渲染，并新增半透明悬浮「回到底部」快捷按钮。
- **工作区自动放行与路径兼容（Gateway）**：自动放行已在 Antigravity 注册的工作区目录，并支持根目录路径大小写不敏感匹配。
- **Windows 终端二维码显示修复（Gateway）**：改用 ANSI 配色与 `U+2584` 下半块字符渲染终端配对二维码，解决 Windows `conhost` 下缺失 `U+2580` 字符导致扫码异常的问题。
- **移动端连接与交互修复**：
  - 切回 Wi-Fi 时更快优先切换至局域网直连，解除下拉刷新偶发阻塞；
  - Android 恢复消息文本直接点选与长按空白区弹出操作菜单；
  - iOS 修复取消右滑返回手势后聊天页顶栏标题偶发消失的问题。
- **发布通道安全隔离（CI/CD）**：自动识别含 `-` 的 SemVer 预发布标签并保护 `Latest` 正式版通道。

---

## 🇺🇸 English Release Notes

### ✨ Grayscale Beta Feature: Decoupled Multi-Gateway Support
Built for power users working across multiple desktops (e.g., office Mac + home workstation / remote dev box). One phone can now pair with multiple completely independent gateways with **zero UI clutter** for single-host users:
- **Hidden Gesture, Zero Mental Overhead**: **Long-press the Home `Multigravity` hero title (~0.22s) and slide down** to reveal the host switcher overlay (mirroring the composer's press-and-slide-up model picker). No new buttons or badges are added to the Home screen or Settings.
- **Live Host Peek & `+ Add Host`**: The overlay concurrently fetches live status (`working` / `unread` / `offline`) for every paired host. Release on a host to switch cleanly with isolated credentials, caches, drafts, and E2EE keys; slide to **`+ Add Host`** to scan and bind another gateway, or pause to tap `⋯` for renaming/unpairing.
- **Concurrent M×N Push & Exact Routing**: All paired gateways deliver push notifications (iOS APNs / Live Activities & Android FCM) independently. When 2+ hosts are paired, notifications automatically show the host name subtitle; tapping any notification or using lock-screen/Live Activity quick actions routes directly to the exact gateway and conversation.
- **Full Backward Compatibility**: The APNs Relay Worker is live at `v1.2.1` with additive passthrough—existing gateways and clients continue working seamlessly without changes.

### 🛠️ Other Improvements & Fixes Since `v1.2.7`
- **Collapsible `/btw` Side Branches (iOS / Android / Web)**: Side-branch Q&A now collapses into an attached toggle strip beneath the parent message to keep the main timeline clean.
- **Swarm Board Polish (iOS / Android / Web)**: Added full Markdown and nested bullet rendering plus a translucent floating scroll-to-bottom button.
- **Workspace Auto-Allow (Gateway)**: Automatically allows registered Antigravity workspaces with case-insensitive root path matching.
- **Windows Terminal QR Fix (Gateway)**: Switched terminal QR rendering to ANSI colors with `U+2584` half-blocks to fix missing `U+2580` glyphs in Windows `conhost`.
- **Mobile Connectivity & UX Fixes**:
  - Faster switch back to LAN on Wi-Fi and unblocked pull-to-refresh;
  - Restored direct text selection on Android alongside blank-area long-press menus;
  - Fixed iOS chat toolbar title disappearing after a cancelled swipe-back gesture.
- **Pre-release Channel Protection (CI/CD)**: Release workflow now auto-detects SemVer pre-release tags and keeps the `Latest` stable channel untouched.
