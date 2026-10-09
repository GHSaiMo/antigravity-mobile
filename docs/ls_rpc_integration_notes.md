# language_server RPC 接入说明（实测版）

> 实测环境：Antigravity 2.22.0（macOS，standalone hub 模式），2026-10-09。
> 方法清单与形状均来自对本机真实运行的 language_server 的**只读探测**（`scripts/ls-rpc-probe.sh`），
> 字段以实测为准；未实测的标为「未验证」。升级后请先跑 `scripts/ls-rpc-snapshot.sh` 再对照本文。

## 0. 工具与回归保护

| 工具 | 作用 |
|---|---|
| `scripts/ls-rpc-snapshot.sh` | 从二进制抽取 `LanguageServerServiceHandler.<Method>` 清单（324 个），与基线 `internal/proxy/testdata/ls_rpc_methods.txt` 比对：列出新增/消失，**已使用的 RPC 消失时退出码为 1**。升级核对完后 `--update` 刷新基线 |
| `internal/proxy/ls_rpc_contract_test.go` | 离线契约测试：Go/Web/Android/iOS 源码里引用的 RPC 名必须在基线中，拼错或被移除会让 `go test` 失败 |
| `scripts/ls-rpc-probe.sh` | 对本机 language_server 发 ConnectRPC(JSON) 请求，用于探测形状；只探测只读方法 |

升级 Antigravity 后的标准动作：`scripts/ls-rpc-snapshot.sh` → 看新增/消失 → 处理 → `--update` → 提交基线。

网关对 `/exa.language_server_pb.*` 整段透传，所以下列 RPC 客户端**无需改网关**就能直接调用；
网关只在需要缓存、聚合或鉴权加固时才需要包一层 `/gateway/*`。

## 1. 已验证可用（读）

| RPC | 请求 | 响应要点 | 产品用途 |
|---|---|---|---|
| `GetAvailableModels` | `{}` | `response.models` 是 map：key 为模型 id，含 `displayName`、`recommended`、`supportsImages`、`supportsThinking`、`maxTokens`、`quotaInfo{remainingFraction,resetTime}` | 动态模型列表 + 按模型组的剩余额度（见 §3 的坑） |
| `SearchConversations` | `{"query":"..."}` | `results[]`：`cascadeId`、`title`、`workspaceName`、`lastModifiedTime`、`snippetMatchRanges[]`（偏移）、`matchSource`（如 `MATCH_SOURCE_AGENT`）、`matchedStepIndex` | 会话全文搜索，可跳转到命中的步骤 |
| `GetTrajectoryFileDiffs` | `{"conversationId"}` | `diffs[]`：`uri`、`lastModifiedAt`、`hasModelEdited`、`firstTouchedStepIndex`、`lastTouchedStepIndex`、`originalContentHash`；**只有文件级元数据，没有 diff 文本** | 「本会话改了哪些文件」列表 |
| `GetCodeActionDiff` | `{"cascadeId","stepIndex"}`，`stepIndex` 必须是 `CORTEX_STEP_TYPE_CODE_ACTION` 步骤 | `diff.unifiedDiff.lines[]`：`{text,type}`，type 为 `UNIFIED_DIFF_LINE_TYPE_INSERT/DELETE/UNCHANGED` | **逐步骤的正向 diff**（与现有回滚预览的 `RevertDiffLine` 同构，可复用渲染组件） |
| `ConvertTrajectoryToMarkdown` | `{"conversationId"}` | `{markdown}`；纯对话文本，不含代码片段（响应开头自带说明） | 会话导出 Markdown，可作长图分享之外的轻量分享 |
| `GetCascadeTrajectorySteps` | `{"cascadeId"}` | `steps[]`；**`startIndex/endIndex` 实测被忽略，会返回全部步骤**（一个 130 步会话约 800KB） | 注意流量；分页要在网关侧做 |
| `GetSidecars` | `{}` | `sidecars[]`：`sidecarId`、`config{builtin:"schedule",displayName,args:[cron,...,prompt],restartPolicy}`、`status`（如 `SIDECAR_STATUS_RUNNING`）、`startTimeMs`、`webPort`、`userConfig{enabled,projectId}` | **Automations（定时任务）列表** |
| `GetSidecarEvents` | `{"sidecarId"}` | `events[]`：`timestampMs`、`commandInvocationTimestampMs`、`payload.newConversation{prompt,conversationId}` | 每次定时运行产生的会话 id，可直接跳转 |
| `ListSidecarLogFiles` | `{"sidecarId"}` | `{logFilenames:["sidecar.log"]}`（`GetSidecarLogs` 未验证） | 定时任务日志 |
| `GetRemoteControlInfo` | `{}` | `{instanceId, companionOrigin:"https://antigravity.google.com"}` | 官方 Remote 的中继是 Google 托管域名，需 Google 账号；佐证本项目「自建链路 + 多账号」的差异化 |
| `GetConversationMetadata` | `{"conversationId"}` | `metadata{createdAt,projectId,rootConversationId,...}` | `rootConversationId` 可能用于识别子代理/分叉关系（未验证） |
| `GetWorkspaceEditState` / `GetArtifactSnapshots` | `{"conversationId"}` | 有数据返回；形状未细看 | 产物历史版本（未验证用法） |

## 2. 有必填参数、尚未打通

| RPC | 实测反馈 | 下一步 |
|---|---|---|
| `GetAgentTeamMetadata` | `project_path is required`；用 `projectPath` / `project_path` 都仍报缺失，字段名或嵌套结构未确定 | 抓桌面端发出的真实请求 |
| `GetTurnDiff` | 接受 `conversationId`，返回 `turnEndIndexExclusive` 与 `userInput`（回合边界与用户输入），**不含 diff 文本**；`turnIndex` 参数被忽略 | 用于确定回合边界，diff 文本用 `GetCodeActionDiff` 按步骤取 |
| `GetPatchAndCodeChange` | `requires eval mode`，普通模式不可用 | 放弃 |
| `GetRevisionArtifact` | `repo_path_uri is required` | 需要仓库路径，未验证 |
| `GetSlashCommands` | `plan model not specified`，需要带模型 | 带 `cascadeConfig`/模型后重试 |
| `GetCascadeModelConfigs` | 空响应 | 桌面端模型下拉的真实来源未确定，见 §3 |
| `ManageSidecar` | `invalid action`（说明接受 action 参数） | 抓包确认 action 取值（暂停/恢复/立即运行？），**写操作，需谨慎** |
| `SendAgentMessage` | `content is required`（子代理消息，另有目标字段待确定） | 抓包 |

## 3. 实测发现的坑与结论

1. **Automations 不是独立 RPC，而是 sidecar**：`config.builtin == "schedule"`，`args[0]` 是 cron，其后是 `agentapi new-conversation <prompt>`；
   每次运行触发 `payload.newConversation.conversationId`。官方技能文档（`~/.gemini/antigravity/builtin/skills/automation/SKILL.md`）里
   定义的 cron 带 `CRON_TZ=` 前缀。所以「手机上看定时任务和它产生的会话」可以只靠 `GetSidecars + GetSidecarEvents` 实现；
   「暂停/立即运行」依赖 `ManageSidecar` 的 action，尚未验证。
2. **`GetAvailableModels` 不能原样当模型下拉用**：返回 25+ 项，含内部占位（`chat_20706`）、已改名的旧 id
   （如 `gemini-2.5-flash` 的 displayName 是「Gemini 3.5 Flash Lite」，同名多 id）。需要用「有 `displayName` 且 `recommended`」过滤并按 displayName 去重，
   或找到桌面端真正使用的配置来源（`GetCascadeModelConfigs` 返回空，需抓包）。
3. **额度是按模型组共享的**：Claude 与 GPT-OSS 共用一个 `remainingFraction`（实测 0.4749，重置时间相同），Gemini 组另一个（0.9556）。
   这和 Cockpit 的 Claude/Gemini 两象限一致，可作为不依赖 Cockpit 的**免登录额度读取**，也可用来校验 Cockpit 数据。
4. **`GetCascadeTrajectorySteps` 的分页参数无效**：不要在移动端直接拉；网关已有 `trajectory_cache.go`，应继续在网关侧裁剪。
5. **LS 以 `--standalone --subclient_type hub` 运行**（2.22.0）：注意 `ps` 里能看到 `--csrf_token` 与 `--host_bridge_token`，
   这是本机敏感信息，探测脚本不打印；日志与 issue 里不要贴 `ps` 全量输出。
6. **不要执行 language_server 二进制取版本**：它不是普通 CLI，`--version` 可能起服务挂住；版本从 `Info.plist` 或进程参数的 `--override_ide_version` 读取。
7. 方法清单提取要用 `LanguageServerServiceHandler.<Method>` 字符串；`LanguageServerService/<Method>` 路径形式会因 Go 字符串拼接粘连噪声，还会漏掉部分方法（如 `RetrieveUserQuotaSummary`）。
8. `SetCascadeTrajectoryMetadata` 是旧版方法名，网关已改写为 `UpdateConversationAnnotations`（`proxy_cascade_handlers.go`），契约测试里已登记为兼容别名。

## 4. 据此调整后的接入优先级

| 优先级 | 功能 | 依赖 RPC | 状态 |
|---|---|---|---|
| 1 | 会话全文搜索 | `SearchConversations` | 已验证，可直接做 |
| 2 | 「本会话改了什么」：文件列表 + 逐步 diff | `GetTrajectoryFileDiffs` + `GetCodeActionDiff` | 已验证；diff 渲染可复用回滚预览组件 |
| 3 | 定时任务（Automations）只读视图：列表、cron、状态、历史运行→跳转会话 | `GetSidecars` + `GetSidecarEvents` | 已验证；启停/立即运行待 `ManageSidecar` 抓包 |
| 4 | 动态模型列表 + 额度 | `GetAvailableModels` | 已验证，需要过滤去重（§3.2） |
| 5 | 会话导出 Markdown | `ConvertTrajectoryToMarkdown` | 已验证 |
| 6 | 子代理可见化与对话 | `GetAgentTeamMetadata`、`SendAgentMessage`、`ForceStopCascadeTree` | 需抓包；`ForceStopCascadeTree` 已在代码中引用 |
| 7 | 终端只读输出、Git 直连 | `ListTerminals`/`StreamTerminalOutput`、`Git*` | 未验证 |

## 5. 已落地（2026-10-09）

| 功能 | 实现 | 说明 |
|---|---|---|
| 会话内容搜索 | 客户端直接调 `SearchConversations`（网关透传） | 列表页搜索框 350ms 防抖、≥2 个字符才查；结果带 `snippet` 与命中区间（**按 Unicode 码点计**，不是 UTF-16/UTF-8 偏移）；已在标题/工作区匹配里出现的会话不重复展示 |
| 导出 MD | 客户端调 `ConvertTrajectoryToMarkdown`，气泡长按「导出 MD」 | 导出的是**整个会话**的对话文本（官方说明里写明不含原始代码片段）；Android 经 FileProvider 分享 `.md`，iOS 经 `ShareSheetView` |
| 本会话改动 | 网关 `POST /gateway/cascade/changes` | 见下文「反转回滚预览」；输出结构与回滚预览一致，客户端复用 Diff 渲染 |
| Git 提交/推送 | 网关 `POST /gateway/git/{status,commit,push}` | 见下文「为什么不用 LS 的 Git RPC」 |

### 5.1 本会话改动：反转回滚预览

`GetRevertPreview(stepIndex=-1)` 返回的是「把整个会话的代码改动全部撤销」的预览，即累计净改动的**反向** diff，
且已经按文件合并了多次编辑。网关把 `INSERT/DELETE` 对调、`CREATE/DELETE` 对调，再在每个连续改动块内把删除行排到插入行之前，
就得到正向累计 diff（`internal/proxy/cascade_changes.go`）。`fromStepIndex` 非空时回滚目标取 `fromStepIndex-1`，可得到「从某一步起」的改动。

比逐步调 `GetCodeActionDiff` 更合适：后者只给单步 diff，要自己合并同文件的多次编辑；但需要看某一步改了什么时它仍然是正确入口
（`{cascadeId, stepIndex}`，`stepIndex` 必须是 `CORTEX_STEP_TYPE_CODE_ACTION` 步骤）。

### 5.2 为什么 Git 不用 LS 的 RPC

- `GitStage {workspaceUri, uris[]}`、`GitCommit {workspaceUri, message}` 可用，且确实在目标目录执行 git（未暂存时会报 `no changes added to commit`）。
- 但**拿不到变更列表**：`GetVersionControlState / GetRepoInfos / GetChangelistStatus / GetWorkspaceChangelists` 对普通 git 仓库都返回 `{}`，
  `GenerateCommitMessage` 返回 `repository does not exist`（仓库需要先登记到 LS 内部的 repo 抽象里，机制未明）。
- 因此网关直接调本机 `git`（`internal/proxy/git_actions.go`），安全边界：客户端只传 `cascadeId`，工作区由网关从会话的 `workspaces` 解析；
  待提交路径必须出现在当前 `git status` 里、不能以 `-` 开头；`push` 不带 `--force`、不接受自定义 remote；`GIT_TERMINAL_PROMPT=0`，不会卡在凭据输入上。
- 提交信息由用户填写（默认建议只列文件名）。保留了「让 Agent 写提交信息并提交」入口，行为与原来的 Commit and Push 胶囊一致。

### 5.3 排队追问：已经是原生队列，并用 tags 精确对应消息 id

- `SendAllQueuedMessages` 与 `DeleteQueuedUserInputStep` 在 language_server 里都直接返回 `deprecated`，不是可用的「原生队列接口」。
- 现行实现用的就是原生机制：`SendUserCascadeMessage` + `deliveryStrategy`（`MESSAGE_DELIVERY_STRATEGY_` 只有三个值：`UNSPECIFIED=0`、`NEXT_INVOCATION=1`、`WHEN_IDLE=2`），
  状态来自 `StreamAgentStateUpdates.pendingAgentMessages`，删除用 `DeleteAgentMessage`。
- **发送接口不返回消息 id**（响应是空对象，请求里也没有消息 id 字段），所以客户端过去只能发完等 350ms，再按文字去队列里猜哪条是自己的。
- **实验结论（2026-10-09，临时会话中实测）**：请求里的 `tags`（字符串数组）会原样存进队列条目——`pendingAgentMessages[].stepPayload` 是 base64 的 protobuf，
  其中 `field 19（user_input）→ field 16（tags）` 就是发送时的标签。两条消息同时排队时标签和文字都能准确读回。
- 删除：请求是 `{"messageId": "<队列条目的服务端 id>", "recipient": "<会话 id>"}`；按标签当 id 删会失败，必须先通过标签查到服务端 id；
  消息已被投递后再删会返回明确错误 `message has already been delivered and cannot be deleted`。
- 实现：网关在转发带 `deliveryStrategy` 的消息时，把客户端已经在发的 `X-Client-Message-Id` 写成 tag（`mgy-client:<id>`，`internal/proxy/queue_client_id.go`），
  读队列时解码出来作为 `QueuedMessageItem.clientMessageId` 返回。纯增量：旧客户端忽略该字段，没有该 tag 的消息（桌面端发的）为空。
  客户端用它精确对应乐观条目与服务端条目，查 id 时优先按 `clientMessageId`，网关支持标注但找不到时**不再回退到按文字**（避免误删另一条同文字消息）。

### 5.3b 斜杠命令

- `GetSlashCommands`（必须带一个模型的 `cascadeConfig`，网关代填）返回桌面端 "/" 菜单的全部内容：系统命令（goal / plan / grill-me / teamwork-preview / learn / boost …）加所有可调用技能（含用户自己装的）。
  每项有 `name / title / description / icon / type / modelFacingText / definitionPath`；`modelFacingText` 是真正注入给模型的文本（plan 约 3.7KB，teamwork-preview 约 15KB），所以菜单接口**不下发它**。
- 发送：`items[]` 里放一个 `ContextScopeItem`：`{"item":{"slashCommand":{"info":{...}}}}`（协议里 `TextOrScopeItem = {text | item}`，`ContextScopeItem.slash_command → SlashCommandScopeItem{info, argument_values}`）。
  实测 language_server 会把它记进会话，**显示文本自动生成为 `/name 用户输入`**，模型确实收到了 `modelFacingText`（回复了技能名）。
  文本条目前补一个空格，显示文本就是 `/plan 内容` 而不是 `/plan内容`。服务端对未知 JSON 字段是静默忽略的，所以字段名只能靠协议描述 + 真实发送验证。
- 网关 `GET /gateway/slash-commands`（`internal/proxy/slash_commands.go`）：缓存 2 分钟；**隐藏桌面端专用的命令**（schedule / automation / browser / generative_ui / ui-extension / plugin / migrate-workflows / agy-customizations / antigravity-guide），
  真实环境下可见 10 项（6 个系统命令 + 4 个技能）。发送时网关把客户端只传的 `{"info":{"name":"plan"}}` 替换成权威 info（含 `modelFacingText`），
  所以客户端轻量、也无法伪造注入文本；未知命令返回 400 且不会到达 language_server；命令名参与重复消息去重，避免「同样文字、不同命令」被误吞。
- 附件块只并入文本条目，不再塞进 `items[0]`（那是命令条目，`text` 与 `item` 互斥）。
- 新会话（草稿）带命令时先建空会话再发送，因为创建接口的初始 prompt 只支持纯文本。
- 未验证：工作区级别的技能（`GetSlashCommands` 传工作区参数结果不变，本仓库没有工作区技能可测）。

### 5.4 模型列表动态化与默认模型（2026-10-09）

- `GET /gateway/models`（`internal/proxy/models.go`）：把 `GetAvailableModels` 整理成客户端可选列表并缓存 5 分钟（失败后 30 秒内不再重试，取不到时返回内置兜底并标记 `live:false`）。
  整理规则：丢弃内部占位（`chat_*`、`isInternal`）和 `recommended=false`；**同一展示名下的多个 id 只留一个**（旧的 `2.5` 别名、`-agent` 别名输给带真实版本号的 id）；
  **退役的 `MODEL_GOOGLE_GEMINI_2_5_*` 枚举不列出**（网关会把它们改映射到 3.8 Flash，列出来等于用户选 A 跑 B）；
  按厂商分组、版本新→旧、同版本 High→Medium→Low 排序。

  **实际使用厂商自己的选择器列表**：`GetAvailableModels` 的响应里除了 `models` 还有 `agentModelSorts`（桌面端「Recommended」选择器的成员与顺序）、
  `defaultAgentModelId`（厂商默认模型）和每个模型的 `tagTitle`。列表成员与顺序以 `agentModelSorts` 为准，上面的启发式规则只在它缺失/为空时作为兜底；
  厂商默认模型优先作为 Gemini 默认值。**带 `tagTitle: "Leaving Soon"` 的模型不提供选择**（仍可按 id 解析，旧会话还在用它时不受影响）。
  2026-10-09 的真实数据下得到 3 个 Gemini（3.8 Flash 的 High/Medium/Low）+ 2 个 Claude + GPT-OSS；3.7 / 3.6 Flash 与 3.1 Pro 均为 Leaving Soon，被排除。
- 网关里原来手写的 `modelEnumMap / enumToCanonicalMap` 保留作兜底，但 **id→枚举优先查实时列表**，枚举→id 在静态表未命中时用实时列表反查，
  所以新模型不用改代码就能被选择、发送和显示。
- 设置里的「默认模型」分别为 Gemini 和 Claude 各选一个具体模型，保存在手机本地；输入框上方的 Gemini / Claude 胶囊保持只显示厂商名，点击切换到对应的默认模型。
  已有会话继续沿用它正在使用的具体模型（`activeModel` 不再被压成两个固定值）；已保存的默认模型若不在实时列表里，打开设置时自动换成网关建议的默认值。
- 每条 Agent 消息带 `model` / `modelName`（来自步骤元数据 `generatorModel`，枚举→id→展示名），会话级带 `activeModelName` 与 `startedAt`（第一个带时间戳的步骤）。

### 5.5 长图分享

- 模型名：Agent 回复用生成它的模型；用户提问用紧随其后那条回复的模型；都没有时回退到会话当前模型的展示名（再不行才由 id 推断）。
- 时间：显示**会话发起时间**（本地时区，`yyyy-MM-dd HH:mm`），不再是分享当下；拿不到发起时间时不显示日期，而不是用分享时间冒充。

### 5.6 低额度预警与数据口径（2026-10-09）

- **数据以 Cockpit 的 4 个窗口为准**（Claude/Gemini × 5h/周）。`GetAvailableModels` 里每个模型的 `quotaInfo` 每个厂商只给一个比例，
  且选哪个窗口不固定（实测 Claude 给的是周额度、Gemini 给的是 5h），不能当权威，也不能用来做预警。
- 预警规则（`internal/cockpit/quota_alert.go`）：只监控**当前账号**（以 language_server 实际在用的账号为准）的 **Gemini 5h**；
  ≤20% 推送提醒、≤5% 推送紧急，每个「账号 × 重置周期」每一档只推一次（状态落盘 `~/.multigravity/quota_alert_state.json`，重启不重复）；
  一次性掉过两档只推紧急；不新增任何界面显示。
- 推送正文：脱敏账号（推送经过第三方推送服务）、按当前时间现算的重置倒计时与时钟，以及「按最近 60 分钟速度预计几点用完」——
  最小二乘外推，样本不足 3 个 / 跨度不足 15 分钟 / 几乎没消耗 / 预计晚于重置时间时都不写。紧急档用 `timeSensitive` 级别 + 告警音，
  不用 Bark 的 `critical`（它会无视静音开关）。
- 环境变量：`QUOTA_ALERT_ENABLED`、`QUOTA_ALERT_WARN_PERCENT`、`QUOTA_ALERT_CRITICAL_PERCENT`（非法值回退 20/5）。
  走现有 Bark / FCM 通道；两者都没配置时不会有任何推送。

### 5.7 升级自检（2026-10-09）

- 网关每连上一个新的 language_server 实例（PID / 版本 / 二进制变化）就自检一次，后台执行：读该进程**二进制里的
  `LanguageServerServiceHandler.<方法>` 符号清单**（只读文件，不发请求，没有副作用；真实 150MB 文件约 25ms），
  对照 `internal/proxy/capabilities.go` 的「功能 → 依赖 RPC」表，结果放在 `GET /gateway/status` 的 `compat`：
  `{version, checked, coreOk, unavailable[], missingRpcs[]}`（不含二进制路径）。
- 二进制路径与版本号：Linux 读 `/proc/<pid>/exe`，macOS 用 `ps -o comm=`，Windows 用 `Get-CimInstance`；版本取自命令行
  `--override_ide_version`（daemon 模式取 daemon 文件里的 `lsVersion`）。读不到二进制 → `checked=false`，**一律放行**，不隐藏任何入口。
- 客户端：`unavailable` 里的功能 ID 对应的入口直接不显示（`search` 内容搜索、`export` 导出 MD、`changes` Changes 胶囊、
  `revert` 撤回、`slash` 斜杠命令）；`coreOk=false`（会话列表 / 读取 / 发送 / 流式 / 审批缺失）时首页顶部显示
  「当前 Antigravity 版本与网关不兼容，请升级网关」提示条（包括列表加载失败的页面）。字段缺失（旧网关）、解析失败一律按放行处理。
  `models` 功能有网关内置列表兜底，不据此隐藏。
- **功能表由契约测试守住**：源码里用到的每个 RPC 必须登记在 `Features` 里（否则新增依赖忘了登记，升级后手机端不会隐藏入口），
  表里的 RPC 必须存在于基线，客户端依赖的功能 ID 不允许改名。新增依赖某个 RPC 的功能时，先加进表，测试会提醒。
- **只检测方法名**。方法还在、字段结构变了的情况检测不到（这类问题靠 `scripts/ls-rpc-snapshot.sh` + 解析 golden 测试 + 真机验证）；
  按约定也不对「Antigravity 版本高于已验证基线」做提示，避免每周升级都打扰。
