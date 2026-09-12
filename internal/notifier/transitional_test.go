package notifier

import (
	"testing"

	"antigravity-mobile/internal/proxy"
)

func TestTransitionalProgressMessage(t *testing.T) {
	positiveCases := []string{
		"正在调用 ego-browser 刷新雪球有效会话并加载最新 WAF 凭证...",
		"端到端全链路测试脚本已在后台执行，正在并行拉取五大宏观槽位研报、推演卡片结构、测试截图绑定并渲染验证高保真 PPTX，稍后将自动播报测试结果。",
		"正在等待槽位检索结果返回...",
		"正在等待研报筛选接口返回并启动全链路验证...",
		"正在执行全链路闭环自动化测试（包含 8201 统一服务连通性、Top 5 研报筛选、后台异步 PDF 下载），测试完成后将自动汇报...",
		"正在执行旧槽位向前兼容性接口请求验证...",
		"正在执行全链路闭环端到端自动化测试...",
		"正在将最新改动推送至 GitHub 远程仓库...",
		"正在拉取最新数据...",
		"已在后台启动编译，等待构建完成...",
		"正在处理中，请稍候...",
		"已派出子代理进行调研，等待调研结果返回...",
		"I have launched the iOS generic device Release build verification and will verify the compilation results once it finishes",
		"I have launched the end-to-end verification script to confirm all 7-day freshness filters. I will inspect the results once ready.",
		"I have executed the end-to-end verification script. Standing by for output.",
		"Testing the dual-layer scoring and LLM evaluation pipeline. Standing by for results.",
		"I have started the end-to-end verification test. Standing by for completion.",
		"Waiting for the background process to complete...",
		"The process is currently running in the background.",
	}

	for _, text := range positiveCases {
		if !IsTransitionalProgressMessage(text) {
			t.Errorf("expected %q to be recognized as transitional progress message, but was not", text)
		}
	}

	negativeCases := []string{
		"所有改动已在上一轮中自动完成提交并成功推送至远端：\n- 当前最新 Commit: db1df14\n- 分支状态: origin/main 已同步",
		"数据已更新完毕，共扫描 50 个组合，其中 3 个净值发生变化。",
		"所有组件、插件、专属技能与后端代码已全部落盘并打通，且已通过端到端自动化全链路测试。",
		"代码已通过所有单元测试，无 lint 错误。",
		"```json\n{\"status\": \"ok\", \"count\": 50}\n```",
		"| 序号 | 组合代码 | 净值 |\n| --- | --- | --- |\n| 1 | ZH218369 | 15.26 |",
		"Done. All tasks finished successfully.",
	}

	for _, text := range negativeCases {
		if IsTransitionalProgressMessage(text) {
			t.Errorf("expected %q NOT to be recognized as transitional progress message, but it was", text)
		}
	}
}

func TestIsCascadeInProgress(t *testing.T) {
	// Case 1: Running tasks > 0
	detailsWithRunningTasks := &proxy.TrajectoryDetails{
		RunningTasks: []proxy.RunningTaskItem{
			{
				ID:          "task-238",
				CommandLine: "ego-browser nodejs -e '...'",
			},
		},
		Steps: []proxy.TrajectoryStep{
			{
				Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
				Status: "CORTEX_STEP_STATUS_DONE",
				PlannerResponse: &struct {
					Response string `json:"response"`
					Thinking string `json:"thinking"`
				}{Response: "正在调用 ego-browser 刷新雪球有效会话并加载最新 WAF 凭证..."},
			},
		},
	}
	inProgress, reason := IsCascadeInProgress(detailsWithRunningTasks, false)
	if !inProgress {
		t.Errorf("expected inProgress=true with RunningTasks > 0, got false")
	}
	if reason != "has active background tasks" {
		t.Errorf("unexpected reason: %s", reason)
	}

	// Case 2: Step actively running
	detailsWithRunningStep := &proxy.TrajectoryDetails{
		Steps: []proxy.TrajectoryStep{
			{
				Type:   "CORTEX_STEP_TYPE_RUN_COMMAND",
				Status: "CORTEX_STEP_STATUS_RUNNING",
			},
		},
	}
	inProgress, _ = IsCascadeInProgress(detailsWithRunningStep, false)
	if !inProgress {
		t.Errorf("expected inProgress=true with running step, got false")
	}

	// Case 3: Subagent actively running
	detailsIdle := &proxy.TrajectoryDetails{
		Steps: []proxy.TrajectoryStep{
			{
				Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
				Status: "CORTEX_STEP_STATUS_DONE",
				PlannerResponse: &struct {
					Response string `json:"response"`
					Thinking string `json:"thinking"`
				}{Response: "Done."},
			},
		},
	}
	inProgress, _ = IsCascadeInProgress(detailsIdle, true)
	if !inProgress {
		t.Errorf("expected inProgress=true with running subagent, got false")
	}

	// Case 4: Transitional statement
	detailsTransitional := &proxy.TrajectoryDetails{
		Steps: []proxy.TrajectoryStep{
			{
				Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
				Status: "CORTEX_STEP_STATUS_DONE",
				PlannerResponse: &struct {
					Response string `json:"response"`
					Thinking string `json:"thinking"`
				}{Response: "正在等待槽位检索结果返回..."},
			},
		},
	}
	inProgress, _ = IsCascadeInProgress(detailsTransitional, false)
	if !inProgress {
		t.Errorf("expected inProgress=true with transitional sentence, got false")
	}

	// Case 5: Truly completed cascade
	detailsComplete := &proxy.TrajectoryDetails{
		Steps: []proxy.TrajectoryStep{
			{
				Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
				Status: "CORTEX_STEP_STATUS_DONE",
				PlannerResponse: &struct {
					Response string `json:"response"`
					Thinking string `json:"thinking"`
				}{Response: "所有改动已在上一轮中自动完成提交并成功推送至远端：\n- 当前最新 Commit: db1df14"},
			},
		},
	}
	inProgress, _ = IsCascadeInProgress(detailsComplete, false)
	if inProgress {
		t.Errorf("expected inProgress=false for truly completed cascade, got true")
	}
}
