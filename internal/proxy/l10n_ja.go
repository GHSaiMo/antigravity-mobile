package proxy

import "bytes"

// Japanese localization layer for the Antigravity Desktop Workbench bundle (main.js).
// Mirrors the O(bundle size) single-pass architecture of l10n.go so Japanese browser users
// receive native Japanese UI chrome while preserving pristine English fallback for any
// uncommon upstream strings (zero Chinese leakage).

var l10nExactPatchesJa = []staticReplacement{
	{old: []byte(`one:"less than a second",other:"less than {{count}} seconds"`), new: []byte(`one:"1 秒未満",other:"{{count}} 秒未満"`)},
	{old: []byte(`one:"1 second",other:"{{count}} seconds"`), new: []byte(`one:"1 秒",other:"{{count}} 秒"`)},
	{old: []byte(`halfAMinute:"half a minute"`), new: []byte(`halfAMinute:"30 秒"`)},
	{old: []byte(`one:"less than a minute",other:"less than {{count}} minutes"`), new: []byte(`one:"1 分未満",other:"{{count}} 分未満"`)},
	{old: []byte(`one:"1 minute",other:"{{count}} minutes"`), new: []byte(`one:"1 分",other:"{{count}} 分"`)},
	{old: []byte(`one:"about 1 hour",other:"about {{count}} hours"`), new: []byte(`one:"約 1 時間",other:"約 {{count}} 時間"`)},
	{old: []byte(`one:"1 hour",other:"{{count}} hours"`), new: []byte(`one:"1 時間",other:"{{count}} 時間"`)},
	{old: []byte(`one:"1 day",other:"{{count}} days"`), new: []byte(`one:"1 日",other:"{{count}} 日"`)},
	{old: []byte(`one:"about 1 week",other:"about {{count}} weeks"`), new: []byte(`one:"約 1 週間",other:"約 {{count}} 週間"`)},
	{old: []byte(`one:"1 week",other:"{{count}} weeks"`), new: []byte(`one:"1 週間",other:"{{count}} 週間"`)},
	{old: []byte(`one:"about 1 month",other:"about {{count}} months"`), new: []byte(`one:"約 1 か月",other:"約 {{count}} か月"`)},
	{old: []byte(`one:"1 month",other:"{{count}} months"`), new: []byte(`one:"1 か月",other:"{{count}} か月"`)},
	{old: []byte(`one:"about 1 year",other:"about {{count}} years"`), new: []byte(`one:"約 1 年",other:"約 {{count}} 年"`)},
	{old: []byte(`one:"1 year",other:"{{count}} years"`), new: []byte(`one:"1 年",other:"{{count}} 年"`)},
	{old: []byte(`one:"over 1 year",other:"over {{count}} years"`), new: []byte(`one:"1 年以上",other:"{{count}} 年以上"`)},
	{old: []byte(`one:"almost 1 year",other:"almost {{count}} years"`), new: []byte(`one:"約 1 年",other:"約 {{count}} 年"`)},
	{old: []byte(`files:["file","files"],folders:["folder","folders"],edits:["file","files"],searches:["search","searches"],terminal:["command","commands"],tasks:["task","tasks"],web:["page","pages"],browser:["browser","browsers"],images:["image","images"],actions:["action","actions"],artifacts:["artifact","artifacts"]`),
		new: []byte(`files:["件のファイル","件のファイル"],folders:["件のフォルダ","件のフォルダ"],edits:["件のファイル","件のファイル"],searches:["回の検索","回の検索"],terminal:["件のコマンド","件のコマンド"],tasks:["件のタスク","件のタスク"],web:["ページ","ページ"],browser:["件のブラウザ","件のブラウザ"],images:["枚の画像","枚の画像"],actions:["件の操作","件の操作"],artifacts:["件のアーティファクト","件のアーティファクト"]`)},
	{old: []byte(`===1?"command":"commands"`), new: []byte(`===1?"件のコマンド":"件のコマンド"`)},
	{old: []byte(`{low:"Low",medium:"Medium",high:"High",max:"Max"}`), new: []byte(`{low:"低",medium:"中",high:"高",max:"最大"}`)},
	{old: []byte(`return"Narrow";case 3:return"Wide";default:return"Default"`), new: []byte(`return"狭い";case 3:return"広い";default:return"標準"`)},
	{old: []byte("`Resets in ${"), new: []byte("`リセットまで ${")},
	{old: []byte(`a.length," projects"`), new: []byte(`a.length," 件のプロジェクト"`)},
	{old: []byte("`System default (${"), new: []byte("`システム既定 (${")},
}

const l10nSettingsNavJa = `{Account:"アカウント",General:"一般",Appearance:"外観",Skin:"スキン",Editor:"エディター",Tab:"Tab 補完",Notifications:"通知",Customizations:"拡張・スキル",App:"アプリ設定",Shortcuts:"ショートカット",Models:"モデル",Developer:"開発者",Browser:"ブラウザ","Provide Feedback":"フィードバック","フィードバック":"フィードバック"}`

var l10nPatchesJa = []l10nPatch{
	rx(`" ago"`, `"in "\+(\w+):(\w+)\+" ago"`, `${1}+"後":${2}+"前"`),
	rx(`"Running":"running"`, `(\w+)=(\w+)\?(\w+)\?"Running":"running":(\w+)\?"Ran":"ran"`, `${1}=${2}?${3}?"実行中":"実行中":${4}?"実行済み":"実行済み"`),
	rx(`.label]`, `(\.map\(\w+=>\[\w+\.screen,\w+\.label\]\)(?:\);|,\["Provide Feedback"[^\]]*\]\]\);)function \w+\((\w+)\)\{return )(\w+)\.get\(\w+\)\?\?\w+\}`, `${1}`+l10nSettingsNavJa+`[${2}]??${3}.get(${2})??${2}}`),
	rx(`"file":"files"} changed`, `(\$\{\w+\}) \$\{\w+===1\?"file":"files"\} changed`, `${1} 件のファイルを変更`),
	rx(`"Thinking for "`, `"Thinking for ",(Math\.max\(1,\w+\)),"s"`, `"思考 ",${1},"秒"`),

	tpl("Thought for {}s", "{1} 秒間思考しました"),
	tpl("Worked for {}", "{1} 作業しました"),
	tpl("See {} more", "さらに {1} 件を表示"),
	tpl("See all ({})", "すべて表示 ({1})"),
	tpl("Show finished ({})", "完了済みを表示 ({1})"),
	tpl("Send all ({})", "すべて送信 ({1})"),
	tpl("Send now: {}", "今すぐ送信: {1}"),
	tpl(`Searched for "{}"`, `「{1}」を検索しました`),
	tpl("Searched for files: {}", "ファイルを検索: {1}"),
	tpl(`Code search: "{}"`, `コード検索: 「{1}」`),
	tpl(`Internal search: "{}"`, `内部検索: 「{1}」`),
	tpl("Listed directory {}", "ディレクトリを一覧表示: {1}"),
	tpl(`Browser task: "{}"`, `ブラウザタスク: 「{1}」`),
	tpl("Opened browser: {}", "ブラウザを開きました: {1}"),
	tpl(`Searched web: "{}"`, `Web 検索: 「{1}」`),
	tpl("Invoked subagent: {}", "サブエージェントを起動: {1}"),
	tpl(`Generated image: "{}"`, `画像を生成: 「{1}」`),
	tpl("Used tool: {}", "ツールを使用: {1}"),
	tpl("Exit code {}", "終了コード {1}"),
	tpl("Working directory: {}", "作業ディレクトリ: {1}"),
	tpl("Waiting for command completion (up to {} seconds)", "コマンド完了を待機中（最大 {1} 秒）"),
	tpl("Question {} of {}", "質問 {1} / {2}"),
	tpl("Forked from {}", "{1} からフォーク"),
	tpl("View Diff for {}", "{1} の差分を表示"),
	tpl("Open conversation {}", "会話 {1} を開く"),
	tpl("Mark all {} conversations as read", "全 {1} 件の会話を既読にする"),
	tpl("Load {} earlier steps", "前の {1} ステップを読み込む"),
	tpl("Load {} later steps", "次の {1} ステップを読み込む"),
	tpl("Model unavailable, retrying in {}s{}", "モデルを利用できません。{1} 秒後に再試行します{2}"),
	tpl("Model unavailable, retrying{}", "モデルを利用できません。再試行中{1}"),
	tpl("Deleted workspace {}", "ワークスペース {1} を削除しました"),
	tpl("Delete workspace {}", "ワークスペース {1} を削除"),
	tpl("Pushed to {}.", "{1} にプッシュしました。"),
	tpl("Committed to {}.", "{1} にコミットしました。"),
	tpl("Push {} commit{} to {}", "{1} 件のコミットを {3} にプッシュ"),
	tpl("Pushed {} commit{} to {}.", "{1} 件のコミットを {3} にプッシュしました。"),
	tpl("Artifacts ({} Files for Conversation)", "アーティファクト（全 {1} ファイル）"),
	tpl("Process exited with code {}.", "プロセスが終了コード {1} で終了しました。"),
	tpl("Exited with code {}.", "終了コード {1} で終了しました。"),
	tpl("Executing task: {}", "タスクを実行中: {1}"),
	tpl(`Creating workspace "{}"...`, `ワークスペース「{1}」を作成中...`),
	tpl("Cost summary: {}", "コスト概要: {1}"),
}

var l10nAnywhereJa = map[string]string{
	"(no file changes detected)":              "(ファイル変更なし)",
	", / for actions":                         "、/ でコマンド",
	"Accept Step":                             "このステップを承認",
	"Action required":                         "対応が必要",
	"Add Context":                             "コンテキストを追加",
	"Add Folder":                              "フォルダを追加",
	"Add MCP":                                 "MCP を追加",
	"Add MCP Servers":                         "MCP サーバーを追加",
	"Add Model":                               "モデルを追加",
	"Add Terminal":                            "新規ターミナル",
	"Add Workspace":                           "ワークスペースを追加",
	"Add context":                             "コンテキストを追加",
	"Add to Chat":                             "チャットに追加",
	"Add to Chat/Quote":                       "選択範囲をチャットに引用",
	"Added (Staged)":                          "追加済み（ステージ済み）",
	"Advanced Settings":                       "詳細設定",
	"Agent Behavior":                          "エージェント動作",
	"Agent Settings":                          "エージェント設定",
	"All Workspaces":                          "すべてのワークスペース",
	"Allow List Terminal Commands":            "許可コマンド一覧",
	"Allow Once":                              "今回のみ許可",
	"Allow once":                              "今回のみ許可",
	"Always Allow":                            "常に許可",
	"Always Ask":                              "毎回確認",
	"Always Proceed":                          "常に続行",
	"Always run":                              "常に実行",
	"Application Settings":                    "アプリ設定",
	"Approve Command":                         "コマンド実行を承認",
	"Approve File Edit":                       "ファイル編集を承認",
	"Archive / Restore":                       "アーカイブ / 復元",
	"Archive This Conversation":               "この会話をアーカイブ",
	"Archive Workspace":                       "ワークスペースをアーカイブ",
	"Archive conversation":                    "会話をアーカイブ",
	"Archive project":                         "プロジェクトをアーカイブ",
	"Archive this conversation":               "この会話をアーカイブ",
	"Archive workspace":                       "ワークスペースをアーカイブ",
	"Archived Only":                           "アーカイブ済みのみ",
	"Artifact Viewer":                         "アーティファクトビューア",
	"Automation":                              "自動化",
	"Automations":                             "自動化",
	"Ask anything, @ to mention":              "何でも質問、@ でメンション",
	"Ask anything, @ to mention, / for actions": "何でも質問、@ でメンション、/ でコマンド",
	"Ask every time":                          "毎回確認する",
	"Ask first":                               "実行前に確認",
	"Auto (detected)":                         "自動検出 (Auto)",
	"Auto-Expand Changes Overview":            "変更概要を自動展開",
	"Auto-Open Edited Files":                  "編集したファイルを自動で開く",
	"Automatic Check for Updates":             "アップデートを自動確認",
	"Autonomous Mode":                         "自律モード",
	"Autonomous mode":                         "自律モード",
	"Available AI Credits":                    "利用可能な AI クレジット",
	"Background Task Output":                  "バックグラウンドタスク出力",
	"Background Tasks":                        "バックグラウンドタスク",
	"Branch Changes":                          "ブランチの変更",
	"Browser Settings":                        "ブラウザ設定",
	"Browser Task":                            "ブラウザタスク",
	"Cancel All Tasks":                        "すべてのタスクを中止",
	"Cancel Task":                             "タスクを中止",
	"Cancel recording":                        "録音をキャンセル",
	"Cancel step":                             "ステップを中止",
	"Chat Settings":                           "チャット設定",
	"Check for Updates":                       "アップデートを確認",
	"Claude and GPT Models":                   "Claude・GPT モデル",
	"Claude and GPT models":                   "Claude・GPT モデル",
	"Clear Search":                            "検索をクリア",
	"Clear filter":                            "フィルターをクリア",
	"Clear search":                            "検索をクリア",
	"Close Settings":                          "設定を閉じる",
	"Close Tab":                               "タブを閉じる",
	"Close Terminal Tab":                      "ターミナルタブを閉じる",
	"Close panel":                             "パネルを閉じる",
	"Close sidebar":                           "サイドバーを閉じる",
	"Code Search":                             "コード検索",
	"Collapse All":                            "すべて折りたたむ",
	"Collapse All Diffs":                      "すべての差分を折りたたむ",
	"Collapse All Folders":                    "すべてのフォルダを折りたたむ",
	"Collapse sidebar":                        "サイドバーを折りたたむ",
	"Command Palette":                         "コマンドパレット",
	"Commit and Push":                         "コミットしてプッシュ",
	"Commit succeeded":                        "コミットしました",
	"Continue Response":                       "応答を続ける",
	"Conversation Archived":                   "会話をアーカイブしました",
	"Conversation History":                    "会話履歴",
	"Conversation ID":                         "会話 ID",
	"Conversation Name":                       "会話名",
	"Conversation Width":                      "会話パネルの幅",
	"Copy Command":                            "コマンドをコピー",
	"Copy Content":                            "内容をコピー",
	"Copy Conversation ID":                    "会話 ID をコピー",
	"Copy File Name":                          "ファイル名をコピー",
	"Copy File Path":                          "ファイルパスをコピー",
	"Copy Image":                              "画像をコピー",
	"Copy Link":                               "リンクをコピー",
	"Copy Path":                               "パスをコピー",
	"Copy code":                               "コードをコピー",
	"Copy conversation markdown":              "会話を Markdown でコピー",
	"Copy output":                             "出力をコピー",
	"Copy thinking":                           "思考過程をコピー",
	"Create Folder":                           "フォルダを作成",
	"Create New Project":                      "新規プロジェクト作成",
	"Create Project":                          "プロジェクトを作成",
	"Create a Project":                        "プロジェクトを作成",
	"Creating terminal...":                    "ターミナルを作成中...",
	"Current Workspace":                       "現在のワークスペース",
	"Current workspace":                       "現在のワークスペース",
	"Customizations & Skills":                 "拡張・スキル",
	"Daily Limit Remaining":                   "1日あたりの残り枠",
	"Danger Zone":                             "危険な操作",
	"Danger zone":                             "危険な操作",
	"Dark Theme":                              "ダークテーマ",
	"Delete Conversation":                     "会話を削除",
	"Delete MCP Server":                       "MCP サーバーを削除",
	"Delete Permanently":                      "完全に削除",
	"Delete Project":                          "プロジェクトを削除",
	"Delete Skill":                            "スキルを削除",
	"Delete Task":                             "タスクを削除",
	"Delete Terminal":                         "ターミナルを削除",
	"Delete Workspace":                        "ワークスペースを削除",
	"Delete conversation":                     "会話を削除",
	"Delete project":                          "プロジェクトを削除",
	"Delete workspace":                        "ワークスペースを削除",
	"Deny List Terminal Commands":             "拒否コマンド一覧",
	"Discard Changes":                         "変更を破棄",
	"Discard unstaged changes":                "未ステージの変更を破棄",
	"Display Options":                         "表示オプション",
	"Expand All":                              "すべて展開",
	"Expand sidebar":                          "サイドバーを展開",
	"Five Hour Limit Remaining":               "5時間残りクォータ",
	"Gemini Models":                           "Gemini モデル",
	"General Settings":                        "一般設定",
	"Keyboard Shortcuts":                      "キーボードショートカット",
	"Light Theme":                             "ライトテーマ",
	"Local Computer":                          "このPC",
	"Local Machine":                           "このPC",
	"Main Agent":                              "メインエージェント",
	"Mark as Read":                            "既読にする",
	"Mark as Unread":                          "未読にする",
	"More options":                            "その他のオプション",
	"More Options":                            "その他のオプション",
	"New Conversation":                        "新しい会話",
	"New conversation":                        "新しい会話",
	"New Conversation in Project":             "プロジェクトで新規会話",
	"New conversation in project":             "プロジェクトで新規会話",
	"New Conversation in Workspace":           "ワークスペースで新規会話",
	"New Workspace":                           "新規ワークスペース",
	"Loading customizations...":               "拡張・スキルを読み込み中...",
	"Refresh customizations":                  "拡張・スキルを更新",
	"Search automations...":                   "自動化を検索...",
	"Search customizations...":                "拡張・スキルを検索...",
	"Start Automation":                        "自動化を開始",
	"This Computer":                           "このPC",
	"This computer":                           "このPC",
	"No Model Selected":                       "モデル未選択",
	"No Models Available":                     "利用可能なモデルがありません",
	"No Project":                              "プロジェクトなし",
	"No conversations yet":                    "会話はまだありません",
	"Open in Editor":                          "エディターで開く",
	"Open Terminal":                           "ターミナルを開く",
	"Pin Conversation":                        "会話をピン留め",
	"Pin conversation":                        "会話をピン留め",
	"Pin this conversation":                   "この会話をピン留め",
	"Project options":                         "プロジェクト設定",
	"Project Settings":                        "プロジェクト設定",
	"Provide Feedback":                        "フィードバック",
	"Recent Conversations":                    "最近の会話",
	"Rename Conversation":                     "会話名を変更",
	"Rename conversation":                     "会話名を変更",
	"Search conversations":                    "会話を検索",
	"Search Conversations":                    "会話を検索",
	"Search...":                               "検索...",
	"Select Model":                            "モデルを選択",
	"Select Project":                          "プロジェクトを選択",
	"Select Workspace":                        "ワークスペースを選択",
	"Send Message":                            "メッセージを送信",
	"Stop All Subagents":                      "すべてのサブエージェントを停止",
	"Stop Subagent":                           "サブエージェントを停止",
	"Stop Task":                               "タスクを停止",
	"Stop execution":                          "実行を停止",
	"Strict Mode":                             "厳格モード",
	"System Prompt":                           "システムプロンプト",
	"Terminal Commands":                       "ターミナルコマンド",
	"Terms of Service":                        "利用規約",
	"Thinking Process":                        "思考プロセス",
	"Thinking...":                             "思考中...",
	"Toggle Sidebar":                          "サイドバーを切替",
	"Toggle Terminal":                         "ターミナルを切替",
	"Tool Permissions":                        "ツール権限",
	"Turbo Mode":                              "ターボモード",
	"Turbo mode":                              "ターボモード",
	"Undo To Here":                            "ここまで元に戻す",
	"Unpin This Conversation":                 "ピン留めを解除",
	"Unpin this conversation":                 "ピン留めを解除",
	"Untitled Conversation":                   "無題の会話",
	"Update Available":                        "アップデートが利用可能",
	"View Usage":                              "使用量を表示",
	"Weekly Limit Remaining":                  "週間残りクォータ",
	"Workspace Actions":                       "ワークスペース操作",
	"Workspace Settings":                      "ワークスペース設定",
	"Write a comment...":                      "コメントを入力...",
	"You have hit your 5-hour limit.":         "5時間枠の上限に達しました。",
	"You have hit your weekly limit.":         "週間枠の上限に達しました。",
	"Your Plan":                               "現在のプラン",
	"Your Plan:":                              "現在のプラン：",
	"Your quota for this model is running low.": "このモデルの残りクォータが少なくなっています。",
	"Zoom In":                                 "拡大",
	"Zoom Out":                                "縮小",
}

var l10nUIPropJa = map[string]string{
	"About":          "アプリについて",
	"Account":        "アカウント",
	"Actions":        "アクション",
	"Activity":       "アクティビティ",
	"Agents":         "エージェント",
	"All":            "すべて",
	"Allow":          "許可",
	"Amend":          "コミットを修正 (Amend)",
	"Antigravity":    "Multigravity",
	"App":            "アプリ設定",
	"Appearance":     "外観",
	"Application":    "アプリ設定",
	"Approval":       "承認待ち",
	"Approve":        "承認",
	"Archive":        "アーカイブ",
	"Artifacts":      "アーティファクト",
	"Automations":    "自動化",
	"Back":           "戻る",
	"Background":     "背景",
	"Block":          "ブロック",
	"Browser":        "ブラウザ",
	"Cancel":         "キャンセル",
	"Changes":        "変更",
	"Clear":          "クリア",
	"Close":          "閉じる",
	"Cloud":          "クラウド",
	"Commands":       "コマンド",
	"Commit":         "コミット",
	"Completed":      "完了",
	"Confirm":        "確認",
	"Conversations":  "会話一覧",
	"Copied":         "コピーしました",
	"Copied!":        "コピーしました！",
	"Copy":           "コピー",
	"Custom":         "カスタム",
	"Customizations": "拡張・スキル",
	"Customize":      "カスタマイズ",
	"Daily":          "毎日",
	"Dark":           "ダーク",
	"Default":        "標準",
	"Delete":         "削除",
	"Deny":           "拒否",
	"Description":    "説明",
	"Developer":      "開発者",
	"Disabled":       "無効",
	"Discard":        "破棄",
	"Display":        "表示",
	"Documents":      "ドキュメント",
	"Done":           "完了",
	"Edit":           "編集",
	"Editor":         "エディター",
	"Enabled":        "有効",
	"Execution":      "実行",
	"Failed":         "失敗",
	"Fast":           "高速",
	"Feedback":       "フィードバック",
	"Files":          "ファイル",
	"Filter":         "フィルター",
	"Folders":        "フォルダ",
	"Fork":           "フォーク",
	"General":        "一般",
	"Generating...":  "生成中...",
	"Global":         "グローバル",
	"High":           "高",
	"Hourly":         "毎時",
	"Installed":      "インストール済み",
	"Interrupted":    "中断",
	"Knowledge":      "ナレッジ",
	"Light":          "ライト",
	"Local":          "このPC",
	"Low":            "低",
	"Marketplace":    "マーケットプレイス",
	"Media":          "メディア",
	"Medium":         "中",
	"Model":          "モデル",
	"Models":         "モデル",
	"Monthly":        "毎月",
	"Narrow":         "狭い",
	"None":           "なし",
	"Normal":         "標準",
	"Notifications":  "通知",
	"OK":             "OK",
	"Open":           "開く",
	"Overview":       "概要",
	"Pin":            "ピン留め",
	"Plan":           "プラン",
	"Plugins":        "プラグイン",
	"Proceed":        "続行 (Proceed)",
	"Project":        "プロジェクト",
	"Projects":       "プロジェクト",
	"Push":           "プッシュ",
	"Queue":          "キュー",
	"Refresh":        "更新",
	"Refreshing":     "更新中...",
	"Refreshing...":  "更新中...",
	"Reject":         "拒否",
	"Reload":         "再読み込み",
	"Remaining":      "残り",
	"Rename":         "名前を変更",
	"Restore":        "復元",
	"Retry":          "再試行",
	"Review":         "確認",
	"Rules":          "ルール",
	"Running":        "実行中",
	"Save":           "保存",
	"Search":         "検索",
	"Settings":       "設定",
	"Share":          "共有",
	"Shortcuts":      "ショートカット",
	"Simplified":     "シンプル",
	"Skills":         "スキル",
	"Slow":           "低速",
	"Split":          "分割表示",
	"Starting":       "起動中",
	"Status":         "ステータス",
	"Steps":          "ステップ",
	"Stopped":        "停止済み",
	"Subagents":      "サブエージェント",
	"Submit":         "送信",
	"Submitting...":  "送信中...",
	"System":         "システム",
	"Tab":            "Tab 補完",
	"Telemetry":      "テレメトリ",
	"Terminal":       "ターミナル",
	"Terminals":      "ターミナル一覧",
	"Theme":          "テーマ",
	"Thinking":       "思考中",
	"Thinking...":    "思考中...",
	"Turbo":          "全自動 (Turbo)",
	"Unarchive":      "アーカイブ解除",
	"Uncommitted":    "未コミット",
	"Undo":           "元に戻す",
	"Unpin":          "ピン留め解除",
	"Upgrade":        "アップグレード",
	"Uploads":        "アップロード",
	"View":           "表示",
	"Weekly":         "毎週",
	"Wide":           "広い",
	"Working":        "実行中...",
	"Working..":      "実行中...",
	"Working...":     "実行中...",
	"Workspace":      "ワークスペース",
	"Worktree":       "ワークツリー",
}

var l10nHeaderPropJa = map[string]string{
	"Conversations": "会話一覧",
	"Workspaces":    "ワークスペース",
}

func applyL10nPatchesJa(data []byte) []byte {
	for _, p := range l10nExactPatchesJa {
		if bytes.Contains(data, p.old) {
			data = bytes.ReplaceAll(data, p.old, p.new)
		}
	}
	for _, p := range l10nPatchesJa {
		data = p.apply(data)
	}
	return data
}

func translateLiteralJa(data []byte, start, end int) (string, bool) {
	content := data[start+1 : end]
	if len(content) < 2 {
		return "", false
	}
	jaAny, inAny := l10nAnywhereJa[string(content)]
	jaProp, inProp := l10nUIPropJa[string(content)]
	jaHdr, inHdr := l10nHeaderPropJa[string(content)]
	if !inAny && !inProp && !inHdr {
		return "", false
	}
	ctx := literalContext(data, start, end)
	if ctx.call {
		return "", false
	}
	if inAny {
		return jaAny, true
	}
	if ctx.compare {
		return "", false
	}
	if inProp && (uiPropKeys[ctx.key] || ctx.child || ctx.tern) {
		return jaProp, true
	}
	if inHdr && ctx.key == "header" {
		return jaHdr, true
	}
	return "", false
}

func translateStringLiteralsJa(data []byte) []byte {
	var out []byte
	last := 0
	i := 0
	for i < len(data) {
		q := bytes.IndexByte(data[i:], '"')
		if q < 0 {
			break
		}
		start := i + q
		end := -1
		limit := start + l10nMaxLiteral
		if limit > len(data) {
			limit = len(data)
		}
		for j := start + 1; j < limit; j++ {
			c := data[j]
			if c == '\\' {
				j++
				continue
			}
			if c == '\n' {
				break
			}
			if c == '"' {
				end = j
				break
			}
		}
		if end > 0 {
			if ja, ok := translateLiteralJa(data, start, end); ok {
				if out == nil {
					out = make([]byte, 0, len(data)+len(data)/32)
				}
				out = append(out, data[last:start+1]...)
				out = append(out, ja...)
				last = end
				i = end + 1
				continue
			}
		}
		i = start + 1
	}
	if out == nil {
		return data
	}
	return append(out, data[last:]...)
}
