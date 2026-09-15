# YoooClaw ✕ Antigravity 集成网桥

本目录收录 YoooClaw 物理外设及 App 与 Antigravity Mobile 网关（:58900）直连注入的桥接层核心实现：

1. `antigravity_bridge.py`：
   - **Gateway-First 代理注入**：优先通过 `http://127.0.0.1:58900/api/...` 发起 `StartCascade` 与 `SendUserCascadeMessage`，自动享受模型合成、CSRF 处理与前端 SSE 缓存清除；
   - **双重探针安全加固**：直连语言服务器时通过 `lsof -nP -a -p <pid> -iTCP -sTCP:LISTEN` 精准锁定端口，并通过 `GetStatus` 实时校验，杜绝误选系统进程（如 `rapportd:49325`）；
   - **Follow-Me 统一游标感知**：实时感知手机端焦点与桌面 IDE 活跃会话；
   - **硬件灯效与屏幕通知**：四色交织流光回显与通知回显。

2. `adapter.py`：
   - `_handle_app_message` 中的拦截注入切入点，携带 `adapter=self`；
   - 投递成功后向手机 App 回显投递目标会话标题，并触发 `run.complete` 结束 pending 等待。

### 部署路径
运行环境中对应的物理文件路径为：
`~/.hermes/hermes-agent/venv/lib/python3.11/site-packages/yoooclaw_app/`
修改后通过 launchd 重启生效：
`launchctl kickstart -k gui/$(id -u)/ai.hermes.gateway`
