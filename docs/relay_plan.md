# 亚太自建中继方案（备用设计）

> 状态：**备用，未实施**。当 Cloudflare 隧道在国内持续卡顿、且 `mgy` 启动日志里的边缘节点位置（`📍 [Cloudflare]`）长期是 `lax/sjc` 时启用。

## 1. 背景与诊断结论（2026-10-07）

- 网关主机是国内电信宽带（IPv6 `240e:`），`cloudflared` 的 8 条 HA 连接全部落在 **LAX**（lax07/09/10/13）。
- 主机到 LAX 的 RTT：IPv6 `2606:4700:a0::/48` 约 145ms，`a8::/48` 约 235ms。
- 手机请求先到某个 CF 边缘，经 CF 骨干网到 LAX，再回国内的主机：**一来一回两次跨洋**。
- `cloudflared --region` 仅支持默认全球 / `us`，没有亚太选项；边缘节点由运营商 anycast 决定，免费计划无法指定。
- Cloudflare China Network 需要企业版 + ICP 备案，不现实。
- 日志中另有：DNS 污染（`103.73.220.188:7844` 超时 56 次）、`argotunnel.com` 解析超时 61 次、QUIC 被干扰（被迫使用 http2）、`http2: stream closed` 700 余次。

## 2. 目标

- 用户侧**零配置**：网关启动后自动拿到公网地址，手机扫码即用（与现在体验一致）。
- 手机 ↔ 网关延迟从“几百 ms”降到约 100ms 以内。
- 不要求用户打洞、装 VPN、注册第三方账号。

## 3. 总体架构

```
手机 App ──HTTPS/WSS──▶ 香港/东京中继服务器 ◀──出站 WSS/yamux 长连接── 家里的 mgy 网关
        (30~60ms)        relay.jiuge.space          (40~80ms，由网关主动发起)
```

1. 网关启动时向调度器（现有 Worker `dispatcher.jiuge.space`）申请一个 relay slot，得到：`subdomain`、`relay_endpoint`、`slot_token`。
2. 网关向 `relay_endpoint` 建立**出站**长连接（WSS + yamux 多路复用），携带 `slot_token`。
3. 中继按 SNI / Host 把 `xxx.relay.jiuge.space` 的请求转发到对应的 yamux 会话，再由网关反向代理到本机 `127.0.0.1:58900`。
4. 手机侧 URL 形态与现在的 `https://xxxx.jiuge.space` 一致，App 无需改协议。

## 4. 服务器选型

| 项 | 建议 |
|---|---|
| 位置 | 香港（首选，国内三网延迟最均衡）；备选东京 |
| 线路 | CN2 GIA / CMI / 优化线路；避免纯国际线路 |
| 规格 | 1C1G 起步，带宽 30Mbps 以上，流量 ≥ 1TB/月 |
| 费用 | 约 ¥25–50 / 月 |
| 备案 | 香港/海外服务器**无需 ICP 备案** |
| 证书 | 通配符证书 `*.relay.jiuge.space`，DNS-01 自动签发（acme.sh / Caddy） |

## 5. 实现路线

### 5.1 中继服务端（二选一）
- **最快**：直接部署 [frp](https://github.com/fatedier/frp) 的 `frps`（`vhostHTTPSPort` + `subdomainHost`），网关内置/捆绑 `frpc`，配置由调度器下发。成熟，但协议与鉴权要按 slot 定制。
- **最贴合**：自写一个轻量 relay（Go，约 500 行）：`yamux` + WSS 入口 + Host 路由。可复用网关现有的 token / slot 机制，便于做配额与审计。

### 5.2 网关侧
- 新增 `internal/tunnel/relay.go`，与 `CloudflareTunnel` 并列，接口保持一致：`Start/Stop/PublicURL/Subdomain`。
- 断线重连：指数退避 + 抖动；心跳 20s（与现有 WS 心跳对齐）。
- 配置：`RELAY_ENABLED`、`RELAY_URL`、`RELAY_TOKEN`（`internal/config` 增加 `RelayConfig`）。

### 5.3 调度器（Worker）
- 新增 `POST /relay/register`：返回 `{subdomain, relay_endpoint, slot_token}`，复用 `GetStableMachineID` 做幂等。
- slot 配额与限速：单 slot 并发连接数、带宽上限，防止滥用。

### 5.4 App 侧：多地址竞速
- 候选地址：`[IPv6 直连, 中继, Cloudflare]`；启动与网络切换时并行探测 `/healthz`，选 RTT 最小者；失败自动降级。
- iOS/Android 已有“路由变更后重连”的逻辑（`StreamWebSocketClient`），只需在路由选择处增加候选列表。

## 6. 配套：IPv6 直连快速通道（可选）

- 网关检测到公网 IPv6 后上报调度器，调度器写灰云 AAAA 记录。
- App 优先尝试直连，成功则延迟最低、零中转成本；失败回落到中继。
- 注意：依赖路由器放行入站；部分家宽会封 80/443，使用高位端口通常可行。

## 7. 安全要点

- 中继只做 L4/L7 转发，**端到端仍由网关的配对 token 鉴权**；中继不保存明文业务数据。
- slot_token 一机一码，可吊销；中继日志不得记录 URL query 里的 token。
- 公网入口只暴露 443；管理端口仅内网。

## 8. 风险与成本

- 需要运维一台服务器与证书续期；单点故障 → 保留 Cloudflare 作为兜底通道。
- 若面向大量用户分发，带宽与流量成本由运营方承担，需要配额策略。
- 海外服务器仍可能遭遇夜间晚高峰丢包，建议选 CN2/CMI 线路并做多地域备份（香港 + 东京）。

## 9. 验收指标

- 手机（蜂窝/WiFi）→ 网关 `/healthz` P50 < 120ms，P95 < 300ms。
- 连续 24h 内 WebSocket 非主动断线 < 5 次。
- 网关重启后 10s 内公网地址恢复可用。
