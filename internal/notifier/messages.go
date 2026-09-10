package notifier

// Notification message templates — centralized for i18n readiness.
const (
	// Notification titles
	MsgTitleApproval     = "⚠️ Antigravity 需要审批"
	MsgTitleQuestion     = "❓ Antigravity 提问"
	MsgTitleRunCommand   = "⚠️ 确认执行终端命令"
	MsgTitleFileAccess   = "⚠️ 跨目录文件访问审批"
	MsgTitleProceed      = "📋 方案已就绪，等待确认"
	MsgTitleCompleted    = "🎉 Antigravity 任务已完成"
	MsgTitleFailed       = "❌ Antigravity 任务执行失败"

	// Notification body templates (use with fmt.Sprintf)
	MsgBodyCommand       = "Agent 申请执行命令: %s"
	MsgBodyWriteFile     = "Agent 申请修改文件: %s"
	MsgBodyReadFile      = "Agent 申请读取外部文件: %s"
	MsgBodyApproval      = "Agent 请求审批: %s"
	MsgBodyGenericAction = "Agent 申请 %s 操作: %s"
	MsgBodyQuestion      = "Agent 提出了新问题: %s"
	MsgBodyAccessFile    = "Agent 申请访问外部文件: %s"
	MsgBodyWaiting       = "Agent 正在等待您的操作: %s"
	MsgBodyProceed       = "「%s」已完成编写，等待您点击 Proceed 确认以继续执行。"
	MsgBodyCompleted     = "「%s」已顺利执行完毕，共执行 %d 个步骤。"
	MsgBodyFailed        = "「%s」执行出现异常或已被终止。"

	// Fallback display names
	MsgUntitledSession   = "未命名会话"
	MsgDefaultPlan       = "实施方案"
	MsgDefaultTask       = "后台任务"
)
