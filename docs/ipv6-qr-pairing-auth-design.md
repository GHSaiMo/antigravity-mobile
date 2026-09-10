# 功能设计说明：IPv6 直连与二维码扫码配对鉴权 (方案 A)

> **定位**：供后续功能开发（网关端与客户端）直接参考的技术实现规格说明书。

---

## 1. 方案概述

通过“Mac 网关生成一次性配对二维码 + 手机扫码获取长效设备凭证（Device Token）”的机制，实现手机端在公网 IPv6 环境下免密安全直连 Mac 端 Antigravity 网关。

* **鉴权策略**：身份认证绑定在**设备独占 Token**（应用层），而非手机来源 IP。彻底解耦 5G / Wi-Fi 切换及 IPv6 隐私扩展导致的 IP 漂移问题。
* **通信协议**：端到端 TLS（HTTPS / WSS）经由 IPv6 / DDNS 域名直连。

---

## 2. 协议与交互规范

### 2.1 配对二维码 Payload 格式
二维码内容为标准 Custom Scheme URI：

```text
agy://pair?host=<MAC_HOST>&port=<PORT>&code=<PAIRING_CODE>&ssl=1
```

* `host`：Mac 端的 DDNS 域名（优先，如 `mac.hal9000.xyz`）或公网 IPv6 地址（如 `[240e:...]`）。
* `port`：网关对外监听端口（默认 `58900`）。
* `code`：网关内存生成的强随机一次性 Token（建议 32 字节十六进制字符，有效期 5 分钟）。
* `ssl`：`1` 表示强制使用 HTTPS / WSS。

### 2.2 接口时序与握手流程

```
手机端 (iOS / PWA)                             Mac 网关 (Go)
       │                                             │
       │ 1. 扫码解析出 host, port, pairing_code        │
       │                                             │
       │ 2. POST /api/v1/auth/pair                   │
       │    Body: {                                  │
       │      "pairing_code": "...",                 │
       │      "device_name": "iPhone 15 Pro",        │
       │      "platform": "ios"                      │
       │    }                                        │
       ├────────────────────────────────────────────>│
       │                                             │ 3. 校验 pairing_code:
       │                                             │    - 验证通过后立即废弃 (防重放)
       │                                             │ 4. 签发独占凭证并持久化:
       │                                             │    - 生成 device_id
       │                                             │    - 生成 device_token
       │                                             │    - 计算 TokenHash 存入本地库
       │ 5. 返回 200 OK:                             │
       │    {                                        │
       │      "device_id": "dev_abc123",             │
       │      "device_token": "tok_xyz789..."        │
       │    }                                        │
       │<────────────────────────────────────────────┤
       │                                             │
       │ 6. 手机将 device_token 存入系统安全存储        │
       │    (iOS Keychain / PWA IndexedDB)           │
       │                                             │
   ════╪═════════════════════════════════════════════╪════
       │              后续正常通信 (所有请求)           │
       │                                             │
       │ 7. HTTP/ConnectRPC 请求                      │
       │    Header: Authorization: Bearer <token>    │
       │    (WebSocket: ?auth_token=<token>)         │
       ├────────────────────────────────────────────>│
       │                                             │ 8. 鉴权中间件拦截:
       │                                             │    - 校验 TokenHash 是否存在且有效
       │                                             │    - 注入 x-codeium-csrf-token
       │                                             │ 9. 转发请求给 language_server
       │ 10. 返回响应 / WebSocket 消息                │
       │<────────────────────────────────────────────┤
```

---

## 3. 网关服务端实现规范 (Go)

### 3.1 凭据存储结构 (`~/.antigravity-mobile/auth_store.json`)
```go
package auth

import "time"

// 已配对授权的设备记录
type PairedDevice struct {
    DeviceID    string    `json:"device_id"`    // 设备唯一 ID (如 dev_d0e1f2)
    DeviceName  string    `json:"device_name"`  // 友好名称 (如 "User's iPhone")
    Platform    string    `json:"platform"`     // ios / pwa / macos
    TokenHash   string    `json:"token_hash"`   // SHA-256(device_token)，不存明文
    CreatedAt   time.Time `json:"created_at"`   // 配对时间
    LastSeenAt  time.Time `json:"last_seen_at"`  // 最近活跃时间
    LastSeenIP  string    `json:"last_seen_ip"` // 最近连接来源 IP (仅用于审计日志展示)
}

// 内存中的一次性扫码配对 Session
type PairingSession struct {
    Code      string
    ExpiresAt time.Time
}
```

### 3.2 路由与 API 规范

| Method | Path | 鉴权要求 | 说明 |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/auth/pair` | 无（凭一次性 Code） | 手机端上报配对码，换取长效 `device_token` |
| `GET`  | `/api/v1/devices` | 需要本机/管理权限 | 列出所有已绑定的设备列表 |
| `DELETE` | `/api/v1/devices/:id` | 需要本机/管理权限 | 吊销踢出指定设备 |
| `*`    | `/codeium.cascade.*` | **必须携带 Device Token** | 所有 ConnectRPC 业务反向代理 |
| `GET`  | `/connect-websocket` | **必须携带 Device Token** | WebSocket 长连接反向代理 |

### 3.3 鉴权中间件实现逻辑 (Middleware)
1. **白名单判定**：
   * 路径为 `/api/v1/auth/pair`，或静态资源 `/web/*` 时，直接放行。
2. **凭据提取**：
   * 优先从 HTTP 请求头提取：`Authorization: Bearer <token>`；
   * WebSocket 请求（浏览器端限制自定义 Header）支持从 Query 参数提取：`?auth_token=<token>`。
3. **验证与状态更新**：
   * 对提取的 Token 计算 SHA-256，与 `auth_store` 中的 `TokenHash` 匹配；
   * 匹配失败：返回 HTTP `401 Unauthorized`；
   * 匹配成功：异步更新该设备的 `LastSeenAt` 和 `LastSeenIP`，继续执行后续代理链（自动注入本地 `x-codeium-csrf-token`）。

### 3.4 终端二维码展示
网关启动或通过命令 `antigravity-mobile pair` 触发时，在控制台打印 ANSI 二维码：
```go
import "github.com/skip2/go-qrcode"

func PrintPairingQRCode(host string, port int, code string) {
    uri := fmt.Sprintf("agy://pair?host=%s&port=%d&code=%code&ssl=1", host, port, code)
    qr, err := qrcode.New(uri, qrcode.Medium)
    if err == nil {
        fmt.Println(qr.ToSmallString(false))
        fmt.Printf("\n请使用手机扫描上方二维码完成配对 (5分钟内有效)\n\n")
    }
}
```

---

## 4. 客户端实现规范

### 4.1 iOS 原生客户端 (Swift)
1. **扫码入口**：
   * 使用 `AVFoundation` 的 `AVCaptureMetadataOutput` 捕获二维码。
   * 正则匹配 `agy://pair` URI，解析出 `host`、`port`、`code`。
2. **配对请求**：
   * 调用 `POST https://[host]:[port]/api/v1/auth/pair`。
   * 携带 `UIDevice.current.name` 作为 `device_name`。
3. **凭据持久化**：
   * 将服务端返回的 `device_token` 与 `host`、`port` 写入 iOS **Keychain Services**（使用 `kSecAttrAccessibleAfterFirstUnlock` 属性）。
4. **请求拦截注入**：
   * 实现统一网络层 Interceptor，为所有上行 ConnectRPC 请求追加：
     `Authorization: Bearer <device_token>`。
5. **失效与重置处理**：
   * 当任意业务请求收到 `401 Unauthorized` 时，判定设备已被 Mac 端吊销。
   * 清除 Keychain 中的 Token，将 UI 重置回“扫码连接”初始引导页。

### 4.2 移动 Web / PWA 客户端 (Vanilla JS)
1. **扫码/解析**：
   * 引入轻量 HTML5 扫码库（如 `html5-qrcode`）或支持 URL 手动粘贴。
2. **存储**：
   * 成功获取 Token 后持久化至 `localStorage` 或 `IndexedDB`。
3. **WebSocket 握手**：
   * 连接时拼接 Token：`new WebSocket("wss://" + host + ":" + port + "/connect-websocket?auth_token=" + token)`。

---

## 5. 运行环境与配套依赖

本功能若在脱离 Cloudflare Tunnel 的纯公网 IPv6 环境下运行，需要以下三项基础设施配合：

1. **Mac 端 IPv6 DDNS 模块**：
   * 家宽 IPv6 前缀会随光猫重拨改变。
   * 网关内需常驻轻量 DDNS 协程，定时（如每 5 分钟）检测本机公网 IPv6，自动同步至 Cloudflare / 腾讯云等 DNS 解析记录（例如 `mac.example.com`）。二维码中的 `host` 统一填写该域名。
2. **自动化 TLS 证书 (ACME)**：
   * iOS 与现代移动浏览器对直连通信强制要求受信任的 HTTPS / WSS 证书。
   * 网关使用 `golang.org/x/crypto/acme/autocert` 配合 DDNS 域名，全自动申请并续期 Let's Encrypt 证书。
3. **路由器 IPv6 端口放行**：
   * 需在家庭主路由器配置防火墙规则：允许外部访问 Mac 本机对应 IPv6 地址的指定端口（如 `58900`）。
