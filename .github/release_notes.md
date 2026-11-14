# 🚀 Multigravity v1.1.6

### ✨ 核心新功能与重要更新

- ⚡ **Antigravity 2.32+ 动态多智能体工作流全生命周期落地（Dynamic Workflows / `run_workflow` 原语架构 - Go 网关 + iOS + Android + Web PWA）**：
  - **快捷发起与模板预设库**：输入栏快捷操作区新增固定于模型按钮右侧的 `Workflow` 按钮，联动 `/workflow` 斜杠指令，支持从底部拉起「Antigravity 动态工作流」模板抽屉（内置并发代码审查、并发单测与故障定位、多维度综合调研、自定义动态工作流四大高频模板），自动注入带占位符的结构化编排提示词；
  - **专属安全审批卡（WorkflowApprovalCard）**：Go 网关智能拦截 `run_workflow` 调用并解析提取 Python 编排脚本、预估阶段、并发 Sub-Agents 拓扑；前端展示脚本概览、可折叠 Python 代码预览（含行数与快速复制）与四档安全决策（允许本次执行 / 本会话始终允许 / 拒绝 / 补充反馈打回）；
  - **阶段拓扑看板（WorkflowStageCard）**：阶段流转轴（Phase 1 ➔ 2 ➔ 3 Stepper 状态图标与单阶段耗时）、并发智能体网格（Sub-agents Matrix Grid）、实时日志流（代码高亮可折叠 Live Logs 控制台）与结果 Schema 产物展示；
  - **原位穿透与双层控制**：轻触任一子智能体即可弹出半屏详情抽屉（`SubagentDetailSheet`），原地翻阅 Prompt、状态及实时产物而不中断主会话；提供看板全局一键终止工作流与抽屉单点终止子智能体的双层熔断控制；
  - **异步分级通知**：接入 Bark 与 FCM 里程碑分级通知，工作流需审批、执行完成及异常时即时推送。
- 📊 **全端实时上下文水位环与生成速率监控（Go 网关 + iOS + Android + Web PWA）**：
  - **官方 Language Server 元数据实时提取**：通过 `GetCascadeTrajectoryGeneratorMetadata` RPC 零侵入提取当前会话真实上下文占用（`usedTokens` / `maxTokens` / `percentage`）、Token 明细（Input / Cache Hit / Output / Thinking）以及生成性能指标（首字延迟 TTFT、流式耗时与实时输出速率 TPS），带 1.2s 高频防抖缓存并随 WebSocket 增量推送；
  - **顶栏上下文圆环胶囊**：在会话顶部轮次胶囊旁内联 SVG / Canvas / SwiftUI 进度环胶囊（`<70%` 翠绿、`70%-89%` 警示橙、`≥90%` 红色脉冲提醒），生成期间实时展示 `⚡ t/s` 输出速率；
  - **上下文审计面板（Audit Bottom Sheet）**：点击水位胶囊即可展开完整物理窗口占用进度条、TTFT / TPS / 流式耗时指标卡、最近一步 Token 构成明细，以及底部的「一键压缩 `/compact`」与「开启新会话」快捷入口。
- 🔥 **Token 历史用量看板与 52 周热力图（Cockpit「账户与配额」全面升级）**：
  - **免拆包本地全量扫描**：新增 `GET /api/v1/cockpit/tokens/history` 网关接口，自动扫描桌面端 Antigravity 与 `antigravity-cli` 的本地 `transcript.jsonl`（15s 内存缓存 + 文件 size/mtime 增量缓存，支持 `?force=true` 强制重扫），并按主流模型阶梯定价自动折算美元与人民币估算费用；
  - **分段 Tab 与 GitHub 风格 52 周热力图**：iOS / Android / Web「账户与配额」面板新增「实时配额 | Token 历史」分段切换，展示今日 / 近 7 日 / 累计总 Token / IDE 与 CLI 占比 / API 折算 KPI 卡片、自动滚至最新一周的 52 周横向热力图，以及点击任意日期格即时刷新的内联明细检查器（Input / Cache / Output / Thinking 与 Top 模型分布）。
- 🛟 **上下文溢出智能诊断与救援气泡（Smart Overflow Rescue）**：
  - 网关侧 `ClassifyErrorKind` 自动识别 `context_overflow`、`rate_limited`、`network_error` 并注入结构化 `errorKind` / `isOverflow` 标记；
  - 三端消息流遇上下文窗口超限错误时，自动将原始报错卡片替换为友好的橙色救援卡片，内嵌 **`一键压缩 /compact`** 与 **`开启新会话`** 动作按钮，支持原地延续当前工作区快速开启新草稿会话。
- 🖼️ **Token 历史长图海报一键导出与分享（iOS + Android 原生）**：
  - 「账户与配额」Token 历史面板顶栏新增海报分享入口，一键将当前 52 周热力图、今日 / 近 7 日 / 累计 Token KPI、IDE 与 CLI 占比及选定日期明细合成高保真长图海报，直通系统原生分享与微信；
- 🕹️ **会话轮次跳跃浮钮体验升级（Prompt Jump Stepper - iOS + Android + Web）**：
  - 扩大为 44px 舒适触控尺寸，引入智能透明度呼吸逻辑（上滑 100% 唤醒、静止 3s 或下滑 30% 半透避让、静止 10s 平滑隐匿、触底与聚焦输入即刻消失）；
- 🌐 **iOS 代理自适应 WebSocket 心跳增强**：
  - `ConversationEventsClient` 引入 `URLSessionWebSocketTask` 作为主力自适应传输，根治 Wi-Fi 代理环境下心跳状态灯误报红色的问题，保留 Tailscale 局域网直连 NWConnection 回退。
