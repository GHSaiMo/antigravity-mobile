# 🚀 Multigravity v1.1.7

### ✨ 核心新功能与重要更新

- 🧩 **Antigravity 2.32.1 原生能力全栈落地（Phase 3 全量接入：Go 网关 + iOS + Android + Web PWA）**：
  - **`@` 上下文提及、对话 Fork 与隔离工作区**：输入框新增 `@` 文件 / 代码片段 / 对话上下文提及补全；支持对话单步或全局 Fork 与 Git Worktree 隔离分支创建；新建会话抽屉新增「新建本地工作区」，并将克隆 GitHub 仓库入口调整至左上角，创建流程更顺手；
  - **三档 Agent 权限模式与记忆库 CRUD**：支持「严格审批 / 离席沙箱放行 / 全自动放行」三档权限模式，设置页与会话页右上角工具按钮长按菜单实时双向同步；工具与技能看板支持 Agent Memories 增删改查，删除操作配备二次确认弹窗及网关侧文件备份；
  - **实时交互终端、子智能体直通与 Git/自动化管理**：新增终端 PTY WebSocket 实时交互流（创建、输入、关闭与后台任务转接）、运行中子智能体消息直通与浏览器子智能体跳过/加白控制、Git 分支查看/切换/丢弃，以及 Automation 定时任务的新建与删除；
  - **MCP 工具控制、嵌入式桥接与原生听写/播报**：支持 MCP Server 启用/禁用与热刷新、MCP App / Sidecar UI 嵌入式桥接，以及原生流式语音听写（Dictation）与回答朗读（Narration）。
- 🔄 **响应式 Connect 流 Hub 与 WebSocket 增量断点续传 (`sinceRev`)**：
  - 网关新增响应式 Connect 流聚合 Hub 与本地 SQLite3 快速查询路径（`local_store.go`），对齐 Antigravity 2.32.1 的 MCP / Elicitation / Swarm 协议并大幅降低热路径开销；
  - 全端接入 WebSocket `sinceRev` 增量断点续传，iOS 新增 Anycast 加速与 LiveActivity 快捷意图，Android 新增通知栏快捷审批/操作按钮，断网重连不丢进度。
- ⚡ **全端流式渲染性能与离线能力升级**：
  - iOS 与 Android 引入流式增量 Markdown 解析与有界 LRU 缓存，消除超长对话持续输出时的卡顿与重排抖动；
  - 三端全量内置离线代码语法高亮，Web 端同步强化矩阵 / LaTeX 公式渲染、草稿媒体与 `/slash` 提及保留及 `agent-embed` 流式稳定性；iOS 修复 `WorkflowTemplateSheet` 跨区拖拽重叠与 `ChatView` 编译器超时问题。
- 🎛️ **模型切换与设置页重构、Token 历史子代理统计修复**：
  - 精简重构设置页结构，区分「首选模型」与「次选模型」：新会话默认使用首选模型，已有会话自动同步电脑端最新模型；会话页短按模型胶囊快切首选与次选，长按可从全部模型中任选；
  - Cockpit Token 历史自动解析归因 Subagent 模型消耗，并在账户配额与历史面板左上角新增海报保存按钮。
- 🚀 **Git/PR 提交快捷手势与二次确认（iOS + Android）**：
  - 当工作区存在未提交改动时，长按底栏 Commit / PR 胶囊可上滑呼出轻量快捷菜单，支持 `Commit Only` 与 `Commit and Push`；
  - 松手后自动填充至文本框进行二次确认与补充编辑，既防止误触又大幅简化移动端高频提交链路。
