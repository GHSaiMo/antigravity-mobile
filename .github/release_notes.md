# 🚀 Multigravity v1.0.0 正式版发布

**Multigravity (`mgy`)** 是专为 AI 智能体打造的全栈移动伴侣与统一本地网关。让您无论身处何地，均可在手机端轻松监控智能体后台思考流、实时下发指令与做出关键决策。

---

### ✨ 核心特性与亮点

#### 📱 移动端对齐与原生体验
- **Android 客户端正式发布**：首发提供预构建且带有自签名的 `Multigravity-v1.0.0.apk`，手机下载后直接点击即可安装使用，零编译门槛。
- **多端体验一致**：对齐原生交互设计、后台运行任务卡片、实时消息队列、富文本/Markdown 渲染以及安全设备解绑机制。
- **iOS 实时活动支持**：深度适配 iOS Live Activities 与灵动岛，重要进度在锁屏与灵动岛常驻呈现。

#### 🌐 智能网络感知与无感直连
- **避开虚拟网卡**：深度优化局域网 IP 探测算法，自动屏蔽 TUN、TAP、Clash、Fake-IP、Docker、虚拟机等各类虚拟网卡干扰，精准优先绑定 Wi-Fi / Ethernet 真实物理网卡。
- **macOS IPv6 智能自适应**：自动探测当前物理网卡的 IPv6 配置状态，并在终端直观呈现 5G/4G 蜂窝网络直连测试与排错指引。
- **三合一复合配对**：配对二维码自动集成局域网内网 IP、公网 IPv6 与云端穿透中继地址，局域网秒连，出差外网自适应无缝切换。

#### 💻 服务端极简极速安装 (`mgy`)
服务端命令行工具已统一命名为 **`mgy`**，体积仅 ~7MB，解压即用：

- **macOS (Apple Silicon & Intel)**：
  ```bash
  curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
  ```

- **Windows (PowerShell)**：
  ```powershell
  irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
  ```

#### 🛠️ 常用 CLI 指令
- `mgy`：前台启动本地网关并打印终端配对二维码
- `mgy pair`：向正在运行的网关即时申请并打印 5 分钟有效的新配对码
- `mgy list`：查看所有已配对授权的移动设备（支持离线/在线状态显示）
- `mgy clear all`：一键清除并撤销所有移动端设备授权
