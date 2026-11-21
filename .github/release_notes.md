# 🚀 Multigravity v1.2.4

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

- 📊 **iOS & Android 可滑动 Token 历史三视图图表与所见即所得海报**：
  - 新增三页左右滑动图表轮播：**近 14 天每日消耗柱状图**、**累计模型占比分析**与**右对齐铺满卡片的 20 周热力图**（彻底消除内层横向滚动手势冲突）；
  - 智能冷启动默认策略：活跃天数 `< 28` 天自动默认展示每日柱状图，`≥ 28` 天默认展示周热力图，手动切换后自动持久化记忆偏好；
  - Token 历史保存/分享长图海报与当前选中的图表视图保持所见即所得同步，完整覆盖全部主题皮肤与中 / 英 / 日三语。
- 🧩 **iOS 全尺寸额度小组件矩阵 & 全新「活跃对话」小组件（`QuotaStatusWidget` & `AgentStatusWidget`）**：
  - 额度监控小组件扩展支持全套 WidgetKit 尺寸（锁屏单行 `Inline` / 圆环 `Circular` / 矩形 `Rectangular`，以及桌面 `Small` / `Medium` / `Large` / iPad `ExtraLarge`）；
  - 新增独立 **`AgentStatusWidget`（活跃对话监控）** 小 / 中 / 大号桌面小组件，在 iPhone、iPad、StandBy 与 Mac 桌面上实时总览多会话运行进度与待审批状态，100% 适配中 / 英 / 日三语。
- 🇯🇵 **Android & PWA 原生日语（`ja`）全量覆盖，实现全端三语大满贯**：
  - Android 原生客户端（`values-ja`、`locales_config.xml`、`L10n.kt`）与嵌入式 PWA Web（`i18n.js`、`manifest-ja.json`）全面支持原生日语并可根据系统/浏览器语言自动识别切换；
  - 至此 iOS、Android、PWA Web、GitHub 文档与官方网站实现 100% 简体中文 / English / 日本語三语体验对齐。
- ⚡ **Cockpit 备选账号智能分级排序**：
  - 备选账号列表自动将 **ULTRA** 订阅账号置顶优先展示、**FREE** 免费账号排至末尾，多账号快速热切更直观高效。
- 🔧 **iOS 长会话滚动稳定性与 Tailscale 全链路穿透修复**：
  - 修复 iOS 超长对话浏览时 `LazyVStack` 偶发白屏、误触发回到底部吸附以及 Markdown 表格渲染高度跳变的问题；
  - 将 iOS 文件下载、气泡图片/媒体加载、QuickLook 预览、语音听写/实时通话及辅助 WebSocket 全面路由至 `NetworkTransport` / `GatewayWebSocketTask` Tailscale ATS 旁路通道，彻底解决纯 HTTP Tailscale / 局域网链路下的媒体与附件加载受阻问题。
- 🌐 **全端实时流稳定性、边缘省流与推送自愈优化**：
  - 修复 iOS 会话流 WebSocket 在代理/穿透网络下的首跳握手问题并增加 `25s` Ping 保活；网关 `/gateway/cascade/messages` 与 iOS / Android 双端全面支持 `ETag / 304 Not Modified` 零字节短路及空闲阶梯退避；
  - 修复 APNs 灵动岛 Push-to-Start 异常时的重试风暴（引入指数退避与环境错配 Token 自动清理），并新增穿透隧道凭据回收后的无感自愈重连。
- 💬 **社区与全端版本号对齐**：
  - 三语 README 新增微信用户群扫码入口；
  - Android `1.2.4`（Version Code `26`）；iOS `1.2.4`（Build `27`）；桌面/服务端网关 `mgy` `1.2.4`。

---

## 🇺🇸 English Release Notes

### ✨ Highlights & New Features

- 📊 **Swipeable 3-View Token History Charts & WYSIWYG Share Posters (iOS & Android)**:
  - Added a 3-page swipeable chart carousel: **14-day daily trend bars**, **cumulative model breakdown**, and a **right-aligned 20-week heatmap** filling the card width without inner scroll gesture conflicts;
  - Smart cold-start defaults (`< 28` active days defaults to daily trend bars, `≥ 28` days defaults to the heatmap) with local preference persistence once manually switched;
  - Synced Token History save/share posters (WYSIWYG) with the currently active chart across all curated themes and localized in Chinese, English, and Japanese.
- 🧩 **Full-Size iOS Quota Widget Suite & New Active Chats Widget (`QuotaStatusWidget` & `AgentStatusWidget`)**:
  - Expanded `QuotaStatusWidget` across all WidgetKit families (`accessoryInline`, `accessoryCircular`, `accessoryRectangular`, `systemSmall`, `systemMedium`, `systemLarge`, and iPad `systemExtraLarge`);
  - Added a dedicated **`AgentStatusWidget` (Active Chats)** supporting `systemSmall`, `systemMedium`, and `systemLarge` across iPhone, iPad, StandBy, and macOS Desktop for at-a-glance multi-session status and approval tracking in Chinese, English, and Japanese.
- 🇯🇵 **Full Native Japanese (`ja`) Support on Android & PWA Web**:
  - Added complete native Japanese localization with automatic system/browser locale detection for Android (`values-ja`, `locales_config.xml`, `L10n.kt`) and PWA Web (`i18n.js`, `manifest-ja.json`), achieving 100% trilingual (`zh` / `en` / `ja`) parity across iOS, Android, PWA, GitHub docs, and the official website.
- ⚡ **Tier-Aware Backup Account Sorting in Cockpit**:
  - Automatically prioritizes **ULTRA** accounts at the top and deprioritizes **FREE** accounts at the bottom of the backup accounts list for faster high-tier hot-swapping.
- 🔧 **iOS Long-Chat Scroll Stability & Tailscale Media/WebSocket Bypass Fixes**:
  - Fixed `LazyVStack` blank screen rendering, false snap-to-bottom jumps, and Markdown table height shifts when scrolling long conversations on iOS;
  - Routed iOS file downloads, inline media loaders, QuickLook previews, audio dictation, live voice calls, and secondary WebSockets through the custom `NetworkTransport` / `GatewayWebSocketTask` Tailscale ATS bypass so plain-HTTP Tailscale and LAN links work seamlessly end-to-end.
- 🌐 **Real-Time Stream Reliability, ETag Bandwidth Savings & Push Self-Healing**:
  - Unified iOS `StreamWebSocketClient` onto `GatewayWebSocketTask` with `25s` keep-alive pings for rock-solid Cloudflare Tunnel and proxy connectivity, plus end-to-end `ETag / 304 Not Modified` short-circuiting and idle polling backoff across the Go gateway, iOS, and Android;
  - Added exponential backoff and automatic invalid/sandbox-mismatched token cleanup for APNs Live Activity pushes, along with automatic Cloudflare Tunnel credential self-recovery.
- 💬 **Community & Version Alignment**:
  - Added WeChat User Group QR code to trilingual READMEs;
  - Android `1.2.4` (Version Code `26`); iOS `1.2.4` (Build `27`); desktop/server gateway `mgy` `1.2.4`.

---

## 🇯🇵 日本語リリースノート

- 📊 **スワイプ切替対応トークン履歴 3 ビューチャート＆WYSIWYG 共有ポスター（iOS & Android）**：**直近14日間の日別推移棒グラフ**・**モデル別累計内訳**・**右揃え20週間ヒートマップ**の3ページカルーセルを新搭載（スクロール競合を解消）。利用日数に応じたスマート初期表示（28日未満は日別グラフ、28日以上はヒートマップ）と選択状態の保存、現在表示中のチャートと連動するロング画像ポスター出力に対応しました。
- 🧩 **iOS フルサイズクォータウィジェット＆新「アクティブチャット」ウィジェット（`QuotaStatusWidget` & `AgentStatusWidget`）**：クォータ監視が全サイズ（`Inline` / `Circular` / `Rectangular` / `Small`〜`ExtraLarge`）に対応したほか、複数セッションの進行状況や承認待ちをホーム画面・iPad・Mac で一覧できる **`AgentStatusWidget`**（小・中・大）を追加しました（日・英・中 3言語対応）。
- 🇯🇵 **Android & PWA Web も完全日本語対応（全プラットフォーム 3言語統一）**：Android アプリおよび組み込み PWA Web がシステム / ブラウザ言語の自動判別によるネイティブ日本語表示に対応し、iOS・Android・PWA・GitHub・公式サイトすべてで日本語・英語・中国語の完全対応を達成しました。
- ⚡ **Cockpit バックアップアカウントの優先度ソート**：予備アカウント一覧で **ULTRA** プランを最上部に優先表示し、**FREE** プランを下部に配置するよう改善しました。
- 🔧 **iOS 長文チャットのスクロール安定化＆Tailscale 通信バイパス改善**：長文会話での `LazyVStack` 空白表示・意図しない最下部スナップ・Markdown テーブルの高さジャンプを修正。さらにファイルダウンロード・画像/メディア読込・QuickLook・音声入力/通話・補助 WebSocket を Tailscale ATS バイパス経路へ統合しました。
- 🌐 **リアルタイムストリーム安定化・ETag 通信削減・APNs 自動復旧**：iOS WebSocket ストリームの接続エンジン刷新と `25秒` キープアライブ追加、ゲートウェイおよび iOS / Android での `ETag / 304` 差分スキップ、APNs ライブアクティビティの指数バックオフとトークン自動修復、Cloudflare トンネル資格情報の自動再取得に対応しました。
- 📦 **バージョン統一**：iOS `1.2.4` (Build `27`) / Android `1.2.4` (Version Code `26`) / ゲートウェイ `mgy` `1.2.4`。
