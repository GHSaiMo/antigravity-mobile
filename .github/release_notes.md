# 🚀 Multigravity v1.2.3

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

- 📱 **iOS 锁屏 / 桌面 / CarPlay / Mac 全端额度小组件（`QuotaStatusWidget`）**：
  - 新增黄金三件套小组件：**锁屏圆环**、**锁屏矩形**与 **`systemSmall` 小方格**（一稿复用覆盖 iPhone 桌面、StandBy 待机、iOS 26 CarPlay 小组件页与 Mac 桌面）；
  - 同模型自动双窗展示 **5 小时 + 7 天剩余额度**与原生实时走秒重置倒计时（自动跟随 App 所选 Gemini / Claude 偏好，免费号优雅回退 7 天窗口）；
  - 通过 App Group（`group.com.ghsaimo.antigravity`）实现后台定时拉新、桌面右上角一键轻刷（免打开 App）、离线无报错快照兜底，点击卡片 via `antigravity://quota` 一步直达额度面板。
- 🚗 **CarPlay 仪表盘 & Apple Watch 实时活动详细步骤升级**：
  - 为 `AgentActivityWidget` 接入 `.supplementalActivityFamilies([.small])`，告别车机仅显示「第 N 步」的空白兜底；
  - 采用驾驶高对比两段式卡片：顶行展示会话标题 + 步数/状态胶囊（`待审批` 自动橙色高亮），主区最多 2 行大字实时显示当前执行步骤或后台任务名，同步适配中 / 英 / 日三语。
- 🇯🇵 **iOS 原生日语（`ja`）全量本地化**：
  - iOS 主 App、灵动岛/小组件 `ActivityExtension` 与系统权限弹窗 `InfoPlist` 实现 100% 原生日语覆盖，系统语言为日语的 iPhone / iPad 打开即自动切换地道日语界面与日语灵动岛/小组件；
  - 全部格式占位符与 `xcodebuild` 无头编译全量校验通过（PWA 与 Android 暂维持现有中/英体系）。
- 📚 **GitHub 与官方文档三语体系上线**：
  - 新增原生日语文档 [`README_JA.md`](https://github.com/GHSaiMo/antigravity-mobile/blob/main/README_JA.md)，`README.md` / `README_EN.md` / `README_JA.md` 顶部统一接入 **简体中文 | English | 日本語** 三语互链；
  - 官方网站同步上线原生日语门户 [mgy.jiuge.space/ja.html](https://mgy.jiuge.space/ja.html)（含 `/ja` 路由、全交互演示日语化、三语切换器与 4 向 `hreflang`）。
- 📦 **全端版本号对齐**：
  - Android `1.2.3`（Version Code `24`）；iOS `1.2.3`（Build `24`）；桌面/服务端网关 `mgy` `1.2.3`。

---

## 🇺🇸 English Release Notes

### ✨ Highlights & New Features

- 📱 **Native iOS Quota Widgets across Lock Screen, Home Screen, StandBy, CarPlay & Mac (`QuotaStatusWidget`)**:
  - Added three native WidgetKit families: **Lock Screen Circular**, **Lock Screen Rectangular**, and **`systemSmall`** (automatically powering iPhone Home Screen, StandBy, iOS 26 CarPlay Widgets, and macOS Desktop);
  - Displays dual **5h + 7d remaining quota bars** with a native live-ticking reset countdown (automatically following your in-app Gemini / Claude preference, with graceful 7d fallback for Free-tier accounts);
  - Backed by App Group (`group.com.ghsaimo.antigravity`) for periodic background gateway refreshes, an interactive Home Screen tap-to-refresh button without opening the app, graceful offline snapshot fallback, and `antigravity://quota` deep-linking straight to the Account Quota sheet.
- 🚗 **Detailed CarPlay Dashboard & Apple Watch Smart Stack Live Activities**:
  - Opted `AgentActivityWidget` into `.supplementalActivityFamilies([.small])`, replacing the sparse compact fallback with a dedicated high-contrast two-row glanceable layout;
  - Shows conversation title + step/status pill (`Approval` highlighted in orange) on top and up to two lines of the live action or active background task below, fully localized in English, Japanese, and Simplified Chinese.
- 🇯🇵 **Full Native Japanese (`ja`) Localization for iOS**:
  - 100% native Japanese localization across the main iOS app, `ActivityExtension` (Dynamic Island, CarPlay, Widgets), and `InfoPlist` permission prompts; Japanese-locale iPhones and iPads automatically display native Japanese UI;
  - Verified format-specifier parity and clean headless `xcodebuild` compilation (Web/PWA and Android localization deferred to a future release).
- 📚 **Trilingual GitHub Docs & Official Website**:
  - Added native Japanese GitHub documentation ([`README_JA.md`](https://github.com/GHSaiMo/antigravity-mobile/blob/main/README_JA.md)) with a unified **简体中文 | English | 日本語** switcher across all READMEs;
  - Launched the native Japanese landing page at [mgy.jiuge.space/ja.html](https://mgy.jiuge.space/ja.html) (with `/ja` route, localized interactive demos, 3-way language switcher, and `hreflang` metadata).
- 📦 **Version Alignment**:
  - Android `1.2.3` (Version Code `24`); iOS `1.2.3` (Build `24`); desktop/server gateway `mgy` `1.2.3`.

---

## 🇯🇵 日本語リリースノート

- 📱 **ロック画面・ホーム画面・CarPlay・Mac 対応クォータウィジェット（`QuotaStatusWidget`）**：ロック画面（円形 / 長方形）と `systemSmall`（iPhone ホーム / StandBy / iOS 26 CarPlay ウィジェット / Mac デスクトップ共用）で **5時間＆7日間クォータ残量とリアルタイムリセットカウントダウン** を常時確認可能。ホーム画面からのワンタップ更新・オフラインキャッシュ表示・タップでクォータ画面へ直行するディープリンク（`antigravity://quota`）に対応。
- 🚗 **CarPlay ダッシュボード＆Apple Watch ライブアクティビティ詳細ステップ表示**：`.supplementalActivityFamilies([.small])` に対応し、運転中もひと目で分かる 2 段構成（セッションタイトル＋ステップ/承認バッジ＋現在の実行アクション）へ刷新しました。
- 🇯🇵 **iOS アプリ完全日本語対応（`ja`）**：メインアプリ・Dynamic Island / ウィジェット拡張・権限ダイアログを 100% ネイティブ日本語化。
- 📚 **GitHub・公式サイトの日本語版を公開**：[`README_JA.md`](https://github.com/GHSaiMo/antigravity-mobile/blob/main/README_JA.md) および日本語公式サイト [mgy.jiuge.space/ja.html](https://mgy.jiuge.space/ja.html) を公開しました。
- 📦 **バージョン統一**：iOS `1.2.3` (Build `24`) / Android `1.2.3` (Version Code `24`) / ゲートウェイ `mgy` `1.2.3`。
