# 功能设计说明：网关源头事件感知与移动端通知推送 (方案 A)

> **定位**：供后续网关端（Go）与移动端（iOS / Bark）开发直接参考的技术实现规格说明书。

---

## 1. 方案背景与目标

在 Antigravity 运行过程中，Agent 常常需要执行长时间的编码、重构或运行测试；同时在关键节点（如执行危险 Shell 命令、写文件、向用户提问、完成方案规划等待确认）会挂起并等待用户操作。

* **核心目标**：当用户离开电脑（如手持 iPhone 在外或离开桌面）时，能在**任务完成**或**需要用户审批操作**的第一时间收到手机端系统级横幅通知，点击通知可一键直达对应会话。
* **架构选型（方案 A：网关源头感知 + Bark 轻量分发）**：
  * **源头白盒感知**：网关本身与 Antigravity `language_server` 实时同步状态，在内存中直接捕获状态机流转与交互事件，**不依赖、不抓取 macOS 系统的通知栏**，具备零系统权限依赖、低延迟、结构化数据完整的优势。
  * **轻量跨端分发**：首期通过开源成熟的 **Bark**（iOS 端 APNs 推送代理）进行分发，免去申请 $99/年 苹果开发者账号与证书配置的复杂度；后续架构保留无缝平替为原生 APNs 的能力。

---

## 2. 总体架构设计

```text
[ Antigravity Language Server ]
             │ (本地 IPC / 状态流转)
             ▼
[ 本地网关 (internal/proxy) ]
  ├── 1. 状态监测钩子 (Notification State Hook)
  │     ├── 实时解析 TrajectoryDetails (Status, PendingInteraction, CanProceed)
  │     └── 事件去重与防抖引擎 (Deduplication / Fingerprint Engine)
  │
  ├── 2. 通知事件调度器 (Notifier Dispatcher)
  │     ├── 组装结构化通知（标题、正文、优先级、提示音）
  │     └── 注入 DeepLink: antigravity://cascade/{cascadeId}
  │
  └── 3. 推送通道适配器 (Provider Adapter)
        ├── [首期默认] Bark Provider ──(HTTPS POST)──> api.day.app / 自建 Bark
        └── [后续扩展] Native APNs Provider ──(HTTP/2)──> Apple APNs Gateway
                                                               │
                                                               ▼
                                                   [ 用户 iPhone (锁屏横幅) ]
                                                               │ (用户点击通知)
                                                               ▼
                                                   [ Antigravity iOS App ]
                                                   (自动跳转并打开对应会话)
```

---

## 3. 触发时机与事件映射规范

网关在轮询/流式解析 `TrajectoryDetails` 时，精确检测以下三类关键事件：

### 3.1 事件类型与判定规则

| 事件类型 | 触发判定条件 | 推荐优先级 / 声音 | 业务含义 |
| :--- | :--- | :--- | :--- |
| **`ACTION_REQUIRED`** | `PendingInteraction != nil` (包含 `permission`, `ask_question`, `run_command`, `file_permission`) | `level: timeSensitive`<br>`sound: alarm` | Agent 遇到权限受限或向用户提问，任务挂起等待决策 |
| **`PLAN_CONFIRMATION`** | `CanProceed == true` 且前一状态为生成中 | `level: timeSensitive`<br>`sound: anticipation` | 实施方案已生成，等待用户点击 Proceed 确认 |
| **`TASK_COMPLETED`** | `Status` 从 `CASCADE_RUN_STATUS_RUNNING` 变为 `CASCADE_RUN_STATUS_COMPLETED` | `level: active`<br>`sound: success` | 整个 Agent 任务全部顺利执行完成 |
| **`TASK_FAILED`** | `Status` 变为 `CASCADE_RUN_STATUS_FAILED` 或异常终止 | `level: active`<br>`sound: failure` | 任务执行失败或遇到致命异常 |

### 3.2 消息体构建规则

#### (1) 操作审批 (Action Required)
* **标题**：`⚠️ Antigravity 需要审批`
* **正文**：
  * 若为命令审批：`Agent 申请执行终端命令: {command}`
  * 若为写文件审批：`Agent 申请修改文件: {filename}`
  * 若为提问：`Agent 提出了新问题: {question_title}`
* **跳转链接**：`antigravity://cascade/{cascadeId}?action=review`

#### (2) 方案待确认 (Plan Confirmation)
* **标题**：`📋 方案已就绪，等待确认`
* **正文**：`Agent 已完成实施计划编写，点击以确认执行。`
* **跳转链接**：`antigravity://cascade/{cascadeId}`

#### (3) 任务完成 (Task Completed)
* **标题**：`🎉 任务执行完成`
* **正文**：`「{cascadeTitle}」已完成，共执行 {totalSteps} 个步骤。`
* **跳转链接**：`antigravity://cascade/{cascadeId}`

---

## 4. Bark 协议集成规范

Bark 提供标准的 REST HTTP API，网关异步通过 `net/http` 发送请求。

### 4.1 请求方式与参数

* **Endpoint**：`https://api.day.app/{bark_key}/` (或自建服务器地址)
* **Method**：`POST`
* **Content-Type**：`application/json`

**请求 Payload 结构**：
```json
{
  "title": "⚠️ Antigravity 需要审批",
  "body": "Agent 申请执行终端命令: npm run build",
  "category": "antigravity_action",
  "group": "Antigravity",
  "level": "timeSensitive",
  "badge": 1,
  "sound": "alarm",
  "icon": "https://raw.githubusercontent.com/taojiuzhen/antigravity/main/assets/icon.png",
  "url": "antigravity://cascade/cas_c8d9e1f2?action=review"
}
```

* `group`：统一设为 `Antigravity`，通知在 iOS 通知中心会自动折叠归类，不占满屏幕。
* `level`：
  * `timeSensitive`（时效性通知）：可穿透 iOS 勿扰模式/专注模式显示。
  * `active`（普通即时通知）：亮屏并发出声响。
* `url`：注入客户端专属 URL Scheme，点击后无缝唤醒并导航至目标 Cascade。

---

## 5. 去重与防抖机制 (Deduplication & Debounce)

由于网关的 Stream 轮询可能以 50ms~500ms 的频率持续扫描，必须防止向手机重复轰炸同一通知。

### 5.1 事件指纹 (Notification Event Key)
每个通知在发送前计算一个唯一特征 Key：
* **审批事件 Key**：`fmt.Sprintf("act:%s:%d:%s", cascadeId, stepIndex, pendingType)`
* **完成事件 Key**：`fmt.Sprintf("done:%s:%d", cascadeId, totalSteps)`

### 5.2 内存滑动窗口与缓存
* 网关内存中维护一个 `sync.Map` 或带 TTL 的 LRU 缓存（默认过期时间 1 小时）。
* 发送前检查 `cache.Contains(eventKey)`：
  * 若已存在，则**直接跳过**；
  * 若不存在，执行发送，并将 `eventKey` 写入缓存。
* 当 Cascade 重新启动新一轮执行（`Status == RUNNING`）时，清理历史的完成态事件 Key。

---

## 6. 网关模块配置规范

在网关配置（环境变量或配置文件 `config.json`）中加入独立的通知配置节：

```json
{
  "notifications": {
    "enabled": true,
    "provider": "bark",
    "bark": {
      "server_url": "https://api.day.app",
      "device_key": "YOUR_BARK_DEVICE_KEY",
      "sound_on_action": "alarm",
      "sound_on_completed": "glass"
    },
    "events": {
      "action_required": true,
      "task_completed": true,
      "task_failed": true
    }
  }
}
```

支持通过环境变量覆盖：
* `ANTIGRAVITY_NOTIFY_ENABLED=true`
* `ANTIGRAVITY_BARK_KEY="abcdef123456"`
* `ANTIGRAVITY_BARK_SERVER="https://api.day.app"`

---

## 7. 客户端路由配合 (iOS URL Scheme)

在客户端 `AntigravityApp.swift` 中注册对 `antigravity://` 的监听：

```swift
@main
struct AntigravityApp: App {
    @StateObject private var router = NavigationRouter.shared
    
    var body: some Scene {
        WindowGroup {
            ContentView()
                .onOpenURL { url in
                    // 解析 antigravity://cascade/{cascadeId}
                    if url.scheme == "antigravity", url.host == "cascade" {
                        let cascadeId = url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                        router.navigateToCascade(cascadeId)
                    }
                }
        }
    }
}
```

---

## 8. 实施路径规划

1. **第一阶段（网关事件感知与通知调度）**：
   - 在 `internal/notifier` 包中实现 `Notifier` 接口与 `BarkProvider`。
   - 在 `internal/proxy/stream.go` 与 `trajectory.go` 中挂载状态变更 Hook 与事件去重逻辑。
   - 增加环境变量与配置读取。
2. **第二阶段（移动端 DeepLink 唤醒）**：
   - 在 iOS 客户端 `Info.plist` 注册 `antigravity` URL Scheme。
   - 在主入口中实现 `onOpenURL` 处理逻辑，实现点击通知后自动选中并高亮显示该 Cascade。
3. **第三阶段（可选扩展：自建 APNs）**：
   - 当具备 Apple 开发者账号后，在 `internal/notifier` 中增加 `ApnsProvider`，无需调整业务判断逻辑，平滑升级原生系统级通知卡片与快捷按钮。
