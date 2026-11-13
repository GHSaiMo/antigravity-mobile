# 移动端「连接心跳」指示器方案

> 目标：打开 App 时一眼看出「手机 ↔ 网关 ↔ 电脑端 Antigravity」是否真的连着，不再靠猜。

## 1. 现状调研

| 层 | 现有机制 | 缺口 |
|---|---|---|
| 网关 `/gateway/events` (`internal/proxy/events.go`) | 已有常驻 WS：连接发 `hello`，列表变化发 `changed`；服务端每 20s 发 WS Ping，45s 读超时 | Ping/Pong 在协议层，**客户端拿不到**（iOS `autoReplyPing`、OkHttp、浏览器均不暴露），所以 UI 无法据此判断「最近一次收到服务端消息是什么时候」 |
| `GET /healthz` (`cmd/gateway/main.go`) | 无鉴权，仅表示网关进程活着 | 不反映电脑端 language_server 是否可用 |
| `GET /readyz` | 反映 `insp.Current().IsHealthy && Port>0`，即电脑端 LS 是否在线 | 无鉴权也有，但客户端未用于 UI；返回 503 时无结构化原因 |
| iOS `ConversationEventsClient` | 有 `isConnected`，指数退避重连、watchdog | 仅内部使用，无 UI 展示、无最近心跳时间 |
| Android `ConnectionManager` / `ConversationEventsClient` | 有 `endpointStatuses`、`/healthz` 探测、`lastProbeTime` | 只在设置/多端点页面使用，列表页没有常驻状态 |
| Web PWA `web/js/nav.js` | `connectListEvents` 有 onclose 重连 | 无 UI |

**结论**：链路已经具备「复用的长连接 + 服务端定时发包」，只差一个**应用层心跳帧**把「电脑端是否在线」带给客户端，再加一个 UI 点。不需要新增连接或轮询。

## 2. 方案总览

```
手机 ──WS(/gateway/events)── 网关 ──(inspector)── 电脑端 language_server
        ▲ beat 帧每 5s（含 desktop 状态）
```

- **服务端**：在 `/gateway/events` 新增应用层帧 `beat`，周期 5s，携带电脑端状态。
- **客户端**：记录 `lastBeatAt`，按「距上次心跳多久」判定三态，在会话列表页标题栏旁显示一个小圆点（点击可看详情）。
- 不做额外 HTTP 轮询；WS 断开时才回退到 `/healthz` 探测用于区分「网关不可达」与「网络本身不通」。

## 3. 协议改动（向后兼容）

`eventsFrame` 增加字段，旧客户端忽略未知 `type`/字段即可：

```json
{"type":"beat","rev":123,"ts":1763004000123,"desktop":"ok"}
```

- `ts`：服务端毫秒时间戳（可用于估算 RTT/时钟偏差，仅展示用）。
- `desktop`：`ok`（`insp.Current()` 健康且 Port>0）/ `down`（电脑端 LS 不可用）/ `switching`（账号切换中，可选）。
- `hello` 同样补上 `desktop`，保证连上即有状态。
- `desktop` 状态变化时**立即**额外推一帧，不必等下个 5s。
- 5s 间隔远低于 20s Ping 与常见隧道空闲超时，也不额外唤醒 detector（detector 仍只在有订阅者时跑）。
- 服务端改动点：`internal/proxy/events.go` 的 `HandleEvents` 循环里加一个 5s ticker；需要把 inspector 注入 `Proxy`（`p.insp` 已存在，见 `automations.go`）。

## 4. 客户端状态与 UI（已与你确认）

**位置**：首页「额度条」（iOS `QuotaStatusBarView`）内部**最右侧**加一个小圆点，与标题 Multigravity 无关，不占导航栏，不碰彩蛋点击区。Android 对应 `ConversationListScreen` 里的额度条，Web 对应 `#quota-status-bar` 胶囊的 `.quota-capsule-right`。

| 状态 | 判定 | 圆点 |
|---|---|---|
| 已连接 | WS 已连 且 距上次 `beat` < 12s 且 `desktop=ok` | 绿，每收到一次 `beat` 轻微呼吸/闪一下 |
| 注意 | WS 已连但 `desktop=down`；或 12s ≤ 距上次心跳 < 30s；或重连中 | 黄 |
| 断开 | WS 未连 或 距上次心跳 ≥ 30s | 红 |

- **交互**：点击圆点不单独处理，整条照旧打开「账户与配额」面板；面板保持纯粹的账号与配额展示，心跳状态专注于首页额度条最右侧圆点。
- **额度条消失时**：接受消失（无额度数据或未拉取到时圆点一并没有），不做额外兜底。注意：断开后已缓存的额度响应仍在，条会保留、圆点变红，所以这是有效信号。
- **回前台**：`scenePhase=.active` / `onResume` / `visibilitychange` 时若心跳已过期，圆点先置黄并立即强制重连，不沿用旧绿点。
- **无障碍**：圆点带 accessibilityLabel（「已连接电脑」「连接异常」「已断开」），不只靠颜色。
- 阈值用客户端本地单调时钟，不依赖服务端 `ts`。

## 5. 各端落地

### Go 网关
1. `eventsFrame` 加 `TS int64`、`Desktop string`；`HandleEvents` 加 5s `beat` ticker 与状态变化推送。
2. `/readyz` 返回体补 `reason`（可选）；保持无鉴权。
3. 测试：`internal/proxy/events_test.go` 增加用例——连接后 ≤1 个周期内收到 `beat`；模拟 inspector 不健康时 `desktop=down`；`eventsBeatInterval` 做成可被测试缩短的变量（同 `eventsPollInterval` 风格）。

### iOS（`ConversationEventsClient.swift` + `ConversationListView.swift`）
- 新增 `@Observable` 字段 `lastBeatAt: Date?`、`desktopState`、`linkState`（计算属性）。
- `receive` 解析 `beat`；用 `TimelineView(.periodic)` 或 1s Timer 刷新相对时间文案。
- 列表页导航栏加 `ConnectionDotView`，点击弹 popover/sheet。
- 场景激活时检查并强制重连。

### Android（`ConversationEventsClient.kt` + 列表页顶栏）
- `StateFlow<LinkState>`；`OkHttp WebSocketListener.onMessage` 解析 `beat`；协程 1s tick 派生状态。
- 复用 `ConnectionManager.testSingleEndpoint` 作为断开时的 `/healthz` 兜底，区分「手机没网」「网关不可达」。
- 顶栏加 `ConnectionDot` Composable；`onResume` 触发检查。

### Web PWA（`web/js/nav.js`）
- `connectListEvents` 的 `onmessage` 处理 `beat`；头部加一个 `<span class="link-dot">`；`visibilitychange` 时检查并重连。

## 6. 边界与风险

- **省电**：5s 小帧在 WS 已存在的前提下开销极小；App 进后台系统本就会挂起 WS，不额外保活。
- **多端点/中继**：经 Cloudflare/中继时 WS 可能「半开」，所以必须以应用层 `beat` 时间而非 `isConnected` 判定，这也是不直接用现有 `isConnected` 的原因。
- **兼容**：新网关 + 旧客户端无影响；新客户端 + 旧网关收不到 `beat`——需降级：无 `beat` 但收到 `hello`/Ping 时仅显示「已连接」（二态），不报红。可用 `/api/v1/version` 或 `compat` 能力位判断。
- **鉴权**：`beat` 走已鉴权的 `/gateway/events`，不泄露额外信息。

## 7. 工作量与排期

| 阶段 | 内容 | 估时 |
|---|---|---|
| 1 | Go 网关 beat + 测试（含 `scripts/test-on-nas.sh` Linux 验证） | 0.5 天 |
| 2 | iOS 状态机 + UI | 0.5–1 天 |
| 3 | Android 状态机 + UI | 0.5–1 天 |
| 4 | Web PWA | 0.25 天 |
| 5 | 联调（LAN/中继/断网/杀电脑端/切后台回前台） | 0.5 天 |

## 8. 验收清单（对应 AGENTS.md 门禁）

- [ ] Go：`go test -count=1 ./...`、`go build -o /dev/null ./cmd/gateway`、`scripts/test-on-nas.sh`
- [ ] Android：`cd android && ./gradlew compileReleaseKotlin`（不得改动签名配置）
- [ ] iOS：`xcodebuild ... clean build`
- [ ] 手测：正常→绿；电脑退出 Antigravity→黄；关网关/断 Wi-Fi→红；切后台 1 分钟回前台→不沿用旧绿点并快速恢复
- [ ] 本机网关重启一律在 tmux `mgy` 会话中执行
- [ ] 若同时 bump 版本号，需提醒是否同步 groupchat runtime 的 zzjs / vip 知识库

## 9. 已确认决策

1. 位置：首页额度条最右侧小圆点（非导航栏）。
2. 额度条消失即接受消失。
3. 三态圆点；点击打开「账户与配额」面板，心跳状态专注于首页额度条最右侧圆点。
4. 三端（iOS/Android/Web）一起做，网关加 5s `beat` 帧，复用 `/gateway/events`。
