# 🚀 Multigravity v1.1.2

### ✨ 新功能与核心体验
- 🎨 **斜杠指令与扩展技能图标 100% 对齐桌面端 (Desktop-Matched Slash Command & Skill Icons)**：
  - 全面提取并同步 Antigravity 官方桌面端原生 SVG 矢量图标资产（覆盖 `/grill-me` 互动问答、`/plan` 实施规划、`/browser` 网页浏览、`/goal` 长期目标、`/teamwork` 团队协作、`/compact` 压缩上下文、`/learn` 经验沉淀、`/schedule` 定时任务、`/boost` 深度推理等核心指令与扩展技能）。
  - 双端（iOS / Android）与 Web 端全量支持内联斜杠指令胶囊药丸徽标（Inline Command Badge / Pill），包含微型技能图标与指令名称，告别朴素的纯文本或通用符号，视觉质感与官方桌面端完美对齐。
  - 深度适配指令选择器（SlashCommandPicker）、用户提问消息气泡（MessageBubble）、消息分享长图卡片（MessageShareCardSheet）以及队列同步分词器，各终端交互动线统一。
- ⚡ **Cockpit 驾驶舱新增 Gemini 4 Argon 额度进度条与套餐梯队徽标 (Dynamic Argon Quota Bar & Tier Badges)**：
  - 驾驶舱全量接入 Gemini 4 Argon 额度实时探测与动态百分比进度条，采用专属琥珀金质感主题色，直观展示剩余额度百分比、用量分布与 5 小时重置倒计时。
  - 新增账号等级梯队（奢华鎏金 `ULTRA` / `PRO` / `FREE`）高辨识度徽标，精简并对齐 iOS 与 Android 双端配额卡片密度，多账号配额状态与健康度一目了然。

### 🛡️ 架构稳固与细节打磨
- 🗂️ **工作区列表自动过滤 Sidecar 定时任务影子项目 (Sidecar Shadow Project Filtering)**：
  - 自动识别并排除 Antigravity 2.0 定时任务（`sidecars`）派生出的同名目录影子 Project ID，防止定时任务 slug 覆盖原有工作区真实名称，并将会话列表中的影子 `projectId` 自动归一至主工作区。
- 🔄 **驾驶舱额度查询超时降级与系统代理自愈 (Cockpit Quota Fallback & Sysproxy Self-Healing)**：
  - 当 Cockpit Tools 外部查询接口超时或受阻时，无缝优雅回退至直连原生额度查询接口，确保多账号额度数据永不断连。
  - 增强本地服务回环与系统代理白名单自愈机制，防止代理工具意外拦截局域网与本地开发握手流量。
- 📱 **iOS 局域网无线连接与 CoreDevice 通道优化**：
  - 强化局域网设备通信与系统代理白名单自适应策略（补齐 `*.coredevice.local`），保障 Xcode 及终端工具与 iPhone 物理真机无线通道的持续稳定。
