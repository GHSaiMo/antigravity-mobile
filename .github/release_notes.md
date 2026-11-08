# 🚀 Multigravity v1.1.3

### ✨ 新功能与核心体验
- 🎙️ **Antigravity 2.29+ 全双工实时语音通话 (`/voice/ws` Full-Duplex Live Voice Call)**：
  - Go 网关、iOS、Android 与 Web PWA 四端全量接入 Antigravity 2.29+ 原生 `/voice/ws`（`jetski-voice-proto`）全双工二进制 Protobuf 实时语音协议，支持 `plan_model` 字符串标识到枚举值自动归一化与网关鉴权透明剥离。
  - 支持 16kHz PCM 麦克风实时上行与 24kHz PCM 流式语音下行低延迟播放、实时语音打断（`clearAudio` 渐变淡出）、双端实时字幕流（`userSpeech` / `voiceTranscript`）、静音切换与 1.0x / 1.25x / 1.5x / 2.0x 倍速调节。
  - 支持在现有会话内一键发起实时语音对话，或从草稿页自动创建新会话并无缝切入沉浸式语音通话卡片（Voice Call Card）；设置页新增「实时语音对话」独立开关（默认关闭以保持传统 `↑` 发送按钮习惯，开启后空闲状态切换为实时语音入口，单击开启双工通话、长按触发离线录音，同时保留 `➕` 附件面板离线录音入口）。
- 📐 **三端 LaTeX 数学公式与自适应 Markdown 表格全面升级 (LaTeX Math & Adaptive Markdown Tables)**：
  - 全面修复并对齐 iOS、Android、Web 三端复杂 LaTeX 数学公式渲染（原生支持 `\underbrace`、`\overbrace`、`\langle` / `\rangle`、`\mathcal`、`\mathbb`、矩阵环境与多层上下标等）及常用 Markdown 语法。
  - iOS 与 Android 原生表格升级为按单元格内容自适应列宽并支持横向平滑滚动，彻底解决多列表格在手机端挤压折行问题。

### 🛡️ 架构稳固与细节打磨
- 🎯 **会话运行状态精准判定与 `Action required` 误报根治 (Session Status Precision)**：
  - 修复批准 `implementation_plan.md` 后会话在 `Working` 执行期间或无 `walkthrough.md` 完结时被误判为 `Action required` 的问题；新增 `trajCache` 快照新鲜度守卫，并将会话列表磁盘 `requestFeedback` 巡检收敛为 `detectProceed` 二次核验。
- ⚙️ **常驻守护进程 (`IsDaemon`) 与回合阻塞任务状态解耦 (Daemon Task Status Parity)**：
  - 区分常驻守护任务（`IsDaemon=true`，如 dev server / 定时监听器）与回合阻塞任务：常驻任务跨回合保持运行不再抑制完成推送，也不会将会话列表或 iOS 灵动岛误标为执行中；取消当前回合时仅终止阻塞任务并保留常驻守护进程。
- 🌐 **iOS PWA 软键盘避让与 Fake-IP 隧道双栈探测加固 (PWA Keyboard & Fake-IP ULA Exemption)**：
  - 修复 iOS Safari / PWA 模式下软键盘弹出遮挡底部输入框与操作栏的问题。
  - Cloudflare 隧道双栈探测豁免全段 IPv6 ULA（`fc00::/7`，含 sing-box `fc00::/18` 与 Mihomo `fdfe:dcba:9876::/64`）及 IPv4 Fake-IP（`198.18.0.0/15`），杜绝 TUN 透明代理环境下的双栈探测假阳性。
