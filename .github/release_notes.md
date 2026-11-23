# 🚀 Multigravity v1.2.6

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

- 📋 **全端原生步骤计划卡片与实时计时器（Go / iOS / Android / Web / Live Activity）**：
  - 代理层与 iOS / Android / PWA Web 全端新增桌面级对齐的**步骤计划卡片（Step Plan Card）**与**实时焦点计时器（Live Focus Timer）**，清晰呈现计划步骤进展与当前耗时，并同步透出至 iOS 灵动岛与锁屏 Live Activity。
- 🛡️ **全 5 池智能额度守护、官方提前回满喜报、异常骤降预警与三语原生推送（Go 网关）**：
  - 额度守护从单一 `Gemini 5h` 全面升级至全 5 池覆盖（**`Gemini 5h` / `Gemini Weekly` / `Claude 5h` / `Claude Weekly` / `Gemini 4 Argon`**），并直接接通无 Cockpit Tools 的本机 `FetchLiveQuota` 实时采样，未装 Cockpit 的新用户开箱即可生效；
  - 新增**官方临时额度重置（Bonus Reset）喜报推送**（精准区分正常到期与提前回满，多池同轮回满自动合并为 1 条），以及**短时骤降 & 本机空闲期静默掉额预警**（结合 `Watcher.IdleSince()` 严防任务结束后的滞后误报）；
  - 网关 CLI 与全部推送通知（审批 / 提问 / Proceed / 完成 / 失败 / 额度守护）全面升级为原生 **简体中文 / English / 日本語** 三语自适应（支持 `--lang ja`、`MGY_LANG=ja` 及 macOS / Windows / Linux 系统语言自动识别）。
- 🧹 **过期锁屏审批状态核验与桌面已处理横幅静默撤回（Go / iOS / Android / APNs Worker）**：
  - 当桌面端或另一设备已完成审批后，网关通过静默推送自动清除手机通知中心中已送达的过期审批横幅；
  - 同步加固 iOS Live Activity 按钮与 Android 通知栏快捷操作的过期状态核验，避免误触过期锁屏审批请求。
- 🔋 **宿主机任务期动态自动防休眠守护（macOS / Windows / Linux）**：
  - 新增跨平台任务期按需防休眠守卫（macOS `caffeinate -i -w <pid>` / Windows `SetThreadExecutionState` / Linux `systemd-inhibit`），仅在存在活跃 Antigravity 会话时自动阻止宿主机空闲休眠（允许正常熄屏省电），全部任务空闲 2 分钟后自动释放，网关退出或异常终止零残留锁；支持 `-prevent-sleep=false` 或 `MGY_PREVENT_SLEEP=0` 关闭。
- ⚡ **E2EE 中继自愈、直连 RTT 优选与双端交互修复（Go / iOS / Android）**：
  - iOS 会话事件流 WebSocket 全面切换至 E2EE `GatewayWebSocketTask` 传输，`RelayPool` 优选直连物理 RTT 进一步降低远程延迟；
  - Android 在 E2EE 中继返回 `502 / 503` 时自动迁移健康节点并在路由切换时同步刷新额度与事件流，同时新增自定义 `MgyPullToRefreshIndicator` 修复下拉刷新收起后的阴影残留拖尾问题。
- 📦 **全端版本号对齐**：
  - Android `1.2.6`（Version Code `34`）；iOS `1.2.6`（Build `34`）；桌面/服务端网关 `mgy` `1.2.6`。

---

## 🇺🇸 English Release Notes

### ✨ Highlights & New Features

- 📋 **Desktop-Parity Step Plan Card & Live Focus Timer (Go / iOS / Android / Web / Live Activity)**:
  - Added native **Step Plan Cards** and a **Live Focus Timer** across the Go proxy, iOS, Android, and Web PWA — mirroring the desktop step checklist and live elapsed duration, and syncing active step progress directly into iOS Dynamic Island and Lock Screen Live Activities.
- 🛡️ **5-Bucket Quota Guardian, Official Early Bonus-Reset Alerts, Anomaly Detection & Native Trilingual Push (Go Gateway)**:
  - Expanded quota monitoring from `Gemini 5h` only to all 5 pools (**`Gemini 5h` / `Gemini Weekly` / `Claude 5h` / `Claude Weekly` / `Gemini 4 Argon`**), wired directly to `FetchLiveQuota` so users without Cockpit Tools receive full alerts out of the box;
  - Added **early bonus-reset notifications** (aggregating multi-pool official refills into a single celebratory push while ignoring normal scheduled resets) plus **rapid-drop & host-idle silent burn alerts** backed by `Watcher.IdleSince()` to prevent post-task false positives;
  - Upgraded gateway CLI locale detection and all push notification templates (approvals, questions, Proceed, completion, failures, and quota alerts) to native **English / 简体中文 / 日本語** (`--lang zh|en|ja`, `MGY_LANG`, and macOS / Windows / Linux auto-detection).
- 🧹 **Stale Lock-Screen Approval Reconciliation & Silent Banner Dismissal (Go / iOS / Android / APNs Worker)**:
  - When an approval is resolved on desktop or another device, the gateway automatically dispatches a silent push to dismiss stale delivered approval banners from mobile notification centers;
  - Hardened stale-request reconciliation for both iOS Live Activity intents and Android notification action buttons to prevent blind or duplicate approvals.
- 🔋 **Dynamic Task-Time Host Idle-Sleep Guard (macOS / Windows / Linux)**:
  - Added cross-platform on-demand host sleep prevention (`caffeinate -i -w <pid>` on macOS, `SetThreadExecutionState` on a locked OS thread on Windows, and `systemd-inhibit` with pipe-EOF cleanup on Linux). Holds an idle-sleep assertion only while Antigravity sessions are active (still allowing display sleep), releases automatically 2 minutes after all sessions go idle, and leaves zero orphan locks on exit or crash (disable via `-prevent-sleep=false` or `MGY_PREVENT_SLEEP=0`).
- ⚡ **E2EE Relay Self-Healing, Direct Physical RTT Preference & Mobile Polish (Go / iOS / Android)**:
  - Routed iOS conversation events WebSockets through E2EE `GatewayWebSocketTask` and prioritized direct physical RTT in `RelayPool`;
  - Added automatic stale-relay migration on HTTP `502 / 503` and synchronized quota/events refresh on route changes in Android, alongside a custom `MgyPullToRefreshIndicator` fixing pull-to-refresh shadow tail artifacts after retraction.
- 📦 **Version Alignment**:
  - Android `1.2.6` (Version Code `34`); iOS `1.2.6` (Build `34`); desktop/server gateway `mgy` `1.2.6`.

---

## 🇯🇵 日本語リリースノート

- 📋 **デスクトップ同等のステップ計画カード＆リアルタイムタイマー（Go / iOS / Android / Web / Live Activity）**：全クライアントにネイティブの **Step Plan Card** と実行経過タイマーを追加し、進行中ステップを iOS Dynamic Island やロック画面ライブアクティビティにもリアルタイム反映します。
- 🛡️ **全 5 プール対応クォータ守護・公式早期リセット通知・異常急減アラート＆完全 3 言語プッシュ通知**：監視対象を **Gemini 5h / 週間・Claude 5h / 週間・Gemini 4 Argon** の全 5 プールへ拡張し、Cockpit Tools 未導入環境でも `FetchLiveQuota` でそのまま動作。予定より早い公式リセットのまとめ喜報通知、短時間急減・アイドル中消費アラート、および **日本語 / English / 简体中文** のネイティブプッシュ通知（`--lang ja` / `MGY_LANG=ja` / OS 自動判別）に対応しました。
- 🧹 **デスクトップ承認済みバナーの自動サイレント消去＆ロック画面承認の整合性強化**：デスクトップ側で承認が完了すると、スマートフォン側に届いた古い承認通知バナーをサイレントプッシュで自動消去し、期限切れボタンの誤操作を防ぎます。
- 🔋 **アクティブ実行時限定のホスト自動スリープ防止（macOS / Windows / Linux）**：エージェント稼働中のみアイドルスリープを防止（画面オフは許可）し、全タスク完了から 2 分後に自動解除。プロセス終了時にもロックを残さない設計です（`MGY_PREVENT_SLEEP=0` で無効化可能）。
- ⚡ **E2EE リレー自動復旧・物理 RTT 優先選択＆モバイル品質向上**：iOS のイベント WebSocket を E2EE `GatewayWebSocketTask` 経由に統一し、`RelayPool` で物理 RTT を優先選択。Android ではリレー `502 / 503` 時の自動フェイルオーバーとルート切替時のクォータ／イベント同期、プルダウン更新後の残像修正を行いました。
- 📦 **バージョン統一**：iOS `1.2.6` (Build `34`) / Android `1.2.6` (Version Code `34`) / ゲートウェイ `mgy` `1.2.6`。
