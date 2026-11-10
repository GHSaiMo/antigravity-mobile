# 🚀 Multigravity v1.1.4

### ✨ 新功能与核心体验
- 🖥️ **全局后台任务中心 (`ListBackgroundTasks` Global Background Task Center)**：
  - **跨会话后台任务全景感知**：Go 网关对接 Antigravity 2.30.0 新增的 `LanguageServerService/ListBackgroundTasks` 接口，推出 `GET/POST /gateway/tasks/background` 与 `POST /gateway/tasks/background/log`，自动补齐任务所属会话标题（`conversationTitle`）与工作区自定义别名（`workspaceName`），并提供白名单安全校验的实时终端日志末尾（Log Tail）读取；
  - **首页动态绿光胶囊条**：在会话列表页顶部 `Gemini 5h` 配额条正下方，当存在运行中的后台任务时动态浮现 **`>_ N 个后台任务运行中（M 个常驻） · 管理 ›`** 胶囊条（0 个任务时完全隐藏，零空间侵占），点击直接呼出 `McpSkillsSheet` 并定位至第 3 个 Tab **「后台任务」**；
  - **聊天页右上角动态感知按钮**：iOS 与 Android 聊天页右上角工具栏按钮根据是否存在运行中的后台任务（当前会话或跨会话）智能切换——有后台任务时显示终端图标（`terminal` / `>_`）并默认进入 **「后台任务」** Tab；无后台任务时保持扳手图标（`wrench.and.screwdriver` / `Build`）并默认进入 **「MCP 服务」** Tab；
  - **全功能任务管理卡片**：在「后台任务」Tab 内按 **「当前会话任务」** 与 **「其他会话任务」** 分组置顶，超过 1 个任务时顶部提供 **「一键停止全部后台任务」**；单张卡片完整呈现任务类型角标（紫色 `常驻守护` / 橙色 `定时任务` / 靛蓝 `后台命令`）、任务摘要、实时运行时长、单任务一键终止、**点击工作区与会话标题一键跨会话跳转**、命令行复制，以及展开**实时查看/刷新终端日志 Tail**。
- 📑 **长对话原生章节目录 TOC (`GetChapterTitles` Conversation Chapters)**：
  - **原生章节自动提炼**：网关新增 `POST /gateway/cascade/chapters`，对接 Antigravity 2.30.0 `GetChapterTitles` RPC，自动解析并按 `stepIndex` 升序归一化章节标题与阶段摘要，注册 `chapter_titles` 能力位；
  - **双模大纲导轨切换**：iOS 与 Android 的 `ConversationNavigatorSheet`（对话步骤导轨）顶部新增 **「章节目录 (N)」/「全部对话 (M)」** 分段切换器，自动将 `chapter.stepIndex` 映射至最接近的对话轮次（`Turn N`），在几十至上百轮的超长对话中实现按主题章节一键直达。
- 🔀 **Git 提交面板联动 GitHub PR 状态 (`GetWorkspaceGitHubPullRequests` GitHub PR Integration)**：
  - **双引擎 PR 状态发现**：网关新增 `POST /gateway/git/prs` 并在 `/gateway/git/overview` 中联动返回当前仓库的 GitHub Pull Requests；优先调用 Antigravity 2.30.0 `GetWorkspaceGitHubPullRequests`，当上游未返回时自动降级调用本机 `gh pr list` 补齐 PR 列表，并自动识别当前分支关联的 PR；
  - **三端 Git 面板直达 PR**：iOS、Android 与 Web 的 Git 提交面板（`GitCommitSheet`）顶部自动展示当前分支关联的 PR 状态胶囊（区分 `Draft` 灰 / `Open` 绿 / `Merged` 紫 / `Closed` 红、PR 编号 `#N`、标题与 `headRef → baseRef` 分支流向）及仓库近期 PR 列表，点击直接跳转 GitHub PR 页面，打通手机端「改代码 ➔ Commit & Push ➔ 查看 PR 状态」完整闭环。
- 🤖 **Antigravity 2.30.0 全新模型目录与实时语音模型对齐 (Model Catalog & Voice Enum 2.30.0)**：
  - **新模型全量映射**：网关、iOS、Android 与 Web 四端同步接入 `MODEL_PLACEHOLDER_M49`（**Gemini 3.8 Flash / Argon Flash**）、`MODEL_PLACEHOLDER_M50`（**Claude Haiku 5.5**）及 Vertex AI Claude 企业别名（`VERTEX_AI_CLAUDE_4_6_OPUS_MAX`、`VERTEX_AI_CLAUDE_4_5_SONNET` 等）；
  - **实时语音与配额引擎同步**：全双工实时语音协议（`/voice/ws` `jetski-voice-proto`）补齐 `M49` 枚举值（`1049`）映射；Cockpit 配额引擎将 `M50` 纳入 Claude 配额家族，确保仅启用 Haiku 5.5 的环境下也能准确聚合展示 Claude 5h / Weekly 额度。

### 🛡️ 架构稳固与细节打磨
- 🎯 **原生 `clientMessageId` 双轨排队消息精准匹配 (Native `clientMessageId` Queue Sync)**：
  - 网关在 `SendUserCascadeMessage` 与 `QueueCascadeMessage` 时同步注入 Antigravity 2.30.0 原生 `clientMessageId` 字段（同时保留 `<!--mgy-qid:...-->` 注释标签向前兼容旧版 LS）；
  - 轨迹解析与 iOS、Android 客户端优先读取原生 `clientMessageId` 匹配乐观消息与排队追问卡片（`QueuedMessagesCard`），彻底消除纯文本模糊比对误差。
- 🛑 **中断回合新增是否一并终止后台任务 (`killBackgroundTasks`) 精细控制**：
  - `POST /gateway/cascade/stop` 支持透传 Antigravity 2.30.0 `CancelCascadeInvocation` 的 `kill_background_tasks` 参数；默认 `false` 仅终止当前回合思考与阻塞步骤，保留常驻守护进程（如 dev server）继续运行，按需传 `true` 可一键强杀全部后台子进程。
- 📊 **三端 Mermaid 图表语法容错自愈 (Mermaid Fallback Auto-Repair)**：
  - 在 iOS、Android 与 Web 三端的 Mermaid 渲染管线中引入 `catch` 二次语法自愈（`sanitizeMermaidCode`）：当 Agent 生成的图表包含未引号包裹的括号 `()`、花括号 `{}`、冒号 `:`、内嵌双引号 `"`、HTML `<br/>` 换行标签或非标准箭头（`—>` / `->`）触发解析报错时，自动转义修复为合法引号节点标签并原地重绘，大幅降低复杂架构图的渲染报错率。
- 🔍 **RPC 探测脚本与基准清单升级 (`ls-rpc-probe.sh`)**：
  - 升级 `scripts/ls-rpc-probe.sh` 自动从混杂内嵌 JSON 的二进制段中提取纯净的 `LanguageServerService` 方法与 `MODEL_PLACEHOLDER_*` 常量，同步更新 `ls_rpc_methods.txt` 至 Antigravity 2.30.0（153 个 RPC 方法）。
