# Antigravity Mobile 📱✨

> **The Full-Stack Mobile Companion for Google Antigravity AI Agent.**  
> 随时随地，在 iPhone、iPad、Android 或任意移动设备上自如操控、对话、监控并指挥运行在 Mac 主机上的 Antigravity 智能体。

[![Swift](https://img.shields.io/badge/Swift-6.0-F05138?style=flat-square&logo=swift&logoColor=white)](https://swift.org)
[![Kotlin](https://img.shields.io/badge/Kotlin-1.9+-7F52FF?style=flat-square&logo=kotlin&logoColor=white)](https://kotlinlang.org)
[![Android](https://img.shields.io/badge/Android-8.0+-3DDC84?style=flat-square&logo=android&logoColor=white)](https://developer.android.com)
[![iOS](https://img.shields.io/badge/iOS-17.0+-000000?style=flat-square&logo=apple&logoColor=white)](https://developer.apple.com/ios/)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-macOS%20%7C%20iOS%20%7C%20Android%20%7C%20Web-blue?style=flat-square)]()
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

<p align="center">
  <img src="images/session_list.jpg" alt="Antigravity Mobile 会话列表与分类信号" width="340" />
  &nbsp;&nbsp;&nbsp;&nbsp;
  <img src="images/native_components.jpg" alt="Antigravity Mobile 原生交互组件、后台任务与指令队列" width="340" />
</p>

---

## 💡 为什么需要 Antigravity Mobile？

**Google Antigravity** 是新一代高自主性 AI 编码与工程 Agent。但在日常工程实践中：
- 复杂任务（如跨仓库重构、端到端测试、深度架构推导）往往需要 Agent 自主运行数十分钟甚至数小时；
- 离开电脑桌（通勤途中、用餐或会议中）无法随时跟进 Agent 思考进度与工具执行结果；
- 当 Agent 需要用户确认（Prompt Feedback / 方案决策 / 命令审批）时，桌面端若无人值守，整个流水线便会陷入停滞。

**Antigravity Mobile** 为解决这一痛点而生：它不仅是一个透明代理网关，更是一套**完整的全栈端到端移动与外设协同系统**——涵盖原生 iOS App、全面功能对齐的 Android 原生 App、极简自嵌入 PWA Web 客户端、随身外设硬件联动网桥，以及具备进程自愈、端到端 TLS 域名中继和跨端活跃游标感知的 Go 本地服务。

---

## 🏛️ 全栈架构设计

| 层级 | 核心组件 | 关键职责与技术特性 |
| :--- | :--- | :--- |
| **📱 移动访问层** | **iOS 原生客户端** (SwiftUI 5) | Swift 6 严格并发、单趟 O(N) LaTeX 渲染、Markdown 富媒体图片渲染与 `ImageViewerSheet` 全手势大图缩放、VS Code 文件图标、Markdown 浮窗预览与伴生摘要、实施方案 Proceed 推进闭环、Gemini 3.8 / Claude 4.6 模型切换胶囊、多模态图片上传、排队追问队列与自适应输入、后台任务实时管控与终止、交互式命令审批卡片、全态分类信号 (`RUNNING` / `ERROR` / `ACTION` / 未读呼吸小蓝点)、左滑删除与长按重命名、0ms 会话焦点上报 (`POST /gateway/cascade/focus`)、灵动岛 (Live Activity) |
| | **Android 原生客户端** (Compose) | **与 iOS 完全功能对齐 (Full Feature Parity)**：Kotlin 1.9+、Jetpack Compose Material 3 纯黑美学、OkHttp + WebSocket 实时打字机流分发、动态 QuotaStatusBar 额度条、Claude/Gemini 5h & Weekly 多账号配额抽屉、新建会话抽屉（工作区选择 / Pure Chat 模式）、Markdown 伴生浮窗与 Proceed 闭环、多步工具折叠胶囊、交互式审批卡片、排队指令管理面板、后台任务感知与终止、工程快捷动作胶囊、ZXing 离线二维码秒级配对、`EncryptedSharedPreferences` 硬件密钥持久化、全态分类信号 |
| | **移动端 PWA / Web** (Vanilla JS) | 零构建打包、嵌入 Go 二进制 (`embed.FS`)、全面对齐 iOS 原生设计系统与 NavigationStack 导航、列表滚动/侧滑手势消抖（防误触进入）、居中对称标题与 38px 悬浮垃圾桶删除、自适应安全区与键盘防遮挡、Markdown 浮窗与方案 Proceed 推进、排队消息与后台任务同步、添加到主屏幕 |
| **🎯 跨端游标与焦点层** | **随人而动游标引擎 (Follow-Me Cursor Engine)** | 毫秒级多端焦点仲裁：活跃长连接流 (Active Stream) > 移动端黏性焦点 (Mobile Sticky 30m) > 桌面端 IDE 活跃焦点 (Desktop Focus) > 磁盘最后活跃会话兜底；提供 0ms 会话焦点上报与预热；防自反保护机制 (Anti-Reflection 1.5s 抑制期) 彻底消除已读回环误判；幽灵会话三重防御过滤 |
| **🦞 物理外设网桥层** | **YoooClaw 物理硬件网桥 (`integrations/yoooclaw/`)** | 随身外设按键录音 ➔ ASR ➔ Gateway-First 代理直连注入活跃 Cascade 会话；双层协同分流（第一层 Hermes 业务守卫放行，第二层统一游标精准定位目标会话）；四色交织 RGB 流光动效与 OLED 屏幕状态回显 |
| **⚡ 远程连接与鉴权层** | **HTTPS 域名中继与端到端 TLS 1.3** | 支持公网域名直连或通过 FRP 隧道穿透配合 Let's Encrypt 证书 (`scripts/issue-agy-tls.sh`)，实现无公网 IPv4 下的域名安全中继；配对二维码优先携带安全 HTTPS 链接 (`ssl=1`)，Mac 本地终止解密，VPS 仅透明转发 TCP 密文，手机端无缝绕过 iOS ATS 拦截并启用 HSTS |
| | **IPv6 双栈直连 (Dual-Stack Direct)** | 网关默认监听 IPv4/IPv6 全网卡，公网 IPv6 / DDNS 直连免中继，极低延迟，客户端蜂窝网络 (Cellular) 智能优先路由 |
| | **二维码扫码配对 (QR Pairing)** | 终端或脚本自动生成一次性 `agy://pair` 配对二维码，扫码秒级签发独占 Device Token，存入系统安全存储 (Keychain / EncryptedSharedPreferences)，与 IP 完全解耦 |
| | **Tailscale / 私有 Mesh VPN (备选)** | 点对点加密 WireGuard 网络，无公网 IP 时安全组网互联 |
| **🖥️ 本地网关层** | **自愈实例探测器 (Inspector)** | 自动嗅探 `language_server` 进程、实时捕获动态端口与鉴权令牌、进程重启零感知毫秒级自愈；离线状态具备指数退避 (5s→10s→20s→40s) 防 CPU 空转 |
| | **ConnectRPC & WebSocket 代理** | 双向流式转发与长连接保活、自动注入 `x-codeium-csrf-token`、免二次编码大图透传优化 (`needsModification`)、单 IP 60次/分 WS Ticket 限流加固、安全沙箱文件代理 (`/api/v1/files/content`) 与 Brain 伴生元数据解析、内置提供 Web 静态资产与排队追问代理 |
| | **Cockpit 配额引擎 (Cockpit Engine)** | 实时提取多账号配额数据、支持双模型 5h/Weekly 四象限监控、一键切号与邮箱脱敏遮罩 |
| | **Bark 实时推送守护 (Notification Watcher)** | 后台持续监听 Agent 状态，任务完成/失败/审批拦截/提问/Proceed 自动触发 Bark 实时推送与 DeepLink 唤醒 |
| **⚙️ 核心引擎层** | **Antigravity Core** | `language_server` 核心智能体进程，运行于 Mac 本地回环 |

---

## ✨ 核心能力与功能特性

### 1. 📱 iOS 原生客户端 (`ios/`)
- **现代化架构**：基于 SwiftUI 5 与 **Swift 6 严格并发模式**（Strict Concurrency Checking）构建，零数据竞态、流畅丝滑。
- **Cockpit 额度监控与多账号看板**：
  - **首页 5h 额度状态条**：实时直观显示当前活跃模型（Gemini）5 小时额度百分比、全宽动态进度条、剩余重置倒计时与具体时钟点（如 `⚡ Gemini 5h 98% 4h 16m (09/12 15:57)`）；
  - **Cockpit Tools 配额抽屉**：展开后一览 Claude / Gemini 5h 与每周四项额度指标；支持多账号一键热切换与即时生效校准；内置邮箱打码遮罩（Masking）保护隐私，支持 5 分钟后台自动静默刷新与手动即时刷新。
- **智能会话状态与多维分类信号 (Multi-State Classification)**：
  - **全态可视化徽标与状态分流**：
    - `RUNNING`（绿色徽标）：Agent 正在高负荷思考、多步推理或并发调用工具链；
    - `ERROR`（红色徽标）：会话执行异常或中断报错（如模型身份限制、命令异常），醒目警示以便第一时间定位排查；
    - `ACTION`（蓝色徽标）：等待用户关键交互或决策（终端命令审批、敏感文件读写授权、澄清提问、以及实施方案 Proceed 推进）；
    - `未读小蓝点`：双层同心圆呼吸微光小蓝点，即时提示有新完成但未阅的会话；
  - **多维上下文元信息**：每个会话卡片清晰标注对应工作区目录（Workspace Folder）、累计步骤统计（如 `24 步骤`、`1,290 步骤`）以及自适应相对时间（`刚刚`、`1分钟前`、`21小时前`、`3天前`）；
  - **即时模糊搜索**：底部常驻搜索框（“搜索会话或工作区...”），支持按会话标题或所属工作区秒级模糊过滤。
- **双模型切换与多模态图片上传**：
  - **模型一键切换**：聊天输入栏内置 **Gemini 3.8** / **Claude 4.6** 模型快速切换胶囊，按需调用最适合的模型；
  - **多模态图片发送**：原生集成系统照片库与相机，一键上传报错截图、设计图纸或架构草图，客户端自适应轻量压缩，直接给 Agent 多模态视觉输入。
- **富媒体 Markdown 图片渲染与全屏交互闭环 (`ImageViewerSheet`)**：
  - **会话与文档内嵌渲染**：对话消息气泡与 Markdown 预览界面全面支持图片渲染，自动识别相对路径、本地绝对路径与网关文件路由；
  - **手势级大图查看器**：点击任意图片一键唤起 `ImageViewerSheet`，支持双击缩放、双指捏合无级缩放、下拉拖拽顺滑退出、保存到系统相册与安全微信分享；
  - **表格与富文本优化**：优化 Markdown 表格自动列宽计算与防塌陷，支持单趟 O(N) 流式 LaTeX 数学公式解析引擎（覆盖 150+ 符号、分数、根号与上下标转换）。
- **Markdown 产物浮窗与实施方案推进闭环 (Plan Proceed Sheet)**：
  - **点击即开原生浮窗**：在对话中点击任意 Markdown 路径、实施方案链接（`implementation_plan.md`、`walkthrough.md` 或 `/brain/` 路径），立即弹出手势驱动的沉浸式 Sheet 浮窗；
  - **伴生摘要与元信息逆向 (Companion Metadata)**：深度适配桌面端 `~/.gemini/antigravity/brain/` 产物结构，自动逆向解析伴生 `.metadata.json`，提取多行高阶方案摘要（Summary）与交互意图；
  - **浮窗底部常驻 Proceed 按钮**：实施方案就绪时，浮窗底部常驻高亮「确认执行 (Proceed)」蓝色操作条，手机上一键确认进入代码实施阶段；
  - **手势交互与全文复制**：支持下拉手势顺畅退出、代码块与富文本自适应排版、错误重试与文本一键复制。
- **追问排队队列与自适应输入 (Queued Messages & Dynamic Input)**：
  - **运行态自适应输入框**：当 Agent 处于长时间多步推理或耗时工具调用中时，输入栏占位符智能自适应切换为「向队列添加指令...」，发送按钮平滑变形为红色停止中止按钮；
  - **可视化排队管理面板**：全宽中文化卡片式排队面板，清晰展示待消费指令序列，支持指令查看、单条一键删除、优先级快速调整与随时编辑；
  - **顺序自动消费**：Agent 结束当前轮次后，队列自动按序拉取并无缝发送给底层智能体执行。
- **后台常驻任务实时监控与精准终止 (Active Background Task Management)**：
  - **实时后台任务感知**：实时感知 Agent 在后台执行的守护进程、后台测试或 Git 远端推送等耗时命令；
  - **任务卡片与精准杀停**：卡片式展示任务类型、状态与执行命令代码块，提供红色「终止」按钮，失控或超时任务在手机上一键精准 kill。
- **工程快捷动作胶囊 (Quick Action Chips)**：
  - 输入栏上方常驻快捷动作胶囊（如 `Commit and Push` 等工程高频动作），单触即可快速触发，大幅降低手机端键盘打字负担。
- **0ms 跨端会话焦点上报**：
  - 手机端进入会话卡片瞬刻通过 `NetworkTransport` 发送 `POST /gateway/cascade/focus`，向网关上报焦点并预热缓存，与外设及桌面端无缝协同。
- **会话管理细节优化**：
  - 原生 **左滑删除**：内嵌红色居中垃圾桶图标与防误触确认弹窗，消除列表跳动；
  - 原生 **长按一步重命名**：长按卡片即刻弹出重命名输入框；
  - 全链路子代理会话过滤，自动屏蔽 Subagent 内部杂音。
- **扫码秒级配对与安全认证**：
  - 内置扫码器直接扫描终端配对二维码，一键获取独占 Device Token 并保存至系统 Keychain；
  - 优先走蜂窝网络直连 (Cellular Priority)，无缝降级 Wi-Fi / VPN / HTTPS 域名中继。

---

### 2. 🤖 Android 原生客户端 (`android/`)
> **与 iOS 客户端实现完全功能对齐 (Full Feature Parity)**，采用现代 Android 顶级架构规范。

- **现代化技术栈**：基于 **Kotlin 1.9+**、**Jetpack Compose Material 3** 暗黑美学设计系统，使用 **Coroutines + Flow** 驱动流式状态，完美适配全面屏手势与高刷显示。
- **首页配额状态条 (`QuotaStatusBar`) 与配额抽屉 (`AccountQuotaSheet`)**：
  - 首页顶部常驻动态配额条，实时计算 Gemini 5h 剩余额度百分比、彩色动态进度条与重置倒计时；
  - 点击弹出全功能配额抽屉：实时呈现 Claude 3.5 / Gemini 5h 与每周四象限配额看板，支持多账号一键热切换与邮箱脱敏。
- **新建会话抽屉 (`NewConversationSheet`)**：
  - 支持工作区模式切换：一键选择指定工程目录开启任务，或开启「纯对话模式 (Pure Chat)」，满足轻量化技术咨询。
- **全态分类信号与上下文标注**：
  - 列表呈现 `RUNNING`（绿色）、`ERROR`（红色）、`ACTION`（蓝色）状态徽标与呼吸微光未读小蓝点；
  - 显示归属工程工作区、累计步骤数与自适应人性化相对时间（刚刚、几分钟前、昨天等）。
- **实时打字机流式长连接与多步工具折叠 (`ToolStepCollapseCard`)**：
  - 基于 **OkHttp 4.12 + WebSocket** 构建健壮长连接流，支持自动重连与离线状态恢复；
  - 自动折叠长串工具调用为轻量胶囊条，标明执行项数与工具类型（如 `⚡ 已思考并执行 12 项操作`），杜绝刷屏卡顿。
- **交互式命令审批与实施方案推进 (Action & Proceed Cards)**：
  - 渲染危险终端命令执行审批卡片，手机端一键执行「批准 (Approve)」或「拒绝 (Reject)」；
  - 方案就绪时呈现高亮 **Proceed** 推进卡片，快速推动智能体进入下一阶段。
- **方案产物原生浮窗 (`MarkdownViewerSheet`)**：
  - 点击会话内 Markdown 链接即刻展开原生浮窗，逆向解析 Brain 伴生元数据摘要，底部常驻 Proceed 一键推进闭环。
- **排队指令管理面板 (`QueuedMessagesCard`)**：
  - Agent 忙碌时自动切换为排队输入状态；排队面板支持查看待发送序列、单条移除、插队立即发送与文本修改。
- **后台常驻任务实时监控 (`RunningTasksCard`)**：
  - 实时感知后台执行的长耗时命令，支持一键发送 Kill 终止信号。
- **工程快捷动作胶囊 (`QuickActionChips`)**：
  - 快捷切换 Gemini 3.8 / Claude 4.6 模型、一键附加图片、快捷触发 Git 提交等指令。
- **离线极速扫码配对与硬件级加密存储**：
  - 原生内嵌 **ZXing 离线二维码扫描器**（无需 Google Play Services，纯离线快速扫码）；
  - 自动解析 `agy://pair` URI，设备凭证安全存入 Android Keystore 保护的 `EncryptedSharedPreferences`。

---

### 3. 🎯 跨端活跃会话游标与“随人而动”焦点引擎 (Unified Active Session Cursor)
- **Follow-Me 动态多端焦点仲裁**：
  - 外设、手机与桌面 IDE 协同调度，按优先级毫秒级仲裁当前用户视线聚焦的活跃 Cascade 会话：
    $$\text{Active Stream (活跃长连接)} > \text{Mobile Sticky (移动端聚焦 30min)} > \text{Desktop Focus (桌面 IDE 切换)} > \text{Disk Fallback (磁盘最后活动)}$$
- **0ms 会话焦点上报 (`POST /gateway/cascade/focus`)**：
  - 移动端进入会话卡片瞬刻无感知上报焦点，网关提前预热会话缓存，降低首字延迟。
- **防自反保护机制 (Anti-Reflection Protection, 1.5s 抑制期)**：
  - 彻底杜绝移动端标记已读触发磁盘更新、进而被文件监听器误判为“桌面鼠标点击”的死循环陷阱。
- **桌面 IDE 焦点实时嗅探 (`StartDesktopFocusWatcher`)**：
  - 毫秒级感知开发者在 Mac 桌面 Antigravity 界面切换的标签页，实现手机与桌面无缝接力。
- **幽灵会话三重防御过滤**：
  - 严格校验标题非空与非默认、工作区目录物理存在性、自动过滤 Subagent 内部通信会话，彻底杜绝空白卡片与 500 报错。

---

### 4. 🦞 物理外设硬件联动与全链路语音注入 (YoooClaw ✕ Antigravity)
- **随身硬件一键直达**：通过随身携带的 YoooClaw 物理外设按键录音，云端 ASR 识别后由网桥直接注入正在运行的 Antigravity 智能体。
- **双层协同分流架构**：
  - **第一层（Session Boundary Guard）**：在消息入站口识别用户意图，普通对话无缝放行给日常生活助手 Hermes，识别到反重力意图时精准拦截；
  - **第二层（Follow-Me Cursor Engine）**：由统一游标引擎毫秒级判定，将语音文本准确注入到用户当前正在查看的代码会话中，杜绝串会话。
- **硬件声光电交互回显**：
  - 切换模式或指令注入成功时，硬件外设即刻激发 RGB LED 四色环形交织流光，并在 OLED 屏幕弹出“已切换到反重力模式”及目标会话标题。

---

### 5. 🌐 嵌入式 Mobile Web & PWA (`web/`)
- **零构建（Zero-Build）**：极简现代原生 JavaScript + CSS，无需 Node.js 或前端打包工具链；利用 Go `embed.FS` 编译进单个二进制。
- **手势消抖机制（防滚动误触）**：精确区分手指纵向滚动 (Scroll)、横向侧滑 (Swipe) 与轻触点击 (Tap)，消除在翻阅长会话列表时不慎误触进入会话的痛点。
- **完全对齐 iOS 原生设计系统**：
  - 对齐 iOS `NavigationStack` 居中对称标题设计，精简返回 Chevron 按钮；
  - 全面对齐 iOS 原生 Sheet：`NewConversationSheet`（工作区与纯对话模式切换）与 `SettingsSheet`（Inset Grouped 分组与安全区对齐）；
  - 侧滑删除按钮重构为 38px 悬浮圆形垃圾桶图标，消除滑动闪烁；
  - 优化 Markdown 表格自动列宽计算与防塌陷，支持移动端自适应排版。
- **PWA 沉浸体验**：支持 iOS Safari「添加到主屏幕」，独立图标、全屏沉浸运行、Service Worker 静态离线缓存支持。

---

### 6. 🔒 强化安全架构与高性能代理引擎
- **HTTPS 域名中继与端到端 TLS 1.3**：
  - 提供自动化脚本 `scripts/issue-agy-tls.sh`（通过 Cloudflare DNS-01 自动化申请并续期 Let's Encrypt 证书）；
  - 支持通过公网域名或 FRP TCP 隧道安全穿透，全链路强制 TLS 1.3 传输；
  - 证书私钥仅存储于 Mac 本地并由本地网关解密，VPS 云服务器仅作为 TCP 转发中继，看不到任何明文内容；
  - 配对二维码自动携带 HTTPS 链接并开启 `ssl=1`，iOS 客户端自动免去 ATS 拦截。
- **全面安全加固 (Security Audit Hardening)**：
  - **请求体硬限制**：所有关键 RPC 与交互接口引入 `io.LimitReader`（1MB / 4KB），有效防御恶意大包导致的内存耗尽 (OOM)；
  - **频率限制**：WebSocket Ticket 接口配置单 IP 60 次/分钟限流；
  - **沙箱路径隔离**：文件下载与预览接口引入白名单与正规化正则校验，禁止跨目录逃逸；敏感会话注解文件权限严格收缩至 `0600`；
  - **本地自签 TLS (`localtls`)**：上游与 Antigravity `language_server` 通信全程采用自签 TLS 客户端连接。
- **深度性能优化 (Performance Audit Tuning)**：
  - **大图透传免二次编码 (`needsModification`)**：对无需模型改写的大型图片上行请求，跳过 JSON 反序列化与二次重构，开销与延迟降低 80%+；
  - **内存防膨胀与 LRU 淘汰**：会话墓碑缓存设置 1024 上限与 10 分钟过期淘汰机制；
  - **进程探查指数退避**：当 Antigravity 未启动时，扫描探针自动进入指数退避（5s→10s→20s→40s），避免高频调用 `ps` 和 `lsof` 消耗 CPU。

---

### 7. 🔔 实时通知推送与 DeepLink (Bark 集成)
- **全自动化智能推送**：无需保持 App 前台，网关后台 Watcher 持续巡检：
  - 任务顺利完成推送（`🎉 Antigravity 任务已完成`，附带步骤数统计）；
  - 异常报错中断推送（`❌ Antigravity 任务执行失败`）；
  - 关键审批提醒推送（`⚠️ Antigravity 需要审批`：命令执行、文件改动等）；
  - 方案就绪推送（`📋 方案已就绪，等待确认`，提示点击 Proceed）；
  - 追问与提问推送（`❓ Antigravity 提问`）。
- **DeepLink 毫秒直达**：点击 iOS 锁屏/横幅通知直接打开对应会话（`antigravity://session/<id>`），立刻审批或继续对话。
- **开箱即用**：只需在 `.env` 中配置 `BARK_URL` 即可立即激活，支持自定义警报提示音（如 alarm、glass）与通知分组。

---

## 📂 项目工程目录

```text
antigravity-mobile/
├── Makefile                    # 统一构建与运维脚本 (build, run, test, tmux, 配对管理)
├── README.md                   # 全栈开源项目核心文档
├── env.example                 # 环境配置模板 (Bark 推送、TLS、端口、FRP 等)
├── images/                     # 项目文档截图与预览图
│   ├── session_list.jpg        # 移动端主界面与多维分类信号/配额监控预览图
│   └── native_components.jpg   # 原生会话交互、后台任务与排队指令预览图
├── go.mod / go.sum             # Go 依赖描述 (Go 1.22+)
├── .github/workflows/ci.yml    # CI/CD 自动化工作流
├── cmd/
│   └── gateway/
│       └── main.go             # 网关服务入口 (自愈巡检 + 路由 + RPC 反代 + 游标管理)
├── internal/
│   ├── auth/                   # 设备认证、扫码配对、Token 管理、Ticket 限流
│   ├── cockpit/                # Cockpit 配额监控、多账号切换与数据解析
│   ├── config/                 # .env 配置加载与通知/网络参数解析
│   ├── inspector/              # Antigravity 本地进程、动态端口嗅探与指数退避探查器
│   ├── localtls/               # 针对 language_server 本地自签 TLS 统一通信客户端
│   ├── notifier/               # Bark 实时推送、DeepLink、后台任务巡检器 (Watcher)
│   ├── proxy/                  # ConnectRPC、WebSocket、游标仲裁、排队追问与后台任务代理核心
│   └── tunnel/                 # FRP 穿透与云端中继管理器
├── web/                        # 移动 Web / PWA 前端资源 (由 Go embed 打包)
│   ├── index.html              # PWA 主入口
│   ├── app.js                  # 响应式交互、手势消抖、Markdown/LaTeX 渲染与状态机
│   ├── style.css               # 移动端现代设计系统与流体排版
│   ├── manifest.json           # Web App 清单 (添加到主屏幕)
│   ├── sw.js                   # 离线 Service Worker 缓存脚本
│   └── icons/                  # 应用图标与 180+ 编程文件类型图标
├── ios/                        # iOS 原生客户端工程 (Xcode 16 / SwiftUI 5 / Swift 6)
│   ├── Antigravity.xcodeproj   # Xcode 工程配置 (包含主 App 与 ActivityExtension Target)
│   ├── Info.plist              # 权限声明与配置
│   ├── ActivityExtension/      # 灵动岛与锁屏实时活动扩展 Target (Live Activity / Dynamic Island)
│   └── Antigravity/
│       ├── App/                # 应用生命周期
│       ├── Models/             # 会话、步骤、配额、排队消息、后台任务模型
│       ├── Services/           # RPC 客户端、WebSocket、扫码配对、Keychain、单趟 LaTeX 引擎、焦点上报
│       ├── ViewModels/         # MVVM 状态控制
│       └── Views/              # 会话列表、聊天界面、配额看板、扫码配对、卡片式交互、全手势大图查看器
├── android/                    # Android 原生客户端工程 (Kotlin 1.9+ / Jetpack Compose)
│   ├── build.gradle.kts        # 根工程构建配置
│   ├── settings.gradle.kts     # 模块设置
│   ├── gradle/wrapper/         # Gradle Wrapper
│   └── app/
│       ├── build.gradle.kts    # 模块构建配置 (Compose, Material 3, OkHttp, ZXing)
│       └── src/main/
│           ├── AndroidManifest.xml # 权限与组件声明 (相机扫码、网络访问)
│           └── java/com/antigravity/mobile/
│               ├── MainActivity.kt # 单 Activity 入口与导航配置
│               ├── data/model/     # 会话、配额、流式 Payload、工程与交互模型
│               ├── data/service/   # ApiClient、WebSocket 客户端、EncryptedPrefs
│               ├── ui/components/  # 配额条、多账号抽屉、新会话抽屉、排队面板、工具折叠胶囊
│               ├── ui/screen/      # 会话列表、聊天界面、离线配对扫码屏幕
│               ├── ui/theme/       # Material 3 暗黑主题色彩与字体规范
│               └── ui/viewmodel/   # 列表、聊天与配对 MVVM 状态机
├── integrations/
│   └── yoooclaw/               # YoooClaw 物理外设与网关联动桥接实现
│       ├── antigravity_bridge.py # 网关直连注入、双重探针、游标同步与灯效控制
│       └── adapter.py          # 消息拦截分流切入点与 App 回显
├── docs/                       # 深度架构设计与运维规范
│   ├── unified_active_session_cursor_design.md        # 统一活跃游标与双端协同感知规范
│   ├── https_cloud_relay_guide.md                     # HTTPS 域名中继与云穿透实战指南
│   ├── markdown_and_chat_image_rendering_design.md    # 移动端富媒体图片与大图缩放交互设计
│   ├── yoooclaw_hardware_integration.md               # 物理外设硬件联动规范
│   └── cloud_server_evaluation_and_migration_guide.md # 云服务器选型与迁移指南
├── scripts/
│   ├── tmux-start.sh           # 后台守护进程启动脚本
│   ├── tmux-stop.sh            # 后台守护进程终止脚本
│   ├── pair.sh                 # 终端生成与打印配对二维码与链接
│   ├── issue-agy-tls.sh        # Let's Encrypt TLS 证书自动化签发与配置脚本
│   ├── devices-list.sh         # 查看已配对设备列表 (支持在线/离线双模)
│   └── devices-clear.sh        # 清除已配对设备 (支持安全二次确认)
└── logs/
    └── .gitkeep                # 运行时日志目录
```

---

## 🚀 快速开始与部署指南

### 第一步：在 Mac 上启动本地网关

本项目网关需要运行在安装有 Antigravity 的 macOS 主机上：

```bash
# 1. 复制环境配置文件 (配置 Bark 推送、域名等，可选)
cp env.example .env

# 2. 编译网关统一二进制文件 (产物位于 bin/gateway)
make build

# 3. 前台启动测试 (默认监听 :58900，终端自动打印配对二维码)
make run
```

**后台持续运行（强烈推荐）：**
```bash
# 通过 tmux 启动后台常驻守护，日志输出至 logs/gateway.log
make tmux-start

# 查看实时日志与配对二维码
tail -f logs/gateway.log

# 随时重新在终端生成并打印配对二维码与链接
make pair

# 查看所有已配对设备列表（支持网关在线实时或离线读取）
make list

# 清除所有已配对设备（带二次安全确认）
make clear all

# 停止后台服务
make tmux-stop
```

在 Mac 浏览器中打开 [http://127.0.0.1:58900](http://127.0.0.1:58900)，当顶部状态指示器显示绿灯且呈现已连接的端口与会话列表时，即说明网关工作正常。

---

### 第二步：配置远程网络访问（满足移动端随时随地直连）

网关默认绑定 IPv4 与 IPv6 全网卡（`:58900`），支持多种远程连接方式：

#### 方案 A：HTTPS 域名中继与 TLS 1.3（强烈推荐，免公网 IPv4 且最安全）
若没有公网 IPv4 地址，可通过云服务器端口转发/FRP 穿透 + Let's Encrypt 证书实现端到端安全中继：
1. 配置域名 DNS 解析（如通过 Cloudflare 灰云解析到你的云服务器 IP）；
2. 执行脚本自动化签发证书：
   ```bash
   export CF_Token="your_cloudflare_dns_api_token"
   ./scripts/issue-agy-tls.sh
   ```
3. 在 `.env` 中填入：
   ```env
   DDNS_HOST=agy.example.com
   GATEWAY_SSL=1
   TLS_CERT_FILE=/Users/your_username/.antigravity-mobile/certs/agy.example.com.fullchain.cer
   TLS_KEY_FILE=/Users/your_username/.antigravity-mobile/certs/agy.example.com.key
   ```
4. 重启网关后，终端打印的配对二维码将直接携带 `https://agy.example.com:58900`，流量在 Mac 本地解密，云端无法窥视明文，完美绕过 iOS ATS 限制。

#### 方案 B：IPv6 / DDNS 公网直连（超低延迟）
1. 绝大多数家庭宽带与移动网络均已原生分配公网 IPv6 地址；
2. 配合 DDNS（将域名解析绑定至 Mac 的 IPv6 网卡），在 `.env` 中配置 `DDNS_HOST=your-ddns-domain.com`；
3. 网关在终端打印的配对二维码将自动携带公网域名，手机无论处于 5G 蜂窝网络还是外部 Wi-Fi 均可极速直连。

#### 方案 C：Tailscale / WireGuard 私有内网
- 将 Mac 主机与手机同时加入同一个 Tailscale 私有局域网；
- 在手机端直接通过 Mac 的 Tailscale IP（如 `http://100.x.y.z:58900`）安全直连。

#### 方案 D：本地局域网 (Wi-Fi)
- 手机与 Mac 处于同一 Wi-Fi 网络下，直接通过 Mac 局域网 IP（如 `192.168.x.x:58900`）直连。

---

### 第三步：配置 iOS Bark 实时推送通知（可选，强烈推荐）

想要离开电脑桌后，在 Agent **任务完成**、**遇到异常** 或 **等待审批/Proceed 确认** 时立刻收到手机推送？

1. 在 iPhone App Store 搜索安装 **Bark**（认准开发者 Finb / 伍峰 的极简纯色图标应用）；
2. 打开 Bark 复制你的专属链接（如 `https://api.day.app/YOUR_DEVICE_KEY/`）；
3. 打开 `.env` 文件，填入 `BARK_URL`：
   ```env
   BARK_URL=https://api.day.app/YOUR_DEVICE_KEY/
   ```
4. 重启网关，推送服务即刻生效！手机收到推送点击即可直达对应会话。

---

### 第四步：运行移动端客户端

#### 选项 1：运行 iOS 原生客户端 (SwiftUI 5 / Swift 6)
1. 用 **Xcode 16+** 打开工程文件 `ios/Antigravity.xcodeproj`；
2. 在工程设置的 **Signing & Capabilities** 页面中，将 **Team** 切换为您自己的 Apple ID 或开发者团队；
3. 将真机 iPhone 连接至 Mac，点击 **Run** 即可完成安装；
4. 首次启动时点击右上角扫码图标直接扫描 Mac 终端打印的配对二维码，即可自动完成设备 Token 交换并安全直连！

#### 选项 2：运行 Android 原生客户端 (Jetpack Compose)
1. 用 **Android Studio (Hedgehog 2023.1.1+)** 打开 `android/` 目录；
2. 等待 Gradle 同步完成（支持 Android 8.0+ / API 26+，预置 Java 17）；
3. 将 Android 手机开启开发者模式并连接电脑（或使用命令行构建）：
   ```bash
   cd android
   ./gradlew assembleDebug
   ```
4. 安装生成的 APK（位于 `android/app/build/outputs/apk/debug/app-debug.apk`）；
5. 打开应用，点击右上角扫码图标扫描终端二维码，即可瞬间完成离线配对！

#### 选项 3：使用 PWA Web 客户端（免安装任何 IDE）
1. 在手机 Safari / Chrome 浏览器中直接打开网关域名或 IP（如 `https://agy.example.com:58900`）；
2. 登录/配对后，点击浏览器底部的「**分享**」按钮；
3. 向下滑动选择「**添加到主屏幕**」；
4. 退出浏览器，点击手机桌面上生成的 Antigravity 图标，即可直接以全屏独立 App 模式使用。

---

### 第五步：联动 YoooClaw 物理外设硬件（可选进阶）

若拥有 YoooClaw 智能硬件设备，可将其无缝对接至 Antigravity Mobile 网关：
1. 参照 [`integrations/yoooclaw/README.md`](integrations/yoooclaw/README.md) 说明部署适配器；
2. 重启外设宿主网关：
   ```bash
   launchctl kickstart -k gui/$(id -u)/ai.hermes.gateway
   ```
3. 按住外设按键说出对 Agent 的指令，硬件将自动触发流光动效并将语音毫秒级注入当前聚焦的会话中。

---

## 🧪 测试与质量验证

```bash
# 1. 运行 Go 网关核心单元与集成测试 (探查器、代理、游标、鉴权、通知)
make test

# 2. 验证 iOS 原生客户端构建 (Swift 6 严格并发检查)
xcodebuild -project ios/Antigravity.xcodeproj \
           -scheme Antigravity \
           -destination "generic/platform=iOS" \
           SWIFT_STRICT_CONCURRENCY=complete \
           SWIFT_VERSION=6 \
           build

# 3. 验证 Android 原生客户端构建
cd android && ./gradlew testDebugUnitTest
```

---

## 🔒 隐私与安全性

- **100% 本地优先**：本项目所有代理逻辑均直接运行在您个人的设备之间，**绝不包含任何第三方云端收集服务器或隐私埋点**；
- **设备级独占凭证**：通过终端配对二维码生成 salted SHA-256 签名的独占 Device Token，iOS 存储于 Keychain，Android 存储于 `EncryptedSharedPreferences` 硬件密钥库，支持在网关随时吊销；
- **端到端 TLS 1.3 传输**：支持全链路 HTTPS / WSS 加密，私钥始终保留在 Mac 本地，网络中继节点无法解密传输内容；
- **深度防攻击加固**：包含 HTTP 请求体硬限制（防 OOM）、WebSocket Ticket 频控限流、沙箱文件白名单校验，以及会话注解文件的 `0600` 严格所有者权限；
- **防自反焦点保护**：从协议层设计 1.5s 抑制期，杜绝多端状态同步过程中的死循环误判。

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。欢迎提交 Issue 与 Pull Request 共同完善！
