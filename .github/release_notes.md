# 🚀 Multigravity v1.2.0

> 🎉 **重磅消息：Multigravity iOS 版现已正式上架 Apple App Store！**  
> 无需本地 Xcode 编译或自签，直接前往 👉 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)** 即可在 iPhone / iPad 上一键安装使用。

### ✨ 核心新功能与重要更新

- 🍎 **iOS 原生 APNs 推送、远程灵动岛（Live Activity）与无状态 Cloudflare Relay**：
  - 全新原生 iOS **APNs 推送通知**与 **远程 Live Activity 灵动岛状态推送**，App 退至后台、锁屏或被系统回收后仍可实时刷新任务进展、弹出审批与完成提醒；
  - 配套无状态 **Cloudflare APNs Worker 中继**，无需自建推送服务器即可安全路由通知，并继续完整兼容原有 Bark 推送与自定义提示音。
- 🤖 **Android 任务通知栏后台保活与状态同步彻底修复**：
  - 新增 `dataSync` 前台保活服务（`AgentTaskForegroundService`）与后台 3 秒 HTTP 兜底状态轮询，彻底解决任务退后台或锁屏完成后通知栏一直卡在「运行中」、必须切回应用才刷新的问题；
  - 复用单通知 ID（零重复卡片），任务完成、方案待审批或异常结束时同步解绑并自动停止前台服务（零闲时耗电），同时弹出高优完成提醒并支持点击直达会话；
  - 修复 `isAwaitingResponse` 状态残留卡死、`CASCADE_RUN_STATUS_RUNNING` 下「停止」快捷按钮丢失、以及从后台切回前台后自动重连 WebSocket 与即时刷新。
- 📎 **iOS 附件上传进度体验对齐**：
  - 对齐 iOS 与 Android 附件上传环形进度显示及流式上传进度回调，大文件上传状态反馈更清晰。
- 🧭 **移动端交互审批卡片长文本优化（iOS / Android / Web）**：
  - 约束多题决策与交互卡片中超长问题文本的高度并支持独立滚动，避免长内容撑爆屏幕或遮挡底部操作按钮。
