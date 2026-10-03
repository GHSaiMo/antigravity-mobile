# 🚀 Multigravity v1.0.5

### ✨ 核心更新与优化 (Highlights)

- 🐧 **首发 Linux x86_64 官方支持与真机/NAS 生产验证**：
  - **原生 Linux x86_64 (amd64) 纯静态构建**：脱离任何 glibc 动态库依赖，开箱即用于各类 Linux 发行版（Debian / Ubuntu / Arch / CentOS / 群晖 DSM / 极空间 / 自建 NAS / 无头服务器等）；并在真实 Linux 环境中完成全链路端到端闭环验证；
  - **Antigravity Daemon Discovery 协议支持**：引入对 `~/.gemini/antigravity/daemon/ls_*.json` 发现文件的原生动态解析，实现 0 子进程开销的毫秒级即时发现与反向代理；
  - **Headless / Standalone 免 CSRF 令牌连通**：放宽对 `--csrf_token` 命令行参数的硬编码限制，全面兼容无头模式或常驻服务模式运行的 Linux 版 Language Server；
  - **双模端口探测 (lsof + ss)**：在 Linux 平台针对非特权用户环境，增加基于 `ss -H -tlnp` 的端口探测降级机制，确保在容器或受限 Linux 系统上依然稳健捕获监听端口；
  - **原生守护与后台自愈**：支持配置为 systemd 用户服务（如跟随 `antigravity-ls.service` 开机自启），进程守护与健康探活平滑一体化。

- 📦 **全平台一键安装脚本体验升级 (`scripts/install.sh`)**：
  - 自动识别 Linux 平台环境与 `x86_64` 架构，下载解压对应包并输出专属提示；
  - 自动创建全局软链接至 `/usr/local/bin/mgy`，在当前终端及新终端即刻生效，无需手动配置环境变量。

- 🛡️ **智能网络边界与局域网免配对直通 (Trust LAN / Localhost Bypass)**：
  - **本机访问 100% 免配对**：真正本机发起访问（`127.0.0.1` / `::1`）自动授权并派发 Admin 会话，直达全功能桌面工作台；
  - **局域网安全策略可配置**：首次安装引导用户按环境选择偏好（`MULTIGRAVITY_TRUST_LAN=1` 信任局域网免配对直通；默认 `0` 保持配对码校验）；
  - **Cloudflare 穿透边界严格隔离**：精准校验请求指纹（`CF-Connecting-IP` / `CF-Ray`），严禁穿透流量免密绕过；
  - **自适应环境感知与友好看板**：Linux 无头环境自动屏蔽浏览器弹窗并在终端输出清晰看板，桌面环境自动唤起默认浏览器。

- 🖥️ **网页桌面版全场景深度汉化与视图自适应优化**：
  - **全功能桌面工作台深度精准汉化**：完整汉化侧边栏、项目权限继承、意见与问题反馈模态框、Token 预算与扩展明细、额度上限耗尽动态提示、变更文件 (Files Changed) 与 Git 提交流程、独立终端 (Standalone Terminals) 与分屏控制面板；
  - **展开/折叠与搜索状态汉化补全**：全面覆盖 `See All` / `See Less` / `See More` / `Collapse All` / `Expand All` 及动态数量统计，汉化 `Artifacts` (交付工件)、`Review` (审核)、`No Results` (未找到结果)；
  - **主题与系统设置全下拉菜单深度汉化**：完整汉化主题预设全量方案（`Catppuccin`, `One Light`, `One Dark Pro`, `Solarized Light`, `Solarized Dark`, `Dracula`, `Monokai`, `Tokyo Night`, `Vesper` 等），以及安全策略、沙箱执行、计划审核、终端权限等所有下拉菜单内部选项；
  - **按屏幕宽度严格自适应三端模式**：彻底移除手动切换按钮（「移动端视图」与「桌面工作台」浮窗），根据视口宽度纯自动适配桌面端（Desktop >= 1024px）、平板端（Tablet 768px~1024px）与手机端（Mobile < 768px），拖拽窗口或切换设备零延迟自适应，历史会话路由精准保留。

- ⚡ **会话切换秒开与响应稳定性加固**：
  - **根治会话转圈与第二会话加载卡顿**：重构会话加载生命周期，在进入会话时并行预加载历史消息，毫秒级优先渲染会话树；对后台轮询增加安全缓存与降级机制，彻底消除加载阻塞；
  - **消除终端代理错误日志噪音**：过滤客户端切换页面或取消连接导致的 `http: proxy error: context canceled` 无效报错，终端保持洁净；
  - **控制台交互式 LAN 策略切换 (`mgy lan`)**：新增 `mgy lan` 交互式指令，随时查看与一键切换局域网放行/配对模式，无需手动配置环境变量；
  - **启动面板优化与脱敏保护**：Cloudflare 专属隧道就绪日志去重，公网穿透域名按需脱敏，本地 127.0.0.1 免配对直连审计告警逻辑修正。

- 🛑 **彻底排查并修复 Web 桌面版停止按钮 (Stop/Cancel) 无响应问题**：
  - **彻底移除底层浮动切换胶囊**：全面消除视图切换胶囊与底栏操作热区的重叠隐患，保障发送与停止按钮 100% 灵敏响应；
  - **严格保护 Material / Google Symbols 字体图标连字**：汉化引擎全面排除 `.google-symbols`、`.material-symbols`、`.material-icons`、`.codicon` 与 SVG，杜绝 `stop` / `stop_circle` 等连字被替换为中文字符导致的图标渲染破损与点击事件阻断；
  - **网关代理层拦截与实时状态广播**：针对 `/CancelCascadeInvocation` 与 `/ForceStopCascadeTree` 增加代理转发与缓存失效联动，取消操作完成后毫秒级重置本地缓存并触达实时流，状态即刻恢复 Idle。

- ⚡ **智能 Anycast 优选与穿透常驻自愈**：
  - 内置国内 Anycast 节点池并发测速（实测 60~70ms），告别晚高峰跨洋绕路卡顿；
  - 规范 Cloudflare 穿透参数并引入常驻守护（Supervisor），休眠或断线自动毫秒级拉起，保障外网 100% 可用。

- 🧩 **多题交互与审批确认体验重构**：
  - 完整支持原生 `askQuestion` 多题批量确认与统一提交，弹出选项时软键盘智能避让；
  - 优化卡片自适应高度与半屏/全屏切换，精准对齐方案 Proceed 执行状态。

- 🔑 **永久固定 Android 发布签名**：
  - 保持永久固定的 Release 签名密钥库，升级直接无缝覆盖安装，杜绝 `INSTALL_FAILED_UPDATE_INCOMPATIBLE`。

---

### 📦 服务端安装 / 更新 (`mgy`)

- **macOS (Apple Silicon & Intel) / 🐧 Linux (x86_64 / NAS)**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

---

### 📱 客户端与全平台资产下载
- **Linux 服务端**：`multigravity-linux-amd64.tar.gz` (全新加入 ⭐)
- **macOS 服务端**：`multigravity-darwin-universal.tar.gz` / `multigravity-darwin-arm64.tar.gz` / `multigravity-darwin-amd64.tar.gz`
- **Windows 服务端**：`multigravity-windows-amd64.zip`
- **Android 客户端**：`Multigravity-v1.0.5.apk` (支持直接覆盖安装)
- **iOS 客户端**：TestFlight 或通过 Xcode 本地构建安装。

