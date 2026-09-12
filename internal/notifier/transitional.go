package notifier

import (
	"regexp"
	"strings"

	"antigravity-mobile/internal/proxy"
)

var transitionalPatterns = []*regexp.Regexp{
	// 1. 正在 + 动词开头的过程句（例如：正在调用 ego-browser 刷新雪球有效会话...）
	regexp.MustCompile(`^正在(调用|执行|等待|运行|刷新|重试|拉取|扫描|分析|处理|构建|打包|推送|生成|验证|检查|排查|爬取|抓取|测试|准备|发起|请求|下载|查询|编译|部署|同步)`),

	// 2. 含有在后台运行/启动等过渡描述
	regexp.MustCompile(`(已在后台|正在后台|在后台执行|在后台运行|已启动后台|已派发后台|已派出子代理|已调用子代理|已启动任务|已发起任务)`),

	// 3. 正在等待...完成/返回/结果
	regexp.MustCompile(`正在等待.*(完成|返回|结果|汇报|响应|结束|就绪|回复)`),

	// 4. 等待...完成后汇报
	regexp.MustCompile(`(测试|执行|扫描|构建|抓取|任务|操作)?完成后将(自动)?(汇报|播报|通知|更新|输出|返回|同步)`),
	regexp.MustCompile(`稍后(将|会)(自动)?(汇报|播报|通知|更新|输出|返回|同步)`),
	regexp.MustCompile(`等待.*(返回|完成|执行结果|执行完毕)`),

	// 5. 结尾带有省略号且处于进行时的短句
	regexp.MustCompile(`^(正在|请稍[候后]|等待中|处理中).*(\.{3}|…)$`),

	// 6. English transitional patterns
	regexp.MustCompile(`(?i)^(i have|i've) (launched|started|executed|initiated|spawned|dispatched) .* (wait|standing by|once it finishes|once completed|results)`),
	regexp.MustCompile(`(?i)standing by for (results|completion|output|response)`),
	regexp.MustCompile(`(?i)waiting for .* to (finish|complete|return)`),
	regexp.MustCompile(`(?i)will (verify|check|inspect|report|notify|update) .* once (it finishes|completed|ready|done)`),
	regexp.MustCompile(`(?i)currently running in the background`),
	regexp.MustCompile(`(?i)proceeding to .* while .* finishes`),
}

// IsTransitionalProgressMessage determines if an agent's response is an intermediate
// transitional statement (过程句) indicating that work is ongoing in the background.
func IsTransitionalProgressMessage(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}

	// Final reports with code fences or markdown tables are deliverables, not simple transitional sentences
	if strings.Contains(t, "```") || strings.Contains(t, "| ---") {
		return false
	}

	for _, re := range transitionalPatterns {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}

// ExtractLatestAgentResponse returns the last non-empty planner response text and its step index.
func ExtractLatestAgentResponse(steps []proxy.TrajectoryStep) (string, int) {
	for i := len(steps) - 1; i >= 0; i-- {
		s := steps[i]
		if s.Type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" && s.PlannerResponse != nil {
			resp := strings.TrimSpace(s.PlannerResponse.Response)
			if resp != "" {
				return resp, i
			}
		}
	}
	return "", -1
}

// IsCascadeInProgress evaluates whether a cascade is genuinely ongoing, based on:
// 1. Active running tasks (details.RunningTasks)
// 2. Trajectory steps still in RUNNING / PENDING / WAITING / GENERATING state
// 3. Actively running subagents belonging to this cascade
// 4. Latest agent message being a transitional statement (过程句) following an async dispatch
func IsCascadeInProgress(details *proxy.TrajectoryDetails, hasRunningSubagents bool) (bool, string) {
	if details == nil {
		return false, ""
	}

	// 1. Check for active background tasks recorded in TrajectoryDetails
	if len(details.RunningTasks) > 0 {
		return true, "has active background tasks"
	}

	// 2. Check for any trajectory step currently running or pending
	for _, s := range details.Steps {
		switch s.Status {
		case "CORTEX_STEP_STATUS_RUNNING":
			return true, "step is actively running"
		case "CORTEX_STEP_STATUS_PENDING", "CORTEX_STEP_STATUS_WAITING", "CORTEX_STEP_STATUS_GENERATING":
			return true, "step is pending/waiting"
		}
	}

	// 3. Check for active subagents
	if hasRunningSubagents {
		return true, "subagent is actively running"
	}

	// 4. Check whether latest agent message is a transitional statement ("过程句")
	latestText, latestIdx := ExtractLatestAgentResponse(details.Steps)
	if latestText != "" {
		if IsTransitionalProgressMessage(latestText) {
			return true, "latest message is a transitional progress statement (过程句)"
		}

		// Contextual check: did the current turn launch a background task or receive background task notice?
		if latestIdx > 0 {
			for i := latestIdx - 1; i >= 0; i-- {
				step := details.Steps[i]
				if step.Type == "CORTEX_STEP_TYPE_USER_INPUT" {
					break // Scoped strictly to the latest turn
				}
				stepContent := step.Content
				if stepContent == "" && step.SystemMessage != nil {
					stepContent = step.SystemMessage.Content
				}
				if step.TaskDetails != nil ||
					(step.RunCommand != nil && strings.Contains(strings.ToLower(step.RunCommand.CommandLine), "task")) ||
					strings.Contains(stepContent, "Tool is running as a background task") ||
					strings.Contains(stepContent, "YOU MUST TAKE ONE OF THE FOLLOWING TWO ACTIONS") {
					// In this turn a background task was injected, and agent output is short status update
					if len(latestText) < 300 && !strings.Contains(latestText, "\n\n") {
						return true, "turn ended after background task launch instruction (Option B)"
					}
				}
			}
		}
	}

	return false, ""
}
