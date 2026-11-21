# 🚀 Multigravity v1.2.2

> 🎉 **Multigravity（反重力智能体移动伴侣）iOS 版现已正式上架 Apple App Store！**  
> 无需本地编译或自签，前往 👉 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)**（覆盖海外 174 个国家/地区，内置零配置**离线演示模式**）；也可加入 👉 **[TestFlight 公开测试](https://testflight.apple.com/join/YM7f4aSK)** 抢先体验最新构建。
>
> 🎉 **Multigravity for iOS is officially available on the Apple App Store!**  
> Install directly from the 👉 **[App Store](https://apps.apple.com/app/id6835508934)** (174 countries & regions, with built-in zero-config **offline Demo Mode**), or join the 👉 **[TestFlight Public Beta](https://testflight.apple.com/join/YM7f4aSK)** for pre-release builds.

---

## 🇨🇳 中文更新说明

### ✨ 核心亮点与新特性

- 🧠 **Antigravity 2.34.0 全端深度对齐（Gemini 4 Neon & Orchestrator）**：
  - 刷新 Language Server RPC 基线至 `2.34.0`（378 个接口全量校验通过）；
  - Go 网关、iOS、Android 与 Web 全端原生支持 **Gemini 4 Neon**（Low / Medium / High / Extra）与 **Orchestrator (Neon)**，补齐模型枚举、紧凑胶囊短名（`Neon · High` / `Orch · Neon`）、默认推荐及 Token 历史账单归因。
- 🧩 **MCP Apps 完整交互闭环 & 点击 Skill 徽章直达 `SKILL.md`**：
  - 网关透明桥接 `ui://` 资源（自动解析 `serverName` 并在首次加载时自动 `LaunchMcpApp`），三端 `<agent-embed>` 补齐 `resources/read` 与工具调用结果回传；
  - 对话气泡内带定义路径的 `/skill` 徽章保持原有简洁外观，点击即可直接复用现有 Markdown 预览页查看对应 `SKILL.md`。
- ⚡ **大仓库 Git 状态缓存 & 单列行号槽 Diff 排版**：
  - 网关 `git status` 增加短时 TTL 缓存与并发请求合并（提交、推送、切分支、丢弃改动时自动失效），显著提升大型代码仓的刷新速度；
  - 三端 Diff 视图升级为单列紧凑行号槽 + 左侧彩色变更竖条，小屏代码审查更清晰。
- ⏰ **定时任务（Automations）详情编辑、下次运行时间对齐与移动端启动修复（iOS / Android / Web）**：
  - 新增全端定时任务详情与编辑页面（支持修改名称、Cron 计划、模型、工作区、提示词及查看运行历史）；
  - 彻底修复移动端新建、启停与「立即运行 Run Now」的状态同步与启动问题；
  - 网关改由 Cron 表达式（含 `CRON_TZ` / `TZ` 时区）直接推算下一次未来运行时间，彻底解决 Antigravity 调度器 Jitter 窗口期内三端显示已过期整点、与电脑端相差 1 小时的问题，手机端修改计划后亦即时同步生效。
- 🧭 **交互提示与手势指南 & 移动端细节打磨（iOS / Android）**：
  - 新增「使用技巧与手势指南」页面（Android 集成至设置平滑侧滑子页），并加入一次性模型胶囊长按提示与左滑轻提示；
  - 权限预设本地缓存消除设置页闪烁，收敛首页项目筛选点击热区至图标防止误触，Android 应用语言统一跟随系统语言；
  - 修复 iOS 根视图 `preferredColorScheme` 覆盖导致弹窗在「跟随系统」外观下样式重置的问题；
  - 新增 iOS 原生及 GitHub 文档日语（`ja`）本地化支持。
- 🛡️ **网关会话标题优先级与删除健壮性修复**：
  - 区分权威标题与首条 Prompt 临时兜底缓存，修复列表接口中旧兜底标题可能盖掉上游 AI `summary` 标题的问题，并支持后续自动升级；
  - 修复上游 `DeleteCascadeTrajectory` 因 `brain/<id>` 目录非空中断导致半删除僵尸会话的问题，自动清理空/残留目录后重试一次删除。
- 📦 **全端版本号对齐**：
  - Android 升至 `1.2.2`（Version Code `23`）；iOS 升至 `1.2.2`（Build `22`）；桌面/服务端网关 `mgy` 同步升至 `1.2.2`。

---

## 🇺🇸 English Release Notes

### ✨ Highlights & New Features

- 🧠 **Antigravity 2.34.0 Full-Stack Parity (Gemini 4 Neon & Orchestrator)**:
  - Updated the Language Server RPC baseline to `2.34.0` (378 RPCs verified);
  - Added full support across Go gateway, iOS, Android, and Web for **Gemini 4 Neon** (Low / Medium / High / Extra) and **Orchestrator (Neon)**, including model enums, compact pill labels (`Neon · High` / `Orch · Neon`), defaults, and Token history attribution.
- 🧩 **Complete MCP Apps Loop & Clickable Skill Pills**:
  - Transparently bridges `ui://` MCP App resources in the gateway (auto-resolving `serverName` and auto-launching via `LaunchMcpApp` on first load) and returns `tools/call` / `resources/read` results back to `<agent-embed>` across all clients;
  - Tapping a `/skill` pill in chat bubbles opens its `SKILL.md` directly in the existing Markdown viewer with zero extra visual clutter.
- ⚡ **Large-Repo Git Status Cache & Single-Gutter Diff View**:
  - Added short-TTL caching with concurrent request deduplication for `git status` (automatically invalidated on commit, push, checkout, and discard) for snappier refreshes in large repositories;
  - Upgraded diff viewers on iOS, Android, and Web with a compact single-column line-number gutter and color-coded change strips.
- ⏰ **Cross-Platform Automations Detail/Edit, Next-Run Alignment & Mobile Launch Fixes (iOS / Android / Web)**:
  - Added full Automations detail and editing views across all clients (schedule, prompt, model, workspace, and run history);
  - Fixed mobile automation start/toggle and "Run Now" reliability issues;
  - Computed `nextRunAt` directly from the Cron expression (including `CRON_TZ`/`TZ`) in the gateway to match Antigravity Desktop exactly, fixing the 1-hour offset during scheduler jitter windows and reflecting mobile schedule edits immediately.
- 🧭 **Tips & Gestures Guide & Mobile UX Polish (iOS / Android)**:
  - Added a dedicated Tips & Gestures Guide (integrated as a smooth slide-in Settings subpage on Android) along with subtle one-time discovery hints for model long-press and swipe actions;
  - Cached Agent permission presets locally to eliminate Settings flicker, narrowed workspace filter tap targets to the icon to avoid accidental taps, and aligned Android app language to follow the system locale;
  - Fixed an iOS issue where root `preferredColorScheme` caused sheets to reset appearance in Follow System mode;
  - Added native Japanese (`ja`) localization for the iOS app and GitHub README.
- 🛡️ **Gateway Title Priority & Resilient Cascade Deletion**:
  - Separated authoritative titles from temporary first-prompt fallbacks so cached prompt snippets never shadow upstream AI `summary` titles and upgrade automatically once ready;
  - Added automatic `brain/<id>` cleanup and one-shot retry when upstream `DeleteCascadeTrajectory` fails with `directory not empty`, preventing half-deleted zombie sessions.
- 📦 **Version Alignment**:
  - Android updated to `1.2.2` (Version Code `23`); iOS updated to `1.2.2` (Build `22`); desktop/server gateway `mgy` updated to `1.2.2`.
