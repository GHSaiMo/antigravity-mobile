package notifier

import (
	"fmt"

	"antigravity-mobile/internal/i18n"
)

// Notification message templates — centralized for i18n readiness.
// Exported constants remain in Chinese for backward compatibility with existing callers and tests;
// internal helpers below select native zh / en / ja copy according to active i18n language.
const (
	// Notification titles
	MsgTitleApproval       = "⚠️ Antigravity 需要审批"
	MsgTitleQuestion       = "❓ Antigravity 提问"
	MsgTitleRunCommand     = "⚠️ 确认执行终端命令"
	MsgTitleFileAccess     = "⚠️ 跨目录文件访问审批"
	MsgTitleProceed        = "📋 方案已就绪，等待确认"
	MsgTitleCompleted      = "🎉 Antigravity 任务已完成"
	MsgTitleFailed         = "❌ Antigravity 任务执行失败"
	MsgTitleCockpitOffline = "⚠️ 座舱助手未能启动"

	// Notification body templates (use with fmt.Sprintf)
	MsgBodyCommand        = "Agent 申请执行命令: %s"
	MsgBodyWriteFile      = "Agent 申请修改文件: %s"
	MsgBodyReadFile       = "Agent 申请读取外部文件: %s"
	MsgBodyApproval       = "Agent 请求审批: %s"
	MsgBodyGenericAction  = "Agent 申请 %s 操作: %s"
	MsgBodyQuestion       = "Agent 提出了新问题: %s"
	MsgBodyAccessFile     = "Agent 申请访问外部文件: %s"
	MsgBodyWaiting        = "Agent 正在等待您的操作: %s"
	MsgBodyProceed        = "「%s」已完成编写，等待您点击 Proceed 确认以继续执行。"
	MsgBodyCompleted      = "「%s」已顺利执行完毕，共执行 %d 个步骤。"
	MsgBodyFailed         = "「%s」执行出现异常或已被终止。"
	MsgBodyCockpitOffline = "Cockpit Tools 自动拉起 2 轮后仍无法连接 (127.0.0.1:18081)，账号配额自动刷新已暂停。"

	// Fallback display names
	MsgUntitledSession = "未命名会话"
	MsgDefaultPlan     = "实施方案"
	MsgDefaultTask     = "后台任务"
)

func msgTitleApproval() string {
	return i18n.T3(MsgTitleApproval, "⚠️ Antigravity Approval Needed", "⚠️ Antigravity 承認が必要です")
}

func msgTitleHighRiskApproval() string {
	return i18n.T3("🚨 高危操作审批", "🚨 High-Risk Approval", "🚨 高リスク操作の承認")
}

func msgTitleWorkflowApproval() string {
	return i18n.T3("⚠️ 动态工作流需审批", "⚠️ Workflow Approval Needed", "⚠️ ワークフローの承認が必要です")
}

func msgTitleQuestion() string {
	return i18n.T3(MsgTitleQuestion, "❓ Antigravity Question", "❓ Antigravity からの質問")
}

func msgTitleRunCommand() string {
	return i18n.T3(MsgTitleRunCommand, "⚠️ Confirm Terminal Command", "⚠️ コマンド実行の確認")
}

func msgTitleFileAccess() string {
	return i18n.T3(MsgTitleFileAccess, "⚠️ File Access Approval", "⚠️ 外部ファイルアクセスの承認")
}

func msgTitleProceed() string {
	return i18n.T3(MsgTitleProceed, "📋 Plan Ready to Proceed", "📋 計画の確認待ち")
}

func msgTitleCompleted() string {
	return i18n.T3(MsgTitleCompleted, "🎉 Antigravity Task Completed", "🎉 Antigravity タスク完了")
}

func msgTitleFailed() string {
	return i18n.T3(MsgTitleFailed, "❌ Antigravity Task Failed", "❌ Antigravity タスク失敗")
}

func msgTitleCockpitOffline() string {
	return i18n.T3(MsgTitleCockpitOffline, "⚠️ Cockpit Tools Offline", "⚠️ Cockpit Tools 起動失敗")
}

func msgBodyWorkflowDesc(desc string) string {
	return i18n.Sprintf3("编排工作流申请执行：%s", "Workflow requests execution: %s", "ワークフロー実行リクエスト: %s", desc)
}

func msgBodyWorkflowTitle(title string) string {
	return i18n.Sprintf3("动态工作流申请执行：%s", "Workflow requests approval: %s", "ワークフロー承認待ち: %s", title)
}

func msgBodyCommand(target string) string {
	return i18n.Sprintf3(MsgBodyCommand, "Agent wants to run command: %s", "Agent がコマンド実行を要求: %s", target)
}

func msgBodyWriteFile(target string) string {
	return i18n.Sprintf3(MsgBodyWriteFile, "Agent wants to edit file: %s", "Agent がファイル編集を要求: %s", target)
}

func msgBodyReadFile(target string) string {
	return i18n.Sprintf3(MsgBodyReadFile, "Agent wants to read file: %s", "Agent が外部ファイル閲覧を要求: %s", target)
}

func msgBodyApproval(desc string) string {
	return i18n.Sprintf3(MsgBodyApproval, "Agent requests approval: %s", "Agent が承認を要求: %s", desc)
}

func msgBodyGenericAction(action, target string) string {
	return i18n.Sprintf3(MsgBodyGenericAction, "Agent requests %s: %s", "Agent が %s 操作を要求: %s", action, target)
}

func msgBodyQuestion(q string) string {
	return i18n.Sprintf3(MsgBodyQuestion, "Agent asked a question: %s", "Agent から質問があります: %s", q)
}

func msgBodyAccessFile(target string) string {
	return i18n.Sprintf3(MsgBodyAccessFile, "Agent wants to access file: %s", "Agent が外部ファイルアクセスを要求: %s", target)
}

func msgBodyWaiting(title string) string {
	return i18n.Sprintf3(MsgBodyWaiting, "Agent is waiting for you: %s", "Agent が操作を待っています: %s", title)
}

func msgBodyProceed(plan string) string {
	return i18n.Sprintf3(MsgBodyProceed, "\"%s\" is ready — tap Proceed to continue.", "「%s」の計画が完了しました。Proceed をタップして続行してください。", plan)
}

func msgBodyCompleted(task string, steps int) string {
	return i18n.Sprintf3(MsgBodyCompleted, "\"%s\" finished (%d steps).", "「%s」が完了しました（全 %d ステップ）。", task, steps)
}

func msgBodyFailed(task string) string {
	return i18n.Sprintf3(MsgBodyFailed, "\"%s\" encountered an error or was stopped.", "「%s」でエラーが発生したか停止されました。", task)
}

func msgBodyCockpitOffline() string {
	return i18n.T3(
		MsgBodyCockpitOffline,
		"Cockpit Tools could not be reached after 2 relaunch attempts (127.0.0.1:18081); auto-refresh paused.",
		"Cockpit Tools に2回再起動を試みましたが接続できません (127.0.0.1:18081)。自動更新を一時停止しました。",
	)
}

func msgDefaultPlan() string {
	return i18n.T3(MsgDefaultPlan, "Implementation Plan", "実装計画")
}

func msgDefaultTask() string {
	return i18n.T3(MsgDefaultTask, "Background Task", "バックグラウンドタスク")
}

func formatSessionPrefix(sessionTitle, body string) string {
	return i18n.Sprintf3("【%s】%s", "[%s] %s", "【%s】%s", sessionTitle, body)
}

func msgTitleCacheMissAlert() string {
	return i18n.T3(
		"⚠️ 缓存连续未命中预警",
		"⚠️ Prompt Cache Miss Alert",
		"⚠️ キャッシュ連続ミス警告",
	)
}

func msgBodyCacheMissAlert(streak int, tokensText string) string {
	return i18n.Sprintf3(
		"连续 %d 步未命中缓存（累计重算 %s 输入），可能正在加速消耗周额度，建议查看或及时中断。",
		"%d consecutive cache misses (%s uncached input) — weekly quota may drain faster. Check or interrupt if needed.",
		"%d ステップ連続でキャッシュ未命中（再計算入力 %s）。週間クォータを急消費している可能性があります。",
		streak,
		tokensText,
	)
}

func msgTitleStallAlert() string {
	return i18n.T3(
		"⏳ 任务长时间无进展",
		"⏳ Task Appears Stalled",
		"⏳ タスクが停止している可能性",
	)
}

func msgBodyStallAlert(stalledMinutes int) string {
	return i18n.Sprintf3(
		"已连续 %d 分钟无新输出或步骤推进，可能已卡住，点击查看或中止。",
		"No new output or step progress for %d min — it may be stuck. Tap to inspect or stop.",
		"%d 分間新しい出力やステップの進行がありません。確認または停止してください。",
		stalledMinutes,
	)
}

func formatCompactTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000.0)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000.0)
	default:
		return fmt.Sprintf("%d", n)
	}
}
