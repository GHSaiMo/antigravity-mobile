# 🚀 Multigravity v1.1.3

### ✨ 新功能与核心体验
- 🎙️ **Antigravity 2.29+ 全双工实时语音通话 (`/voice/ws` Full-Duplex Live Voice Call)**：
  - **四端全量打通**：Go 网关、iOS、Android 与 Web PWA 四端全量接入 Antigravity 2.29+ 原生 `/voice/ws`（`jetski-voice-proto`）全双工二进制 Protobuf 实时语音协议，支持 `plan_model` 字符串标识到枚举值自动归一化与网关鉴权透明剥离；
  - **高保真流式音频管道**：支持 16kHz PCM 麦克风实时上行与 24kHz PCM 流式语音下行低延迟播放、实时语音打断（`clearAudio` 渐变淡出）、双端实时字幕流（`userSpeech` / `voiceTranscript`）、静音切换与 1.0x / 1.25x / 1.5x / 2.0x 倍速调节；
  - **沉浸式通话卡片与开麦开关**：支持在现有会话内一键发起实时语音对话，或从草稿页自动创建新会话并无缝切入沉浸式语音通话卡片（`VoiceCallCard`）；设置页新增「实时语音对话」独立开关（默认关闭以保持传统 `↑` 发送按钮习惯，开启后空闲状态切换为实时语音入口，单击开启双工通话、长按触发离线录音，同时保留 `➕` 附件面板离线录音入口）；
  - **折叠态语音消息气泡扩充**：未展开状态下的语音气泡由单一图标扩充为醒目的「波形图标 + 语音交互」圆角胶囊药丸，视觉更长更显眼、可点击感显著增强；
  - **分享长图全面对齐语音气泡**：分享长图自动识别语音交互消息，隐藏底层 `.wav` 音频文件卡片，统一对齐呈现 Antigravity 语音矢量图标与「语音交互」胶囊，并提炼展示脱敏净化后的多轮对话；
  - **实时通话底栏浮条拉高并支持双行字幕**：优化底栏双工实时通话卡片（`VoiceCallCard`）纵向比例与空间，字幕区域支持完整显示两行实时人机转写文本，彻底告别单行文本溢出截断；
  - **展开详情多轮对话深度净化**：彻底正则剔除 `<voice_instructions>` 等系统级提示词注入与规则说明杂音，优先提炼 `<talker_agent_context>` 对话块，仅保留「用户」与「智能体」纯净对话轮次；长按复制同步净化；
  - 🧭 **对话大纲导轨语音气泡统一对齐**：修复对话大纲与步骤导轨（`ConversationNavigatorSheet`）泄露 `<voice_instructions>` 等协议注入标签的问题，自动识别语音轮次并对齐主会话界面渲染「语音交互」胶囊气泡与脱敏提炼后的人声文本，杜绝任何内部协议回显；
  - **异常自查气泡大号化与 Markdown 强化**：连接失败排查气泡整体尺寸放大，正文字号提升至 **15sp**（与普通对话气泡严格对齐），以 Markdown 语法对核心排查步骤（**重新登录**、**开启代理 TUN 模式**、**检查电脑麦克风**）加粗强调并对域名 `*.googleapis.com` 行内代码高亮；
  - 💡 **双端实时语音通话屏幕常亮活跃**：在 Android 与 iOS 实时语音通话期间，自动联动前台视图常亮标志与系统空闲休眠定时器（`isIdleTimerDisabled` / `keepScreenOn`），彻底杜绝通话中手机因无触碰而自动暗屏休眠导致断流；通话结束或异常挂断时立即恢复系统默认息屏机制，零权限要求；
  - 🌊 **双端抗抖动平滑缓冲管道 (150ms Jitter Buffer)**：针对跨国代理与移动网络偶发丢包重传导致音频帧到达抖动、播放器频繁下溢饿死（Buffer Underrun）引起断续卡顿的问题，在 Android 与 iOS 客户端引入轻量级 Jitter Buffer 蓄水池，AI 开口起播时预先平滑缓冲 ~150ms 并在 120ms 安全超时后自适应放音，Android 底层 AudioTrack 缓冲池扩容至 500ms，彻底抹平网络微抖动；打断（`clear_audio`）与挂断时瞬间清空队列，零残留；
  - **iOS 硬件级声学回声消除与降噪 (AEC)**：iOS 客户端全双工通话启用硬件级 `VoiceProcessing`（AEC 回声消除与 AGC 自动增益），彻底消除外放自激循环与啸叫。
- 📐 **三端 LaTeX 数学公式与自适应 Markdown 表格全面升级 (LaTeX Math & Adaptive Markdown Tables)**：
  - 全面修复并对齐 iOS、Android、Web 三端复杂 LaTeX 数学公式渲染（原生支持 `\underbrace`、`\overbrace`、`\langle` / `\rangle`、`\mathcal`、`\mathbb`、矩阵环境与多层上下标等）及常用 Markdown 语法；
  - iOS 与 Android 原生表格升级为按单元格内容自适应列宽并支持横向平滑滚动，彻底解决多列表格在手机端挤压折行问题。
- 📊 **免 Cockpit 本地直读实时配额与 Free 账号双 Weekly 真实周期对齐 (Live Quota & Free Tier Alignment)**：
  - 未安装或未配置 Cockpit Tools 时，网关直接通过 Antigravity 本地服务 RPC 实时读取当前使用账号的 4 维配额（Claude 5h、Claude Weekly、Gemini 5h、Gemini Weekly）及 Gemini 4 Argon 额度，并在移动端顶部状态条与详情面板实时展示；
  - 详情面板下方备用账号区域自动呈现友好引导，提示用户安装 Cockpit Tools 并通过 `mgy cockpit` 交互式配置，享受多备用账号一键切换；
  - **Free 账号纯净双 Weekly 对齐**：针对 Free 账号彻底剥离 5h 滚动配额（`Claude 5h` / `Gemini 5h`），不再兜底注入伪 5h 默认数据；修复模型层回退时将 7 天 Weekly 周期误赋给 5h 的错位问题，使 `Claude Weekly` 与 `Gemini Weekly` 精准对齐真实的 7 天恢复倒计时；
  - **移动端与 Web 自动收敛为单排双列**：iOS、Android 原生客户端与 Web 控制台针对 Free 账号自动从 2×2 四象限收敛为单排双列卡片（左侧 Claude Weekly、右侧 Gemini Weekly），顶部 Quota 状态条自动切换为 Gemini Weekly 进度展示。

### 🛡️ 架构稳固与细节打磨
- 🌐 **桌面 Web 工作台设置弹窗深度汉化 (Desktop Web Settings Localization Parity)**：
  - 适配 Antigravity 2.29+ 新版侧边栏导航映射，彻底汉化设置分类项（`常规`、`外观`、`快捷键`、`通知`、`模型`、`意见反馈`）；
  - 全面汉化权限卡片（`本机电脑`、`远程`、`修改于 N 个项目` 计数联动）与语音模式设置（`声音`、`麦克风`、`系统默认` 输入设备、`音量` 及说明文本），对齐桌面客户端原生中文体验。
- 🧼 **多端协议回显净化与工具步骤对齐 (Internal Protocol Echo Filtration)**：
  - 彻底过滤模型在部分回合中偶发的内部 `<SYSTEM_MESSAGE>` 协议回显杂音，在 Go 网关层、iOS 原生客户端与 Web 端对齐电脑桌面版处理逻辑，确保移动端呈现纯净对话流。
- 🎯 **会话运行状态精准判定与 `Action required` 误报根治 (Session Status Precision)**：
  - 修复批准 `implementation_plan.md` 后会话在 `Working` 执行期间或无 `walkthrough.md` 完结时被误判为 `Action required` 的问题；新增 `trajCache` 快照新鲜度守卫，并将会话列表磁盘 `requestFeedback` 巡检收敛为 `detectProceed` 二次核验。
- ⚙️ **常驻守护进程 (`IsDaemon`) 与回合阻塞任务状态解耦 (Daemon Task Status Parity)**：
  - 区分常驻守护任务（`IsDaemon=true`，如 dev server / 定时监听器）与回合阻塞任务：常驻任务跨回合保持运行不再抑制完成推送，也不会将会话列表或 iOS 灵动岛误标为执行中；取消当前回合时仅终止阻塞任务并保留常驻守护进程。
- 🌐 **iOS PWA 软键盘避让与 Fake-IP 隧道双栈探测加固 (PWA Keyboard & Fake-IP ULA Exemption)**：
  - 修复 iOS Safari / PWA 模式下软键盘弹出遮挡底部输入框与操作栏的问题；
  - Cloudflare 隧道双栈探测豁免全段 IPv6 ULA（`fc00::/7`，含 sing-box `fc00::/18` 与 Mihomo `fdfe:dcba:9876::/64`）及 IPv4 Fake-IP（`198.18.0.0/15`），杜绝 TUN 透明代理环境下的双栈探测假阳性。
