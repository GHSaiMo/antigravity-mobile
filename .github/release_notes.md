# 🚀 Multigravity v1.1.5

### ✨ 核心新功能与重要更新

- ⏰ **定时任务（Automations）移动端全链路打通与本地双保险（Go 网关 + iOS + Android）**：
  - **官方 Language Server 协议精准对齐**：全面逆向并重构网关 `/gateway/automations` 与 `/gateway/automations/manage` 路由，精准适配 Antigravity Language Server `GetSidecars` 的层级结构，完整解析 Cron 表达式、Prompt 指令、DisplayName 与运行状态（RUNNING / STOPPED）；
  - **纯本地磁盘双保险**：内置本地目录扫描机制，即使 Language Server 离线或休眠，网关直接扫描本地 `~/.gemini/config/sidecars/*/sidecar.json` 及 `config.json`，确保手机端随时随地秒级读取全部定时任务配置；
  - **下次执行时间精准推算与上次记录**：从 `sidecar.log` 提取调度器排期的 UTC 时间并精准转换为本地时区时间（`NextRunAt`）；自动读取 `sidecar_data/<id>/events/*.json` 获取完整历史触发记录；
  - **对话一键穿梭**：历史记录中携带每次定时触发生成的 `conversationId`，手机端点击记录即可直接在手机上打开当次生成的完整对话，查看执行产物与日志；
  - **手机端「立即运行 (Run Now)」与启停控制**：网关对齐 `SIDECAR_ACTION_ENABLE`、`SIDECAR_ACTION_DISABLE`、`SIDECAR_ACTION_RESTART` 与官方原生的 `SIDECAR_ACTION_RUN_NOW`（立即执行一次）；iOS / Android 定时卡片新增「立即运行」按钮与「下次/上次」执行时间指示；设置页「MCP 工具与技能看板」副标题清晰标注涵盖「定时任务 / 后台」。
- 🐝 **原生 `/swarm` 蜂群多智能体协同看板桌面级复刻（iOS & Android）**：
  - 完整复刻 Antigravity 官方桌面端蜂群看板体验，像素风小蜜蜂头像（Pixel Bee Avatars）、@mention 消息流与协作板；
  - 成员名单（Roster）与实时状态感知（waiting / working / stopped / killed）；
  - 交互式蜂群拓扑图（Topology Graph），展现智能体之间的协作网络。
- 💓 **连接心跳指示器（Go 网关 + iOS + Android + Web PWA）**：
  - 首页额度条最右侧新增小圆点，一眼确认手机与电脑端是否真的连上：🟢 已连接（每次心跳轻微呼吸）、🟡 连接不稳 / 电脑端 Antigravity 未就绪 / 重连中、🔴 已断开；
  - 网关在 `/gateway/events` 长连接上新增应用层 `beat` 帧（每 5 秒，携带电脑端状态，状态翻转立即推送），向后兼容旧客户端；旧网关下自动降级为「已连接」二态；
  - 额度详情面板标题统一升级为「账户与配额」，界面更加紧凑纯粹；回到前台时若心跳过期会立即重连，不再沿用旧的绿点。
- 🎨 **视觉细节与暗黑模式适配优化**：
  - 优化 Android 暗黑模式下文本与错误卡片配色显示，对齐 Ultra Aurora 渐变徽标质感；
  - 统一外观菜单选择器与状态指示，多账号无感切换后平滑刷新模型目录。
