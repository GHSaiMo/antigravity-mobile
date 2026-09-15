# YoooClaw 物理外设 ✕ Antigravity 智能体集成设计规范

本文档记录 YoooClaw 物理外设与 Antigravity Mobile 网关互联的技术逆向成果、全链路通信协议、消息截流方案、多模路由状态机以及已实测验证的硬件灯效与屏幕通知规范。

---

## 1. 背景与核心诉求

* **物理外设**：YoooClaw 智能硬件（带按压按键、RGB LED 阵列与屏幕/通知回显，底层经由 iOS ANCS 蓝牙与手机 App 互联）。
* **现有通路**：外设按键录音 ➔ 云端 ASR 转文字 ➔ 经 WebSocket 隧道下发到 Mac 上的 `yoooclaw-hermes-plugin` ➔ 注入 Hermes 聊天会话。
* **目标通路**：
  1. 识别并截留指定语音指令，**阻止其流向 Hermes**；
  2. 将文本直接注入到 **Antigravity Mobile 网关（:58900）** 对应的 **Cascade 会话** 中；
  3. 支持通过语音指令在“反重力模式（Antigravity）”与“生活模式（Hermes）”之间无缝切换；
  4. 切换到反重力模式时，硬件立即下发**四色环形交织流光**并弹出**“已切换到反重力模式”**的屏幕通知。

---

## 2. 全链路通信拓扑与协议逆向

```
┌─────────────────┐        BLE/ANCS        ┌─────────────────┐
│ YoooClaw 物理外设 │ ◄───────────────────► │  YoooClaw App   │
└────────┬────────┘                        └────────┬────────┘
         │ (按键语音流上传)                          │ (云端中继)
         ▼                                          ▼
┌────────────────────────────────────────────────────────────┐
│         YoooClaw 云端 (openclaw-service.yoooclaw.com)       │
└─────────────────────────────┬──────────────────────────────┘
                              │
                              │ 持久 WebSocket 隧道
                              │ wss://openclaw-service.yoooclaw.com/message/messages/ws/plugin
                              │ (认证: apiKey 来自 ~/.yoooclaw/credentials.json)
                              ▼
┌────────────────────────────────────────────────────────────┐
│        本地拦截分流层 (yoooclaw-hermes-plugin / 独立桥接)     │
│        - 单消费者排他锁: ~/.yoooclaw/standalone-relay.flock │
│        - 帧调度器: RelayAppTransport.handle_frame           │
└──────────────┬──────────────────────────────┬──────────────┘
               │ (普通消息 / 生活模式)           │ (代码模式 / 反重力指令)
               ▼                              ▼
      ┌─────────────────┐            ┌─────────────────────────────┐
      │  Hermes 会话    │            │ Antigravity Mobile Gateway  │
      │  (~/.hermes/)   │            │ (127.0.0.1:58900)           │
      └─────────────────┘            └──────────────┬──────────────┘
                                                    │
                                                    ▼
                                     ┌─────────────────────────────┐
                                     │ Antigravity Core            │
                                     │ (language_server: 57576)    │
                                     │ Cascade 会话多步自主执行      │
                                     └─────────────────────────────┘
```

### 2.1 鉴权与长连接细节
* **API Key 存储**：`~/.yoooclaw/credentials.json` 中的 `apiKey`（格式如 `ock-SHQL...`）。
* **WebSocket 连接 URL**：
  ```text
  wss://openclaw-service.yoooclaw.com/message/messages/ws/plugin?apiKey=<API_KEY>
  ```
* **心跳保活**：客户端每 10s 发送文本 `"ping"`，云端响应 `"pong"`。
* **单消费者排他机制**：本地文件锁 `~/.yoooclaw/standalone-relay.flock`。

### 2.2 消息下发与拦截点
* **云端下发帧格式**（V1 Command Envelope）：
  ```json
  {
    "type": "request",
    "path": "/agent/commands",
    "id": "req_...",
    "body": {
      "version": 1,
      "command": "message.send",
      "requestId": "...",
      "payload": {
        "sessionId": "app_...",
        "message": {
          "id": "msg_user_...",
          "content": "这是一句测试消息，328 954。"
        }
      }
    }
  }
  ```
* **精准拦截动刀点**：
  `yoooclaw_app/adapter.py` 中的 `_handle_app_message(self, message: AppMessage)`：
  在此处提取 `message.text`，若命中 Antigravity 路由，**不调用** `await self.handle_message(...)`，改为调用 Antigravity Mobile 网关接口，实现 100% 无感截流。

---

## 3. 双层协同路由系统设计（已实测落地）

### 3.1 第一层：YoooClaw App 会话标题守卫（Session Boundary Guard · 已落地）
彻底废弃依赖脆弱的模式切换语音指令或外部 JSON 文件（如 `current_target.json`），将流向控制权**100% 交由 YoooClaw App 当前选中的会话标题**决定：

* **底层实现**：
  在 `yoooclaw_app/antigravity_bridge.py` 中，收到 `message.send` 帧后提取 mandatory 的 `sessionId`，直接通过只读 URI (`file:...mode=ro`) 查询 Hermes 状态库 `~/.hermes/state.db` 中的 `sessions.title`。
* **分流规则**：
  ```python
  def is_antigravity_session(title: str) -> bool:
      if not title:
          return False
      t = title.strip().lower()
      return "反重力" in t or "antigravity" in t
  ```
* **分流表现**：
  - 若用户当前处于【2026-09-14 通知总结】、【生活管家】等非反重力会话：**100% 原生 Bypass 放行给 Hermes**（无论是打字、点击按钮还是硬件按键对讲）；
  - 只有当用户在 YoooClaw App 中选中或切换至标题包含【反重力】或【antigravity】的会话时，才触发截流并交由第二层处理。

### 3.2 第二层：Follow-Me 统一游标仲裁与注入
在第一层确认注入反重力后，消息交由 Antigravity Mobile 网关（:58900）进行端到端高可靠注入：

1. **游标仲裁（Follow-Me Unified Cursor）**：
   - 优先判断手机端是否在线或处于息屏粘性（Sticky）保持期（30分钟）；
   - 手机端未活跃时，自动感知桌面 IDE 当前聚焦的活动会话（通过内核监听 `annotations/*.pbtxt`）；
   - 详细规则参见规范文档：`docs/unified_active_session_cursor_design.md`。
2. **幽灵空白会话三重防御（Ghost Session Filter）**：
   - 过滤掉未初始化的临时标签页（必须 `title != ""`、`title != "未命名会话"` 且 `~/.gemini/antigravity/brain/<id>` 目录存在）。
3. **消息注入与失败自愈（Fallback Guarantee）**：
   - 构造 RPC 请求调用 `SendUserCascadeMessage`；
   - 若网关返回非 2xx（如 upstream 异常），注入函数返回 `False`，自动安全回退给 Hermes 原生处理，**杜绝丢弃用户消息**。

```bash
# Antigravity 消息注入标准契约
POST http://127.0.0.1:58900/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage
Content-Type: application/json
Authorization: Bearer <device_token>

{
  "cascadeId": "<ARBITRATED_CASCADE_ID>",
  "items": [{"text": "清理无用依赖并重新打包"}]
}
```

---

## 4. 硬件灯效与屏幕通知规范（已实测验证）

### 4.1 切换至“反重力模式”灯效（已实测通过）
* **视觉效果**：Google Antigravity 经典四色色相环闭环交织流光（整整齐齐 2.0 秒，无缝接力流动，极度醒目与高级）。
* **CLI 执行指令**：
  ```bash
  yoooclaw light send \
    --segments '[
      {"mode":"color_flow","duration_s":0.5,"interval_ms":60,"brightness":255,"direction":"ltr","window":2,"color":{"r":255,"g":210,"b":0},"background":{"r":255,"g":20,"b":20,"brightness":180}},
      {"mode":"color_flow","duration_s":0.5,"interval_ms":60,"brightness":255,"direction":"ltr","window":2,"color":{"r":0,"g":220,"b":60},"background":{"r":255,"g":210,"b":0,"brightness":180}},
      {"mode":"color_flow","duration_s":0.5,"interval_ms":60,"brightness":255,"direction":"ltr","window":2,"color":{"r":0,"g":100,"b":255},"background":{"r":0,"g":220,"b":60,"brightness":180}},
      {"mode":"color_flow","duration_s":0.5,"interval_ms":60,"brightness":255,"direction":"ltr","window":2,"color":{"r":255,"g":20,"b":20},"background":{"r":0,"g":100,"b":255,"brightness":180}}
    ]' \
    --title "Antigravity" \
    --reason "已切换到反重力模式" \
    --format json
  ```
* **时序与流光解构**：
  1. `0.0s - 0.5s`：艳阳赤红底色 + 明艳金黄波浪高速掠过；
  2. `0.5s - 1.0s`：明艳金黄底色 + 极客翠绿波浪高速掠过；
  3. `1.0s - 1.5s`：极客翠绿底色 + 科技蔚蓝波浪高速掠过；
  4. `1.5s - 2.0s`：科技蔚蓝底色 + 艳阳赤红波浪闭环收尾。
* **屏幕 / App 回显**：
  * 标题：`Antigravity`
  * 正文：`已切换到反重力模式`

### 4.2 切换至“生活模式 (Hermes)”灯效
* **CLI 指令**：
  ```bash
  yoooclaw light send \
    --preset green-breath \
    --title "Hermes" \
    --reason "已切换到生活模式" \
    --format json
  ```
* **视觉效果**：温和的极客绿光缓慢呼吸（8 秒），表示回归日常管家。

### 4.3 任务执行与交付状态灯效
* **任务处理中**：`blue-wave`（蓝光波浪流动，代表 Agent 级联思考与多步工具执行）；
* **任务成功交付**：`green-steady`（绿光常亮 5 秒，代表代码修改完毕并通过校验）；
* **任务异常报错**：`red-strobe-3`（红光爆闪 3 次，提示需要人工干预或报错）。

---

## 5. 开发实施与落地状态

1. **阶段 1：会话标题守卫与本地拦截（✅ 已上线实测）**
   * 在 `yoooclaw_app/adapter.py` 与 `antigravity_bridge.py` 注入基于 `sessionId` 的标题探测；
   * 连接 `~/.hermes/state.db`（只读模式）判定 `is_antigravity_session`；
   * 非反重力会话 100% Bypass 原生 Hermes 处理，消除所有误拦截。
2. **阶段 2：Antigravity 网关注入与幽灵会话防御（✅ 已上线实测）**
   * 集成 `antigravity-mobile` 网关 HTTP RPC 接口 (`SendUserCascadeMessage`)；
   * 实施幽灵会话三重防御（过滤空标题、临时未初始化标签页、校验 `brain` 目录物理存在）；
   * 实现注入失败安全回退保障（返回 False，不丢失任何用户输入）。
3. **阶段 3：Follow-Me 跨端统一游标与协同感知（🚀 实施中）**
   * 详见规范文档：`docs/unified_active_session_cursor_design.md`；
   * 手机端 `POST /gateway/cascade/focus` 毫秒级上报（Swift 6 / NetworkTransport）；
   * 桌面端 `annotations/*.pbtxt` 内核监听与防自反机制（消除手机已读触发的反向误判）；
   * 息屏 30 分钟 Sticky 粘性游标保持。
