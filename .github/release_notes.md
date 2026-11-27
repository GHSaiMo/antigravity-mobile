# 🚀 Multigravity v1.2.8

> 🎉 **Multigravity（反重力智能体移动伴侣）iOS 版现已正式上架 Apple App Store！**  
> 无需本地编译或自签，前往 👉 **[App Store 下载 Multigravity](https://apps.apple.com/app/id6835508934)**（覆盖海外 174 个国家/地区，内置零配置**离线演示模式**）；也可加入 👉 **[TestFlight 公开测试](https://testflight.apple.com/join/YM7f4aSK)** 抢先体验最新构建。
>
> 🎉 **Multigravity for iOS is officially available on the Apple App Store!**  
> Install directly from the 👉 **[App Store](https://apps.apple.com/app/id6835508934)** (174 countries & regions, with built-in zero-config **offline Demo Mode**), or join the 👉 **[TestFlight Public Beta](https://testflight.apple.com/join/YM7f4aSK)** for pre-release builds.
>
> 🎉 **Multigravity iOS 版は Apple App Store で正式配信中！**  
> 👉 **[App Store からダウンロード](https://apps.apple.com/app/id6835508934)**（174 カ国・地域対応、オフラインデモモード内蔵）または 👉 **[日本語公式サイト](https://multigravity.app/ja.html)** をご覧ください。

---

## 🇨🇳 中文更新说明

> 💡 **Android 升级说明**：`v1.2.8`（`versionCode 45`）沿用自 `v1.2.7` 起启用的独立私有正式签名（SHA-256: `79:D8:15:B5:05:C1:81:44:AE:72:E7:30:38:02:43:A6:5B:FD:C6:3D:5E:13:4E:77:70:EB:3D:90:2D:35:5C:A9`）。当前为 `v1.2.7` 或 `v1.2.8-beta.*` 的用户可直接在 App 内「设置 → 关于 → 版本」一键覆盖升级；仅 `v1.2.6` 及更早老版本需先卸载旧版再安装一次。

### ✨ 核心亮点与新特性

- 🤖 **智能辅助套件（AI Assist）与四语种原生体验打磨（Go / iOS / Android / Web）**：
  - 全端新增「设置 → 智能辅助」，底层通过本机 Antigravity 原生接口无状态执行，不产生额外会话、开关跨设备同步：
    - **智能翻译（Smart Translation）**：支持新会话标题自动翻译覆盖，以及步骤清单（Step Plan）、实时状态行（Live Focus）与 iOS 灵动岛/实时活动实时翻译；**英文端严格保持默认关闭并按设备显式抑制**，支持中文/日语及法/韩/德/西等非中日英系统母语动态目标语言（UI 干净降级英文、翻译精准指向设备母语），内置混排与拉丁语系防回声校验、去截断与自动回退重试；
    - **下一步提示词建议（Next-Prompt Suggestions）**：每轮回复完成后在输入栏 `+` 右侧内联展示建议胶囊（首行完整延伸再折行），支持**短按填入、长按 1.5 秒进度条防呆直发（提前松手取消）**、审批/草稿自动隐藏与多轮原语种匹配，并在「使用技巧与快捷手势」中补充说明；
    - **长会话防失忆（Post-Compact Rehydration）**：支持在本机 `~/.gemini/config/hooks.json` 注册轻量 `PreInvocation` Hook，在 `/compact` 或自动压缩后的下一轮调用前自动单次回灌近期意图与改动文件（自动脱敏凭据）。
  - 完成 `v1.2.8` 新增界面在 **简中 / 日语 / 英文 / 非中日英降级英文** 四种场景下的全端排查与实机修复：补齐 iOS/Android/Web 全部缺失翻译词条与 Go 网关三语 API/CLI 文案，消除非中文界面中的中文泄漏、Swarm 协作板正文误译、浅色模式对比度与导航栏折行问题。
- 🔄 **远程网关一键自更新 & 宿主机更新二级页（Go / iOS / Android / Web）**：
  - 将「设置 → 关于 → 宿主机更新」重构为横向二级子页（iOS 原生 `NavigationStack` 推入与右滑返回，Android/Web 平滑横滑与跟手右滑返回）；
  - 同页查看电脑端 Antigravity 桌面端与 `mgy` 网关版本状态，支持直接从手机或 Web 远程触发 `mgy` 下载、SHA-256 校验、热替换二进制与自动重启重连。
- 🌐 **一机多主机（解耦多网关）、Antigravity 2.35.2 全套原生能力与局域网 `mgy hub`（Go / iOS / Android / Web）**：
  - **解耦多网关**：完全隔离的多主机凭证/E2EE/缓存、聊天顶栏标题左右轻扫极速切机、居中主机滑选器（支持卡片外纵向拖拽）与后台主机待审批微提醒；新增无相机链接粘贴配对按钮与 `mgy pair -copy/-uri`；
  - **Antigravity 2.35.2 原生能力**：新增浏览器实时预览（Live View）与触控接管、远程 SSH 主机管理、插件市场检索安装与 Plugin/MCP OAuth 闭环、四档权限预设、工作区代码/文件搜索、产物历史快照、Worktree PR 与会话标星；
  - **局域网多机聚合 (`mgy hub`)**：新增 `mgy hub` 多节点聚合与独立 `mgy` 6 位配对码被动局域网发现，同步转发 `Accept-Language` 与跨节点 AI Assist 偏好。
- ⚡ **空闲安全阈值自动切号、上下文占比拆解与动态定价（Go / iOS / Android / Web）**：
  - 配额页新增「阈值自动切号」，支持为 Gemini / Claude 的 5h 或 Weekly 额度池设置阈值，默认勾选「仅在空闲时切换」不打断活跃生成；
  - 上下文审计新增**上下文占比拆解（Context Breakdown）**与 Top 3 最高占用步骤点击直跳定位（iOS 半屏自适应防裁切），新增动态模型定价与配额预估；修复 Claude 驱动 Orchestrator 正确归属 Claude 额度池，保留经典双列模型选择器与配额窗口并升级 `XHigh` 命名与边框。
- 💬 **定时任务升级、多条有序斜杠、HTML 预览卡片与交互修复（Go / iOS / Android / Web）**：
  - 对齐桌面端定时任务「Prompt 创建」流程，对话中的定时任务触发提示词气泡默认折叠并支持点按展开，保留独立定时任务工作区名与闹钟标识；
  - 支持在单条消息中组合多个有序 `/command` 斜杠指令；自动识别面向用户的 `.html` 产物并展示可交互预览卡片（保留真实扩展名、精简多余页内搜索按钮）；
  - 重构 `/btw` 旁路为贴合卡片底部的内收抽屉与完整树形连线，统一 `/btw` 图标为问号气泡；Swarm 协作板支持完整 Markdown 嵌套列表与悬浮回底按钮；修复发送消息后因乱序旧帧导致蓝色用户气泡短暂消失及轮次/回底按钮闪跳、Android 填写项键盘弹出与首次长按气泡遮挡等问题。
- 🛡️ **跨平台后台自启服务、推送脱敏与 E2EE v2 / 中继加固（Go / Worker / iOS / Android / Web）**：
  - 新增跨平台 `mgy service`（macOS `launchd` / Linux `systemd` / Windows 任务计划，带按大小轮转日志与 Windows 无黑框隐藏）；新增 Antigravity CLI (`agy`) 探测、Linux `agy` 守护自愈与 Antigravity 2.0 `projectId` 去重；
  - 新增推送脱敏（`MGY_PUSH_REDACT`）、自建中继（BYO-Relay）、强制 E2EE v2（防重放与 WS 序号保护）、丢包感知多样本中继探活与运行时 Ping 逃逸，以及严格的多网关 epoch / 401 解绑隔离。
- 📦 **全端版本号对齐**：
  - Android `1.2.8`（Version Code `45`）；iOS `1.2.8`（Build `53`）；桌面/服务端网关 `mgy` `1.2.8`；Web / PWA Service Worker `v89`。

---

## 🇺🇸 English Release Notes

> 💡 **Android Upgrade Note**: `v1.2.8` (`versionCode 45`) uses the same private release signing key introduced in `v1.2.7` (SHA-256: `79:D8:15:B5:05:C1:81:44:AE:72:E7:30:38:02:43:A6:5B:FD:C6:3D:5E:13:4E:77:70:EB:3D:90:2D:35:5C:A9`). Users on `v1.2.7` or `v1.2.8-beta.*` can upgrade seamlessly in-app via **Settings → About → Version**; only users on `v1.2.6` or earlier need a one-time uninstall before installing.

### ✨ Highlights & New Features

- 🤖 **AI Assist Suite & 4-Locale Localization Polish (Go / iOS / Android / Web)**:
  - Added **Settings → AI Assist**, running statelessly via local Antigravity native RPC with zero extra sessions and cross-device synced switches:
    - **Smart Translation**: Auto-translates new conversation titles, plus live translation for Step Checklists, Live Focus status lines, and iOS Dynamic Island / Live Activities. **Strictly defaults OFF and hidden on English clients** (with per-client/per-device suppression on the gateway), while seamlessly supporting Chinese, Japanese, and non-ZH/JA/EN system locales (e.g., French, Korean, German, Spanish: clean English UI fallback paired with native-language translation targeting, Latin/mixed-script echo validation, untruncated titles, and fallback retry).
    - **Next-Prompt Suggestions**: Surfaces a concise follow-up chip inline beside `+` after each reply (expanding full-width on the first line before wrapping) with **tap-to-fill and 1.5s hold-to-send progress bar** (release early to cancel; documented in Tips & Gestures), automatic suppression on approvals/Proceed/Continue/drafts, and multi-turn native-language matching.
    - **Post-Compact Rehydration**: Registers a lightweight local `PreInvocation` hook in `~/.gemini/config/hooks.json` to inject recent user intent and modified files once after `/compact` or auto-compaction (with credential redaction).
  - Audited and verified all `v1.2.8` views across **Chinese, Japanese, English, and non-ZH/JA/EN (English UI fallback)** on real iOS/Android simulators and Chromium: closed all missing EN/JA strings and hardcoded Chinese gateway/API/CLI leaks, shielded Swarm messages from DOM translation, and fixed light-mode contrast and navbar/tab wrapping issues.
- 🔄 **Remote Gateway Self-Update & Unified Host Updates Subpage (Go / iOS / Android / Web)**:
  - Moved **Settings → About → Host Updates** into a native horizontal sliding subpage across iOS (`NavigationStack` push & swipe-back), Android, and Web;
  - Inspect both Antigravity Desktop and `mgy` Gateway versions in one place and trigger remote `mgy` downloads, SHA-256 verification, in-place binary replacement, and automatic reconnect straight from mobile or Web.
- 🌐 **Decoupled Multi-Gateway, Antigravity 2.35.2 Native Capabilities & LAN `mgy hub` (Go / iOS / Android / Web)**:
  - **Multi-Gateway Switching**: Fully isolated multi-host credentials/E2EE/caches, horizontal title swipe cycling, centered host slide picker (with out-of-card vertical drag), subtle inactive-host approval indicators, and camera-free pairing link paste + `mgy pair -copy/-uri`.
  - **Antigravity 2.35.2 Capabilities**: Browser Live View & touch takeover, remote SSH host management, Marketplace discovery & Plugin/MCP OAuth closure, 4-tier permission presets, workspace file/code search, artifact snapshots, Worktree PRs, and conversation starring.
  - **LAN Multi-Node Hub (`mgy hub`)**: Added multi-node aggregation and passive LAN discovery with 6-char pair codes for standalone `mgy`, plus `Accept-Language` and cross-node AI Assist preference propagation.
- ⚡ **Idle-Safe Threshold Auto-Switch, Context Breakdown & Dynamic Pricing (Go / iOS / Android / Web)**:
  - Added **Threshold Auto-Switch** in Quotas for Gemini/Claude 5h or Weekly pools (waiting for the active turn to finish idle by default);
  - Added **Context Breakdown** inspection with clickable Top-3 consumer jump-to-step (auto-fitting half-sheet on iOS), dynamic model pricing & quota estimates, Claude-backed Orchestrator quota pool alignment, and restored classic 2-column model picker with `XHigh` styling.
- 💬 **Desktop-Aligned Automations, Multi-Slash Commands, HTML Previews & Ergonomics (Go / iOS / Android / Web)**:
  - Aligned Automations with desktop **Create-with-Prompt**, default-collapsed scheduled-task prompt bubbles (tap to expand), and independent scheduled workspace names with trailing alarm icons;
  - Added **multiple ordered `/slash` commands**, interactive **HTML artifact preview cards** with real file extensions, cleaner viewers, recessed **`/btw` branch drawer** with tree spine and unified question-bubble icons, full Swarm Markdown + floating scroll-to-bottom button, and fixes for newly sent user bubbles briefly vanishing, Android write-in keyboard focus, and first-run tooltip overlap.
- 🛡️ **Cross-Platform `mgy service`, Push Redaction & E2EE v2 / Relay Hardening (Go / Worker / iOS / Android / Web)**:
  - Added cross-platform `mgy service` auto-start (macOS/Linux/Windows) with rotating logs and Windows `HideWindow` suppression, `agy` CLI discovery & Linux daemon recovery, and Antigravity 2.0 `projectId` deduplication;
  - Added push redaction (`MGY_PUSH_REDACT`), BYO-Relay, Force-E2EE v2 replay/WS-seq protection, loss-aware multi-sample relay probing with runtime ping escape, and strict multi-gateway epoch/401 isolation.
- 📦 **Version Alignment**:
  - Android `1.2.8` (Version Code `45`); iOS `1.2.8` (Build `53`); desktop/server gateway `mgy` `1.2.8`; Web / PWA Service Worker `v89`.

---

## 🇯🇵 日本語リリースノート

> 💡 **Android アップグレードのご案内**：`v1.2.8`（`versionCode 45`）は `v1.2.7` と同一の独立プライベート署名（SHA-256: `79:D8:15:B5:...35:5C:A9`）を使用しています。`v1.2.7` または `v1.2.8-beta.*` からはアプリ内「設定 → バージョン情報」よりワンタップで上書き更新できます（`v1.2.6` 以前の旧版のみ初回アンインストールが必要です）。

- 🤖 **AI アシストスイート＆4 ロケール完全磨き込み（Go / iOS / Android / Web）**：ローカル Antigravity ネイティブ RPC をステートレス実行する**スマート翻訳**（会話タイトル・ステップ計画・リアルタイム状態・iOS Dynamic Island／Live Activity 翻訳。英語環境では既定オフ＆端末別抑止を維持し、日本語および非中日英のシステム言語でも英語 UI フォールバック＋母語翻訳に柔軟対応）、**次プロンプト提案チップ**（タップで入力・1.5 秒長押しプログレス直送信・承認/下書き時の自動非表示）、**ロングセッション記憶保護（`/compact` 後の自動再注入 Hook）**を全端に搭載。さらに中国語・日本語・英語・非中日英（英語 UI フォールバック）の 4 環境で実機シミュレータ＆Chromium 検証を行い、新規画面の未翻訳・文字切れ・ライトモード視認性・Swarm 本文誤訳を一掃しました。
- 🔄 **ゲートウェイ遠隔セルフアップデート＆「ホスト更新」サブページ**：**設定 → バージョン情報 → ホスト更新**を横スライドの専用サブページへ刷新し、Antigravity デスクトップ版と `mgy` ゲートウェイのバージョン確認から SHA-256 検証付き遠隔更新・自動再起動までスマホ／Web で完結できます。
- 🌐 **マルチゲートウェイ切替・Antigravity 2.35.2 ネイティブ機能・LAN `mgy hub`**：複数ホストの認証・E2EE・キャッシュを完全分離し、タイトル横スワイプ切替・ホストスライドピッカー・リンク貼り付けペアリングを追加。ブラウザライブプレビュー＆操作引き継ぎ、リモート SSH 管理、プラグインマーケット＆OAuth、4 段階権限、ワークスペース検索、成果物スナップショット、Worktree PR、お気に入り星印、6 桁コードによる `mgy hub` LAN 自動検出に対応しました。
- ⚡ **しきい値自動アカウント切替・コンテキスト内訳＆動的料金推定**：Gemini / Claude（5 時間枠・週間枠）の残量しきい値でアイドル待機後に安全切替する自動切替、Top 3 消費ステップへジャンプできるコンテキスト内訳、動的コスト推定、`XHigh` 表示対応の 2 列モデルピッカーを搭載しました。
- 💬 **自動化プロンプト作成・複数順序付きスラッシュ・HTML プレビュー＆UX 改善**：デスクトップ準拠のプロンプト自動化作成と折りたたみトリガー吹き出し、複数 `/command` の順序実行、インタラクティブ HTML 成果物カード、カード下部の `/btw` 引き出しドロワー、Swarm の完全 Markdown＆最下部ジャンプ、送信直後のユーザー吹き出し消失修正を収録しました。
- 🛡️ **クロスプラットフォーム `mgy service`・通知秘匿・E2EE v2＆リレー強化**：macOS / Linux / Windows 常駐サービス管理とログローテーション、`agy` CLI 検出、通知本文秘匿（`MGY_PUSH_REDACT`）、BYO-Relay、強制 E2EE v2（リプレイ・WS 連番保護）、パケットロス感知リレー切替を実装しました。
- 📦 **バージョン統一**：iOS `1.2.8` (Build `53`) / Android `1.2.8` (Version Code `45`) / ゲートウェイ `mgy` `1.2.8` / Web PWA `v89`。
