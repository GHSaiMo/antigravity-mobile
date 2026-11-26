> 🧪 **预发布测试版 / Pre-release Build (`v1.2.8-beta.2`)**
>
> - 此版本仅供灰度内测与尝鲜体验，**不会推送给正式版用户**（`mgy update`、Android 应用内自动更新及一键安装脚本均自动跳过预发布版，`v1.2.7` 仍保持为 `Latest` 正式版）。
> - **网关更新至本测试版**：运行 `mgy update v1.2.8-beta.2`；测试完毕后运行 `mgy update` 即可切回最新正式版。
> - **Android 测试安装**：直接下载下方 `Multigravity-v1.2.8-beta.2.apk` 安装即可（与 `v1.2.7` / `v1.2.8-beta.1` 同私有正式签名，可直接覆盖升级，后续亦可无缝覆盖升级至 `v1.2.8` 正式版）。
> - **iOS 测试安装**：通过 TestFlight 安装 `1.2.8 (42)`。
>
> ---
>
> - This is a **Pre-release** build for grayscale testing and **will not be pushed to stable users** (`mgy update`, Android in-app updater, and install scripts ignore pre-releases; `v1.2.7` remains the `Latest` stable release).
> - **Gateway CLI**: run `mgy update v1.2.8-beta.2` to install this build, or `mgy update` to return to the latest stable release.
> - **Android**: download and install `Multigravity-v1.2.8-beta.2.apk` below (signed with the same private release key as `v1.2.7` for seamless in-place upgrades).
> - **iOS**: install `1.2.8 (42)` via TestFlight.

---

## 🇨🇳 简体中文更新日志

### ✨ `beta.2` 核心新增与体验打磨

#### 1. 多网关多入口无缝切换（iOS / Android 双端对齐）
在保持单主机用户**零视觉打扰**与大标题纯净 `Multigravity` 视觉的前提下，为绑定 2 台及以上主机的用户补齐了更顺手的切换与状态感知体验：
- **左右轻扫标题极速轮换**：无论大标题展开态还是收拢居中态，在顶部标题区**左右轻扫**即可直接循环切换上一台/下一台主机（无需等待 0.22s，不弹窗），3 台及以上主机在顶部切换胶囊自动附带 `(2/3)` 方位进度；垂直拖动 100% 让给列表滚动与下拉刷新。
- **列表上滑居中标题自动变 `[当前主机名 ▾]`**：绑定 `>= 2` 台主机时，列表向上滑动收拢后顶部居中标题自动切换为当前主机名与旋转小三角，**短按直接展开同一套主机切换面板**（支持实时预览各机状态、重命名、解绑与 `+ 添加主机`），同时大/小标题均继续保留 **0.22s 长按下拉滑选**。
- **后台主机待审批微提醒**：当其他非激活主机存在待审批操作时，收拢态 `[主机名 ▾]` 旁亮起一枚 5.5pt 静态橙点，平时零打扰、关键审批不遗漏。
- **全链路细节补齐**：切机自动回到列表顶部；断网错误页明确提示 `无法连接「主机名」` 并提供「切换其他主机」快捷按钮；设置页「设备配对」标明当前主机名防误操作；「手势与使用技巧」补录多主机切换说明，并补全多网关全量中/英/日三语本地化。

#### 2. 自 `beta.1` 以来的其他功能与修复
- **无相机链接配对增强（Gateway / iOS / Android）**：新增 `mgy pair -copy` / `-uri` 快捷导出配对链接，移动端新增一键粘贴按钮并在链接配对流程中完整保留 E2EE 密钥配置。
- **iOS 26/27 聊天页左边缘滑动返回修复（iOS）**：修复在 iOS 26/27 下聊天详情页左侧边缘右滑返回失效问题，并避免与代码横向滚动和长按滑选器冲突。
- **Orchestrator Plan & Execute 配置保留（Gateway / iOS）**：完整保留 Orchestrator Plan & Execute 配置与合成模型 ID。
- **CLI 参数兼容（Gateway）**：`mgy` 新增支持 `-help` 与 `-version` 单横杠参数。

### ✨ 累计 `v1.2.8-beta.1` 核心灰度特性
- **一机多主机（解耦多网关切换）**：一台手机绑定多台完全解耦的桌面网关，独立隔离凭证、E2EE 密钥与本地缓存；支持并发拉取各机状态、多端并发推送与点击通知/灵动岛自动路由至对应主机和会话。
- **`/btw` 侧边分支折叠条、Swarm 看板 Markdown/嵌套列表与回到底部按钮、工作区自动放行及 Windows 终端二维码渲染修复**。

---

## 🇺🇸 English Release Notes

### ✨ What's New & Polished in `beta.2`

#### 1. Multi-Entry Multi-Gateway Switching (iOS & Android Parity)
Keeping **zero visual clutter** for single-host users and preserving the clean `Multigravity` large title, `beta.2` rounds out multi-host ergonomics for users with 2+ paired gateways:
- **Horizontal Title Swipe Cycling**: Swipe left or right across either the expanded `Multigravity` large title or the collapsed centered title to cycle smoothly between hosts with haptic feedback (and `(2/3)` index indicator when 3+ hosts are paired), while vertical drags always yield cleanly to scrolling and pull-to-refresh.
- **Collapsed Centered `[Host Name ▾]` Switcher**: When 2+ hosts are paired and the list scrolls up, the centered inline title becomes your active host name with a rotating chevron. **Tap** to open the same live-status host switcher overlay directly (with rename/remove & `+ Add Host`), while **0.22s long-press & slide down** remains available in both large and collapsed states.
- **Subtle Inactive-Host Approval Dot**: When another background host has pending approvals, a quiet 5.5pt static orange dot appears beside `[Host Name ▾]`—completely hidden otherwise.
- **End-to-End Polish**: Auto-scrolls to top after switching hosts; offline error screen now names the unreachable host with a direct **Switch Host** action; Settings shows the active host name under Pairing Status; added Multi-Host guide to Gestures & Tips along with complete zh/en/ja localization.

#### 2. Additional Improvements & Fixes Since `beta.1`
- **Enhanced Camera-Free Link Pairing (Gateway / iOS / Android)**: Added `mgy pair -copy` / `-uri`, one-tap paste buttons on mobile, and preserved E2EE parameters through link pairing.
- **iOS 26/27 Swipe-Back Fix (iOS)**: Restored reliable leading-edge swipe-back in chat on iOS 26/27 without interfering with horizontal scroll containers or slide pickers.
- **Orchestrator Plan & Execute Config Preservation (Gateway / iOS)**: Preserved Orchestrator Plan & Execute settings and synthetic model IDs.
- **CLI Flag Compatibility (Gateway)**: Added support for single-dash `-help` and `-version` flags.

### ✨ Included from `v1.2.8-beta.1`
- **Decoupled Multi-Gateway Support**: Pair one phone with multiple independent desktop gateways with isolated credentials, E2EE keys, and caches, concurrent live status peeks, and multi-host push/Live Activity routing.
- **Collapsible `/btw` Side Branches, Swarm Board Markdown & scroll-to-bottom button, automatic workspace allow-listing, and Windows terminal QR rendering fixes**.
