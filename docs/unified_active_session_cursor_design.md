# 统一活跃会话游标管理与双端协同感知设计规范
(Unified Active Session Cursor Management & Cross-Device Focus Specification)

> **版本**：v1.2 (2026-09-15)  
> **更新记录**：  
> - **对齐 Swift 6 并发基线（Commit `906a54f`）**：将 iOS 端焦点上报从裸 `URLSession.shared` 升级为 `NetworkTransport.shared` 异步任务，遵循 `Sendable` 规范并消除非结构化并发警告；  
> - **兼容 IPv6 直连与 ATS 绕过（Commit `a184736` / `b4eaa1a`）**：通过 `NetworkTransport` 统一调度，防止普通 `URLSession` 对公网 IPv6 字面量触发 Apple ATS 阻断；  
> - **新增防自反机制（Anti-Reflection Protection）**：彻底解决手机端标记已读时触发 `UpdateConversationAnnotations` 写入磁盘，导致文件监听器将“手机已读”误判为“桌面鼠标点击”的焦点回环 Bug；  
> - **对齐安全与性能审计加固（Commit `1f3642b`）**：落地 S6 `writeJSONError` 统一错误契约、S9 WebSocket/HTTP 鉴权加固（短生命周期 One-Time Ticket 规范）与 P7 立即首推；  
> - **整合第一层 YoooClaw 会话标题守卫（`is_antigravity_session`）**：与第二层统一游标仲裁构成“双层协同架构”；  
> - **细化幽灵会话三重防御过滤**：强制校验 Title 非空非未命名、Brain 工作区目录物理存在、Subagent 内部会话剔除。

---

## 1. 架构目标与系统拓扑

### 1.1 核心目标
构建**“随人而动（Follow-Me）”的跨端会话焦点游标引擎**：
外设硬件（C·ONE / YoooClaw）、手机端（Antigravity iOS App）与桌面端（Antigravity IDE）深度协同，在**毫秒级**内准确感知用户“当前眼睛聚焦的 Cascade 会话”，并将硬件语音对讲、快捷指令或文本输入**100% 精准注入目标会话**，彻底解决串会话、延迟丢消息和幽灵空白会话 500 报错。

### 1.2 双层协同分流全景架构

系统按职责划分为**两层递进路由**：
* **第一层：业务流向分流（Session Boundary Guard）**  
  发生在 Hermes 消息入站口（`yoooclaw_app/adapter.py`）。通过比对当前 App 会话的 `sessionId` 与标题：
  - 若为【通知总结】或普通 Hermes 会话：**100% Bypass 放行**给 Hermes 原生处理；
  - 只有会话标题包含【反重力】或【antigravity】时，才进入第二层。
* **第二层：精准游标仲裁（Follow-Me Cursor Engine）**  
  由 Antigravity Mobile 网关（端口 58900）主导。在确认要注入反重力后，根据手机端与桌面端的最新聚焦动作，毫秒级判定具体注入到哪一个 Cascade 代码会话。

```
                    ┌─────────────────────────┐
                    │    物理硬件 / 对讲输入    │
                    │   (C·ONE / YoooClaw)    │
                    └────────────┬────────────┘
                                 │ 语音识别后文本送出 (带 sessionId)
                                 ▼
                    ┌─────────────────────────┐
                    │  第一层：Hermes 守卫层   │
                    │ (yoooclaw-hermes-plugin)│
                    └────────────┬────────────┘
                                 │
           ┌─────────────────────┴─────────────────────┐
           │ (非反重力会话, 如通知总结)                    │ (标题含反重力/antigravity)
           ▼                                           ▼
┌──────────────────────┐                     ┌───────────────────────────┐
│ Hermes Agent 原生处理 │                     │  第二层：网关统一游标中心   │
│ (日常闲聊/知识库/工具)  │                     │   (Port 58900 Gateway)    │
└──────────────────────┘                     └─────────────┬─────────────┘
                                                           │
                                            动态时间戳仲裁 (Follow-Me)
                                                           │
                           ┌───────────────────────────────┴───────────────────────────────┐
                           ▼                                                               ▼
                 ┌──────────────────┐                                            ┌──────────────────┐
                 │  手机端即时焦点   │                                            │  桌面端即时焦点   │
                 │ (Antigravity iOS)│                                            │ (Antigravity IDE)│
                 │  - 点击即发 Ping  │                                            │  - FSEvents 监听 │
                 │  - 息屏粘性保持   │                                            │  - 幽灵会话过滤   │
                 └──────────────────┘                                            └──────────────────┘
```

---

## 2. 现有机制深度逆向与痛点剖析

### 2.1 移动端现行痛点
目前网关通过 WebSocket 长连接（`ws://127.0.0.1:58900/gateway/cascade/stream?cascadeId=xxx&ticket=...`，基于 Commit `1f3642b` S9 规范，先调用 `POST /api/v1/auth/ws-ticket` 换取 30 秒一次性凭据）来标识手机活跃会话：
1. **进入时延迟（Timing Gap）**：
   手机端 `ChatView` 必须先执行 `await viewModel.loadMessages()`（HTTP 拉取完整消息），拉取成功后才换取 WS Ticket 并建立 WebSocket。从用户手指点击会话列表项到 WebSocket 握手成功并触发 P7 首推，存在 **0.5s ~ 2s 的感知空档**。在此期间说话，网关处于“手机无活跃会话”状态。
2. **息屏与切后台立刻“失忆”**：
   iOS 系统规范要求 App 切后台或息屏时断开 WebSocket。当用户在手机上选好会话、息屏把手机装进口袋时，iOS 触发 `disconnectStream()`。网关在 `defer p.ClearActiveStream()` 中**立即将活跃会话清空为 `""`**！
   此时通过 C·ONE 硬件对讲，网关因手机端无会话而盲退到电脑桌面，导致“明明在手机上选好了会话，息屏一按硬件却打进了电脑会话”。

### 2.2 桌面端焦点逆向真相
通过对 Antigravity 桌面端（Electron IDE）及核心进程（PID 80981 `language_server`）的系统调用逆向分析证实：
* **官方底层事件通道**：用户在电脑 IDE 中每次切换 Tab 标签页或在左侧历史列表中点击会话，IDE 会立即向本地 `language_server` 发起 `UpdateConversationAnnotations` RPC。
* **磁盘实时落地**：`language_server` 立即同步写入本地元数据文件：
  `~/.gemini/antigravity/annotations/<cascade_id>.pbtxt`
  ```protobuf
  title:"雪球雷达扫描与反爬排查" last_user_view_time:{seconds:1789438083 nanos:429000000} marked_as_unread:false
  ```
  该文件的修改时间（mtime）和内部 `last_user_view_time` 会在 **毫秒级内刷新**，是极高可靠性的桌面焦点指标。

### 2.3 幽灵会话陷阱（Ghost Session Trap）
* 桌面 IDE 在点击新建、或启动临时标签页时，会产生尚未初始化、没有标题、没有 `brain` 工作区目录的空 `.pbtxt` 文件。
* 若直接按最后查看时间排序，网关极易错误选中这类“幽灵空白会话”，向其发送消息将引发 LanguageServer 返回 `HTTP 500: Internal Server Error`。
* **铁律过滤准则**：必须同时满足 **`title` 非空且不为未命名** 且 **`~/.gemini/antigravity/brain/<cascade_id>` 目录存在**，才承认为有效真实会话。

---

## 3. 统一游标管理与双端仲裁引擎设计

### 3.1 数据模型（Go 网关层）

```go
type CursorSource string

const (
    CursorSourceMobile  CursorSource = "mobile"
    CursorSourceDesktop CursorSource = "desktop"
)

type UnifiedCursor struct {
    CascadeID   string       `json:"cascade_id"`
    Title       string       `json:"title"`
    Source      CursorSource `json:"source"`
    UpdatedAt   time.Time    `json:"updated_at"`
    IsSticky    bool         `json:"is_sticky"`     // 是否处于手机息屏粘性保持状态
    TimeSkewMs  int64        `json:"time_skew_ms"`  // 相对最新动作的偏移
}
```

网关内部维护状态：
* `mobileCascadeID`: 手机端最后上报的会话 ID
* `mobileTitle`: 手机端会话标题
* `mobileFocusedAt`: 手机端最后聚焦时间戳
* `mobileIsStreaming`: 手机当前是否保持着 WebSocket 活跃流
* `desktopCascadeID`: 电脑端最后聚焦的会话 ID
* `desktopTitle`: 电脑端会话标题
* `desktopFocusedAt`: 电脑端最后聚焦时间戳

### 3.2 动态游标仲裁规则（Follow-Me Engine）

当需要获取当前全局游标（例如 `/gateway/status` 探测或网桥注入）时，网关执行以下裁决：

```
                              [触发游标查询]
                                    │
                  ┌─────────────────┴─────────────────┐
                  ▼                                   ▼
        手机端时间戳 (t_mobile)             电脑端时间戳 (t_desktop)
                  │                                   │
                  └─────────────────┬─────────────────┘
                                    │
                                    ▼
                         比对 t_mobile 与 t_desktop
                                    │
             ┌──────────────────────┴──────────────────────┐
             │                                             │
      t_mobile > t_desktop                          t_desktop >= t_mobile
             │                                             │
             ▼                                             ▼
  【手机端动作较新】                               【电脑端动作较新】
  - 检查手机 WS 是否在线                           - 校验电脑会话是否为真实会话
    ├─ 在线 ➔ 游标锁定手机 (非粘性)                   (排除无 title、无 brain 幽灵会话)
    └─ 离线 ➔ 检查 t_mobile 是否在                 - 游标锁定电脑桌面当前活跃会话
       TTL (30分钟) 内                                (Source: Desktop)
       ├─ 在内 ➔ 游标锁定手机 (Sticky 粘性模式)
       └─ 超时 ➔ 回退至电脑端有效会话
```

---

## 4. 接口与客户端改造规范（符合 Commit 1f3642b 安全基线）

### 4.1 网关端：新增轻量 Focus 接口（落地 S6 与 S9 安全规范）

#### `POST /gateway/cascade/focus`
手机端点击会话第一毫秒调用的轻量接口（耗时 `< 3ms`）。

* **请求方式**：`POST`
* **路径**：`/gateway/cascade/focus`
* **鉴权要求（安全审计基线）**：
  依据 Commit `1f3642b` 安全规范，**标准 HTTP 请求禁止在 URL query 传参 long-lived token**。必须携带 HTTP Header：
  ```http
  Authorization: Bearer <device_token>
  Content-Type: application/json
  ```
* **请求体（JSON）**：
  ```json
  {
    "cascadeId": "5b16507c-5aa5-4713-9d9c-ae6b4bb97316",
    "source": "ios"
  }
  ```
* **网关处理实现（Go 伪代码，严格遵循 S6 writeJSONError 契约）**：
  ```go
  func (p *Proxy) handleFocusSession(w http.ResponseWriter, r *http.Request) {
      if r.Method != http.MethodPost {
          writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
          return
      }

      // 1. S9 鉴权提取: 严格通过 Bearer Header (禁止 URL query)
      token := ExtractToken(r)
      if token == "" {
          writeJSONError(w, http.StatusUnauthorized, "Missing authentication token")
          return
      }
      if dev, ok := p.authStore.ValidateToken(token); !ok || dev == nil {
          writeJSONError(w, http.StatusUnauthorized, "Invalid or revoked token")
          return
      }

      var req struct {
          CascadeID string `json:"cascadeId"`
          Source    string `json:"source"`
      }
      if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&req); err != nil || req.CascadeID == "" {
          writeJSONError(w, http.StatusBadRequest, "Invalid payload or missing cascadeId")
          return
      }

      // 2. 补齐 Title 并校验有效性
      title := p.lookupCascadeTitle(req.CascadeID, p.languageServerPort, p.csrfToken)
      now := time.Now()

      p.cursorMu.Lock()
      p.mobileCascadeID = req.CascadeID
      p.mobileTitle = title
      p.mobileFocusedAt = now
      // 开启 1.5s 防自反抑制窗口：阻止随后因 markConversationAsRead 写入磁盘触发的伪桌面事件
      p.suppressDesktopFocusUntil = now.Add(1500 * time.Millisecond)
      p.cursorMu.Unlock()

      // 3. 预热内存缓存 (P7 联动，加速后续流推)
      go p.prewarmCascadeCache(req.CascadeID)

      w.Header().Set("Content-Type", "application/json")
      json.NewEncoder(w).Encode(map[string]interface{}{
          "status":     "ok",
          "cascade_id": req.CascadeID,
          "title":      title,
          "focused_at": now.UTC().Format(time.RFC3339Nano),
      })
  }
  ```
* **响应示例**：
  ```json
  {
    "status": "ok",
    "cascade_id": "5b16507c-5aa5-4713-9d9c-ae6b4bb97316",
    "title": "仅接管语音输入文字",
    "focused_at": "2026-09-15T10:08:03.429Z"
  }
  ```

#### 增强现有 `/gateway/status`
扩展状态响应，直接返回仲裁后的统一游标：
```json
{
  "status": "connected",
  "active_stream_cascade_id": "5b16507c-5aa5-4713-9d9c-ae6b4bb97316",
  "active_stream_title": "仅接管语音输入文字",
  "unified_cursor": {
    "cascade_id": "5b16507c-5aa5-4713-9d9c-ae6b4bb97316",
    "title": "仅接管语音输入文字",
    "source": "mobile",
    "is_sticky": true,
    "updated_at": "2026-09-15T10:08:03.429Z"
  }
}
```

---

### 4.2 iOS 客户端：即时触发焦点上报（对齐 Swift 6 与 NetworkTransport）

#### 1. `APIClient.swift` 增加方法
> **设计考量**：
> 1. **Swift 6 结构化并发兼容（Commit `906a54f`）**：废除旧有的裸 `URLSession.shared.dataTask` 逃逸闭包，改用 `Task { [transport] in ... }`，符合 `Sendable` 隔离要求；
> 2. **NetworkTransport 统一通道（Commit `a184736` / `b4eaa1a`）**：通过 `transport.send(request:)` 自动注入 Bearer Token，并在 IPv6 公网环境下自动由 `NWConnection` 绕过 iOS ATS 对 HTTP IPv6 字面量的拦截限制；
> 3. **Fire-and-Forget**：后台微秒级发出，完全不阻塞 UI 主线程和列表导航过渡动画。

```swift
// APIClient.swift
public func notifySessionFocus(cascadeId: String, baseURL: URL) {
    guard !cascadeId.isEmpty else { return }
    let endpoint = baseURL.appendingPathComponent("gateway/cascade/focus")
    
    var req = URLRequest(url: endpoint)
    req.httpMethod = "POST"
    req.timeoutInterval = 3.0
    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
    
    let payload: [String: String] = [
        "cascadeId": cascadeId,
        "source": "ios"
    ]
    req.httpBody = try? JSONEncoder().encode(payload)
    
    // 遵循 Swift 6 并发安全规范，委托底层 NetworkTransport 调度
    Task { [transport] in
        _ = try? await transport.send(request: req)
    }
}
```

#### 2. `ConversationListView.swift` 点击第 0 毫秒触发
在用户点击跳转的微秒级瞬间上报焦点，无需等待大体积 Trajectory 数据拉取：
```swift
.navigationDestination(for: ConversationItem.self) { item in
    ChatView(conversation: item, isNewConversation: item.stepCount == 0)
        .id(item.id)
        .onAppear {
            guard !item.isDraft else { return }
            // 立即上报焦点（第 0 毫秒，不等 loadMessages）
            if let url = AppSettings.shared.gatewayURL {
                APIClient.shared.notifySessionFocus(cascadeId: item.id, baseURL: url)
                Task {
                    await APIClient.shared.markConversationAsRead(cascadeId: item.id, baseURL: url)
                }
            }
        }
}
```

---

### 4.3 桌面端：文件系统内核监听与防自反机制（Go 网关层）

Go 网关在启动时初始化文件监听器，监控 `~/.gemini/antigravity/annotations/`：
```go
// 监听 ~/.gemini/antigravity/annotations/*.pbtxt 变更
func (p *Proxy) StartDesktopFocusWatcher(ctx context.Context) {
    annDir := filepath.Join(os.Getenv("HOME"), ".gemini/antigravity/annotations")
    brainDir := filepath.Join(os.Getenv("HOME"), ".gemini/antigravity/brain")
    
    // 每当捕获到写入事件
    onFileChanged := func(filename string) {
        if !strings.HasSuffix(filename, ".pbtxt") { return }
        cascadeID := strings.TrimSuffix(filepath.Base(filename), ".pbtxt")
        
        now := time.Now()
        
        p.cursorMu.Lock()
        // 1. 防自反机制（Anti-Reflection Protection）：
        // 当手机端打开会话或标记已读时，网关代理的 UpdateConversationAnnotations 也会促使
        // language_server 写入 annotations/<cid>.pbtxt。
        // 若当前处于防自反抑制窗口，或该文件属于手机刚聚焦的会话，坚决不将其视作桌面鼠标动作！
        if now.Before(p.suppressDesktopFocusUntil) || (p.mobileCascadeID == cascadeID && time.Since(p.mobileFocusedAt) < 2*time.Second) {
            p.cursorMu.Unlock()
            return
        }
        p.cursorMu.Unlock()
        
        // 2. 严格过滤幽灵会话 (三重验证)
        if _, err := os.Stat(filepath.Join(brainDir, cascadeID)); os.IsNotExist(err) {
            return // 排除没有 brain 目录的临时/未初始化标签页
        }
        
        // 3. 解析 title 与 last_user_view_time
        title, viewTime := parseAnnotationFile(filename)
        if title == "" || title == "未命名会话" {
            return
        }
        
        // 4. 更新桌面焦点
        p.cursorMu.Lock()
        if viewTime.After(p.desktopFocusedAt) {
            p.desktopCascadeID = cascadeID
            p.desktopTitle = title
            p.desktopFocusedAt = viewTime
            log.Printf("[Cursor] Desktop focus migrated to %s (%s) at %v", cascadeID, title, viewTime)
        }
        p.cursorMu.Unlock()
    }
}
```

---

## 5. 外设协同与用户体验完整走查

| 场景 | 用户行为 | 系统游标状态 | C·ONE 硬件盲对讲投递结果 |
| :--- | :--- | :--- | :--- |
| **场景 1** | 用户在电脑前看代码《雪球雷达》 | `DesktopFocusedAt` 处于最新 | 投递到 **电脑《雪球雷达》** 会话 |
| **场景 2** | 用户拿起手机，在手机 App 点开《仅接管语音》会话 | 手机点击第 0 毫秒发送 Focus Ping，`MobileFocusedAt` 刷新并超过电脑 | 投递到 **手机《仅接管语音》** 会话 |
| **场景 3** | 用户把手机**息屏锁屏放进口袋**出门 | 手机 WS 断开，进入 **Sticky 粘性模式**（保留 30 分钟） | 依然牢牢投递到 **手机《仅接管语音》** 会话，绝不串回电脑 |
| **场景 4** | 用户回到电脑前，在 IDE 中鼠标点开了《PPT制作》标签页 | 电脑触发 FSEvents，`DesktopFocusedAt` 瞬间超过手机的 `MobileFocusedAt` | 游标自动游走到 **电脑《PPT制作》**，无缝接续工作 |

---

## 6. 容灾与边界守卫

1. **时钟偏差保护（Clock Drift Guard）**：
   手机上报的时间戳若因客户端手机系统时间偏差异常，网关以**“网关收到 Ping 的服务器单调时钟时刻”**作为判定标准，消除多设备系统时间不准导致的抖动。
2. **防死锁与失效回退（Fallback Guarantee）**：
   若当前游标指向的会话在注入时发生异常（如进程崩溃或返回 500），网关除了触发硬件红色爆闪警报外，自动安全回退给本地 Hermes 原生处理，**保证 100% 不丢用户消息**。
3. **幽灵会话硬件屏障**：
   在任何情况下，未初始化（未命名的空白草稿）会话绝不参与仲裁，彻底杜绝 500 Internal Server Error 循环。
