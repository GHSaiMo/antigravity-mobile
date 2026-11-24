# 🚀 Multigravity v1.2.7

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

> ⚠️ **Android 升级提示（一次性签名迁移）**：自本版本起，Android 客户端启用全新独立私有安全签名（SHA-256: `79:D8:15:B5:05:C1:81:44:AE:72:E7:30:38:02:43:A6:5B:FD:C6:3D:5E:13:4E:77:70:EB:3D:90:2D:35:5C:A9`），并新增**应用内检查更新 & SHA-256 校验一键安装**（支持国内镜像兜底与 Obtainium 追踪）。  
> **存量 `v1.2.6` 及更早版本的安卓用户**：本次升级请先**卸载旧版再安装新版**（打开后扫码 3 秒即可恢复配对）；完成本次切换后，未来所有版本将永久支持一键应用内覆盖升级（应用内更新弹窗也已提前内置此提示）。

### ✨ 核心亮点与新特性

- 🌍 **全端三语深度打磨、桌面工作台自动切语与按设备语言推送（Go / iOS / Android / Web）**：
  - 桌面工作台（`/`）与扫码配对页新增基于浏览器 `Accept-Language` 的 **简体中文 / English / 日本語** 自动协商与侧栏底部原生语言切换器（`Auto / English / 日本語 / 简体中文`）：英文保留上游原生界面文案，日语新增桌面工作台本地化层与三语 RPC 配额汇总；
  - PWA 移动网页版（`/?view=pwa`）在设置页恢复持久化手动语言切换，扩充 `1,930` 组英/日对称词条与动态 DOM / Toast / 对话框翻译，并严格跳过聊天消息正文与会话标题防止误译；
  - Go 网关推送与 iOS Live Activity（审批 / 提问 / 完成 / 失败 / 额度守护）改为按**每台已配对移动设备上报的客户端语言**独立本地化，彻底清除锁屏实时活动中的硬编码中文；
  - 完成 iOS 与 Android 全量中/英/日三语核对、长文案截断溢出与原生排版修复，语音排障引导气泡支持三语本地化且每设备仅触发一次。
- 📱 **iPhone Duo 折叠形态原生适配与 iOS 流畅度革新（iOS）**：
  - 全面适配 **iPhone Duo**（iOS 27.1+）合盖侧边竖轨与居中聊天标题、展开 342pt 比例双栏工作台以及 **Book 90° 半折物理折痕对齐**（`ReservedRegion(.division)`），通过 `ChatSessionStore` 在折叠 / 展开 / 旋转全过程无缝保留实时会话流与草稿；
  - 引入原子化滚动钉选与 `Equatable` Markdown 块，彻底消除长回复流式输出时的下半屏闪烁；新增桌面图标 **主屏快捷操作（Home Screen Quick Actions）**与统一 Deep Link 路由、原生 App Store 评价提醒、取消归档平滑退出动画与折叠面板严格边界裁剪。
- 🚀 **全面对齐 Antigravity 2.35.0 协议与核心交互升级（Go / iOS / Android / Web）**：
  - 对齐 **Antigravity 2.35.0** RPC 基线：新增**会话归档与取消归档**、运行中轮次模型切换提示、插件权限撤销、队列阻塞原因标签、限定范围写入审批、Swarm 协作板分页、**运行中 Swarm 蜂群自动置顶 + 像素蜜蜂徽标**、子代理分组及真实运行模型解析、HTML 预览页内查找（Find in Page）与 `/btw` 搜索路由；同步上线历史预加载上限控制、内部会话文件只读保护与 Auto / Turbo 预设解耦；
  - 用原生可展开 `Working` 进度行取代旧版思考气泡且默认折叠思考过程；重做 **`/btw` 旁问分支**；升级 **`ask_question` 多选交互**与标题徽标并修复第 5 选项问题；重做 **GitHub 仓库克隆**（本地目录选择器、原地新建目录、异步进度条与自动注册项目）；将 Orchestrator 变体收入 Neon / Carbon / Argon 子菜单并在模型选择器内直览池配额；保留 `Leaving Soon` 模型可选，仅过滤已不可用模型。
- 🛡️ **高危审批 Diff 预检、收件箱置顶、停滞告警与锁屏快批（Go / iOS / Android / Web）**：
  - 新增 **Agent Inbox 审批置顶**、任务长时间停滞告警与独立可定制**通知偏好子页**（支持精细化开关各类告警，缓存未命中审计默认转入后台静默模式）；
  - 高危命令审批新增摘要头、紧凑可折叠代码 Diff 预览与醒目高危推送标题；新增 iOS 锁屏快捷审批按钮与高危锁屏审批防护，并修复排队消息在竞态下静默丢失的问题。
- ⚡ **E2EE `req-bin2` 流式附件上传、双级高清图与配额精细化（Go / iOS / Android / Web）**：
  - 全端升级 **E2EE `req-bin2` 流式附件上传**，提供真实上传进度反馈与幂等重试，修复 Inbox 附件在上游移动后丢失及 iOS 排队发送后草稿附件未清空的问题；
  - 新增**双级图片预览**（解决缩略图锁定无法加载高清原图问题），并在 HTML / Generative UI 预览中完整注入主题 Token；
  - 上下文指示器新增**会话 Token 汇总与压缩上限提示**，Token 历史满 1 小时自动刷新，合并 Gemini 池后自动抑制镜像 Argon 重复配额与重复告警，修复满额无 `resetTime` 误判与双端模型用量分布表对齐。
- 📦 **全端版本号对齐**：
  - Android `1.2.7`（Version Code `38`）；iOS `1.2.7`（Build `38`）；桌面/服务端网关 `mgy` `1.2.7`。

---

## 🇺🇸 English Release Notes

> ⚠️ **Android Upgrade Notice (One-Time Signing Key Migration)**: Starting with this release, Android APKs are signed with a brand-new private release keystore (SHA-256: `79:D8:15:B5:05:C1:81:44:AE:72:E7:30:38:02:43:A6:5B:FD:C6:3D:5E:13:4E:77:70:EB:3D:90:2D:35:5C:A9`) and include a **built-in SHA-256 verified in-app updater** (plus Obtainium tracking).  
> **Existing Android users on `v1.2.6` or earlier**: Please **uninstall the old app once and install this release** (re-pairing takes 3 seconds via QR scan). All future updates will support seamless in-place one-tap upgrades inside the app.

### ✨ Highlights & New Features

- 🌍 **Full Trilingual Polish, Auto-Localized Desktop Workbench & Per-Device Push (Go / iOS / Android / Web)**:
  - The Desktop Workbench (`/`) and pairing page now negotiate **English / 简体中文 / 日本語** automatically from browser `Accept-Language` and expose a subtle sidebar footer language selector (`Auto / English / 日本語 / 简体中文`), preserving authentic upstream English while adding a full Japanese desktop layer and trilingual RPC quota summary;
  - The PWA Web Console (`/?view=pwa`) restores a persistent manual language picker in Settings with `1,930` symmetric EN/JA keys and dynamic DOM/toast/dialog translation while strictly shielding user/AI chat messages and titles from unintended translation;
  - Gateway push notifications and iOS Live Activities (approvals, questions, completion, errors, and quota guardian) are now localized **per paired device's reported app language**, removing all hardcoded Chinese strings from Live Activities;
  - Completed comprehensive zh/en/ja localization, layout overflow, and native UX audits across iOS and Android (plus once-per-install trilingual voice troubleshooting tips).
- 📱 **Native iPhone Duo Foldable Adaptation & Smoother iOS Streaming (iOS)**:
  - Full native **iPhone Duo** (iOS 27.1+) adaptation: cover display trailing vertical rail & centered chat title, unfolded 342pt dual-pane workspace, **90° Book posture physical hinge crease alignment** via `ReservedRegion(.division)`, and seamless live session/draft preservation across fold, unfold, and rotation via `ChatSessionStore`;
  - Eliminated lower-screen streaming flicker via atomic scroll pinning and equatable Markdown blocks; added **Home Screen Quick Actions** & unified deep-link shortcuts, delight-triggered native App Store review prompts, smooth unarchive exit animations, and strict inline accordion clipping.
- 🚀 **Antigravity 2.35.0 Protocol Alignment & Core Interaction Upgrades (Go / iOS / Android / Web)**:
  - Aligned with the **Antigravity 2.35.0** RPC baseline: **conversation archiving**, running-turn model picker hints, plugin permission grant revocation, queue blocker labels, scoped write approvals, Swarm board paging, **running Swarm session bubbling to top with pixel bee badges**, subagent grouping & actual runtime model resolution, HTML preview Find-in-Page, and `/btw` search routing; plus capped history preload, internal session file protection, and decoupled Auto/Turbo presets;
  - Replaced legacy thinking bubbles with a native expandable `Working` row and collapsed thinking disclosure by default; redesigned **`/btw` side-question branches**; added **`ask_question` multi-select**, title badges, and option 5 fix; redesigned **GitHub repo cloning** with a local directory picker, in-place mkdir, async progress, and automatic workspace registration; folded Orchestrator variants into Neon/Carbon/Argon submenus with inline pool quota in the model picker; kept `Leaving Soon` models selectable and filtered only unavailable ones.
- 🛡️ **High-Risk Approval Diff Preview, Inbox Pinning, Stall Alerts & Lock-Screen Guard (Go / iOS / Android / Web)**:
  - Added **Agent Inbox approval pinning**, task stall alerts, and a dedicated customizable **Notification Preferences** subpage (with prompt cache-miss auditing defaulted silently in the background);
  - Added high-risk approval summaries, compact collapsible diff previews, distinct high-risk push titles, iOS lock-screen quick approval actions with high-risk guards, and protection against silent queued message loss.
- ⚡ **Streaming E2EE `req-bin2` Uploads, Dual-Tier Image Previews & Quota Refinements (Go / iOS / Android / Web)**:
  - Upgraded attachment uploads across Go, iOS, and Android to **streaming E2EE `req-bin2`** with real upload progress and idempotent retry, preserving Inbox attachments on upstream moves and clearing iOS draft files on queued sends;
  - Added **dual-tier image previews** (fixing thumbnail lock-in) and full Generative UI theme token injection inside HTML previews and embeds;
  - Added **session token summaries & compaction ceiling hints** to the context meter, 1-hour auto-refresh for token history, mirrored Argon quota/alert deduplication after Gemini pool merge, ready-state handling for full quotas without `resetTime`, and aligned mobile model usage breakdown sheets.
- 📦 **Version Alignment**:
  - Android `1.2.7` (Version Code `38`); iOS `1.2.7` (Build `38`); desktop/server gateway `mgy` `1.2.7`.

---

## 🇯🇵 日本語リリースノート

> ⚠️ **Android アップグレードのご案内（初回のみの署名移行）**：本バージョンより Android APK は新しい独立プライベートリリース署名（SHA-256: `79:D8:15:B5:05:C1:81:44:AE:72:E7:30:38:02:43:A6:5B:FD:C6:3D:5E:13:4E:77:70:EB:3D:90:2D:35:5C:A9`）に切り替わり、**SHA-256 検証付きのアプリ内自動アップデート**（および Obtainium 追従）を搭載しました。  
> **`v1.2.6` 以前の Android 版をご利用の方**：今回のみ一度**旧版をアンインストールしてから `v1.2.7` をインストール**してください（起動後 QR スキャン 3 秒で再ペアリング完了）。次回以降はアプリ内からワンタップで上書き更新できます。

- 🌍 **全クライアント 3 言語完全磨き込み・デスクトップ Web 自動言語切替＆デバイス別言語プッシュ**：デスクトップワークベンチ（`/`）とペアリング画面がブラウザの `Accept-Language` に応じて **日本語 / English / 简体中文** を自動判別し、サイドバー下部に言語スイッチャーを追加（英語は公式原文を維持し、日本語ローカル層と 3 言語クォータ要約を新設）。PWA Web（`/?view=pwa`）でも設定画面から永続的な言語切替が可能になり（1,930 項目の日英対称辞書＋チャット本文保護）、ゲートウェイの APNs / FCM / Live Activity 通知も各端末のアプリ言語ごとに個別ローカライズされます。iOS・Android 全画面の 3 言語表記漏れ・はみ出し・音声トラブルシュート案内も一括修正しました。
- 📱 **iPhone Duo 折りたたみネイティブ対応＆iOS ストリーミング刷新**：**iPhone Duo**（iOS 27.1+）のカバー画面縦レール・中央タイトル、展開時 342pt 2 ペイン作業台、**Book 90° 姿勢での物理折り目揃え**（`ReservedRegion(.division)`）、および開閉・回転時のライブセッション継続に対応。さらにアトミックなスクロール固定と `Equatable` Markdown ブロックによりストリーミング時の画面下部ちらつきを解消し、ホーム画面クイックアクションやディープリンク、滑らかなアーカイブ解除アニメーションも追加しました。
- 🚀 **Antigravity 2.35.0 準拠＆コア操作の全面アップグレード**：**会話アーカイブ**、実行中ターンのモデル案内、プラグイン権限の取り消し、キュー待機理由ラベル、スコープ付き書き込み承認、Swarm ページング＆**稼働中 Swarm セッションの最上位表示＋ドット絵ミツバチバッジ**、サブエージェントの実稼働モデル表示、HTML ページ内検索（Find in Page）に対応。従来の思考バブルをネイティブな展開式 `Working` 行へ刷新し、`/btw` サイド質問ブランチ、`ask_question` 複数選択、ローカルディレクトリ選択＆フォルダ作成付き GitHub リポジトリクローン、モデル選択内のプール残量表示も搭載しました。
- 🛡️ **高リスク承認 Diff プレビュー・受信トレイ固定・停滞アラート＆ロック画面クイック承認**：Agent Inbox の承認ピン留め、タスク停滞アラート、カテゴリ別**通知設定サブページ**を追加。高リスクコマンド承認には要約ヘッダーと折りたたみ式 Diff プレビュー、高リスク専用通知タイトル、iOS ロック画面クイック承認と誤操作ガードを備え、キュー送信メッセージの消失も防止します。
- ⚡ **E2EE `req-bin2` ストリーミング添付アップロード＆クォータ／コンテキスト改善**：Go / iOS / Android 間で実進捗・冪等リトライ付きの E2EE ストリーミングアップロードへ移行し、2 段階高解像度画像プレビュー、Generative UI フルテーマトークン注入、コンテキストメーターのトークン集計＆圧縮上限ガイド、1 時間ごとのトークン履歴自動更新、Gemini プール統合後の重複 Argon 通知抑制を実装しました。
- 📦 **バージョン統一**：iOS `1.2.7` (Build `38`) / Android `1.2.7` (Version Code `38`) / ゲートウェイ `mgy` `1.2.7`。
