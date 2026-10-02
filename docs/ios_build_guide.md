# Multigravity iOS 原生应用本地构建与真机调试指南 🍎📱

本文档指导开发者如何在 macOS 环境下使用 Xcode 本地编译、调试并安装 **Multigravity iOS 客户端**。

---

## 🛠️ 1. 环境准备要求

在开始构建前，请确保你的开发环境满足以下要求：

- **操作系统**：macOS Sonoma (14.0) 或更高版本（推荐 macOS Sequoia 15+）
- **开发工具**：Xcode 15.3 或更高版本（推荐 Xcode 16.0+，支持 Swift 6 严格并发检查）
- **开发语言**：Swift 6.0 / SwiftUI 5
- **调试设备**：
  - 物理 iPhone / iPad（iOS 17.0+）
  - 或 Xcode 模拟器（iOS 17.0+）
- **Apple ID**：支持任意个人免费 Apple 账号（无需付费开发者计划即可通过 Xcode 免费签名自签调试 7 天）

---

## 📂 2. 工程目录结构说明

iOS 原生客户端工程位于仓库根目录的 `ios/` 文件夹下：

```text
ios/
├── Multigravity.xcodeproj          # Xcode 主工程文件
├── Multigravity/                   # Swift 源码与资源目录
│   ├── App/                        # 应用入口 (MultigravityApp.swift)
│   ├── Views/                      # SwiftUI 视图组件
│   │   ├── Chat/                   # 实时对话流、多模态渲染与打字机动效
│   │   ├── Interaction/            # 原生选择题决策卡片 (InteractionCardView)
│   │   ├── Pairing/                # 扫码与手动配对抽屉
│   │   ├── Cockpit/                # 四象限配额看板与多账号热切
│   │   └── Settings/               # 多通道网络测速与设置
│   ├── ViewModels/                 # 核心状态机与并发流控制
│   ├── Services/                   # WebSocket 长连接与网络传输
│   ├── Models/                     # 数据模型与 Protobuf 解析
│   └── Assets.xcassets/            # 官方应用图标 (AppIcon-1024) 与配色资源
└── build/                          # 本地派生数据与构建缓存
```

---

## 🚀 3. 极速打开与依赖解析

1. **进入工程目录**：
   双击打开 `ios/Multigravity.xcodeproj`，或在终端执行：
   ```bash
   open ios/Multigravity.xcodeproj
   ```

2. **自动解析 Swift Package 依赖**：
   工程使用 **Swift Package Manager (SPM)** 进行纯净原生依赖管理（无 CocoaPods 或 Carthage 依赖）。
   - 打开 Xcode 后，Xcode 会自动解析外部依赖包（如图片缩放库 `ZLPhotoBrowser` 等）。
   - 如果遇到依赖解析中断，可手动在 Xcode 顶部菜单栏点击：
     **File ➔ Packages ➔ Resolve Package Versions**。

---

## ✍️ 4. 配置个人开发者签名 (Signing)

为了能够在真实 iPhone / iPad 上安装运行，需要配置个人签名：

1. 在 Xcode 左侧导航栏中，点击最顶部的 **`Multigravity` 蓝底工程图标**。
2. 在主界面左侧列表中选择 **TARGETS ➔ Multigravity**。
3. 切换至 **`Signing & Capabilities`** 选项卡：
   - 勾选 **`Automatically manage signing`**（自动管理签名）；
   - 在 **`Team`** 下拉菜单中选择你的个人 Apple ID（若未添加，点击 `Add an Account...` 登录你的 Apple 账号即可）；
   - 将 **`Bundle Identifier`** 中的后缀修改为你独有的标识符（例如由 `com.multigravity.app` 改为 `com.yourname.multigravity`），避免与已有 App ID 冲突。

---

## 📱 5. 真机连接与排查规范

### 连接设备
- 使用 USB-C / Lightning 数据线将 iPhone 连接至 Mac；
- 在 iPhone 弹窗中选择 **“信任此电脑”** 并输入锁屏密码；
- 在 Xcode 顶部运行设备选择器中，将目标由模拟器切换为你的物理 iPhone。

> 🚨 **Xcode 设备连接与无线调试排查避坑规范**：
> - 若使用无线局域网调试时出现连接中断，且 Mac 本机**有线网卡（以太网）已连接并有网**：
>   - **必须优先确认或关闭 Mac 本机 Wi-Fi**，避免双网卡处于同一局域网网段时引发 mDNS (Bonjour) 组播分流与非对称路由冲突；
>   - 检查系统代理软件（如 Clash / Surge / Surge VIF）是否接管了局域网握手流量导致 `remotepairingd` 无法直连设备。

---

## 🔨 6. 本地一键构建与安装

### 方式 A：Xcode 图形界面一键运行
直接按下快捷键 **`Cmd + R`**（或点击 Xcode 左上角的 ▶ 运行按钮），Xcode 将自动编译工程并将 App 推送安装到手机上。

*首次安装提示*：
- 若 iPhone 提示 *“不受信任的开发者”*：
  前往 iPhone **设置 ➔ 通用 ➔ VPN 与设备管理 ➔ 开发者 App**，点击信任你的 Apple 账号证书即可。
- iOS 16+ 用户需确保在 **设置 ➔ 隐私与安全性 ➔ 开发者模式** 中开启“开发者模式”并重启设备。

---

### 方式 B：终端命令行静默编译校验
若在 CI/CD 或终端执行本地代码改动验证，可运行以下命令（免代码签名测试编译）：

```bash
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcodebuild \
  -project ios/Multigravity.xcodeproj \
  -scheme Multigravity \
  -destination 'generic/platform=iOS' \
  clean build CODE_SIGN_IDENTITY="" CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=NO
```

当终端输出 `** BUILD SUCCEEDED **` 时，即表明 iOS 客户端代码编译完全正常。

---

## 🔑 7. 首次启动与扫码连接

1. 确保电脑终端已启动网关服务：
   ```bash
   mgy
   ```
2. 终端将输出一个专属的 ASCII 配对二维码；
3. 打开 iPhone 上的 Multigravity 应用，点击主界面的 **“扫码配对”** 按钮，对准电脑终端屏幕上的二维码一扫，即可秒级建立端到端加密连接！
