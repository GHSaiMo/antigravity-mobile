# Multigravity 📱✨

<p align="center">
  <b>简体中文</b> &nbsp;|&nbsp; <a href="README_EN.md">English</a> &nbsp;|&nbsp; <a href="README_JA.md">日本語</a>
</p>

> **The Full-Stack Mobile & Remote Companion for Google Antigravity AI Agent.**  
> 随时随地，在 iPhone、iPad、Android 或手机浏览器上实时对话、审批决策、调度多智能体工作流，并掌握上下文水位与配额消耗。  
> 
> *代码仓库保持为 `antigravity-mobile`，产品对外正式品牌命名为 **Multigravity**，全局命令行工具为 **`mgy`**。*

[![Release](https://img.shields.io/github/v/release/GHSaiMo/antigravity-mobile?color=blue&style=flat-square&logo=github)](https://github.com/GHSaiMo/antigravity-mobile/releases/latest)
[![App Store](https://img.shields.io/badge/App%20Store-iOS%20Official-0D96F6?style=flat-square&logo=apple&logoColor=white)](https://apps.apple.com/app/id6835508934)
[![TestFlight](https://img.shields.io/badge/TestFlight-Public%20Beta-007AFF?style=flat-square&logo=apple&logoColor=white)](https://testflight.apple.com/join/YM7f4aSK)
[![Android](https://img.shields.io/badge/Android-APK%20Direct%20Install-3DDC84?style=flat-square&logo=android&logoColor=white)](https://github.com/GHSaiMo/antigravity-mobile/releases/latest)
[![Linux](https://img.shields.io/badge/Linux-x86__64%20%7C%20NAS-FCC624?style=flat-square&logo=linux&logoColor=black)](https://github.com/GHSaiMo/antigravity-mobile/releases/latest)
[![macOS](https://img.shields.io/badge/macOS-Apple%20Silicon%20%2B%20Intel-000000?style=flat-square&logo=apple&logoColor=white)](https://github.com/GHSaiMo/antigravity-mobile/releases/latest)
[![Windows](https://img.shields.io/badge/Windows-x86__64-0078D6?style=flat-square&logo=windows&logoColor=white)](https://github.com/GHSaiMo/antigravity-mobile/releases/latest)
[![Kotlin](https://img.shields.io/badge/Kotlin-1.9+-7F52FF?style=flat-square&logo=kotlin&logoColor=white)](https://kotlinlang.org)
[![Swift](https://img.shields.io/badge/Swift-6.0-F05138?style=flat-square&logo=swift&logoColor=white)](https://swift.org)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

<p align="center">
  <img src="images/hero.jpg" alt="Multigravity —— Google Antigravity 的原生移动伴侣" width="760" />
</p>

<p align="center">
  🌐 <b>官网</b>：<a href="https://mgy.jiuge.space">mgy.jiuge.space</a> &nbsp;·&nbsp; 🍎 <b>App Store</b>：<a href="https://apps.apple.com/app/id6835508934">iOS 正式版下载</a> &nbsp;·&nbsp; 🧪 <b>TestFlight</b>：<a href="https://testflight.apple.com/join/YM7f4aSK">公开测试通道</a> &nbsp;·&nbsp; 📦 <a href="https://github.com/GHSaiMo/antigravity-mobile/releases/latest">最新版 v1.2.4</a> &nbsp;·&nbsp; 💬 <a href="#wechat-group">微信用户群</a>
</p>

> 🎉 **反重力智能体移动伴侣 iOS 版现已正式上架 Apple App Store！**  
> 无需本地编译或自签，直接前往 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)**（支持美区等海外 174 地区，中文系统自动展示简体中文介绍与展示图，并内置零配置**离线演示模式**）；想抢先体验最新公测版也可加入 **[TestFlight 公开测试](https://testflight.apple.com/join/YM7f4aSK)**。

https://github.com/user-attachments/assets/37360cde-3875-45bf-947e-7774e800094c

---

## ⚡ 极速开始

### 1. 安装桌面 / 服务器网关 (`mgy`)

支持 **macOS (Apple Silicon / Intel)**、**Linux (x86_64 / NAS / 无头服务器)** 与 **Windows (x86_64)**，自动探测本地代理并在直连受阻时自动切换加速节点：

**macOS / Linux / Windows (Git Bash)：**
```bash
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
```

**Windows (PowerShell)：**
```powershell
irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
```

<details>
<summary>🌐 海外或已配置终端代理用户（GitHub 官方源）</summary>

```bash
# macOS / Linux / Git Bash
curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
```

```powershell
# Windows PowerShell
irm https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
```
</details>

### 2. 安装或打开移动端
- **iOS**：🎉 现已正式上架 Apple App Store，直接前往 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)** 一键安装（中文系统自动优先展示中文介绍与演示截图），也可通过 **[TestFlight 公开测试](https://testflight.apple.com/join/YM7f4aSK)** 抢先体验最新构建（如需自行源码编译调试，仍可参考 [iOS 本地构建与真机调试指南](docs/ios_build_guide.md)）。
- **Android**：前往 [GitHub Releases](https://github.com/GHSaiMo/antigravity-mobile/releases/latest) 直接下载安装 `Multigravity-v1.2.4.apk`（支持覆盖升级，请仅从官方 Releases 下载）。
- **PWA / Web**：无需安装 App，手机浏览器直接访问网关地址（或 `/?view=pwa`）即可添加到主屏幕。

### 3. 启动并扫码配对
```bash
mgy
```
网关前台启动后会在终端打印一次配对二维码（默认内置 Cloudflare Tunnel HTTPS 域名 + 局域网地址），手机打开 Multigravity 扫码即可秒连。

---

## 🖼️ 界面一览

<table>
  <tr>
    <td align="center" valign="top" width="25%"><img src="images/screen_list.jpg" alt="会话列表与后台任务" /><br/><b>全局视野</b><br/><sub>状态徽标、后台任务胶囊与额度均速刻度</sub></td>
    <td align="center" valign="top" width="25%"><img src="images/screen_question.jpg" alt="多题交互决策" /><br/><b>一触即决</b><br/><sub>上下文水位速率可见，多题点选一次提交</sub></td>
    <td align="center" valign="top" width="25%"><img src="images/screen_plan.jpg" alt="方案预览与 Proceed" /><br/><b>方案放行</b><br/><sub>Mermaid 流程图与方案同屏预览，一键 Proceed</sub></td>
    <td align="center" valign="top" width="25%"><img src="images/screen_pdf.jpg" alt="交付物预览" /><br/><b>交付物直接看</b><br/><sub>销售表格、原生柱状图与 PDF 产物直接览</sub></td>
  </tr>
  <tr>
    <td align="center" valign="top" width="25%"><img src="images/screen_workflow.jpg" alt="动态多智能体工作流" /><br/><b>多智能体工作流</b><br/><sub>阶段流水线、并发子代理与日志实时穿透</sub></td>
    <td align="center" valign="top" width="25%"><img src="images/screen_approval.jpg" alt="Workflow 脚本安全审批" /><br/><b>安全审批门禁</b><br/><sub>精准拦截编排脚本，支持源码预览与细粒度放行</sub></td>
    <td align="center" valign="top" width="25%"><img src="images/screen_quota.jpg" alt="账户与配额看板" /><br/><b>账户与配额</b><br/><sub>5h / 每周额度均速刻度、Token 历史与热切账号</sub></td>
    <td align="center" valign="top" width="25%"><img src="images/screen_tasks.jpg" alt="工具与任务看板" /><br/><b>工具与任务看板</b><br/><sub>MCP、技能、插件、定时任务与守护进程全掌控</sub></td>
  </tr>
</table>

> 以上截图来自客户端内置**演示模式**（v1.2.0 最新中文界面），未配对时即可直接体验。

---

## ✨ 重点特性

### 1. 全端一致的实时流式体验
- **三端全覆盖与原生三语自适应**：原生 **iOS**（SwiftUI / Swift 6）、原生 **Android**（Jetpack Compose Material 3）与零构建嵌入式 **PWA Web**，全端支持**简体中文 / English / 日本語**自动识别与切换，提供「跟随系统 / 浅色 / 深色」原生外观与独立「精简模式」二级降噪开关。
- **低延迟增量推流与断点续传**：响应式 Connect 流 Hub + WebSocket `sinceRev` 增量断点续传、长思考静默期心跳保活与内容哈希校验，配合增量 Markdown LRU 缓存、步骤导轨、长对话目录（TOC）及稳态立即发送，超长会话依然丝滑。
- **专业级富媒体排版与交互**：180+ 语言离线语法高亮（含 `R / R Markdown`）、单列行号槽 + 词级行内代码 Diff、LaTeX 公式、Mermaid 容错渲染、**MCP Apps (`ui://`) 交互式组件桥接**、**文档页内查找（Find in Page）**及高密度 PDF 懒加载预览。

### 2. 关键决策、分级权限与模型协同
- **原生交互卡片、分级权限与工作区信任**：完整支持多题决策（`askQuestion`）、高危命令审批、三档权限模式（本地缓存防闪烁），以及未信任工作区告警横幅一键授信。
- **全面对齐 Antigravity 2.34.0 模型代际**：原生支持 **Gemini 4 Neon**（含 Extra 思考档）、**Orchestrator (Neon)**、Hydrogen / Carbon / Claude 5.5 全系模型；支持设置首选/次选模型并同步桌面最新选择，短按模型胶囊快切、长按任选全部模型。
- **方案放行、`/btw` 旁问与撤回**：支持预览方案一键 `Proceed` 推进、对话 Fork/Worktree 分叉、代码回滚与排队追问；支持从输入框或气泡发起独立 **`/btw` 旁问**并一键回插主对话；点击对话内 `/skill` 徽章可直达 `SKILL.md` 定义预览。

### 3. 多智能体工作流、蜂群看板、终端与自动化
- **动态工作流全生命周期 (`run_workflow`)**：内置常用模板快捷发起，精准拦截 Python 编排脚本并弹出安全审批卡，实时展示阶段流水线、并发子智能体网格、终端日志与 JSON 产物；单子代理运行时点击浮条即可一步直达详情。
- **蜂群看板与实时终端**：桌面级复刻蜂群协作板（`/swarm`），原生接入 PTY 终端 WebSocket 实时交互流、MCP Server 开关热刷新、Agent Memories 增删改查与浏览器子智能体跳过/加白控制。
- **定时任务与后台任务中心**：三端支持创建、详情查看、完整编辑与启停 Automations（含离线本地扫描双保险与「立即运行 Run Now」），跨会话监控全局后台任务并实时查看/刷新终端日志 Tail。

### 4. 全双工实时语音与多模态交互
- **实时语音通话、听写与朗读**：四端支持 Antigravity 全双工实时语音协议（iOS 启用硬件级回声消除 AEC），支持原生流式语音听写（Dictation）、回答语音朗读（Narration）、实时字幕、打断与倍速播放。
- **多模态输入与产出**：支持 `@` 上下文提及、微信式相册切换器与跨相册全量多选、音视频（`≤200MB`，含 `.amr` 语音）与常规文件（`≤50MB`）环形进度流式上传、**微信聊天记录 ZIP 智能识别、离线归档缓存与原生排版预览**、双端统一长按自由选词浮层、iOS 文稿扫描、AI 生图与 7 款精选皮肤（含 Solarized Light、Nord 等桌面配色）长图海报分享。

### 5. 上下文水位、速率监控与配额仪表盘
- **实时上下文水位与速率**：聊天顶栏实时展示上下文占用圆环与 `TTFT / TPS` 吐字速率，遇到上下文溢出自动升级为带「一键压缩 `/compact`」和「开启新会话」的救援卡片。
- **配额匀速刻度与可滑动 Token 三视图图表**：实时监控 Claude / Gemini（5h & Weekly）及 Argon 额度，叠加均速消耗刻度针与节奏分析；内置可左右滑动的 **Token 历史三视图轮播**（近 14 天每日柱状图、模型累计占比与 20 周热力图，含智能冷启动默认与偏好记忆）、成本统计与所见即所得多皮肤报告海报一键保存/分享，配合 **Cockpit Tools v1.4+ SQLite 索引**与 **ULTRA 优先分级排序**支持多账号无感热切。
- **全尺寸额度与活跃对话小组件 & CarPlay 实时步骤（iOS）**：内置全套尺寸锁屏与桌面额度小组件（`Inline` / `Circular` / `Rectangular` / `Small`～`ExtraLarge`）及 **`AgentStatusWidget` 活跃对话监控小组件**（`Small`～`Large`），覆盖 iPhone、iPad、StandBy、iOS 26 CarPlay 与 Mac 桌面，支持 5h/7d 双窗走秒倒计时、一键轻刷与多会话审批总览；CarPlay 仪表盘与 Apple Watch 智能叠放同步支持两段式高对比实时步骤卡片。

### 6. 移动端工程闭环与外设生态
- **代码与工作区管理直达**：支持首页按项目 / Chat 原地筛选、新建本地工作区、克隆 GitHub 仓库、会话全文秒搜（SQLite FTS5）、导出 Markdown、查看 `Changes` Diff、带短时缓存提速的原生 `Commit & Push` 与分支管理；会话列表与气泡自动关联 **GitHub PR 实时状态胶囊与详情弹窗**，同时支持 `/slash` 斜杠命令、交互提示与手势指南及 MCP/技能健康看板。
- **跨端游标与物理外设**：Follow-Me 游标引擎智能仲裁桌面与移动端活跃会话；支持 `integrations/yoooclaw/` 随身硬件按键语音直连注入活跃会话。

### 7. 零配置公网穿透、智能选路与安全推送
- **多通道并发测速选路**：同时管理 Cloudflare Tunnel 专属 HTTPS 域名、局域网 Wi-Fi 与 Tailscale，一键并发探活并自动切换最低延迟链路。
- **分级消息通知与灵动岛**：iOS 原生支持 **APNs 推送与远程 Live Activity 灵动岛状态更新**（经无状态 Cloudflare Relay 转发，开箱即用）；Android 支持 `dataSync` 前台保活同步与 FCM 推送，在任务完成、等待审批、工作流节点及低额度预警时即时唤醒。
- **本地优先安全架构**：纯本地直连，无第三方云端中转；设备凭据存入系统 Keychain / EncryptedSharedPreferences，集成敏感路径黑名单与 WS 限流防刷。

---

## 🏛️ 架构概览

```text
手机端 (iOS / Android / PWA) ──[LAN / Tailscale / Cloudflare HTTPS]──▶ mgy Go 网关 ──▶ Antigravity Core (language_server)
```

| 层级 | 核心组件 | 说明 |
| :--- | :--- | :--- |
| **移动客户端** | iOS (SwiftUI) · Android (Compose) · PWA (`web/`) | 完整交互体验、语音通话、工作流看板、额度与 Token 审计 |
| **本地网关 (`mgy`)** | Go 单一二进制 (`cmd/gateway` / `internal/`) | 实例自愈发现、WS/ConnectRPC 代理、FTS5 检索、配额与通知引擎 |
| **远程与外设** | Cloudflare Tunnel · 智能选路 · YoooClaw | 零配置公网 HTTPS 穿透、多链路并发测速、硬件语音网桥 |

---

## 🛠️ 常用 CLI 与配置

```bash
mgy                # 前台启动网关并打印配对二维码
mgy pair           # 申请新的 5 分钟有效配对二维码
mgy cockpit        # 配置 Cockpit 报表服务与 Token
mgy cloudflare     # 配置 Cloudflare 穿透隧道 (别名: mgy cf)
mgy list           # 查看已配对设备
mgy clear all      # 吊销所有已配对设备 (或 mgy clear <dev_id>)
mgy help           # 查看完整命令与启动参数
```

- **默认配置目录**：`~/.multigravity/.env`（自动兼容迁移旧版 `~/.antigravity-mobile/`）。
- **核心环境变量**：`MULTIGRAVITY_PORT` (`58900`)、`CF_TUNNEL_ENABLED` (`1`)、`APNS_ENABLED` (`1`)、`MGY_LANG` (`zh` / `en` 自适应)、`MULTIGRAVITY_LOG_LEVEL` (`info`)。
- **进阶文档**：安装自测与排错详见 [安装与运维指南](docs/installation_and_testing_guide.md)，RPC 接入细节详见 [language_server RPC 说明](docs/ls_rpc_integration_notes.md)。

<details>
<summary>🗑️ 彻底卸载网关与清理配置</summary>

```bash
# macOS / Linux / Git Bash
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/uninstall.sh | bash
```

```powershell
# Windows PowerShell
irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/uninstall.ps1 | iex
```
*(支持 `--keep-config` 保留配置或 `--keep-cf` 保留 Cloudflare 引擎)*
</details>

---

## 💻 源码编译

```bash
make build          # 编译 bin/mgy
make run            # 本地测试运行
make install-local  # 安装到 ~/.local/bin/mgy
make test           # 运行 Go 单元与集成测试
```

---

<a id="wechat-group"></a>
## 💬 微信用户群

欢迎扫码加入 **mgy 微信用户群**，交流使用心得、反馈建议或第一时间获取版本更新动态（二维码 7 天内有效，若过期请提交 [Issue](https://github.com/GHSaiMo/antigravity-mobile/issues) 提醒更新）：

<p align="center">
  <img src="images/wechat_group.jpg" alt="mgy 微信用户群二维码" width="260" />
</p>

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。欢迎提交 Issue 与 Pull Request！
