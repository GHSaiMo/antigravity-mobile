# 🚀 Multigravity v1.2.5

> 🎉 **Multigravity（反重力智能体移动伴侣）iOS 版现已正式上架 Apple App Store！**  
> 无需本地编译或自签，前往 👉 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)**（覆盖海外 174 个国家/地区，内置零配置**离线演示模式**）；也可加入 👉 **[TestFlight 公开测试](https://testflight.apple.com/join/YM7f4aSK)** 抢先体验最新构建。
>
> 🎉 **Multigravity for iOS is officially available on the Apple App Store!**  
> Install directly from the 👉 **[App Store](https://apps.apple.com/app/id6835508934)** (174 countries & regions, with built-in zero-config **offline Demo Mode**), or join the 👉 **[TestFlight Public Beta](https://testflight.apple.com/join/YM7f4aSK)** for pre-release builds.
>
> 🎉 **Multigravity iOS 版は Apple App Store で正式配信中！**  
> 👉 **[App Store からダウンロード](https://apps.apple.com/app/id6835508934)**（174 カ国・地域対応、オフラインデモモード内蔵）または 👉 **[日本語公式サイト](https://mgy.jiuge.space/ja.html)** をご覧ください。

---

## 🇨🇳 中文更新说明

### ✨ 核心亮点与新特性

- 🔐 **全栈零知识端到端加密（`mgy-e2ee-v1`）与多地域 VPS 中继池**：
  - 全新引入 **`mgy-relay`** 零知识反向中继服务（`cmd/relay`、`internal/relay`）与多节点 **`RelayPoolProvider`**，支持并发 RTT 测速、滞回防抖钉选、故障自动切换、周期性节点池刷新及 **Cloudflare Tunnel 自动无缝回退**；
  - Cloudflare Worker 调度层升级至 **`v1.7.0`**，支持多 VPS `RELAY_POOL` 调度（含 `host_template` 与节点级 `secret_env` 独立 HMAC 签名密钥）、区域优先级排序与无状态 `GET /api/relay/pool`；
  - 全端落地 **`mgy-e2ee-v1`** 加密协议（ECDH P-256 + HKDF-SHA256 + AES-256-GCM），中继网关启用 `RelayGuardMiddleware` 严禁明文流量通过（直返 `HTTP 426`），完整覆盖加密 HTTP RPC（`/api/v1/e2ee/rpc`）、加密 WebSocket（`/api/v1/e2ee/ws`）、加密配对（`/api/v1/e2ee/pair`）与存量设备静默升级（`/api/v1/auth/e2ee/upgrade`）；
  - 网关内置**双轨平滑迁移**机制，配对二维码与 `mgy cf` 状态页同步清晰标注 E2EE 中继状态，并优化端口监听优先于 `admin_token` 初始化（避免端口冲突时误刷凭据）；未升级的老版本客户端继续沿用 Cloudflare Tunnel，待所有已配对设备完成 E2EE 升级后无缝切换至零知识中继；
  - iOS（含 WidgetKit 小组件）、Android（API 26+ JCA 与全量 WebSocket）及 PWA Web 全面透明接入 E2EE 传输与无状态 `RelayPoolResolver`，并新增区域 VPS 直连 DNS 旁路避开 Cloudflare Anycast 绕路。
- 🧭 **超长会话全量轮次大纲、按需跳转与尾部窗口磁盘缓存（Go / iOS / Android）**：
  - 代理层与 iOS / Android 双端新增**全量会话轮次大纲（Turn Outline）**及按需加载跳转能力，配合尾部窗口磁盘缓存机制，超长会话既可毫秒级秒开最新对话，又能通过目录瞬时定位并按需拉取任意历史轮次，大幅降低内存占用。
- 🌍 **安装/卸载脚本全量原生中/英/日三语自适应与智能路由**：
  - `scripts/install.sh`、`install.ps1`、`uninstall.sh`、`uninstall.ps1` 全面支持 **简体中文 / English / 日本語** 原生本地化提示，并依据系统语言与代理连通状态智能选择 GitHub 直连或国内镜像加速源；macOS 卸载脚本新增自动 `launchctl bootout` 彻底清理已注册的 LaunchAgents。
- 🤖 **移动端 Token 分享海报修复、Android 相册排序与文档下载重构**：
  - 修复 iOS & Android Token 历史分享长图卡片截断当日模型列表的问题，现完整展示当日全量模型消耗明细；
  - 修复 Android 相册选择器（`AttachmentPickerSheet`）因 AOSP `QUERY_ARG_SORT_COLUMNS` 多列拼接导致「最近项目」倒序（最旧照片排在最前）的问题，改用显式 SQL 双降序与时间戳精确排序；
  - 重构 Android 文档下载状态流与进度弹窗：独立单调递增 `documentDownloadProgress` 流避免并发状态刷新导致的进度条跳回或闪烁，取消下载立即中断底层 OkHttp 请求并清理唯一临时文件，同时修复带 `?uri=` 参数的附件文件名提取。
- 🎨 **iOS 离线演示模式（Demo Mode）升级、工具本地化补齐与三语视觉资产焕新**：
  - 升级 iOS 内置离线 Demo Mode（`DemoContent` / `DemoGateway` / `QuotaWidgetShared`），完整对齐 Antigravity 2.34、Gemini 4 Neon / Orchestrator / Opus 5.5、Swarm 蜂群与小组件真实数据结构；
  - 补全 `write_file`、`run_workflow`、`swarm`、`post_message`、`read_board`、`swarm_wait`、`checkpoint` 及会话运行态标签的三语本地化；
  - 中 / 英 / 日三语 README 同步换装全新分语言 12 图暗黑实机界面画廊与 Hero 横幅。
- 📦 **全端版本号对齐**：
  - Android `1.2.5`（Version Code `32`）；iOS `1.2.5`（Build `33`）；桌面/服务端网关 `mgy` `1.2.5`。

---

## 🇺🇸 English Release Notes

### ✨ Highlights & New Features

- 🔐 **Zero-Knowledge End-to-End Encryption (`mgy-e2ee-v1`) & Multi-Region VPS Relay Pool**:
  - Introduced the **`mgy-relay`** zero-knowledge reverse-relay server (`cmd/relay`, `internal/relay`) and multi-node **`RelayPoolProvider`** featuring concurrent RTT probing, hysteresis pinning, automatic failover, periodic pool refreshes, and seamless **Cloudflare Tunnel fallback**;
  - Upgraded the Cloudflare Worker dispatcher to **`v1.7.0`** with multi-VPS `RELAY_POOL` orchestration (supporting `host_template` and per-node `secret_env` HMAC signing secrets), region ordering, and stateless `GET /api/relay/pool`;
  - Implemented cross-platform **`mgy-e2ee-v1`** (ECDH P-256 + HKDF-SHA256 + AES-256-GCM) with strict `RelayGuardMiddleware` enforcement (rejecting plaintext over relay with `HTTP 426`), covering encrypted HTTP RPC (`/api/v1/e2ee/rpc`), encrypted WebSockets (`/api/v1/e2ee/ws`), encrypted pairing (`/api/v1/e2ee/pair`), and silent legacy upgrade (`/api/v1/auth/e2ee/upgrade`);
  - Wired **dual-track migration** in the gateway with clear E2EE relay labels in pairing QR / `mgy cf` status and port-binding before `admin_token` initialization so older paired clients continue operating via Cloudflare Tunnel until every paired device completes the silent E2EE upgrade;
  - Added transparent E2EE transport and stateless `RelayPoolResolver` across iOS (including WidgetKit), Android (API 26+ JCA and all WebSockets), and Web PWA, plus direct regional VPS DNS bypass for Cloudflare Anycast accelerators.
- 🧭 **Full-Session Turn Outline, On-Demand Turn Jump & Tail-Window Disk Cache (Go / iOS / Android)**:
  - Added full-session turn outline indexing and on-demand turn jumping paired with tail-window disk caching across the Go proxy, iOS, and Android — keeping ultra-long conversations fast to open while enabling instant TOC jumps to any historical turn on demand.
- 🌍 **Trilingual (`zh` / `en` / `ja`) Install & Uninstall Scripts with Locale-Aware Routing**:
  - Upgraded `scripts/install.sh`, `install.ps1`, `uninstall.sh`, and `uninstall.ps1` with native Chinese, English, and Japanese localization, locale- and proxy-aware GitHub direct vs. mirror routing, and automatic `launchctl bootout` LaunchAgent cleanup during macOS uninstall.
- 🤖 **Token Share Poster Fix, Android Photo Ordering Fix & Resilient Document Downloads**:
  - Fixed iOS & Android Token History share cards so all daily models are displayed completely without truncation;
  - Fixed Android `AttachmentPickerSheet` showing Recents in oldest-first order (caused by AOSP `QUERY_ARG_SORT_COLUMNS` multi-column SQL expansion) by switching to explicit `DATE_ADDED DESC, _ID DESC` ordering;
  - Refactored Android document downloads with a dedicated monotonic `documentDownloadProgress` flow, smooth animated progress bars, instant OkHttp call cancellation and temp-file cleanup, and accurate filename decoding for `?uri=` query links.
- 🎨 **Upgraded iOS Offline Demo Mode, Tool Localizations & Trilingual README Visuals**:
  - Refreshed iOS built-in Demo Mode (`DemoContent`, `DemoGateway`, `QuotaWidgetShared`) to reflect Antigravity 2.34, Gemini 4 Neon / Orchestrator / Opus 5.5, Swarm Hive, and live widget snapshots;
  - Completed trilingual localization for newer tool names (`write_file`, `run_workflow`, `swarm`, `post_message`, `read_board`, `swarm_wait`, `checkpoint`) and conversation status labels;
  - Updated all trilingual READMEs with localized dark-mode 12-screen galleries and Hero banners.
- 📦 **Version Alignment**:
  - Android `1.2.5` (Version Code `32`); iOS `1.2.5` (Build `33`); desktop/server gateway `mgy` `1.2.5`.

---

## 🇯🇵 日本語リリースノート

- 🔐 **ゼロ知識エンドツーエンド暗号化（`mgy-e2ee-v1`）＆マルチリージョン VPS リレープール**：ゼロ知識リバースリレー `mgy-relay` とマルチノード `RelayPoolProvider`（並列 RTT 計測・ヒステリシス固定・自動フェイルオーバー・Cloudflare Tunnel 自動フォールバック）を新搭載。Cloudflare Worker を `v1.7.0`（`host_template` / ノード別 `secret_env` 対応）へ刷新し、ECDH P-256 + HKDF-SHA256 + AES-256-GCM による E2EE 通信（リレー経路の平文は `HTTP 426` で厳格拒否）、QR/`mgy cf` での E2EE リレー表示、ポート先行バインド、および既存端末向けデュアルトラック自動移行を実装しました。
- 🧭 **超長文セッション向け全ターン目次・オンデマンドジャンプ＆末尾ウィンドウキャッシュ**：Go プロキシ・iOS・Android に全ターン目次（Turn Outline）とオンデマンド読み込みジャンプ、末尾ウィンドウディスクキャッシュを追加。巨大セッションでも最新発言を即座に開きつつ、任意の過去ターンへ高速移動できます。
- 🌍 **インストール／アンインストールスクリプトの完全 3 言語対応**：`install.sh` / `install.ps1` / `uninstall.sh` / `uninstall.ps1` が日本語・英語・中国語にネイティブ対応し、ロケールとプロキシ環境に応じて最適ダウンロード経路を自動選択。macOS アンインストール時の LaunchAgents `bootout` 掃除も追加しました。
- 🤖 **トークン共有ポスター全モデル表示・Android アルバム並び順＆ダウンロード刷新**：iOS / Android のトークン履歴共有カードで当日の全モデルを省略せず表示するよう修正。Android アルバムの古い順バグ修正と単調増加プログレス・即時キャンセル対応も実装しました。
- 🎨 **iOS オフラインデモモード刷新・ツール名ローカライズ補完＆3 言語 README 刷新**：Demo Mode を Antigravity 2.34 / Gemini 4 Neon / Swarm 準拠へ全面更新し、新ツール名（`swarm` / `run_workflow` 等）のローカライズと日・英・中 3 言語 README の 12 枚ダーク UI スクリーンショットを刷新しました。
- 📦 **バージョン統一**：iOS `1.2.5` (Build `33`) / Android `1.2.5` (Version Code `32`) / ゲートウェイ `mgy` `1.2.5`。
