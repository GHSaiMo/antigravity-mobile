# Privacy Policy for Multigravity

**Effective Date:** November 15, 2026  
**Contact & Support:** [https://github.com/GHSaiMo/antigravity-mobile/issues](https://github.com/GHSaiMo/antigravity-mobile/issues)

Multigravity ("the App") is an open-source mobile companion client for connecting directly to a self-hosted Multigravity gateway running on your own computer. We respect your privacy and have designed the App so that you retain full control of your data.

## 1. Data Collection & Analytics
- **No Tracking or Third-Party Analytics:** Multigravity does not include third-party advertising SDKs, behavioral tracking, crash-reporting telemetry, or analytics services.
- **No Developer-Operated Cloud Servers:** All conversations, code previews, files, terminal outputs, and project states are transmitted directly between your iOS device and the gateway address that you explicitly configure (via local LAN IP, Tailscale, or your own HTTPS domain/tunnel).

## 2. Information Stored on Your Device
To make the App function seamlessly across sessions, the following information is stored locally on your iOS device (for example in `UserDefaults` and app container storage):
- Gateway host addresses, ports, TLS preferences, and pairing authentication tokens you enter or scan via QR code.
- Cached conversation summaries, project workspaces, UI preferences (such as theme, layout, or model selection), and local speech/file caches.

You can remove cached data or disconnect your paired gateway at any time inside **Settings** inside the App.

## 3. Device Permissions
The App only requests iOS system permissions when you trigger a related feature:
- **Camera:** Used exclusively to scan pairing QR codes displayed by your self-hosted gateway.
- **Local Network:** Used to discover (`_multigravity._tcp` / `_antigravity._tcp`) and communicate with your gateway over your local Wi-Fi or personal VPN.
- **Microphone & Speech Recognition:** Used only when you activate voice input to dictate prompts.
- **Photo Library:** Used only when you choose to attach photos/videos to a prompt or save generated images/snapshots to your photo library.

## 4. Demo Mode
When you launch **Demo Mode**, all sample conversations, files, workflows, and terminal outputs are simulated entirely offline on your device without any network requests.

## 5. Open Source & Changes
Multigravity is distributed under the MIT License at [https://github.com/GHSaiMo/antigravity-mobile](https://github.com/GHSaiMo/antigravity-mobile). Updates to this Privacy Policy will be published in this repository.

---

# Multigravity 隐私政策

**生效日期：** 2026 年 11 月 15 日  
**支持与反馈：** [https://github.com/GHSaiMo/antigravity-mobile/issues](https://github.com/GHSaiMo/antigravity-mobile/issues)

Multigravity（下称“本应用”）是一款用于连接您个人电脑上自托管 Multigravity 网关的开源移动端伴侣客户端。我们尊重您的隐私，应用在架构上确保所有数据均由您自行掌控。

## 1. 数据收集与统计分析
- **无广告追踪与第三方统计：** 本应用不包含任何第三方广告 SDK、行为追踪、崩溃遥测或数据统计服务。
- **无开发者托管云端服务器：** 所有对话内容、代码预览、文件、终端输出与项目状态均仅在您的 iOS 设备与您手动配对的个人网关地址（局域网 IP、Tailscale 或您自建的 HTTPS 隧道）之间直接通信。

## 2. 本地存储的信息
为保障断线重连与使用体验，应用仅在您的 iOS 设备本地（`UserDefaults` 与沙盒目录）保存以下必要信息：
- 您主动输入或扫码配对的网关主机地址、端口、TLS 选项与配对鉴权凭证；
- 会话历史缓存、工作区列表、界面偏好设置（主题、模型选择等）与本地图片/语音临时缓存。

您随时可在应用内**设置**页面中清除本地缓存或取消网关配对。

## 3. 系统权限用途说明
本应用仅在您主动触发对应功能时请求以下系统权限：
- **相机权限：** 仅用于扫描您个人网关生成的配对二维码；
- **本地网络权限：** 仅用于在局域网或个人 VPN 内自动发现（Bonjour）并连接您的电脑网关；
- **麦克风与语音识别权限：** 仅在您使用语音输入发送指令时启用；
- **相册权限：** 仅在您主动选择图片附件或将卡片/图片保存到系统相册时使用。

## 4. 演示模式
在**演示模式**下，所有示例项目、对话记录、工作流与终端输出均在设备本地离线模拟生成，不会发起任何网络请求。
