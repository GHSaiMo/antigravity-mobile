package proxy

import (
	"bytes"
	"regexp"
	"strings"
)

// Patches for text the bundle assembles at runtime (template literals, pluralisation
// tables, date-fns locale data). Each patch has a cheap literal anchor so the regexp only
// runs when its anchor occurs. They match raw (untranslated) upstream source and run
// before the literal pass.

type staticReplacement struct {
	old []byte
	new []byte
}

type l10nPatch struct {
	anchor string
	re     *regexp.Regexp
	repl   string
	// window > 0: the pattern has no literal prefix, so run it only on the bytes around each
	// occurrence of anchor ([pos-window, pos+window]) instead of scanning the whole bundle.
	window int
}

// tpl builds a patch from a template-literal text: every "{}" in en matches a JS
// "${...}" placeholder, and "{n}" in zh re-inserts the n-th captured placeholder.
func tpl(en, zh string) l10nPatch {
	parts := strings.Split(en, "{}")
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteString(`(\$\{[^}]*\})`)
		}
		sb.WriteString(regexp.QuoteMeta(p))
	}
	repl := zh
	for n := 9; n >= 1; n-- {
		repl = strings.ReplaceAll(repl, "{"+string(rune('0'+n))+"}", "${"+string(rune('0'+n))+"}")
	}
	return l10nPatch{anchor: parts[0], re: regexp.MustCompile(sb.String()), repl: repl}
}

func rx(anchor, pattern, repl string) l10nPatch {
	return l10nPatch{anchor: anchor, re: regexp.MustCompile(pattern), repl: repl, window: 160}
}

// apply replaces matches of the patch in data.
func (p l10nPatch) apply(data []byte) []byte {
	if p.window == 0 {
		if !bytes.Contains(data, []byte(p.anchor)) {
			return data
		}
		return p.re.ReplaceAll(data, []byte(p.repl))
	}
	anchor := []byte(p.anchor)
	var out []byte
	last, i := 0, 0
	for {
		j := bytes.Index(data[i:], anchor)
		if j < 0 {
			break
		}
		pos := i + j
		lo, hi := pos-p.window, pos+len(anchor)+p.window
		if lo < last {
			lo = last
		}
		if hi > len(data) {
			hi = len(data)
		}
		loc := p.re.FindSubmatchIndex(data[lo:hi])
		if loc == nil || lo+loc[1] <= pos {
			i = pos + len(anchor)
			continue
		}
		out = append(out, data[last:lo+loc[0]]...)
		out = p.re.Expand(out, []byte(p.repl), data[lo:hi], loc)
		last = lo + loc[1]
		i = last
	}
	if out == nil {
		return data
	}
	return append(out, data[last:]...)
}

// l10nExactPatches are verbatim snippet replacements (date-fns English locale, ...).
var l10nExactPatches = []staticReplacement{
	{old: []byte(`one:"less than a second",other:"less than {{count}} seconds"`), new: []byte(`one:"不到 1 秒",other:"不到 {{count}} 秒"`)},
	{old: []byte(`one:"1 second",other:"{{count}} seconds"`), new: []byte(`one:"1 秒",other:"{{count}} 秒"`)},
	{old: []byte(`halfAMinute:"half a minute"`), new: []byte(`halfAMinute:"半分钟"`)},
	{old: []byte(`one:"less than a minute",other:"less than {{count}} minutes"`), new: []byte(`one:"不到 1 分钟",other:"不到 {{count}} 分钟"`)},
	{old: []byte(`one:"1 minute",other:"{{count}} minutes"`), new: []byte(`one:"1 分钟",other:"{{count}} 分钟"`)},
	{old: []byte(`one:"about 1 hour",other:"about {{count}} hours"`), new: []byte(`one:"约 1 小时",other:"约 {{count}} 小时"`)},
	{old: []byte(`one:"1 hour",other:"{{count}} hours"`), new: []byte(`one:"1 小时",other:"{{count}} 小时"`)},
	{old: []byte(`one:"1 day",other:"{{count}} days"`), new: []byte(`one:"1 天",other:"{{count}} 天"`)},
	{old: []byte(`one:"about 1 week",other:"about {{count}} weeks"`), new: []byte(`one:"约 1 周",other:"约 {{count}} 周"`)},
	{old: []byte(`one:"1 week",other:"{{count}} weeks"`), new: []byte(`one:"1 周",other:"{{count}} 周"`)},
	{old: []byte(`one:"about 1 month",other:"about {{count}} months"`), new: []byte(`one:"约 1 个月",other:"约 {{count}} 个月"`)},
	{old: []byte(`one:"1 month",other:"{{count}} months"`), new: []byte(`one:"1 个月",other:"{{count}} 个月"`)},
	{old: []byte(`one:"about 1 year",other:"about {{count}} years"`), new: []byte(`one:"约 1 年",other:"约 {{count}} 年"`)},
	{old: []byte(`one:"1 year",other:"{{count}} years"`), new: []byte(`one:"1 年",other:"{{count}} 年"`)},
	{old: []byte(`one:"over 1 year",other:"over {{count}} years"`), new: []byte(`one:"超过 1 年",other:"超过 {{count}} 年"`)},
	{old: []byte(`one:"almost 1 year",other:"almost {{count}} years"`), new: []byte(`one:"将近 1 年",other:"将近 {{count}} 年"`)},
	// pluralisation table behind "Explored 3 files, 2 searches" (singular == plural in Chinese)
	{old: []byte(`files:["file","files"],folders:["folder","folders"],edits:["file","files"],searches:["search","searches"],terminal:["command","commands"],tasks:["task","tasks"],web:["page","pages"],browser:["browser","browsers"],images:["image","images"],actions:["action","actions"],artifacts:["artifact","artifacts"]`),
		new: []byte(`files:["个文件","个文件"],folders:["个文件夹","个文件夹"],edits:["个文件","个文件"],searches:["次搜索","次搜索"],terminal:["条命令","条命令"],tasks:["个任务","个任务"],web:["个网页","个网页"],browser:["个浏览器","个浏览器"],images:["张图片","张图片"],actions:["个操作","个操作"],artifacts:["个产物","个产物"]`)},
	{old: []byte(`===1?"command":"commands"`), new: []byte(`===1?"条命令":"条命令"`)},
	// reasoning-effort suffix next to model names ("Gemini 3.8 Flash High")
	{old: []byte(`{low:"Low",medium:"Medium",high:"High",max:"Max"}`), new: []byte(`{low:"低",medium:"中",high:"高",max:"极高"}`)},
	// conversation width toggle group
	{old: []byte(`return"Narrow";case 3:return"Wide";default:return"Default"`), new: []byte(`return"窄";case 3:return"宽";default:return"默认"`)},
	// quota reset countdown: "Resets in 3d 17h"
	{old: []byte("`Resets in ${"), new: []byte("`距重置 ${")},
}

const l10nSettingsNav = `{Account:"账户与计划",General:"常规偏好",Appearance:"外观主题",Skin:"产品皮肤",Editor:"编辑器设置",Tab:"Tab 智能补全",Notifications:"消息与通知",Customizations:"扩展与技能",App:"客户端偏好",Shortcuts:"键盘快捷键",Models:"模型与配额",Developer:"开发者调试",Browser:"浏览器设置"}`

var l10nPatches = []l10nPatch{
	// relative times: date-fns formatDistance suffixes ("3 hours ago" / "in 3 hours")
	rx(`" ago"`, `"in "\+(\w+):(\w+)\+" ago"`, `${1}+"后":${2}+"前"`),
	// "Explored 3 files, ran 1 command"
	rx(`"Running":"running"`, `(\w+)=(\w+)\?(\w+)\?"Running":"running":(\w+)\?"Ran":"ran"`, `${1}=${2}?${3}?"正在运行":"正在运行":${4}?"已运行":"已运行"`),
	// settings navigation shows the screen id when it has no label (General, Appearance, ...)
	rx(`.label]));function `, `(\.map\(\w+=>\[\w+\.screen,\w+\.label\]\)\);function \w+\((\w+)\)\{return )(\w+)\.get\((\w+)\)\?\?(\w+)\}`, `${1}${3}.get(${4})??`+l10nSettingsNav+`[${4}]??${5}}`),
	// "${n} files changed"
	rx(`"file":"files"} changed`, `(\$\{\w+\}) \$\{\w+===1\?"file":"files"\} changed`, `${1} 个文件已更改`),
	// "Thinking for 5s" (JSX children)
	rx(`"Thinking for "`, `"Thinking for ",(Math\.max\(1,\w+\)),"s"`, `"思考 ",${1},"秒"`),

	tpl("When toggled on, {} will use your AI credits to fulfill model requests once you're out of model quota. {} will always use your model quota first before using AI credits.", "开启后，当模型配额用尽时，{1} 将使用您的 AI 点数来满足模型请求；{2} 始终会优先使用模型配额，之后才使用 AI 点数。"),
	tpl("Thought for {}s", "已思考 {1} 秒"),
	tpl("Worked for {}", "已工作 {1}"),
	tpl("See {} more", "查看另外 {1} 项"),
	tpl("See all ({})", "查看全部 ({1})"),
	tpl("Show finished ({})", "显示已完成 ({1})"),
	tpl("Send all ({})", "全部发送 ({1})"),
	tpl("Send now: {}", "立即发送：{1}"),
	tpl(`Searched for "{}"`, `已搜索“{1}”`),
	tpl("Searched for files: {}", "已搜索文件：{1}"),
	tpl(`Code search: "{}"`, `代码搜索：“{1}”`),
	tpl(`Internal search: "{}"`, `内部搜索：“{1}”`),
	tpl("Listed directory {}", "已列出目录 {1}"),
	tpl(`Browser task: "{}"`, `浏览器任务：“{1}”`),
	tpl("Opened browser: {}", "已打开浏览器：{1}"),
	tpl(`Searched web: "{}"`, `已搜索网页：“{1}”`),
	tpl("Invoked subagent: {}", "已调用子智能体：{1}"),
	tpl(`Generated image: "{}"`, `已生成图片：“{1}”`),
	tpl("Used tool: {}", "已使用工具：{1}"),
	tpl("Exit code {}", "退出码 {1}"),
	tpl("Working directory: {}", "工作目录：{1}"),
	tpl("Typing '{}' in Browser", "在浏览器中输入“{1}”"),
	tpl("Typed '{}' in Browser", "已在浏览器中输入“{1}”"),
	tpl("Sending {} to command", "正在向命令发送 {1}"),
	tpl("Sent {} to command", "已向命令发送 {1}"),
	tpl("Rejected sending {} to command", "已拒绝向命令发送 {1}"),
	tpl("Suggested sending {} to command", "建议向命令发送 {1}"),
	tpl("Error sending {} to command", "向命令发送 {1} 出错"),
	tpl("Waiting for command completion (up to {} seconds)", "等待命令完成（最长 {1} 秒）"),
	tpl("Question {} of {}", "问题 {1} / {2}"),
	tpl("Forked from {}", "分叉自 {1}"),
	tpl("View Diff for {}", "查看 {1} 的差异"),
	tpl("Open conversation {}", "打开会话 {1}"),
	tpl("Move to Group: {}", "移动到分组：{1}"),
	tpl("Mark all {} conversations as read", "将全部 {1} 个会话标记为已读"),
	tpl("Are you sure you want to mark all {} conversations as read? This action cannot be undone.", "您确定要将全部 {1} 个会话标记为已读吗？此操作无法撤销。"),
	tpl(`Are you sure you want to delete the hook "{}"?`, `您确定要删除 Hook“{1}”吗？`),
	tpl("Load {} earlier steps", "加载更早的 {1} 个步骤"),
	tpl("Load {} later steps", "加载更晚的 {1} 个步骤"),
	tpl("Load older messages, showing {} of {}", "加载更早的消息，当前显示 {1} / {2}"),
	tpl("No more older messages, showing {} of {}", "没有更早的消息，当前显示 {1} / {2}"),
	tpl("Model unavailable, retrying in {}s{}", "模型不可用，{1} 秒后重试{2}"),
	tpl("Model unavailable, retrying{}", "模型不可用，正在重试{1}"),
	tpl("Shared with: {}", "共享给：{1}"),
	tpl("Deleted workspace {}", "已删除工作区 {1}"),
	tpl("Unmounted workspace {}", "已卸载工作区 {1}"),
	tpl("Unmount workspace {}", "卸载工作区 {1}"),
	tpl("Delete workspace {}", "删除工作区 {1}"),
	tpl("Set up {}", "设置 {1}"),
	tpl("Review permissions for {}", "查看 {1} 的权限"),
	tpl("Continue with {}", "使用 {1} 继续"),
	tpl("Sign in to use {}!", "登录以使用 {1}！"),
	tpl("Sorry, this account is ineligible to use {}", "抱歉，此账号无资格使用 {1}"),
	tpl("Further action is required to use {}", "需要进一步操作才能使用 {1}"),
	tpl("Valid until {}", "有效期至 {1}"),
	tpl("Pushed to {}.", "已推送到 {1}。"),
	tpl("Committed to {}.", "已提交到 {1}。"),
	tpl("Amended commit on {}.", "已修订 {1} 上的提交。"),
	tpl("Publish {} to origin", "将 {1} 发布到 origin"),
	tpl("Published {} to origin.", "已将 {1} 发布到 origin。"),
	tpl("Push {} commit{} to {}", "推送 {1} 个提交到 {3}"),
	tpl("Pushed {} commit{} to {}.", "已推送 {1} 个提交到 {3}。"),
	tpl("Agent needs permission to execute JavaScript on {}", "智能体需要权限才能在 {1} 上执行 JavaScript"),
	tpl("Agent needs permission to act on {}", "智能体需要权限才能操作 {1}"),
	tpl("Save rule to always allow {}?", "要保存规则以始终允许 {1} 吗？"),
	tpl("Yes, save rule for '{}' in this conversation", "是，将“{1}”的规则保存到此会话"),
	tpl("Yes, and always allow '{}' in this conversation", "是，并在此会话中始终允许“{1}”"),
	tpl("Yes, save rule for '{}' in this workspace", "是，将“{1}”的规则保存到此工作区"),
	tpl("Yes, and always allow '{}' in this workspace", "是，并在此工作区中始终允许“{1}”"),
	tpl("Yes, save rule for '{}' globally", "是，将“{1}”的规则保存为全局规则"),
	tpl("Requires manual confirmation: {}", "需要手动确认：{1}"),
	tpl("Conflicts with your configured Ask permission: {}", "与您配置的“询问”权限冲突：{1}"),
	tpl("Side question failed: {}", "附带提问失败：{1}"),
	tpl("Best of N ({})", "Best of N（{1}）"),
	tpl("Compare responses from {} models side-by-side to find the best result.", "并排对比 {1} 个模型的回复，找出最佳结果。"),
	tpl("Unused capacity: {} tokens", "未使用容量：{1} 个 Token"),
	tpl("Artifacts ({} Files for Conversation)", "产物（本会话共 {1} 个文件）"),
	tpl("Page {} failed to load", "第 {1} 页加载失败"),
	tpl("Go to page {}", "前往第 {1} 页"),
	tpl("Go to slide {}", "前往第 {1} 张幻灯片"),
	tpl("Comment on page {}", "第 {1} 页的评论"),
	tpl("Search match on page {}", "第 {1} 页的搜索匹配项"),
	tpl("Bounding box on page {}", "第 {1} 页的边界框"),
	tpl("Edit comment: {}", "编辑评论：{1}"),
	tpl("Enlarged media: {}", "放大的媒体：{1}"),
	tpl("Artifact image {}", "产物图片 {1}"),
	tpl("User uploaded media {}", "用户上传的媒体 {1}"),
	tpl("Open the {} plugin", "打开 {1} 插件"),
	tpl("Opens external link: {}", "打开外部链接：{1}"),
	tpl("More actions for {}", "{1} 的更多操作"),
	tpl("Auth code for {}", "{1} 的授权码"),
	tpl("Failed to enable plugin {}", "启用插件 {1} 失败"),
	tpl("Process exited with code {}.", "进程已退出，退出码 {1}。"),
	tpl("Exited with code {}.", "已退出，退出码 {1}。"),
	tpl("Unnamed skill {}", "未命名技能 {1}"),
	tpl("Filter skills and rules: {}", "筛选技能与规则：{1}"),
	tpl("Cost summary: {}", "费用汇总：{1}"),
	tpl("Executing task: {}", "正在执行任务：{1}"),
	tpl(`Creating workspace "{}"...`, `正在创建工作区“{1}”...`),
	tpl("A file already exists at {}.", "{1} 处已存在同名文件。"),
	tpl(`The folder “{}” does not exist. Would you like to create it?`, `文件夹“{1}”不存在，是否创建？`),
	tpl("You have {} conversations in progress. Updating will interrupt them, and they will automatically resume once the update completes.", "您有 {1} 个进行中的会话。更新将中断这些会话，并在更新完成后自动恢复。"),
	tpl("You have {} conversations in progress. Updating will cancel them and you'll need to manually restart them.", "您有 {1} 个进行中的会话。更新将取消这些会话，您需要手动重新启动。"),
	tpl("Minimum length is {}", "最小长度为 {1}"),
	tpl("Maximum length is {}", "最大长度为 {1}"),
	tpl("Input must match {}", "输入必须匹配 {1}"),
	tpl("Minimum value is {}", "最小值为 {1}"),
	tpl("Maximum value is {}", "最大值为 {1}"),
	tpl("Select at least {} items", "至少选择 {1} 项"),
	tpl("Select at most {} items", "最多选择 {1} 项"),
	tpl("Receive product updates, tips, and promotions from Google {} via email.", "通过电子邮件接收来自 {1} 的产品更新速递、使用技巧与官方资讯。"),
	tpl("When toggled on, {} collects usage data to help Google enhance performance and features.", "开启后，{1} 将收集匿名使用诊断数据，以帮助提升系统性能与体验。"),
}

func applyL10nPatches(data []byte) []byte {
	for _, p := range l10nExactPatches {
		if bytes.Contains(data, p.old) {
			data = bytes.ReplaceAll(data, p.old, p.new)
		}
	}
	for _, p := range l10nPatches {
		data = p.apply(data)
	}
	return data
}
