# Multigravity (mgy) 全平台安装、本地测试与卸载运维指南

本文档系统介绍了 **Multigravity (`mgy`)** 服务端在 **macOS / Windows** 环境下的完整生命周期管理，包括**一键安装原理、多场景本地自测方案、纯净卸载流程、配置管理以及常见故障排查**。

---

## 目录

1. [一键安装方式与原理](#1-一键安装方式与原理)
   - [1.1 推荐安装命令 (macOS / Windows)](#11-推荐安装命令)
   - [1.2 安装脚本底层自适应逻辑](#12-安装脚本底层执行逻辑)
2. [全局配置与数据目录规范](#2-全局配置与数据目录规范)
3. [本地开发与功能自测方案](#3-本地开发与功能自测方案)
   - [场景一：下载与架构匹配流程自测](#场景一下载与架构匹配流程自测)
   - [场景二：纯净首次安装模拟（Clean Slate Test）](#场景二纯净首次安装模拟clean-slate-test)
   - [场景三：macOS IPv6 状态智能检测与自动修复测试](#场景三macos-ipv6-状态智能检测与自动修复测试)
   - [场景四：移动端 5G/4G 外网直连验证与光猫排错](#场景四移动端-5g4g-外网直连验证与光猫排错)
   - [场景五：CLI 原生指令与多设备管理测试](#场景五cli-原生指令与多设备管理测试)
4. [完整卸载方案](#4-完整卸载方案)
   - [方法 A：终端命令行卸载（推荐）](#方法-a终端命令行卸载推荐)
   - [方法 B：使用一键卸载脚本 (Bash & PowerShell)](#方法-b使用一键卸载脚本)
   - [方法 C：清理 Shell PATH（可选）](#方法-c清理-shell-path可选)
5. [官方版本发布规范与偏好准则 (Release Guidelines)](#5-官方版本发布规范与偏好准则)
6. [常见问题与故障排查 (FAQ)](#6-常见问题与故障排查-faq)

---

> 💡 **Release 规范速查**：关于 Release 目标平台白名单（禁止未实测 Linux/Windows ARM64）、Release Notes 纯净度要求与多端自动化构建规范，请参阅专注文档：[`docs/release_preferences_and_workflow.md`](file:///Users/hal9000/Projects/antigravity-mobile/docs/release_preferences_and_workflow.md)。

## 1. 一键安装方式与原理

### 1.1 推荐安装命令

#### 🍎 macOS / 🪟 Windows (Git Bash) 用户：
在终端执行以下命令，脚本会自动探测操作系统（Darwin/Windows）与芯片架构（arm64/amd64），极速拉取并安装专属 ~7MB 二进制：
```bash
# 国内网络加速一键安装（免代理、免翻墙，默认推荐）
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash

# 海外或已配置终端代理用户（GitHub 官方源）
curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
```

#### 🪟 Windows (PowerShell / Windows Terminal) 用户：
打开 PowerShell，直接执行原生 PowerShell 一键安装命令（自动释放文件锁、配置用户 PATH 及 WindowsApps 目录）：
```powershell
# 国内网络加速一键安装（免代理、免翻墙，默认推荐）
irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex

# 海外或已配置终端代理用户（GitHub 官方源）
irm https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.ps1 | iex
```

#### 💻 本地源码编译安装
在拉取本项目源码后，本地即时编译并安装到全局 PATH：
```bash
# macOS / Linux
go build -ldflags="-s -w -X 'main.Version=1.0.0'" -o ~/.local/bin/mgy ./cmd/gateway
codesign -s - -f ~/.local/bin/mgy 2>/dev/null || true

# Windows (PowerShell)
go build -ldflags="-s -w -X 'main.Version=1.0.0'" -o "$HOME\.local\bin\mgy.exe" ./cmd/gateway
```

---

### 1.2 安装脚本底层执行逻辑

安装脚本 [`scripts/install.sh`](file:///Users/hal9000/Projects/antigravity-mobile/scripts/install.sh) 与 [`scripts/install.ps1`](file:///Users/hal9000/Projects/antigravity-mobile/scripts/install.ps1) 做到**全平台自适应、零依赖、轻量化与高鲁棒性**，核心步骤如下：

```mermaid
flowchart TD
    Start([开始安装]) --> CheckOS{检测操作系统}
    CheckOS -- macOS --> DetectMacArch[识别 M系列(arm64) 或 Intel(amd64)]
    CheckOS -- Linux --> DetectLinuxArch[识别 Linux arm64 或 amd64]
    CheckOS -- Windows --> DetectWinArch[识别 Windows x86_64 或 ARM64]
    
    DetectMacArch & DetectLinuxArch & DetectWinArch --> DetectProxy[智能探测本机常用代理端口]
    
    DetectProxy --> Download[按优先级下载 ~7MB 专属单架构包]
    Download -- 失败或超时 --> FallbackMirror[自动切换加速镜像 / 通用胖二进制]
    
    Download & FallbackMirror --> Extract[解压至 ~/.local/bin/mgy]
    Extract --> Security{系统平台特性处理}
    Security -- macOS --> Codesign[消除隔离属性 xattr + Ad-hoc codesign]
    Security -- Windows --> WinApps[注册至 WindowsApps 目录 + 用户 PATH]
    Security -- Linux --> Chmod[chmod +x 赋权]
    
    Codesign & WinApps & Chmod --> CheckEnv[初始化 ~/.multigravity/.env 配置]
    CheckEnv --> Done([安装成功，打印版本与说明])
```
```

1. **精准架构匹配**：
   - 自动将 Apple Silicon 识别为 `Apple Silicon (M系列) (arm64)`，Intel Mac 识别为 `amd64`。
   - 优先下载对应架构的 **~7MB 轻量包**（相比 15MB Universal 包体积缩减 50% 以上），秒级极速解压。
2. **自适应代理嗅探**：
   - 在未配置终端环境变量的情况下，自动通过 `nc -z` 嗅探本地常见的代理软件监听端口（7890: Clash、10808: V2Ray、6152: Surge、1080 等），一旦探测到立即自动附带 `--proxy` 参数下载。
3. **安全隔离绕过与代码签名**：
   - 自动移除 macOS Gatekeeper 隔离属性：`xattr -d com.apple.quarantine ~/.local/bin/mgy`。
   - 自动进行本地 Ad-hoc 签名：`codesign -s - -f ~/.local/bin/mgy`，彻底避免 macOS 弹出「无法验证开发者」安全拦截。
4. **环境免打扰注入与全局即刻可用**：
   - 安装至用户主目录下的 `~/.local/bin/mgy`，并自动优先软链接至已在系统 `$PATH` 中的目录（如 `/opt/homebrew/bin` 或 `/usr/local/bin`），实现安装完成即在当前终端直接可用（**免手动 `source ~/.zshrc`**）。
   - 同时在 `~/.zshrc` 与 `~/.zprofile`（或 `~/.bashrc`）中自动持久化追加环境变量，保证任何新开终端、IDE 终端及远程会话持久有效。

---

## 2. 全局配置与数据目录规范

Multigravity 统一遵循行业标准的配置与数据隔离规范，全部数据均收敛在用户主目录的 `~/.multigravity/` 下：

| 路径 | 用途说明 | 权限与安全性 |
| :--- | :--- | :--- |
| `~/.local/bin/mgy` | 网关主执行程序（二进制文件） | `0755`，免 root 权限 |
| `~/.multigravity/.env` | 全局环境变量配置文件（端口、IPv6、FRP、Bark 等） | `0600`，仅当前用户可读写 |
| `~/.multigravity/admin_token` | 管理员特权密钥（留空时首次启动自动生成 64 位安全密钥） | `0600`，私密凭证 |
| `~/.multigravity/auth_salt` | 配对设备 Token 加盐哈希 Salt | `0600`，私密凭证 |
| `~/.multigravity/auth_store.json` | 已授权配对设备白名单列表（含设备名、公钥哈希、最后活跃时间） | `0600`，私密存储 |
| `~/.multigravity/certs/` | 本地自签名或 Let's Encrypt TLS 证书目录 | `0700`，存放私钥与证书链 |
| `~/.multigravity/logs/` | 运行日志存放目录 | `0755` |

### 核心环境变量规范（在 `.env` 中配置）

- `MULTIGRAVITY_PORT`：网关监听端口（默认 `58900`）。
- `MULTIGRAVITY_HOST`：监听地址（默认留空双栈绑定全部网卡；设为 `127.0.0.1` 仅限本机）。
- `INCLUDE_PUBLIC_IPV6`：是否广播公网 IPv6（默认 `1`；设为 `0` 关闭）。
- `MULTIGRAVITY_PREFER_IPV6`：是否优先纯 IPv6 二维码（默认 `0` 生成双栈复合码；设为 `1` 强制优先 IPv6）。
- `DDNS_HOST`：公网域名或固定公网 IP。
- `BARK_URL`：iOS Bark 推送凭证。
- `FRP_ENABLED` / `FRP_SERVER_ADDR` / `FRP_REMOTE_PORT`：FRP 穿透配置。

---

## 3. 本地开发与功能自测方案

为了在开发迭代或发布前进行全流程闭环验证，建议按照以下场景执行自测：

### 场景一：下载与架构匹配流程自测
**测试目标**：验证在线一键安装脚本能否正常识别架构、选用正确包并完成安装。

1. 仅移除现有的二进制执行文件：
   ```bash
   rm -f ~/.local/bin/mgy
   ```
2. 运行国内加速安装指令：
   ```bash
   curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
   ```
3. **预期验收结果**：
   - 终端正确打印芯片架构，例如：`🖥️ 检测到系统架构: Apple Silicon (M系列) (arm64)`；
   - 成功从镜像地址下载 `multigravity-darwin-arm64.tar.gz`（约 7.1MB）；
   - 执行 `which mgy` 输出 `~/.local/bin/mgy`；
   - 执行 `mgy version` 正确输出版本号（如 `1.0.0`）。

---

### 场景二：纯净首次安装模拟（Clean Slate Test）
**测试目标**：完全模拟一个新用户从未安装过 Multigravity 的初始环境。

1. 彻底清空二进制与配置目录：
   ```bash
   rm -f ~/.local/bin/mgy && rm -rf ~/.multigravity
   ```
2. 重新执行安装命令：
   ```bash
   curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/install.sh | bash
   ```
3. **预期验收结果**：
   - 终端提示 `📝 已生成全局默认配置: ~/.multigravity/.env`；
   - 启动网关服务：
     ```bash
     mgy
     ```
   - 首次启动自动生成管理员密钥：`~/.multigravity/admin_token`；
   - 控制台直接渲染出清晰的 ASCII 二维码与外网直连验证指引。

---

### 场景三：macOS IPv6 状态智能检测与自动修复测试
**测试目标**：验证当 Mac 网络设置中「配置 IPv6」被意外关闭时，网关能否自动帮用户切换为「自动」并恢复公网直连。

1. **模拟关闭 IPv6**（需输入一次电脑管理员密码）：
   ```bash
   # 查看当前主网卡服务名称（例如 Ethernet 或 Wi-Fi）
   networksetup -listnetworkserviceorder
   # 将其 IPv6 设为关闭
   sudo networksetup -setv6off "Ethernet"
   ```
2. **确认已关闭**：
   ```bash
   networksetup -getinfo "Ethernet" | grep IPv6
   # 输出显示: IPv6: Off
   ```
3. **启动网关观察自愈逻辑**：
   ```bash
   mgy
   ```
4. **预期验收结果**：
   - 网关控制台输出：
     ```text
     🌐 检测到 macOS 当前网络服务「Ethernet」未开启 IPv6 (原配置: Off)。
     ⚡ 已自动帮您切换为「配置 IPv6: 自动」！正在等待网络接口分配公网 IPv6 地址...
     🎉 成功获取公网 IPv6 地址: 240e:...
     ```
   - 检查系统状态已自动恢复：
     ```bash
     networksetup -getinfo "Ethernet" | grep IPv6
     # 输出显示: IPv6: Automatic
     ```

---

### 场景四：移动端 5G/4G 外网直连验证与光猫排错
**测试目标**：验证外网蜂窝移动数据能否不经由任何第三方中继服务器，直连回 Mac 桌面网关。

1. 在 Mac 上运行 `mgy`（或在已有服务运行时运行 `mgy pair`）：
   - 控制台底部会输出专用的测试地址：
     ```text
     📱 移动端 5G/4G 外网直连验证指引:
        1. 手机断开家中 Wi-Fi（切换至 5G/4G 移动蜂窝网络）；
        2. 手机自带浏览器直接访问测试地址:
           http://[240e:3a1:1c3:4e80:...]:58900
     ```
2. **拿手机验证**：
   - **关闭手机 Wi-Fi**，开启 5G/4G 蜂窝移动网络；
   - 用手机 Safari 或 Chrome 打开上述测试地址；
3. **排错说明**：
   - ✅ **能正常打开网页**：说明光猫/路由器已放行 IPv6 入站流量，外网直连完全畅通，打开手机 App 扫码即可瞬间连通！
   - ⚠️ **打不开或连接超时**：说明家庭宽带光猫或主路由器开启了『IPv6 防火墙入站阻断』。请登录光猫后台（常见 `192.168.1.1`）或主路由器设置，将「IPv6 防火墙」关闭，或在防火墙规则中放行 TCP `58900` 端口即可。

---

### 场景五：CLI 原生指令与多设备管理测试
**测试目标**：验证本地命令行工具的核心管理子命令功能。

```bash
# 1. 启动主服务
mgy

# 2. 另外打开一个终端窗口，随时获取最新配对二维码
mgy pair

# 3. 优先获取强制纯 IPv6 二维码
mgy pair -ipv6

# 4. 查看当前已授权的移动设备
mgy list

# 5. 清理指定设备或一键解绑所有设备
mgy clear all

# 6. 查看网关版本
mgy version
```

---

## 4. 完整卸载方案

卸载脚本已全面支持**一网打尽的纯净卸载**，包含：
1. **停止进程**：停止运行中的 `mgy` 网关与 `cloudflared` 穿透守护进程；
2. **清理程序与软链接**：移除 `mgy` 二进制及全局软链接（`/usr/local/bin/mgy`、`/opt/homebrew/bin/mgy`、`~/.local/bin/mgy`、`WindowsApps\mgy.exe`）；
3. **清理 Cloudflare 引擎**：移除由项目部署的 `cloudflared` 穿透引擎（`~/.local/bin/cloudflared` 与 `~/.multigravity/bin/cloudflared`）；
4. **清空配置与数据**：彻底删除 `~/.multigravity` 配置目录（包含所有配置文件、设备授权 `tokens.json`、配对信息 `pairings.json` 与日志缓存）。

---

### 方法 A：一键在线彻底卸载（推荐，包含 Cloudflare 与全部配置）

无需下载源码，直接在终端执行一行命令即可完成彻底清理：

#### 🍎 macOS / 🐧 Linux / 🪟 Windows (Git Bash) 用户：
```bash
# 国内网络加速一键彻底卸载（默认推荐，包含 mgy + cloudflared + 全部配置）
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/uninstall.sh | bash

# 海外或已配置终端代理用户（GitHub 官方源）
curl -fsSL https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/uninstall.sh | bash
```

#### 🪟 Windows (PowerShell / Windows Terminal) 用户：
```powershell
# 国内网络加速一键彻底卸载（默认推荐，包含 mgy.exe + cloudflared.exe + 全部配置）
irm https://ghfast.top/https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/uninstall.ps1 | iex

# 海外或已配置终端代理用户（GitHub 官方源）
irm https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/scripts/uninstall.ps1 | iex
```

---

### 方法 B：使用本地卸载脚本与进阶参数

若已拉取项目源码，可直接运行本地脚本并按需传入参数：

#### 1. macOS / Linux / Git Bash 用户：
```bash
# 默认彻底卸载（停止进程 + 移除 mgy + 移除 cloudflared + 清空 ~/.multigravity）
./scripts/uninstall.sh

# 进阶选项 1：仅卸载执行程序与 cloudflared，保留 ~/.multigravity 配置目录（方便后续重新安装）
./scripts/uninstall.sh --keep-config

# 进阶选项 2：卸载 mgy 与配置，但保留已下载的 cloudflared 二进制供其他程序复用
./scripts/uninstall.sh --keep-cf
```

#### 2. Windows PowerShell 用户：
```powershell
# 默认彻底卸载（停止进程 + 移除 mgy.exe + 移除 cloudflared.exe + 清空 ~/.multigravity）
.\scripts\uninstall.ps1

# 进阶选项：仅卸载执行程序，保留配置目录
.\scripts\uninstall.ps1 -KeepConfig

# 进阶选项：保留已下载的 cloudflared.exe
.\scripts\uninstall.ps1 -KeepCloudflared
```

---

### 方法 C：手动命令行极速清理

若希望完全手动清理，可在终端直接执行：
```bash
# 1. 停止进程
pkill -f mgy || true
pkill -f "cloudflared.*tunnel" || true

# 2. 移除二进制文件与全局链接
rm -f ~/.local/bin/mgy ~/.local/bin/cloudflared /usr/local/bin/mgy /opt/homebrew/bin/mgy 2>/dev/null || true

# 3. 清除所有配置文件与配对授权数据
rm -rf ~/.multigravity
```

---

### 方法 C：清理 Shell PATH（可选）

安装脚本曾自动在您的 Shell 配置文件中追加了 `~/.local/bin` 的 PATH 导出。若您的系统中没有其他工具使用 `~/.local/bin`，并希望彻底复原：

1. 打开 Shell 配置文件（zsh 用户为 `~/.zshrc`，bash 用户为 `~/.bash_profile`）：
   ```bash
   nano ~/.zshrc
   ```
2. 找到并删除以下两行：
   ```bash
   # Multigravity CLI PATH
   export PATH="$HOME/.local/bin:$PATH"
   ```
3. 保存并刷新配置：
   ```bash
   source ~/.zshrc
   ```

---

## 5. 官方版本发布规范与偏好准则 (Release Guidelines)

为了保障生产与公开发布的纯净度与稳定性，项目建立并固化了以下发版准则（详细准则见 [`docs/release_preferences_and_workflow.md`](file:///Users/hal9000/Projects/antigravity-mobile/docs/release_preferences_and_workflow.md)）：

1. **严格限制发布平台（仅限充分实测平台）**：
   - 官方 Release 仅上架 5 个核心资产：
     - `Multigravity-v<version>.apk`（Android 客户端，带自签名直接安装）
     - `multigravity-darwin-arm64.tar.gz`（macOS Apple Silicon）
     - `multigravity-darwin-amd64.tar.gz`（macOS Intel）
     - `multigravity-darwin-universal.tar.gz`（macOS 双架构通用胖二进制）
     - `multigravity-windows-amd64.zip`（Windows x86_64）
   - **未经充分测试的 Linux 与 Windows ARM64 严禁进入 Release**。
2. **Release Notes 拒绝 Commit 刷屏**：
   - CI 配置禁止自动追加 `generate_release_notes`。
   - 统一采用干净专业、格式优美的 Release 说明（`.github/release_notes.md`）。
3. **网关网络嗅探高保真**：
   - 必须确保网关（`mgy`）编译包含最新的网络探测逻辑：自动屏蔽各类虚拟网卡（Clash / TUN / Docker / VM / Fake-IP 等），精准优先绑定 Wi-Fi / Ethernet 真实物理网卡。
4. **Git Tag 与源码包强制对齐**：
   - 重新打包发版时必须同步强制更新 Git Tag（`git tag -f <tag> && git push -f origin <tag>`），确保 GitHub 源码包与预编译二进制完全对齐。

---

## 6. 常见问题与故障排查 (FAQ)

### Q1: 安装后在终端输入 `mgy` 提示 `command not found`？
- **原因**：当前终端窗口是在安装完成前打开的，尚未加载更新后的 `PATH` 环境变量。
- **解决办法**：
  - 新开一个终端标签页；
  - 或在当前终端执行：
    ```bash
    source ~/.zshrc   # zsh 用户
    # 或
    source ~/.bash_profile # bash 用户
    ```

### Q2: 提示 `bind: address already in use` 端口冲突？
- **原因**：后台已有正在运行的 `mgy` 进程或其它服务占用了 `58900` 端口。
- **解决办法**：
  ```bash
  # 查找占用 58900 端口的进程
  lsof -i :58900
  # 停止已有 mgy 进程
  pkill -f mgy
  ```

### Q3: 提示 macOS 安全拦截或 Gatekeeper 弹窗？
- **原因**：二进制未经过苹果企业开发者证书签名。
- **解决办法**：安装脚本已自带本地 Ad-hoc 签名。若手动拷贝二进制，可手动执行：
  ```bash
  xattr -d com.apple.quarantine ~/.local/bin/mgy
  codesign -s - -f ~/.local/bin/mgy
  ```

### Q4: 手机扫码后提示「网络连接超时」？
- **排查顺口溜**：
  1. **同一 Wi-Fi 下**：确认手机与 Mac 处于同一局域网（且路由器未开启 AP 隔离）；
  2. **外网 4G/5G 下**：手机关闭 Wi-Fi，访问 `http://[IPv6地址]:58900` 测试光猫是否放行了入站连接；
  3. **无公网 IPv6 时**：建议在 `~/.multigravity/.env` 中配置 FRP 云服务器中继（参考 [HTTPS 域名中继配置指南](file:///Users/hal9000/Projects/antigravity-mobile/docs/https_cloud_relay_guide.md)）。

