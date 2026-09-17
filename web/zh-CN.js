/**
 * Antigravity Desktop Web Workbench - 全功能 UI 精准中文化引擎 (zh-CN)
 * 涵盖：侧边栏、主工作区、模型与配额 (Models & Usage)、系统设置 (Settings)、
 * 危险区域 (Danger Zone)、权限管理、扩展与技能、快捷键、文件浏览器与菜单。
 * 严格保护：代码块 (<pre>, <code>, monaco)、终端 (xterm) 与用户对话输入。
 */
(function () {
  "use strict";

  // 1. 静态短语精确字典 (精确匹配 / 去除首尾空格后匹配)
  const exactDict = new Map([
    // --- 侧边栏与主导航 ---
    ["New Conversation", "新建会话"],
    ["Conversation History", "历史会话"],
    ["Scheduled Tasks", "定时任务"],
    ["Projects", "工程项目"],
    ["Settings", "系统设置"],
    ["Untitled Conversation", "未命名会话"],
    ["No conversations yet", "暂无历史会话"],
    ["Open Conversation History", "打开历史会话"],
    ["Open Scheduled Tasks", "打开定时任务"],
    ["Collapse sidebar", "收起侧边栏"],
    ["Expand sidebar", "展开侧边栏"],
    ["Close sidebar", "关闭侧边栏"],
    ["Toggle Sidebar", "切换侧边栏"],
    ["Toggle Auxiliary Pane", "切换辅助面板"],
    ["Toggle Terminal", "切换终端面板"],
    ["Toggle File Viewer", "切换文件浏览器"],
    ["Toggle Editor", "切换代码编辑器"],
    ["New Editor Window", "新建编辑器窗口"],
    ["Close Tab", "关闭标签页"],
    ["Open Workspace", "打开工作区"],
    ["Split Terminal", "分屏终端"],
    ["Close Terminal Tab", "关闭终端标签"],
    ["New Terminal Tab", "新建终端标签"],
    ["Open Command Palette", "打开全局命令面板"],
    ["Command Palette", "全局命令面板"],
    ["Open File Search", "全局文件检索"],
    ["File Picker", "快速定位文件"],
    ["Code Search", "代码搜索"],
    ["Open Workspace Selector", "打开工作区选择器"],
    ["Open Keyboard Shortcuts", "查看快捷键设置"],
    ["Open Conversation Picker", "快速切换会话"],
    ["Focus Input", "聚焦到底部输入框"],
    ["Find in Pane", "在面板内查找"],
    ["Zoom In", "放大界面"],
    ["Zoom Out", "缩小界面"],
    ["Reset Zoom", "重置缩放"],
    ["Check for Updates", "检查版本更新"],
    ["Update Available", "发现新版本"],
    ["Reload", "重新加载"],

    // --- 模型、配额与额度 (Models & Usage) ---
    ["Models & Usage", "模型与配额使用"],
    ["Manage your model quota and credits.", "管理您的模型使用配额与点数。"],
    ["Plan", "当前订阅计划"],
    ["Your Plan", "当前计划"],
    ["Your Plan:", "当前套餐："],
    ["Upgrade", "立即升级"],
    ["See Plans", "查看套餐方案"],
    ["Purchase Credits", "购买点数"],
    ["Model Credits", "模型点数 (AI Credits)"],
    ["Enable AI Credit Overages", "启用 AI 点数超额替补"],
    ["Available AI Credits", "可用 AI 点数"],
    ["See Activity", "查看使用记录"],
    ["Get More AI Credits", "获取更多 AI 点数"],
    ["Model Quota", "模型配额"],
    ["No quota information available.", "暂无可用的额度配额信息。"],
    ["Refresh quota and credits data", "刷新配额与点数数据"],
    ["Gemini Models", "Gemini 模型群"],
    ["Claude and GPT models", "Claude 与 GPT 模型群"],
    ["Claude and GPT Models", "Claude 与 GPT 模型群"],
    ["Weekly Limit Remaining", "每周剩余额度"],
    ["Five Hour Limit Remaining", "5小时剩余额度"],
    ["Daily Limit Remaining", "每日剩余额度"],
    ["Monthly Limit Remaining", "每月剩余额度"],
    ["Limit Remaining", "剩余额度"],
    ["Remaining", "剩余"],
    ["Select Model", "选择模型"],
    ["Select another model", "选择其他模型"],
    ["No Model Selected", "未选择模型"],
    ["Select Model to Send Message", "请选择模型以发送消息"],
    ["Local execution", "本地执行"],
    ["Cloud execution", "云端执行"],
    ["Local", "本地执行"],
    ["Cloud", "云端执行"],
    ["View Usage", "查看额度使用"],
    ["Model", "模型选择"],
    ["Fast", "极速"],
    ["High", "强推理 (High)"],
    ["Medium", "标准 (Medium)"],
    ["Low", "轻量 (Low)"],
    ["Thinking", "深度思考"],

    // --- 设置导航总览 (Settings Sidebar) ---
    ["Account", "账户与计划"],
    ["General", "常规偏好"],
    ["Application", "客户端偏好"],
    ["Application Settings", "客户端设置"],
    ["Appearance", "外观主题"],
    ["Editor", "编辑器设置"],
    ["Editor Settings", "编辑器设置"],
    ["Tab", "Tab 智能补全"],
    ["Browser", "浏览器设置"],
    ["Browser Settings", "浏览器设置"],
    ["Notifications", "消息与通知"],
    ["Notification Preferences", "通知偏好设置"],
    ["Customizations", "扩展与技能"],
    ["App", "客户端偏好"],
    ["Shortcuts", "键盘快捷键"],
    ["Labs", "实验室特性"],
    ["CitC Settings", "CitC 代码库设置"],
    ["Best of N", "并行多选 (Best of N)"],
    ["Models", "模型与配额"],
    ["Developer", "开发者调试"],
    ["Jetski Chat", "Jetski 对话配置"],
    ["Regroup Google3 Chats", "重构对话分组"],
    ["Provide Feedback", "意见反馈"],
    ["Close Settings", "关闭设置"],
    ["Back", "返回"],
    ["Show all", "显示全部"],
    ["Not in Project", "未关联项目"],
    ["Conversations", "所有会话"],


    // --- 独立单词与补全 ---
    ["Project", "工程项目"],
    ["Workspace", "工作区"],
    ["Configure default behaviors, skills, and MCP servers.", "配置默认行为策略、技能库与 MCP 外部服务节点。"],
    ["Configure global allowed and denied resource permissions.", "配置全局允许与拒绝的资源访问权限。"],

    // --- 危险区域与项目删除 (Danger Zone & Deletion) ---
    ["Danger Zone", "危险区域"],
    ["Danger zone", "危险区域"],
    ["Delete Project", "删除工程项目"],
    ["Delete Workspace", "删除工作区"],
    ["Agent settings and permissions for conversations outside of projects.", "非项目会话的智能体配置与执行权限。"],
    ["Agent settings and permissions for conversations outside of workspaces.", "非工作区会话的智能体配置与执行权限。"],
    ["Manage project folders, agent settings, and permissions.", "管理项目目录、智能体配置与专属执行权限。"],
    ["Folders", "项目工作目录"],
    ["Add Folder", "添加目录"],
    ["Permission Settings", "权限策略配置"],
    ["Permission Preset", "权限预设模式"],
    ["Controls the actions the agent can take.", "控制智能体允许自主执行的操作范围。"],
    ["Turbo", "全自动极速 (Turbo)"],
    ["File Access Rules", "文件访问规则"],
    ["Configure allowed and denied paths for file reads and writes.", "配置允许或禁止读取与写入的文件路径。"],
    ["Network Access Rules", "网络访问规则"],
    ["Configure allowed and denied URLs for reading.", "配置允许或禁止读取的网络网址。"],
    ["Terminal Commands", "终端命令权限"],
    ["Configure allowed terminal commands.", "配置允许执行的终端命令白名单与黑名单。"],
    ["Commands Outside Sandbox", "沙箱外指令权限"],
    ["Configure allowed commands outside the sandbox.", "配置允许在沙箱隔离环境外执行的高权限指令。"],
    ["MCP Tools", "MCP 外部工具"],
    ["Configure external tools via Model Context Protocol.", "通过 Model Context Protocol 配置外部工具。"],
    ["Agent Behavior", "智能体行为偏好"],
    ["Artifact Review Policy", "交付产物人工审查策略"],
    ["Whether the agent asks you to review its documents.", "智能体在生成或修改文档产物时是否需要人工审核。"],
    ["Inherit Global", "继承全局设置"],
    ["Global Permissions", "全局权限规则"],
    ["Tool Permissions", "工具权限管理"],
    ["Modify permissions for file, terminal, and MCP tools.", "配置与修改文件系统、终端命令及 MCP 工具的执行权限。"],
    ["File Permissions", "文件访问权限"],
    ["Network Permissions", "网络请求权限"],
    ["GitHub Permissions", "GitHub 授权管理"],
    ["Manage fine-grained permissions for GitHub.", "管理访问 GitHub 代码仓库的细粒度授权策略。"],
    ["GitHub", "GitHub 访问策略"],
    ["Global", "全局生效"],
    ["Learn more", "了解更多"],
    ["Learn more.", "了解更多。"],
    ["Open", "查看与配置"],
    ["Edit", "编辑规则"],

    // --- 常规设置 (General Settings) ---
    ["Configure agent execution, queued message delivery, and permissions.", "配置智能体自主执行模式、消息队列与全局权限规则。"],
    ["Execution", "任务执行配置"],
    ["Queued Messages", "队列等待消息"],
    ["Configure when follow-up messages are sent.", "配置追加消息的发送时机。"],
    ["Queue", "排队执行"],
    ["Send Immediately", "立即发送"],
    ["Browser Javascript Execution Policy", "浏览器 JS 代码执行策略"],
    ["Controls whether the agent can run custom JavaScript to automate complex browser actions.", "控制智能体是否可以执行自定义 JavaScript 代码以驱动复杂的网页交互。"],
    ["Request Review", "每次请求审查"],
    ["Browser Actuation Rules", "浏览器操作规则"],
    ["Configure allowed and denied URLs for browser actuation.", "配置允许或禁止智能体进行交互点击操作的网页规则。"],
    ["Requires manual review for all terminal commands and file accesses outside of the working folders.", "对所有终端指令及工作目录外的文件访问均需人工审批。"],
    ["Agents run in a secure sandbox that restricts access to external resources outside of your trusted folders.", "智能体在受保护的安全沙箱中运行，限制对受信任目录之外外部资源的访问。"],
    ["Terminal commands always require review and the agent cannot access files outside of its given workspaces.", "终端指令始终需要人工审批，且智能体无法访问指定工作区之外的文件。"],
    ["Agent Non-Workspace File Access", "跨工作区文件访问权限"],
    ["Allows the agent to access files outside of your current workspace.", "允许智能体跨工程访问当前工作区目录之外的文件。"],
    ["Agent cannot modify files outside of the workspace in strict mode.", "严格模式下，智能体禁止修改当前工作区目录之外的文件。"],
    ["Outside of folders file access policy", "工作目录外文件访问策略"],
    ["Configures how the agent tries to access files outside of its working folders.", "配置智能体尝试访问工作目录以外文件时的行为策略。"],
    ["Confirm the command is safe to run outside of the sandbox with full network and disk access.", "请确认此命令在具有完整网络和磁盘权限的沙箱外环境中运行是安全的。"],
    ["Select one of the two options. Agent settings and permissions can be further customized below.", "请选择其中一种预设模式。智能体设置与细化权限可在下方进一步自定义。"],
    ["Select one of the three options. Agent settings and permissions can be further customized below.", "请选择其中一种预设模式。智能体设置与细化权限可在下方进一步自定义。"],

    // --- 应用偏好 (Application Settings) ---
    ["Manage Antigravity app settings.", "管理 Antigravity 客户端应用设置。"],
    ["Prevent Sleep", "防止系统休眠"],
    ["Prevent the computer from sleeping while the app is running.", "在 Antigravity 运行处理任务时阻止计算机进入休眠状态。"],
    ["Keep In Menu Bar", "常驻顶部菜单栏"],
    ["Keep the app accessible from the menu bar and running in the background when all windows are closed.", "关闭所有窗口后仍保持应用在后台运行，并可通过顶部菜单栏快速唤出。"],
    ["Remote Control", "远程控制与多端联动"],
    ["Enable Remote Control", "启用远程控制服务"],
    ["Work with local agents from another device.", "支持从手机、平板或其他设备随时远程连接并操作本地智能体。"],
    ["Notifications", "消息与通知"],
    ["Notification Settings", "系统通知权限设置"],
    ["To modify notification settings, open your operating system's system preferences.", "如需调整通知提示音与横幅，请前往操作系统的系统偏好设置中配置。"],
    ["Open System Preferences", "打开系统偏好设置"],
    ["Advanced Settings", "高级开发者设置"],
    ["Enable Telemetry", "发送匿名诊断与性能数据"],
    ["Marketing Emails", "接收产品更新与资讯邮件"],
    ["When toggled on, Antigravity collects usage data to help Google enhance performance and features.", "开启后，Antigravity 将收集匿名使用诊断数据，以帮助 Google 提升系统性能与体验。"],
    ["Receive product updates, tips, and promotions from Google Antigravity via email.", "通过电子邮件接收来自 Google Antigravity 的产品更新速递、使用技巧与官方资讯。"],
    ["Automatically prompt you to restart the app when a new update is available. When disabled, you can check for updates manually from the app menu.", "发现新版本时自动提示重启应用更新。关闭后可在菜单中手动检查更新。"],

    // --- 外观主题设置 (Appearance Settings) ---
    ["Configure the agent's visual theme and display preferences.", "配置智能体交互界面的主题样式与显示偏好。"],
    ["Chat Settings", "对话界面偏好"],
    ["Verbose Agent Chat", "展开详细思考过程"],
    ["Display and preserve intermediate thinking steps.", "显示并完整保留智能体的推理演进与中间步骤。"],
    ["Conversation Width", "对话区域显示宽度"],
    ["Configure the maximum width of the conversation panel.", "配置对话主面板的最大视觉宽度。"],
    ["Narrow", "居中窄屏 (Narrow)"],
    ["Default", "标准舒适 (Default)"],
    ["Wide", "宽屏通栏 (Wide)"],
    ["Theme", "外观主题"],
    ["Light Theme", "浅色模式"],
    ["Dark Theme", "深色模式"],
    ["Preset", "预设配色"],
    ["Default Light", "经典浅白"],
    ["Default Dark", "深邃炭黑"],
    ["Background", "背景颜色"],
    ["Foreground", "前景色/正文"],
    ["Accent", "强调色"],

    // --- 扩展、技能与 Token (Customizations) ---
    ["Configure default behaviors, skills, and MCP servers. Learn more.", "配置默认行为策略、技能库 (Skills) 与 MCP 服务节点。了解更多。"],
    ["Token Usage", "扩展上下文 Token 消耗"],
    ["Skills", "技能库 (Skills)"],
    ["Mcp Tools", "MCP 外部工具"],
    ["Rules", "规则库 (Rules)"],
    ["Plugins", "插件中心 (Plugins)"],
    ["MCP Servers", "MCP 节点"],
    ["Installed Skills", "已启用技能"],
    ["Installed MCP Servers", "已安装 MCP 服务"],
    ["Refresh MCP servers", "刷新 MCP 服务节点"],
    ["Refresh skills paths", "刷新技能库路径"],
    ["Manage Skills", "管理技能库"],
    ["Manage Hooks", "管理生命周期 Hooks"],
    ["Build With Google Plugins", "官方精选插件库"],
    ["Include default customizations, such as default skills.", "默认自动载入官方内置技能库 (Skills)。"],
    ["Browse and enable plugins from the Build With Google catalog.", "浏览并启用 Build With Google 官方插件市场的扩展插件。"],
    ["Configure hooks that run on agent lifecycle events.", "配置在智能体生命周期事件触发时自动执行的 Hooks 脚本。"],

    // --- 快捷键设置 (Shortcuts) ---
    ["Keyboard shortcuts for quick navigation and control.", "用于快速导航与交互操作的常用键盘快捷键列表。"],
    ["RECOMMENDED", "推荐快捷键"],
    ["NAVIGATION", "界面导航"],
    ["CONVERSATION", "对话交互"],
    ["LAYOUT CONTROLS", "布局与面板控制"],
    ["Toggle Model Selector", "切换模型选择菜单"],
    ["Toggle Voice Recording", "开启/关闭语音录入"],
    ["Add to Chat/Quote", "引用选中文本到对话"],
    ["Previous Pane Tab", "切换到上一个面板标签"],
    ["Next Pane Tab", "切换到下一个面板标签"],
    ["Open Settings", "打开系统设置"],
    ["Select Previous Conversation", "切换至上一个会话"],
    ["Select Next Conversation", "切换至下一个会话"],

    // --- 常用操作与上下文菜单 ---
    ["Proceed", "确认执行 (Proceed)"],
    ["Always Proceed", "始终自动执行"],
    ["Always Ask", "每次询问确认"],
    ["Approve", "批准执行"],
    ["Reject", "拒绝"],
    ["Cancel", "取消"],
    ["Confirm", "确认"],
    ["Save", "保存"],
    ["Delete", "删除"],
    ["Rename", "重命名"],
    ["Retry", "重试"],
    ["Copy", "复制"],
    ["Copied!", "已复制!"],
    ["Commit and Push", "提交并推送 (Git)"],
    ["Copy File Path", "复制文件绝对路径"],
    ["Copy File Name", "复制文件名"],
    ["Copy Path", "复制路径"],
    ["Copy Link", "复制分享链接"],
    ["Delete Conversation", "删除会话"],
    ["Rename Conversation", "重命名会话"],
    ["Archive this conversation", "归档此会话"],
    ["Pin this conversation", "置顶此会话"],
    ["Unpin this conversation", "取消置顶"],
    ["Mark as Read", "标记为已读"],
    ["Mark as Unread", "标记为未读"],
    ["Mark Read", "标记为已读"],
    ["Mark Unread", "标记为未读"],
    ["Split", "分屏查看"],
    ["Split Vertically", "垂直分屏"],
    ["Split Horizontally", "水平分屏"],
    ["More options", "更多选项"],
    ["Pin conversation", "置顶会话"],
    ["Archive conversation", "归档会话"],
    ["Stop execution", "停止执行"],
    ["Project options", "项目设置选项"],
    ["Undo changes up to this point", "撤销至此步的所有变更"],
    ["Mark all as read", "全部标记为已读"],
    ["Copy conversation markdown", "复制完整会话 Markdown"],
    ["Accept Step", "接受此步骤"],
    ["Reject Step", "拒绝此步骤"],
    ["Continue Response", "继续输出"],
    ["Add to Chat", "添加到对话"],
    ["Quote Selection", "引用选中内容"],
    ["Comment on Selection", "对选区添加批注"],
    ["Pinned Conversations", "置顶会话"],
    ["Move to Group", "移动至分组"],
    ["New Group", "新建分组"],
    ["Rename Group", "重命名分组"],
    ["Background Tasks", "后台任务"],
    ["Subagents", "子智能体 (Subagents)"],
    ["Documents", "交付文档"],
    ["Uploads", "上传文件"],
    ["Files", "工作区文件"],
    ["Recent Files", "最近打开文件"],
    ["Knowledge", "知识库资产"],

    // --- 状态与执行提示 ---
    ["Working..", "智能体处理中..."],
    ["Working...", "智能体处理中..."],
    ["Thinking...", "深度思考中..."],
    ["Generating...", "正在生成响应..."],
    ["Completed", "执行完成"],
    ["Failed", "执行失败"],
    ["Interrupted", "已中断"],
    ["Stopped", "已停止"],
    ["Filter", "筛选"],
    ["Search", "搜索"],
    ["Clear", "清除"],
    ["All", "全部"],
    ["Dark", "深色模式"],
    ["Light", "浅色模式"],
    ["System", "跟随系统"]
  ]);

  // 2. 动态正则匹配规则 (处理带变量、数字、时间的文本)
  // 注意：使用 RegExp 构造函数以避免转义歧义
  const regexRules = [
    // 输入框占位符
    [new RegExp("^Ask anything, @ to mention, / for actions$", "i"), "输入任何问题，输入 @ 引用，输入 / 触发动作..."],
    [new RegExp("^Ask anything, @ to mention$", "i"), "输入任何问题，输入 @ 引用文件..."],

    // 套餐与计划
    [new RegExp("^Your Plan:\\s*(.+)$", "i"), "当前套餐：$1"],
    [new RegExp("^You can upgrade to a Google AI Ultra plan to receive higher rate limits\\.?$", "i"), "您可以升级至 Google AI Ultra 套餐以获取更高的速率限制与并发额度。"],
    [new RegExp("^When toggled on,\\s*(.+?)\\s*will use your AI credits to fulfill model requests once you're out of model quota\\.\\s*(.+?)\\s*will always use your model quota first before using AI credits\\.?$", "i"), "开启后，当模型额度耗尽时，系统将使用 AI 点数继续响应模型请求。系统始终会优先消耗免费额度，之后再使用 AI 点数。"],
    [new RegExp("^Available AI Credits:\\s*(.+)$", "i"), "可用 AI 点数：$1"],

    // 配额刷新时间
    [new RegExp("^You have used some of your weekly limit,\\s*it will fully refresh in (\\d+)\\s*days?,\\s*(\\d+)\\s*hours?\\.?$", "i"), "您已消耗部分每周额度，将在 $1 天 $2 小时后完全刷新。"],
    [new RegExp("^You have used some of your weekly limit,\\s*it will fully refresh in (\\d+)\\s*days?\\.?$", "i"), "您已消耗部分每周额度，将在 $1 天后完全刷新。"],
    [new RegExp("^You have used some of your weekly limit,\\s*it will fully refresh in (\\d+)\\s*hours?,\\s*(\\d+)\\s*minutes?\\.?$", "i"), "您已消耗部分每周额度，将在 $1 小时 $2 分钟后完全刷新。"],
    [new RegExp("^You have used some of your weekly limit,\\s*it will fully refresh in (\\d+)\\s*hours?\\.?$", "i"), "您已消耗部分每周额度，将在 $1 小时后完全刷新。"],
    [new RegExp("^You have used some of your weekly limit,\\s*it will fully refresh in (.+)$", "i"), "您已消耗部分每周额度，将在 $1 后完全刷新。"],
    [new RegExp("^You have used some of your 5-hour limit,\\s*it will fully refresh in (\\d+)\\s*hours?,\\s*(\\d+)\\s*minutes?\\.?$", "i"), "您已消耗部分 5 小时额度，将在 $1 小时 $2 分钟后完全刷新。"],
    [new RegExp("^You have used some of your 5-hour limit,\\s*it will fully refresh in (\\d+)\\s*hours?\\.?$", "i"), "您已消耗部分 5 小时额度，将在 $1 小时后完全刷新。"],
    [new RegExp("^You have used some of your 5-hour limit,\\s*it will fully refresh in (\\d+)\\s*minutes?\\.?$", "i"), "您已消耗部分 5 小时额度，将在 $1 分钟后完全刷新。"],
    [new RegExp("^You have used some of your 5-hour limit,\\s*it will fully refresh in (.+)$", "i"), "您已消耗部分 5 小时额度，将在 $1 后完全刷新。"],
    [new RegExp("^You have used some of your (\\d+)-hour limit,\\s*it will fully refresh in (.+)$", "i"), "您已消耗部分 $1 小时额度，将在 $2 后完全刷新。"],
    [new RegExp("^it will fully refresh in (.+)$", "i"), "将在 $1 后完全刷新。"],

    // Token 预算与项目设置
    [new RegExp("^([\\d.]+)%\\s*of the customization budget is available\\.?$", "i"), "可用个性化扩展预算仍有 $1%。"],
    [new RegExp("of the customization budget is available", "i"), "的扩展预算仍可用"],
    [new RegExp("^Show (\\d+) breakdowns?$", "i"), "展开 $1 项明细"],
    [new RegExp("^\\(([\\d,]+) tokens\\)\\s*([\\d.]+)%$", "i"), "($1 Tokens) $2%"],


    [new RegExp("^Permanently delete (.+?) including (\\d+) active conversations?\\.?$", "i"), "永久删除 $1（包含 $2 个活动会话）。"],
    [new RegExp("^Permanently delete (.+?)\\s*\\.?$", "i"), "永久删除 $1。"],

    // 项目管理与删除
    [new RegExp("^Agent settings and permissions for conversations outside of (projects|workspaces)\\.?$", "i"), "非$1会话的智能体配置与执行权限。"],
    [new RegExp("^Manage (project|workspace) folders, agent settings, and permissions\\.?$", "i"), "管理$1目录、智能体配置与专属执行权限。"],
    [new RegExp("^Delete (Project|Workspace)$", "i"), "删除$1"],
    [new RegExp("^This (project|workspace) is managed by the (.+?) automation and can only be deleted by deleting that automation\\.?$", "i"), "此$1由 $2 自动化任务管理，只能通过删除该自动化任务来删除。"],
    [new RegExp("^A (project|workspace) with this name already exists\\.?$", "i"), "已存在同名的$1。"],

    // 智能体执行与步数
    [new RegExp("^Ran (\\d+) commands?$", "i"), "已执行 $1 条指令"],
    [new RegExp("^Running (\\d+) commands?$", "i"), "正在运行 $1 条指令..."],
    [new RegExp("^Explored (\\d+) files?, (\\d+) folders?$", "i"), "已探索 $1 个文件，$2 个目录"],
    [new RegExp("^Explored (\\d+) files?, (\\d+) search(?:es)?$", "i"), "已探索 $1 个文件，$2 次检索"],
    [new RegExp("^Explored (\\d+) files?$", "i"), "已探索 $1 个文件"],
    [new RegExp("^Explored (\\d+) folders?$", "i"), "已探索 $1 个目录"],
    [new RegExp("^Thought for (\\d+)s?$", "i"), "深度思考 $1 秒"],
    [new RegExp("^Thought for (\\d+)m (\\d+)s?$", "i"), "深度思考 $1 分 $2 秒"],
    [new RegExp("^(\\d+) steps?$", "i"), "$1 个步骤"],
    [new RegExp("^(\\d+) conversations?$", "i"), "$1 个会话"],
    [new RegExp("^(\\d+)m ago$", "i"), "$1 分钟前"],
    [new RegExp("^(\\d+)h ago$", "i"), "$1 小时前"],
    [new RegExp("^(\\d+)d ago$", "i"), "$1 天前"],
    [new RegExp("^just now$", "i"), "刚刚"],

    // 设置描述长文本动态支持
    [new RegExp("^Also includes Global Permissions when working in this project\\. Learn more\\.?$", "i"), "在当前项目中工作时同时继承全局权限。了解更多。"],
    [new RegExp("^The breakdown below shows token usage from customizations like skills, rules, and MCP\\. If the budget is exceeded, large customizations will be truncated automatically\\.?$", "i"), "下方明细展示了技能 (Skills)、规则 (Rules) 及 MCP 等扩展占用的上下文 Token 额度。若超出上限，体积较大的扩展将被自动截断。"],
    [new RegExp("^Configure default behaviors, skills, and MCP servers\\. Learn more\\.?$", "i"), "配置默认行为策略、技能库 (Skills) 与 MCP 服务节点。了解更多。"],
    [new RegExp("^Configure global allowed and denied resource permissions\\. Learn more\\.?$", "i"), "配置全局允许与拒绝的资源访问权限。了解更多。"],
    [new RegExp("^Browser settings have moved to the Browser section of General settings\\. Go to General settings$", "i"), "浏览器设置已整合至常规偏好设置中的“浏览器”专区。前往常规设置"]
  ];

  // 3. 安全检测：判断是否为不可汉化的代码块或数据区域
  const IGNORE_TAGS = new Set(["SCRIPT", "STYLE", "CODE", "PRE", "NOSCRIPT", "SVG", "PATH"]);

  function shouldIgnoreElement(el) {
    if (!el || !el.tagName) return true;
    if (IGNORE_TAGS.has(el.tagName)) return true;

    // 排除编辑器、终端、代码高亮容器
    const className = typeof el.className === "string" ? el.className : "";
    if (
      className.includes("monaco-editor") ||
      className.includes("syntax-highlight") ||
      className.includes("xterm") ||
      className.includes("terminal") ||
      className.includes("code-block") ||
      className.includes("language-")
    ) {
      return true;
    }

    // 排除可编辑区本身的内容文本（但允许汉化 placeholder）
    if (el.isContentEditable) return true;

    return false;
  }

  // 4. 单一文本翻译
  function translateString(str) {
    if (!str || typeof str !== "string") return str;
    const trimmed = str.trim();
    if (!trimmed) return str;

    // 优先静态查表
    if (exactDict.has(trimmed)) {
      const translated = exactDict.get(trimmed);
      return str.replace(trimmed, translated);
    }

    // 正则动态查表
    for (let i = 0; i < regexRules.length; i++) {
      const [re, repl] = regexRules[i];
      if (re.test(trimmed)) {
        const translated = trimmed.replace(re, repl);
        return str.replace(trimmed, translated);
      }
    }

    return str;
  }

  // 5. 遍历并翻译节点
  function translateNode(node) {
    if (!node) return;

    if (node.nodeType === Node.TEXT_NODE) {
      const parent = node.parentElement;
      if (parent && shouldIgnoreElement(parent)) return;

      const original = node.nodeValue;
      if (!original || !original.trim()) return;

      // 如果当前文本与上次翻译的一致，无需重复处理
      if (node._agy_orig === original && node._agy_res === node.nodeValue) {
        return;
      }

      const translated = translateString(original);
      if (translated !== original) {
        node._agy_orig = original;
        node._agy_res = translated;
        node.nodeValue = translated;
      }
      return;
    }

    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node;
      if (shouldIgnoreElement(el)) return;

      // 翻译 input / textarea 的 placeholder
      if (el.placeholder) {
        const trPlaceholder = translateString(el.placeholder);
        if (trPlaceholder !== el.placeholder) {
          el.placeholder = trPlaceholder;
        }
      }

      // 翻译 aria-label 或 title 提示
      if (el.getAttribute("aria-label")) {
        const label = el.getAttribute("aria-label");
        const trLabel = translateString(label);
        if (trLabel !== label) {
          el.setAttribute("aria-label", trLabel);
        }
      }
      if (el.title) {
        const trTitle = translateString(el.title);
        if (trTitle !== el.title) {
          el.title = trTitle;
        }
      }

      // 递归子节点
      for (let child = el.firstChild; child; child = child.nextSibling) {
        translateNode(child);
      }
    }
  }

  // 6. 初始扫描与全量 DOM 监听
  function runLocalization() {
    if (document.body) {
      translateNode(document.body);
    }
  }

  // 监听动态 DOM 变动
  let timer = null;
  const observer = new MutationObserver((mutations) => {
    if (timer) return;
    timer = requestAnimationFrame(() => {
      timer = null;
      for (let i = 0; i < mutations.length; i++) {
        const m = mutations[i];
        if (m.type === "childList") {
          for (let j = 0; j < m.addedNodes.length; j++) {
            translateNode(m.addedNodes[j]);
          }
        } else if (m.type === "characterData") {
          translateNode(m.target);
        }
      }
    });
  });

  // 启动观察器
  function init() {
    runLocalization();
    if (document.body) {
      observer.observe(document.body, {
        childList: true,
        subtree: true,
        characterData: true
      });
    } else {
      document.addEventListener("DOMContentLoaded", () => {
        runLocalization();
        observer.observe(document.body, {
          childList: true,
          subtree: true,
          characterData: true
        });
      });
    }
    // 周期扫描兜底（处理某些 React 异步重渲染）
    setInterval(runLocalization, 800);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
