# 🚀 Multigravity v1.2.0

> 🎉 **重磅消息：Multigravity（反重力智能体移动伴侣）iOS 版现已正式上架 Apple App Store！**  
> 无需本地 Xcode 编译或自签，直接前往 👉 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)**（海外 174 地区，中文系统设备自动优先展示简体中文商店页与展示图，并内置零配置**离线演示模式**）；如需第一时间抢先体验最新构建，也可通过 👉 **[TestFlight 公开测试通道](https://testflight.apple.com/join/YM7f4aSK)** 安装。

### ✨ 核心新功能与重要更新

- 🍎 **iOS 原生 APNs 推送、远程灵动岛（Live Activity）与无状态 Cloudflare Relay**：
  - 全新原生 iOS **APNs 时效性推送通知**与 **远程 Live Activity 灵动岛状态推送**，App 退至后台、锁屏或被系统回收后仍可实时刷新任务进展、弹出审批与完成提醒；
  - 修复点击通知横幅唤醒 App 时的主线程断言问题，确保冷启动与后台点击跳转均稳定直达目标会话；
  - 配套无状态 **Cloudflare APNs Worker 中继**，无需自建推送服务器即可开箱安全路由通知，并全面移除旧版 Bark 依赖与配置。
- 💬 **iOS 微信聊天记录归档离线缓存与附件传输升级**：
  - 新增 `WeChatArchiveCacheManager` 离线归档缓存与加载优化，大体积微信聊天记录 ZIP 包二次查阅秒开，并优化附件与多媒体流式上传及网络异常处理；
  - 对齐 iOS 与 Android 附件上传环形进度显示及流式上传进度回调，大文件上传状态反馈更清晰。
- ✨ **双端自由选词浮层对齐与原生弹窗交互打磨（iOS / Android）**：
  - 对齐 iOS 与 Android 长按「选择文本」自由选词弹窗体验，修复 iOS 图片预览布局适配问题；
  - 移除 iOS 各功能 Sheet 顶部冗余关闭/完成按钮，统一原生拖拽指示条视觉规范；
  - 约束多题决策与交互审批卡片中超长问题文本的高度并支持独立滚动（iOS / Android / Web），避免长内容撑爆屏幕或遮挡底部操作按钮。
- 🤖 **Android 任务通知栏后台保活与状态同步彻底修复**：
  - 新增 `dataSync` 前台保活服务（`AgentTaskForegroundService`）与后台 3 秒 HTTP 兜底状态轮询，彻底解决任务退后台或锁屏完成后通知栏一直卡在「运行中」、必须切回应用才刷新的问题；
  - 复用单通知 ID（零重复卡片），任务完成、方案待审批或异常结束时同步解绑并自动停止前台服务（零闲时耗电），同时弹出高优完成提醒并支持点击直达会话；
  - 修复 `isAwaitingResponse` 状态残留卡死、`CASCADE_RUN_STATUS_RUNNING` 下「停止」快捷按钮丢失、以及从后台切回前台后自动重连 WebSocket 与即时刷新。
- 🔄 **网关新增 `mgy update` 一键自更新与启动版本检测**：
  - 新增 `mgy update` 子命令（支持 `-check`、`-force`、`-version <tag>`），沿用多镜像直连优先 + 本地代理兜底下载策略，全平台支持归档解压、macOS 自动清隔离属性与 ad-hoc 签名、二进制自检及原子原地替换；
  - GitHub Release 产物同步附带 `checksums.txt` 支持 SHA-256 完整性校验，网关启动时新增 12 小时限频静默新版本提示（可通过 `MULTIGRAVITY_UPDATE_CHECK=0` 关闭）。
