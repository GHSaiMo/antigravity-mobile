> 🧪 **预发布测试版 / Pre-release Build (`v1.2.8-beta.3`)**
>
> - 此版本仅供灰度内测与尝鲜体验，**不会推送给正式版用户**（`mgy update`、Android 应用内自动更新及一键安装脚本均自动跳过预发布版，`v1.2.7` 仍保持为 `Latest` 正式版）。
> - **网关更新至本测试版**：运行 `mgy update v1.2.8-beta.3`（已安装过旧版 `beta.3` 可加 `-force` 强制覆盖）；测试完毕后运行 `mgy update` 即可切回最新正式版。
> - **Android 测试安装**：直接下载下方 `Multigravity-v1.2.8-beta.3.apk` 安装即可（`versionCode 44`，与 `v1.2.7` / 早先 `beta` 同私有正式签名，可直接覆盖升级，后续亦可无缝覆盖升级至 `v1.2.8` 正式版）。
> - **iOS 测试安装**：通过 **[TestFlight 公开测试链接](https://testflight.apple.com/join/YM7f4aSK)** 加入并安装 `1.2.8 (52)`（已对齐 `beta.3` 全量能力）。
>
> ---
>
> - This is a **Pre-release** build for grayscale testing and **will not be pushed to stable users** (`mgy update`, Android in-app updater, and install scripts ignore pre-releases; `v1.2.7` remains the `Latest` stable release).
> - **Gateway CLI**: run `mgy update v1.2.8-beta.3` (or add `-force` to refresh an earlier `beta.3` install), and `mgy update` to return to the latest stable release.
> - **Android**: download and install `Multigravity-v1.2.8-beta.3.apk` below (`versionCode 44`, signed with the same private release key as `v1.2.7` for seamless in-place upgrades).
> - **iOS**: join via **[TestFlight Public Beta](https://testflight.apple.com/join/YM7f4aSK)** and install `1.2.8 (52)` (aligned with `beta.3`).

---

## 🇨🇳 简体中文更新日志

### ✨ `beta.3` 核心新增与体验升级（自 `beta.2` 以来）

#### 1. 智能辅助套件（AI Assist）全端上线（Gateway / iOS / Android / Web）
通过本机 Antigravity 原生接口无状态执行，不产生额外会话，开关跨设备同步：
- **智能翻译（Smart Translation）**：支持新会话英文标题自动翻译并覆盖，支持步骤清单（Step Plan）与实时执行状态（Live Focus）实时翻译（同步覆盖 iOS 灵动岛与实时活动），并内置输出校验、去截断与回退重试机制。
- **下一步提示词建议（Next-Prompt Suggestions）**：每轮回复完成后轻量预测 1 条建议胶囊，内联展示在输入栏 `+` 右侧（优先首行延伸占满宽度后再折行展示完整文案），采用**短按填入输入框、长按 1.5 秒进度条防呆直发（提前松手自动取消）**交互并在「使用技巧与快捷手势」中补充说明，出现审批卡片或已有草稿时自动隐藏。
- **长会话防失忆（Post-Compact Rehydration）**：支持在本机 `~/.gemini/config/hooks.json` 注册轻量原生 `PreInvocation` Hook，在 `/compact` 或自动压缩后的下一轮调用前自动单次回灌近期目标、改动文件与未完成要点（自动脱敏凭据）。

#### 2. 远程网关一键自更新 & 统一「宿主机更新」二级页（Gateway / iOS / Android / Web）
- **手机/Web 端远程更新 `mgy` 网关**：在设置页「关于 → 宿主机更新」中可同时查看电脑端 Antigravity 桌面端与 `mgy` 网关版本状态，支持远程一键下载、SHA-256 校验、热替换二进制并自动重启重连。
- **设置页横向二级菜单体验**：将「宿主机更新」由叠层弹窗重构为设置页二级菜单（iOS 使用 `NavigationStack` 原生推入与右滑返回，Android 与 Web 支持平滑左右切换及跟手右滑返回）。

#### 3. 配额监控、阈值自动切号与上下文审计（Gateway / iOS / Android / Web）
- **空闲安全阈值自动切号（Idle-Safe Auto-Switch）**：支持按 Gemini / Claude 的 5h 或 Weekly 额度池设定触发阈值，默认等待本轮生成空闲后自动切换账号，避免打断运行中的任务；同步打磨 iOS 额度页「阈值自动切号」展开交互。
- **上下文占比拆解与动态定价**：新增上下文细分（Context Breakdown）审计视图并优化 iOS 半屏自适应展示；新增动态模型定价与配额消耗预估，修复 Claude 驱动的 Orchestrator 模型正确归属 Claude 额度池。

#### 4. 会话、定时任务、斜杠指令与预览体验打磨（Gateway / iOS / Android / Web）
- **定时任务（Automations）全面对齐桌面端**：支持「用 Prompt 创建定时任务」流程；对话中的定时任务触发提示词气泡默认折叠并支持点按展开；保留独立定时任务工作区名称与闹钟标识。
- **多条有序斜杠指令**：支持在单条消息中输入并按顺序组合多个 `/command` 斜杠指令。
- **HTML 产物预览卡片与预览器精简**：自动识别面向用户的 HTML 产物并以可交互预览卡片呈现；保留 HTML 与带标题文档链接的真实文件扩展名；移除各预览页多余的页内搜索按钮。
- **模型选择器与交互细节修复**：保留经典双列模型选择器与配额窗口显示并升级 `XHigh` 命名与胶囊边框；优化双端主机滑选器居中宽度与卡片外纵向拖拽；统一 `/btw` 旁路图标为问号气泡；修复消息发送后因乱序旧帧与空闲缓存导致新气泡短暂消失及 iOS 顶部轮次/回底按钮闪跳的问题；修复分支抽屉顶部锚点跳动、Android 交互填写项软键盘弹出及 iOS 移除主机时导航标题闪烁问题。

#### 5. 网关跨平台服务、CLI 发现与中继稳定性加固（Gateway）
- **跨平台后台自启与日志轮转**：新增 `mgy service` 支持 macOS / Linux / Windows 开机自启与按大小轮转日志，Windows 后台子进程默认静默隐藏控制台窗口。
- **Antigravity CLI 与局域网被动发现**：支持 Antigravity CLI (`agy`) 探测与 `~/.gemini/antigravity-cli` 回退、Linux `agy` 守护进程自动重启与 OAuth Token 同步；支持独立 `mgy` 通过 6 位配对码进行局域网 Hub 被动发现；按 `projectId` 去重 Antigravity 2.0 项目；修复上游 Marketplace 作者对象与精选货架索引解析。
- **中继探活与多网关隔离加固**：中继链路切换升级为丢包感知的多样本测速、本地重探与运行时 Ping 逃逸；严格隔离多网关 epoch、云端路由与 401 解绑处理；增强上游恢复、proxy-wrapper 发现与 503 诊断信息。

### ✨ 累计 `v1.2.8-beta.1` / `beta.2` 核心灰度特性
- **Antigravity 2.35.2 全套原生能力**：浏览器实时预览与触控接管、远程 SSH 主机管理、插件市场检索安装与 OAuth 闭环、四档权限策略、工作区代码/文件搜索、产物历史快照、Worktree PR 与会话标星。
- **一机多主机（解耦多网关）与多入口切换**：完全隔离的多主机凭证/E2EE/缓存、左右轻扫标题极速切机、收拢居中主机切换器与后台待审批微提醒。
- **`/btw` 旁路内收抽屉与完整树形连线、推送脱敏、自建中继（BYO-Relay）、强制 E2EE v2、局域网 `mgy hub` 聚合与无相机链接配对增强**。

---

## 🇺🇸 English Release Notes

### ✨ What's New & Improved in `beta.3` (Since `beta.2`)

#### 1. AI Assist Suite Across All Clients (Gateway / iOS / Android / Web)
Powered statelessly by Antigravity's native local RPC with zero extra sessions and cross-device synced toggles:
- **Smart Translation**: Auto-translate new English conversation titles, plus live translation for Step Checklists and Live Focus status lines (including iOS Dynamic Island & Live Activities), with output validation, untruncated titles, and fallback retry.
- **Next-Prompt Suggestions**: Predicts a concise follow-up chip after each reply, rendered inline beside `+` (expanding along a full first line before wrapping so the full text stays readable) with **tap-to-fill and 1.5s hold-to-send with a live progress bar** (release early to cancel; documented in Tips & Gestures) and auto-hiding when approvals or drafts are active.
- **Post-Compact Rehydration**: Registers a lightweight local `PreInvocation` hook in `~/.gemini/config/hooks.json` to inject recent user intent and modified files once after `/compact` or auto-compaction (with credential redaction).

#### 2. Remote Gateway Self-Update & Unified Host Updates Subpage (Gateway / iOS / Android / Web)
- **One-Tap Remote `mgy` Gateway Updates**: Inspect both Antigravity Desktop and `mgy` Gateway versions under **Settings → About → Host Updates**, and trigger remote download, SHA-256 verification, in-place binary replacement, and automatic reconnect right from mobile or Web.
- **Native Horizontal Settings Subpage**: Moved Host Updates out of stacked modals/dialogs into a horizontal sliding Settings subpage (native `NavigationStack` push & swipe-back on iOS; smooth horizontal slide & interactive swipe-back on Android and Web).

#### 3. Quota Auto-Switch, Dynamic Pricing & Context Breakdown (Gateway / iOS / Android / Web)
- **Idle-Safe Auto-Switch**: Automatically switch accounts when a chosen quota pool (Gemini/Claude 5h or Weekly) falls below your threshold—waiting for the active turn to finish idle by default—with a refined expandable section on iOS.
- **Context Breakdown & Dynamic Pricing**: Added context usage breakdown inspection with auto-fitting half-sheet sizing on iOS, dynamic model pricing and quota estimates, and fixed Claude-backed Orchestrator models to map to the Claude quota pool.

#### 4. Automations, Multi-Slash Commands, HTML Previews & UI Polish
- **Desktop-Aligned Automations**: Added Create-with-Prompt flow, collapsed scheduled-task prompt bubbles by default with tap-to-expand, and preserved independent scheduled workspace names with trailing alarm icons.
- **Multiple Ordered Slash Commands**: Compose and execute multiple ordered `/commands` in a single message across all clients.
- **HTML Artifact Preview Cards & Cleaner Viewers**: Surfaces user-facing HTML artifacts as interactive preview cards, preserves real file extensions for HTML and titled document links, and removes redundant in-page search buttons from previewers.
- **Model Picker & Ergonomics**: Restored classic 2-column model picker and quota window while keeping `XHigh` naming and chip border; polished centered host slide picker drag behavior, unified `/btw` question-bubble icons, fixed newly sent user bubbles briefly vanishing due to out-of-order stale snapshots/idle caching and iOS turn-counter/jump-to-bottom flickers, and fixed branch drawer top anchoring, Android write-in keyboard focus, and iOS host-removal title stability.

#### 5. Cross-Platform Background Service, CLI Discovery & Relay Hardening (Gateway)
- **Cross-Platform `mgy service` & Rotating Logs**: Added background auto-start service management across macOS/Linux/Windows, size-based log rotation, and Windows `HideWindow` console suppression.
- **Antigravity CLI & Passive LAN Discovery**: Added `agy` CLI discovery with `~/.gemini/antigravity-cli` fallback, Linux `agy` daemon restart and OAuth token sync, passive LAN discovery via 6-char pair code for standalone `mgy` hubs, Antigravity 2.0 project deduplication by `projectId`, and fixed upstream Marketplace author object & featured shelf parsing.
- **Loss-Aware Relay Probing & Multi-Gateway Isolation**: Added multi-sample loss-aware relay probing, local reprobe, and runtime ping escape; isolated multi-gateway epochs, cloud URLs, and 401 unpair handling; hardened upstream recovery, proxy-wrapper discovery, and 503 diagnostics.

### ✨ Included from `v1.2.8-beta.1` & `beta.2`
- **Antigravity 2.35.2 Native Capabilities**: Browser Live View & touch takeover, remote SSH host management, Marketplace discovery & Plugin/MCP OAuth closure, 4-tier permission presets, workspace file/code search, artifact snapshots, Worktree PRs, and conversation starring.
- **Decoupled Multi-Gateway & Multi-Entry Switching**: Isolated multi-host credentials/E2EE/caches, horizontal title swipe cycling, collapsed host switcher, and subtle inactive-host approval indicator.
- **Unified `/btw` recessed branch drawer & tree spine, push redaction, BYO-Relay, Force-E2EE v2, LAN `mgy hub` aggregation, and camera-free link pairing enhancements**.
