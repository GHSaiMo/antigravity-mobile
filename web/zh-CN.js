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

    // --- 项目选择下拉面板与操作 (Project Selector & Dropdown) ---
    ["Search Projects", "搜索工程项目"],
    ["Search Recent Workspaces", "搜索近期工作区"],
    ["Search projects...", "搜索工程项目..."],
    ["Search workspaces...", "搜索工作区..."],
    ["New Project", "新建工程项目"],
    ["Quick Start", "快速开始"],
    ["No Project", "未关联项目"],
    ["Project Settings", "工程项目设置"],
    ["Workspace Settings", "工作区设置"],
    ["Open Project Picker", "打开项目选择器"],
    ["Open Workspace Selector", "打开工作区选择器"],
    ["Select Project", "选择工程项目"],
    ["Select project", "选择工程项目"],
    ["Select CitC Workspace", "选择 CitC 工作区"],
    ["Select CitC workspace", "选择 CitC 工作区"],
    ["Select Cog Workspace", "选择 Cog 工作区"],
    ["Select a folder to create a new project.", "选择一个本地文件夹以创建新项目。"],
    ["Select a folder.", "选择一个本地文件夹。"],
    ["Instantly create a new project and folder to start building.", "立即创建新项目与专属目录并开始构建。"],
    ["Create a new project using normal folders and/or citc workspaces.", "使用常规文件夹或 CitC 工作区创建新项目。"],
    ["Work outside of any project.", "在未关联任何项目的状态下工作。"],
    ["Work in a CitC workspace.", "在 CitC 工作区中工作。"],
    ["Work in a Cog workspace.", "在 Cog 工作区中工作。"],
    ["Work in an ABFS workspace.", "在 ABFS 工作区中工作。"],
    ["Open project settings", "打开项目设置"],
    ["Open workspace settings", "打开工作区设置"],
    ["Create New Project", "创建新工程项目"],
    ["Create Project", "创建工程项目"],
    ["Create a Project", "创建工程项目"],
    ["New Workspace", "新建工作区"],
    ["Add Workspace", "添加工作区"],
    ["Workspace Actions", "工作区操作"],
    ["Environment Actions", "环境操作"],
    ["Copy workspace", "复制工作区"],
    ["Copy project", "复制工程项目"],
    ["Archive Workspace", "归档工作区"],
    ["Archive Environment", "归档环境"],
    ["Archive workspace", "归档工作区"],
    ["Archive project", "归档工程项目"],
    ["Display Options", "显示选项"],
    ["No matching projects", "未找到匹配的项目"],
    ["No matching workspaces", "未找到匹配的工作区"],
    ["No projects found", "未找到工程项目"],
    ["No workspaces found", "未找到工作区"],
    ["No matching items", "未找到匹配项"],
    ["No matching results", "未找到匹配结果"],
    ["No matching customizations found.", "未找到匹配的个性化扩展。"],
    ["No matching flags", "未找到匹配的标志"],
    ["Google3 projects are being deprecated. Select a CitC workspace instead.", "Google3 项目已被废弃，请选择 CitC 工作区。"],
    ["Google3 projects are deprecated. Learn more", "Google3 项目已被废弃。了解更多"],
    ["Change VCS in General settings, under Advanced", "可在“常规偏好 - 高级”中修改版本控制系统 (VCS)"],

    // --- 全局通用按钮、操作与工具提示 (Buttons & Tooltips) ---
    ["More Actions", "更多操作"],
    ["More actions", "更多操作"],
    ["More options", "更多选项"],
    ["Group Actions", "分组操作"],
    ["Selection Actions", "选中项操作"],
    ["Stop All Subagents", "停止所有子智能体"],
    ["Stop Subagent", "停止子智能体"],
    ["Stop Task", "停止任务"],
    ["Cancel All Tasks", "取消所有任务"],
    ["Cancel step", "取消步骤"],
    ["Cancel Task", "取消任务"],
    ["Send Now", "立即发送"],
    ["Autonomous Mode", "全自主模式"],
    ["Autonomous mode", "全自主模式"],
    ["Add Context", "添加上下文"],
    ["Add Model", "添加模型"],
    ["Add Terminal", "新建终端"],
    ["Clear Search", "清空搜索"],
    ["Clear search", "清空搜索"],
    ["Clear filter", "清空筛选"],
    ["Good response", "回答准确"],
    ["Bad response", "回答欠佳"],
    ["Insert in terminal", "插入终端"],
    ["Open Diff", "查看对比差异"],
    ["Remove From Split", "移出分屏"],
    ["Your quota for this model is running low.", "此模型的可用配额即将耗尽。"],
    ["Click to open docs", "点击打开官方文档"],
    ["Open settings menu", "打开设置菜单"],
    ["Dismiss Tip", "不再提示"],
    ["Dismiss announcement", "关闭公告"],
    ["Dismiss error", "忽略错误"],
    ["Dismiss notification", "忽略通知"],
    ["Dismiss toast", "关闭提示"],
    ["Select an option", "请选择一项"],
    ["Select Environment", "选择运行环境"],
    ["Select Default Branch", "选择默认分支"],
    ["Select Worktree", "选择工作树"],
    ["Select License", "选择许可证"],
    ["Select Theme", "选择主题"],
    ["Next question", "下一题"],
    ["Previous question", "上一题"],
    ["Next match", "下一个匹配项"],
    ["Previous match", "上一个匹配项"],
    ["Next match (Enter)", "下一个匹配项 (Enter)"],
    ["Previous match (Shift+Enter)", "上一个匹配项 (Shift+Enter)"],
    ["Next Page", "下一页"],
    ["Previous Page", "上一页"],
    ["Current Page", "当前页"],
    ["File Explorer", "文件浏览器"],
    ["File path breadcrumbs", "文件路径导航"],
    ["Fork Conversation", "分叉新会话"],
    ["Go Back", "返回上一页"],
    ["Go Forward", "前进到下一页"],
    ["Match case", "区分大小写"],
    ["Match whole word", "全字匹配"],
    ["Use regular expression", "使用正则表达式"],
    ["Stage change", "暂存改动"],
    ["Unstage change", "取消暂存"],
    ["Discard unstaged changes", "放弃未暂存的改动"],
    ["Staged Changes", "已暂存的改动"],
    ["Untracked (Unstaged)", "未跟踪（未暂存）"],
    ["Delete conversation", "删除会话"],
    ["Delete Conversation", "删除会话"],
    ["Delete Task", "删除任务"],
    ["Delete Terminal", "删除终端"],
    ["Delete Skill", "删除技能"],
    ["Delete MCP Server", "删除 MCP 服务"],
    ["This skill is installed in your workspace", "此技能已安装在您的工作区"],
    ["Search conversations...", "搜索历史会话..."],
    ["Search conversations (by name or Cascade ID)", "搜索会话（通过名称或 ID）"],
    ["Search customizations...", "搜索扩展与技能..."],
    ["Search flags", "搜索配置项"],
    ["Search for commands...", "搜索命令..."],
    ["Search for conversations...", "搜索会话..."],
    ["Search metrics...", "搜索指标..."],
    ["Search steps...", "搜索执行步骤..."],
    ["Search MCP servers by name", "按名称搜索 MCP 服务"],
    ["Search all skills on Agent Market…", "在智能体市场搜索所有技能…"],
    ["Search skills…", "搜索技能…"],
    ["Search across files...", "在文件中全局检索..."],
    ["Search GoB repositories...", "搜索代码仓库..."],
    ["Select category to search...", "选择检索分类..."],
    ["Enter directory path...", "输入目录路径..."],
    ["Enter file or directory path...", "输入文件或目录路径..."],
    ["Enter project name...", "输入工程项目名称..."],
    ["Enter workspace name...", "输入工作区名称..."],
    ["Enter tool name or server...", "输入工具名称或服务节点..."],
    ["Prompt to execute on schedule...", "定时自动执行的提示词指令..."],
    ["Enter command (e.g., git, blaze)...", "输入命令（如 git, blaze）..."],
    ["Type absolute path or navigate folders...", "输入绝对路径或浏览文件夹..."],
    ["Write a comment...", "添加批注评论..."],
    ["(Optional) Tell us more...", "（可选）提供更多反馈详情..."],
    ["(Optional) Tell us more or type your reason...", "（可选）提供更多详情或说明原因..."],
    ["Copy Content", "复制内容"],
    ["Copy Path", "复制路径"],
    ["Copy Command", "复制命令"],
    ["Copy code", "复制代码"],
    ["Copied!", "已复制！"],
    ["Copied", "已复制"],
    ["Double-click to reset panel sizes", "双击重置面板尺寸"],
    ["Drag to resize, double-click to reset", "拖拽调整尺寸，双击重置"],
    ["Fit the whole trajectory", "完整适配轨迹视图"],
    ["Open side-by-side view", "打开并排分屏视图"],
    ["Side-by-side layout", "并排布局"],
    ["Stacked layout", "堆叠布局"],
    ["Layout Controls", "布局控制"],
    ["Mark all as read", "全部标记为已读"],
    ["Experimental model. Click to provide feedback or opt out.", "实验性模型。点击可提供反馈或退出。"],
    ["Agent can scroll on browser pages to access more content.", "智能体可在浏览器页面上滚动以访问更多内容。"],
    ["Working directory: ", "工作目录："],
    ["Describe a plugin and the agent builds it", "描述插件需求，智能体自动为您构建"],
    ["Dev mode: Localhost server automatically detected", "开发模式：已自动检测到本地服务器"],

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
    ["Antigravity", "Multigravity"],
    ["Google Antigravity", "Multigravity"],
    ["Manage Antigravity app settings.", "管理 Multigravity 客户端应用设置。"],
    ["Prevent Sleep", "防止系统休眠"],
    ["Prevent the computer from sleeping while the app is running.", "在 Multigravity 运行处理任务时阻止计算机进入休眠状态。"],
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
    ["When toggled on, Antigravity collects usage data to help Google enhance performance and features.", "开启后，Multigravity 将收集匿名使用诊断数据，以帮助提升系统性能与体验。"],
    ["When toggled on, Multigravity collects usage data to help Google enhance performance and features.", "开启后，Multigravity 将收集匿名使用诊断数据，以帮助提升系统性能与体验。"],
    ["When toggled on, Google Multigravity collects usage data to help Google enhance performance and features.", "开启后，Multigravity 将收集匿名使用诊断数据，以帮助提升系统性能与体验。"],
    ["Receive product updates, tips, and promotions from Google Antigravity via email.", "通过电子邮件接收来自 Multigravity 的产品更新速递、使用技巧与官方资讯。"],
    ["Receive product updates, tips, and promotions from Google Multigravity via email.", "通过电子邮件接收来自 Multigravity 的产品更新速递、使用技巧与官方资讯。"],
    ["Receive product updates, tips, and promotions from Multigravity via email.", "通过电子邮件接收来自 Multigravity 的产品更新速递、使用技巧与官方资讯。"],
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
    ["Loading token usage...", "正在加载 Token 消耗数据..."],
    ["Loading token usage.", "正在加载 Token 消耗数据。"],
    ["Loading token usage", "正在加载 Token 消耗数据"],
    ["There are no customizations enabled", "当前未启用任何个性化扩展。"],
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
    ["Delete conversation", "删除会话"],
    ["Rename Conversation", "重命名会话"],
    ["Rename conversation", "重命名会话"],
    ["Archive this conversation", "归档此会话"],
    ["Archive This Conversation", "归档此会话"],
    ["Pin this conversation", "置顶此会话"],
    ["Pin This Conversation", "置顶此会话"],
    ["Unpin this conversation", "取消置顶"],
    ["Unpin This Conversation", "取消置顶此会话"],
    ["Pin", "置顶"],
    ["Unpin", "取消置顶"],
    ["Archive", "归档"],
    ["Unarchive", "取消归档"],
    ["Archive / Restore", "归档 / 恢复"],
    ["Restore", "恢复"],
    ["Delete Permanently", "永久删除"],
    ["Mark as Read", "标记为已读"],
    ["Mark as Unread", "标记为未读"],
    ["Mark Read", "标记为已读"],
    ["Mark Unread", "标记为未读"],
    ["Split", "分屏查看"],
    ["Split Vertically", "垂直分屏"],
    ["Split Horizontally", "水平分屏"],
    ["Split Right", "分屏到右侧"],
    ["Split Down", "分屏到下方"],
    ["Replace With New", "替换为新建"],
    ["Remove From Split", "移出分屏"],
    ["View Debug", "查看调试信息"],
    ["Fork", "分叉会话"],
    ["Share", "分享"],
    ["Telemetry", "诊断遥测"],
    ["Conversation Name", "会话名称"],
    ["Conversation ID", "会话 ID"],
    ["Worktree Name", "工作树名称"],
    ["Workspace Name", "工作区名称"],
    ["Project Name", "工程项目名称"],
    ["More options", "更多选项"],
    ["Pin conversation", "置顶会话"],
    ["Archive conversation", "归档会话"],
    ["Stop execution", "停止执行"],
    ["Permanently delete", "永久删除"],
    ["including", "包含"],
    ["This will permanently delete", "这将永久删除"],
    ["within it.", "及其内部所有数据。"],
    ["within it", "及其内部所有数据"],
    ["Are you sure you want to delete the", "您确定要删除此"],
    ["Are you sure you want to delete the project", "您确定要删除此工程项目"],
    ["Are you sure you want to delete the workspace", "您确定要删除此工作区"],
    ["Delete project", "删除工程项目"],
    ["Delete workspace", "删除工作区"],
    ["Working", "正在执行..."],
    ["Done", "已完成"],
    ["Edited files", "已编辑文件"],
    ["Editing files", "正在编辑文件"],
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
    ["System", "跟随系统"],

    // --- 状态与空数据提示 (Empty States & Status) ---
    ["No subagents", "暂无子智能体"],
    ["No active terminals", "暂无活动终端"],
    ["No active terminals. Click + to create one.", "暂无活动终端。点击 + 创建新终端。"],
    ["Creating terminal...", "正在创建终端..."],
    ["No documents", "暂无交付文档"],
    ["No uploads", "暂无上传文件"],
    ["No artifacts generated", "未生成任何工件"],
    ["No browser pages open", "未打开任何浏览器页面"],
    ["No changes to review", "没有需要审核的变更"],
    ["No changes to amend", "没有可追加的变更"],
    ["No commit to amend", "没有可追加的提交"],
    ["No commits to push", "没有待推送的提交"],
    ["No files or folders found", "未找到任何文件或文件夹"],
    ["No groups yet", "暂无分组"],
    ["No skills or rules yet.", "暂无技能或规则。"],
    ["No skills or rules match this filter.", "没有匹配此筛选条件的技能或规则。"],
    ["No MCP servers yet. Use Add to browse the store.", "暂无 MCP 服务节点。点击添加前往浏览。"],
    ["No MCP servers installed", "未安装任何 MCP 服务节点"],
    ["No custom agents yet.", "暂无自定义智能体。"],
    ["No Custom Agents or Plugins", "暂无自定义智能体或插件"],
    ["No plugins available.", "暂无可用插件。"],
    ["No matching plugins found.", "未找到匹配的插件。"],
    ["No plugins match your search.", "没有匹配搜索的插件。"],
    ["No plugins available in the marketplace.", "插件市场暂无可用插件。"],
    ["No folders added yet.", "暂未添加任何文件夹。"],
    ["No environments found", "未找到运行环境"],
    ["No models available", "暂无可用的模型"],
    ["No Models Available", "暂无可用的模型"],
    ["No projects created", "暂未创建工程项目"],
    ["No description available.", "暂无可用描述。"],
    ["No description available", "暂无可用描述"],
    ["No data available", "暂无可用数据"],
    ["No logs available.", "暂无可用日志。"],
    ["No events recorded", "暂无记录事件"],
    ["No events recorded.", "暂无记录事件。"],
    ["No more older messages", "没有更早的历史消息了"],

    // --- 展开与收起控制 (Show / Hide Toggles) ---
    ["Show Less", "收起"],
    ["Show less", "收起"],
    ["Show All", "显示全部"],
    ["Show More", "展开更多"],
    ["Show more", "展开更多"],
    ["Show details", "显示详细信息"],
    ["Show token", "显示 Token"],
    ["Show Error", "显示错误信息"],
    ["Show JavaScript Result", "显示 JavaScript 运行结果"],
    ["Show Whitespace Changes", "显示空白字符变动"],
    ["Show fewer resources", "收起资源"],
    ["Show all resources", "显示全部资源"],
    ["Show in File Explorer", "在文件浏览器中显示"],
    ["Show in File Manager", "在系统文件管理器中显示"],
    ["Show Thumbnail Sidebar", "显示缩略图侧边栏"],
    ["Hide Error", "隐藏错误"],
    ["Hide breakdown", "收起明细"],
    ["Hide tools", "隐藏工具调用"],
    ["Hide Whitespace Changes", "隐藏空白字符变动"],
    ["Hide token", "隐藏 Token"],
    ["Hide Thumbnail Sidebar", "隐藏缩略图侧边栏"],

    // --- 刷新状态 (Refresh & Refreshing) ---
    ["Refreshing...", "正在刷新..."],
    ["Refreshing", "正在刷新..."],
    ["refreshing", "正在刷新..."],
    ["Refreshing Browser page", "正在刷新浏览器页面"],
    ["Refreshed Browser page", "已刷新浏览器页面"],
    ["Refresh skills", "刷新技能库"],
    ["Refresh Skills", "刷新技能库"],
    ["Refresh custom agents", "刷新自定义智能体"],
    ["Refresh gcert Credentials", "刷新 gcert 凭据"],

    // --- 新版常用操作与菜单按钮 ---
    ["Add context", "添加上下文"],
    ["Add Context", "添加上下文"],
    ["Send message", "发送消息"],
    ["Send Message", "发送消息"],
    ["Select Agent", "选择智能体"],
    ["Select agent", "选择智能体"],
    ["Main Agent", "主智能体"],
    ["Main agent", "主智能体"],
    ["Message input", "消息输入"],
    ["Media", "媒体文件"],
    ["Mentions", "@提及引用"],
    ["Actions", "/快捷指令"],
    ["Loading models...", "正在加载模型列表..."],
    ["Refresh", "刷新"],
    ["Agents", "智能体群"],
    ["Getting scripts...", "正在获取脚本..."],
    ["Click to Open Docs", "点击打开官方文档"],
    ["Click to open docs", "点击打开官方文档"],
    ["Terminal", "集成终端"],
    ["Group By", "分组方式"],
    ["Group by", "分组方式"],
    ["Sort Conversations", "会话排序"],
    ["Sort conversations", "会话排序"],
    ["Last Updated", "最近更新时间"],
    ["Last Prompt", "最后提问时间"],
    ["Alphabetical (A-Z)", "字母顺序 (A-Z)"],
    ["Date Added", "创建添加时间"],
    ["View", "视图与展示"],
    ["Archived Only", "仅显示归档会话"],
    ["Display", "界面显示"],
    ["Subtitles", "副标题与信息展示"],
    ["Worktree", "工作树路径 (Worktree)"],
    ["No Project", "未关联项目"],
    ["New Conversation in Project", "在项目中新建会话"],
    ["Conversation History", "历史会话"],
    ["Create New Project", "创建新工程项目"],
    ["Type", "输入"],
    ["and select", "并选择"],
    ["to have the agent generate a plan.", "即可让智能体生成计划。"],
    ["to have the agent generate a plan", "即可让智能体生成计划"],
    ["Configure the browser subagent. It requires", "配置浏览器智能体，该功能需要已安装"],
    ["to be installed.", "。"],
    ["to be installed", "。"],
    ["Close", "关闭"],
    ["Queue until after the current turn.", "在当前轮次结束后排队发送。"],
    ["Interrupt the agent and send immediately.", "打断智能体并立即发送。"],
    ["Keyboard shortcuts", "键盘快捷键"],
    ["Security Preset", "安全预设策略"],
    ["Turbo Mode", "极速模式 (Turbo)"],
    ["Turbo mode", "极速模式 (Turbo)"],
    ["Learn more about", "了解更多关于"],
    ["Plan Review Policy", "计划审核策略"],
    ["Always Ask", "每次询问确认"],
    ["Inherit Global", "继承全局设置"],
    ["Terminal & Tooling Permissions", "终端与工具权限"],
    ["Request Review", "每次请求审查"],
    ["Automatic Check for Updates", "自动检查版本更新"],
    ["Contrast", "对比度"],
    ["Strong", "高对比 (Strong)"],
    ["Default Light", "经典浅白"],
    ["Default Dark", "深邃炭黑"],
    ["Context Menus", "右键上下文菜单"],
    ["Custom Context Menus", "自定义右键快捷菜单"],
    ["Replace the default browser right-click menu with quick actions.", "使用智能体快捷操作替换浏览器默认的右键菜单。"],
    ["No token data available.", "暂无 Token 消耗数据。"],
    ["Add MCP", "添加 MCP 服务"],
    ["Add MCP Servers", "添加 MCP 服务"],
    ["Loading MCP servers...", "正在加载 MCP 服务..."],
    ["Customize", "自定义配置"],
    ["Agent Settings", "智能体设置"],
    ["Local Permissions", "项目本地权限"],
    ["Loading workspace customizations...", "正在加载工作区扩展配置..."],
    ["RECOMMENDED", "推荐常用快捷键"],
    ["NAVIGATION", "页面与窗口导航"],
    ["CONVERSATION", "对话与交互操作"],
    ["LAYOUT CONTROLS", "布局与面板控制"],
    ["Manage your plan, credentials, and general preferences.", "管理您的计划订阅、身份凭据与通用偏好设置。"],
    ["Manage Multigravity app settings.", "管理 Multigravity 客户端设置与偏好。"],
    ["Not Signed In", "未登录账户"],
    ["Sign In", "立即登录"],
    ["By using this app, you agree to its", "使用此应用即表示您同意其"],
    ["Terms of Service", "服务条款"],
    ["Mark as read", "标记为已读"],
    ["Mark as Read", "标记为已读"],
    ["Status", "按状态分组"],
    ["None", "不分组"],
    ["Allow Once", "仅允许一次"],
    ["Always Allow", "始终允许"],
    ["Allow", "允许"],
    ["Deny", "拒绝"],
    ["Run in Terminal", "在终端中运行"],
    ["Approve Command", "批准执行命令"],
    ["Approve File Edit", "批准修改文件"],
    ["Keep Changes", "保留改动"],
    ["Discard Changes", "放弃改动"],
    ["Show Diff", "显示差异对比"],
    ["Hide Diff", "隐藏差异对比"],
    ["Inline Diff", "行内差异"],
    ["Side-by-Side Diff", "并排差异"],
    ["Thinking Process", "深度思考过程"],
    ["Hide Thinking", "收起思考过程"],
    ["Show Thinking", "展开思考过程"],
    ["Thought for", "深度思考耗时"],
    ["Interactive Folder Picker", "文件夹浏览选择器"],
    ["Open Folder", "打开文件夹"],
    ["Select Folder", "选择文件夹"],
    ["New Folder", "新建文件夹"],
    ["Folder Name", "文件夹名称"],
    ["Create Folder", "创建文件夹"],

    // --- 意见与问题反馈 (Feedback Modal) ---
    ["Feedback Type", "反馈类型"],
    ["Bug Report", "问题缺陷报告 (Bug Report)"],
    ["Feature Request", "功能建议需求 (Feature Request)"],
    ["Auth and Billing", "身份认证与账单 (Auth and Billing)"],
    ["Remote Control Issue", "远程控制连接问题 (Remote Control Issue)"],
    ["General Feedback", "常规体验反馈 (General Feedback)"],
    ["Description", "问题描述"],
    ["Please describe the issue in detail. The more actionable your feedback, the quicker our team can address your request. Some helpful information includes:", "请详细描述您遇到的问题。反馈信息越具体，我们就能越快处理您的请求。有帮助的信息包括："],
    ["Steps to reproduce the issue", "重现此问题的操作步骤"],
    ["Expected behavior", "预期表现行为"],
    ["Actual behavior", "实际表现行为"],
    ["Any error messages", "出现的任何报错信息"],
    ["Any relevant information", "其他任何相关信息"],
    ["Steps to Reproduce", "复现步骤"],
    ["Please list the steps to reproduce the issue", "请列出重现该问题的详细步骤..."],
    ["Please describe the feature you'd like to see. The more detailed the requirements, the easier it will be for our team to incorporate your ideas. Some helpful information includes:", "请描述您希望获得的新功能。需求越详尽，团队就越容易采纳您的构想。有帮助的信息包括："],
    ["What is missing in your workflow", "在您的工作流中缺少什么能力"],
    ["What you would like to see to address this gap in your workflow", "您希望通过何种方式或界面来填补该工作流空白"],
    ["How this feature would help you and other users", "该功能如何为您及其他用户带来帮助"],
    ["Please describe your auth or billing issue. More details will help our support team resolve your issue quicker. Some helpful information includes:", "请描述您的身份认证或计费账单问题。更详尽的信息将帮助支持团队更快解决。有帮助的信息包括："],
    ["What quota or feature is being incorrectly limited", "哪些配额或功能被错误地限制"],
    ["What functionality you expect your account tier to have available that is missing", "您期望当前账户等级拥有但实际缺失的权益与功能"],
    ["Any error messages seen when trying to log in", "尝试登录时出现的任何报错信息"],
    ["Use this form if you cannot connect to Remote Control. If your instance is already connected and you are experiencing issues in the browser, report directly from the Remote Control web page so that relevant logs are included. Some helpful information includes:", "如果您无法连接至远程控制 (Remote Control)，请使用此表单。如果已连接且仅在浏览器端遇到问题，请直接在远程控制网页端反馈以便附带相关日志。有帮助的信息包括："],
    ["Whether it never connects or is intermittent/flaky", "是始终无法连接还是偶发中断/不稳定"],
    ["Did restarting the desktop app help?", "重启桌面客户端是否能解决问题？"],
    ["For any feedback that does not fit into the above categories.", "适用于不属于上述分类的任何其他建议与反馈。"],
    ["Describe the bug you encountered...", "请描述您遇到的问题与缺陷..."],
    ["Describe the feature you would like to see...", "请描述您希望实现的新特性..."],
    ["Describe your auth or billing issue...", "请描述您的认证或账单问题..."],
    ["Describe your Remote Control issue...", "请描述您的远程控制连接问题..."],
    ["Enter your feedback here...", "在此输入您的反馈意见..."],
    ["Attach Antigravity server logs", "附加本地服务运行日志"],
    ["Attach Multigravity server logs", "附加本地服务运行日志"],
    ["Attach a screenshot (optional)", "附加截图（可选）"],
    ["Attaching logs requires an email address", "附加日志需要提供邮箱地址"],
    ["We recommend attaching logs. Attaching logs will help the Antigravity team act on and prioritize your feedback.", "建议勾选附加日志，这将帮助开发团队更快定位并优先处理您的反馈。"],
    ["We recommend attaching logs. Attaching logs will help the Multigravity team act on and prioritize your feedback.", "建议勾选附加日志，这将帮助开发团队更快定位并优先处理您的反馈。"],
    ["Visit", "访问"],
    ["Legal Help", "法律援助中心 (Legal Help)"],
    ["to ask for content changes for legal reasons.", "以法律原因为由申请内容变更。"],
    ["to ask for content changes for legal reasons", "以法律原因为由申请内容变更"],
    ["Submitting...", "正在提交..."],
    ["Submit", "提交反馈"],
    ["Send Feedback", "发送反馈"],
    ["Feedback", "意见反馈"],

    // --- 扩展、技能与 Token 预算明细 (Customizations & Token Budget) ---
    ["The breakdown below shows token usage from customizations like rules, skills, and MCP. If a budget is exceeded, large rules are demoted to path pointers and large customizations are excluded automatically.", "下方明细展示了规则 (Rules)、技能 (Skills) 及 MCP 等扩展占用的上下文 Token 额度。若超出预算上限，体积较大的规则将被降级为路径指针，体积较大的扩展将被自动排除。"],
    ["The breakdown below shows token usage from customizations like skills, rules, and MCP. If a budget is exceeded, large rules are demoted to path pointers and large customizations are excluded automatically.", "下方明细展示了技能 (Skills)、规则 (Rules) 及 MCP 等扩展占用的上下文 Token 额度。若超出预算上限，体积较大的规则将被降级为路径指针，体积较大的扩展将被自动排除。"],
    ["There are no customizations enabled.", "当前未启用任何个性化扩展。"],
    ["Customization token budget exceeded. Large customizations are excluded from context.", "扩展 Token 预算已超出上限。体积较大的扩展已从上下文中排除。"],
    ["Rules and customization token budgets exceeded. Large rules are demoted to path pointers and large customizations are excluded from context.", "规则与扩展 Token 预算均已超出上限。体积较大的规则已被降级为路径指针，体积较大的扩展已从上下文中排除。"],
    ["Rules token budget exceeded. Large rules are demoted to path pointers.", "规则 Token 预算已超出上限。体积较大的规则已被降级为路径指针。"],
    ["Customizations & Skills", "扩展与技能"],
    ["Search and discover customizations to extend your Agent's capabilities.", "搜索并发现扩展以增强智能体的能力。"],
    ["Refresh customizations", "刷新扩展与技能"],

    // --- 项目权限继承与范围 (Project Permissions & Inheritance) ---
    ["Also includes", "同时包含"],
    ["when working in this project.", "在当前项目中生效。"],
    ["when working in this project", "在当前项目中生效"],
    ["Inherits your Global Permissions when working in this project.", "在当前项目中工作时继承您的全局权限。"],
    ["Manually customize individual settings.", "手动自定义各项偏好设置。"],

    // --- 配额上限与额度耗尽提示 (Quota & Usage Limits) ---
    ["You have hit your weekly limit.", "您已达到每周额度上限。"],
    ["You have hit your 5-hour limit.", "您已达到 5 小时额度上限。"],
    ["If on a supported paid plan, you can use AI credits in the interim or upgrade to a higher tier.", "若订阅了支持的付费计划，期间您可以使用 AI 点数或升级到更高等级。"],
    ["the 5-hour limit does not currently apply", "目前不适用 5 小时额度限制"],
    ["the weekly limit does not currently apply", "目前不适用每周额度限制"],
    ["To continue using this model now, enable AI Credit overages.", "如需现在继续使用此模型，请启用 AI 点数超额替补。"],
    ["AI Credits Used to Generate Response", "已使用 AI 点数生成此响应"],
    ["Baseline model quota reached", "已达到基础模型配额上限"],
    ["Model quota reached", "已达模型配额上限"],
    ["Insufficient AI Credits", "AI 点数不足"],
    ["Your AI credits balance is too low to continue.", "您的 AI 点数余额不足，无法继续操作。"],

    // --- 面板、分屏与终端管理器 (Panes, Splits & Terminals) ---
    ["Standalone Terminals", "独立终端"],
    ["Standalone terminals", "独立终端"],
    ["Terminals", "终端列表"],
    ["Restore split view", "恢复分屏视图"],
    ["Maximize split view", "最大化分屏视图"],
    ["Maximize Inspector", "最大化检查器"],
    ["Maximize Trajectory", "最大化轨迹面板"],
    ["Maximize Timeline", "最大化时间线"],
    ["Maximize Pane", "最大化面板"],
    ["Restore Pane", "恢复面板"],
    ["Equalize Split Panes", "均分分屏面板"],
    ["Remove from Split", "移出分屏"],
    ["Remove From Split", "移出分屏"],
    ["Split Down", "向下分屏"],
    ["Split Right", "向右分屏"],
    ["Split Terminal", "分屏终端"],
    ["Split Conversation Horizontally", "水平分屏会话"],
    ["Split Conversation Vertically", "垂直分屏会话"],
    ["View Split Diff", "查看分屏差异"],
    ["Close panel", "关闭面板"],
    ["Close skills panel", "关闭技能面板"],
    ["Resize terminal panes", "调整终端面板大小"],
    ["Resize side question panel", "调整提问侧栏大小"],
    ["Artifact Viewer", "产物查看器"],
    ["Browser Subagent Viewer", "浏览器智能体查看器"],
    ["Background Task Output", "后台任务输出"],
    ["No background tasks", "暂无后台任务"],
    ["Browser Task", "浏览器任务"],
    ["Custom View", "自定义视图"],
    ["Sidecar View", "Sidecar 视图"],
    ["Auxiliary Pane", "辅助面板"],
    ["Next Aux Pane Tab", "下一个辅助面板标签"],
    ["Previous Aux Pane Tab", "上一个辅助面板标签"],
    ["Go Back in Pane", "在面板中后退"],
    ["Go Forward in Pane", "在面板中前进"],
    ["Open in Preview Pane", "在预览面板中打开"],
    ["Open in Side Pane", "在侧边面板中打开"],
    ["Open preview in embedded pane", "在内嵌面板中打开预览"],
    ["Side Pane UI Available", "侧边面板可用"],
    ["Open File", "打开文件"],
    ["New Terminal", "新建终端"],
    ["Open Terminal", "打开终端"],
    ["Delete Terminal", "删除终端"],
    ["Failed to create terminal", "创建终端失败"],

    // --- 代码变更与审查 (Review Changes, Diff & Git) ---
    ["Files Changed", "变更文件"],
    ["Files changed", "变更文件"],
    ["No file changes", "无文件变更"],
    ["(no file changes detected)", "(未检测到文件变更)"],
    ["Review Changes", "审查代码变更"],
    ["Review changes", "审查代码变更"],
    ["Modified Files", "已修改文件"],
    ["Both staged and unstaged changes are being committed.", "已暂存和未暂存的更改都将被提交。"],
    ["Only staged changes are being committed.", "仅已暂存的更改将被提交。"],
    ["Commit", "提交 (Git)"],
    ["Amend", "修改上次提交 (Amend)"],
    ["Push", "推送 (Git Push)"],
    ["Commit succeeded", "提交成功"],
    ["Amend succeeded", "修改提交成功"],
    ["Push succeeded", "推送成功"],
    ["Failed to Commit", "提交失败"],
    ["Failed to Amend", "修改提交失败"],
    ["Failed to Push", "推送失败"],
    ["Failed to Generate Commit Message", "生成提交信息失败"],
    ["Branch Changes", "分支变更"],
    ["Include unstaged changes", "包含未暂存的修改"],
    ["Changes", "变更"],
    ["Added (Staged)", "已添加（已暂存）"],
    ["Uncommitted", "未提交更改"],
    ["Revert file changes", "还原文件更改"],
    ["Open Commit Graph", "打开提交记录图"],
    ["Configure Branches", "配置分支"],
    ["Configure Worktree Branches", "配置工作树分支"],
    ["New Worktree", "新建工作树"],
    ["New worktree", "新建工作树"],
    ["Select branch", "选择分支"],
    ["No branches", "暂无分支"],
    ["changed", "已修改"],
    ["modified", "已修改"],
    ["added", "已添加"],
    ["deleted", "已删除"],
    ["renamed", "已重命名"],
    ["copied", "已复制"],

    // --- 智能体状态、操作与界面导航 ---
    ["Failed to create workspace.", "创建工作区失败。"],
    ["Failed to archive workspace", "归档工作区失败"],
    ["Failed to delete conversation", "删除会话失败"],
    ["Failed to fork conversation", "分叉会话失败"],
    ["Failed to rename conversation", "重命名会话失败"],
    ["Failed to save project", "保存工程项目失败"],
    ["Failed to start conversation", "启动会话失败"],
    ["Failed to stop agent", "停止智能体失败"],
    ["Failed to stop conversation", "停止会话失败"],
    ["Could not create project", "无法创建工程项目"],
    ["Error Loading Models", "加载模型列表出错"],
    ["Close split view and go to forked conversation", "关闭分屏并前往分叉会话"],
    ["Find in Conversation", "在会话中查找"],
    ["Rename This Conversation", "重命名此会话"],
    ["Current Workspace", "当前工作区"],
    ["Current workspace", "当前工作区"],
    ["All Workspaces", "所有工作区"],
    ["Group By Project", "按工程项目分组"],
    ["Group By Workspace", "按工作区分组"],
    ["Group name", "分组名称"],
    ["Move to New Group", "移动至新分组"],
    ["Remove from Group", "从分组中移除"],
    ["Expand All Folders", "展开所有文件夹"],
    ["Collapse All Folders", "折叠所有文件夹"],
    ["Conversation ID", "会话 ID"],
    ["Conversation Name", "会话名称"],
    ["Conversation Archived", "会话已归档"],
    ["Conversation copied as Markdown to clipboard", "会话已复制为 Markdown 到剪贴板"],
    ["Conversation unavailable", "会话当前不可用"],
    ["Copy thinking", "复制思考过程"],
    ["Copy Trajectory ID", "复制轨迹 ID"],
    ["Copy trajectory ID", "复制轨迹 ID"],
    ["Copy Conversation ID", "复制会话 ID"],
    ["Copy Config File Path", "复制配置文件路径"],
    ["Copy debug info", "复制调试信息"],
    ["Copy error to clipboard", "复制错误信息到剪贴板"],
    ["Copy raw string value", "复制原始字符串"],
    ["Copy schema JSON", "复制 Schema JSON"],
    ["Copy this subtree JSON", "复制子树 JSON"],
    ["Copy section content", "复制本节内容"],
    ["Copy output", "复制输出"],
    ["Copy value", "复制值"],
    ["Download Diagnostics", "下载诊断日志"],
    ["Download PDF", "下载 PDF"],
    ["Download SVG", "下载 SVG"],
    ["Start Voice Recording", "开始语音录制"],
    ["Stop Voice Recording", "停止语音录制"],
    ["Cancel recording", "取消录制"],
    ["Proceed with Plan", "按此计划执行"],
    ["Other (write your answer)", "其他（填写您的回答）"],
    ["Input required", "需要用户输入"],
    ["Action required", "需要用户操作"],
    ["Needs Attention", "需要关注"],
    ["Got it", "知道了"],
    ["In Progress", "正在处理中"],
    ["Streaming Generation", "正在流式生成"],
    ["Files modified by the agent in this conversation", "智能体在此会话中修改的文件"],
    ["Explain and Fix in Current Conversation", "在当前会话中解释并修复"],
    ["Open in Built-in Browser", "在内置浏览器中打开"],
    ["Open in external browser", "在外部浏览器中打开"],
    ["Show Remote Control QR code", "显示远程控制二维码"],
    ["Share Conversation (Preview)", "分享会话（预览版）"],
    ["Discover helpful skills & plugins", "探索实用的技能与插件"],
    ["Explore the plugin marketplace", "浏览插件市场"],
    ["Paste auth code", "粘贴授权码"],
    ["Paste code here", "在此粘贴代码"],
    ["Reload app", "重新加载应用"],
    ["Restart Main Language Server", "重启主语言服务器"],
    ["Search by name or Cascade ID...", "按名称或 Cascade ID 搜索..."],
    ["Search tabs, files, plugins, subagents, artifacts, tasks...", "搜索标签页、文件、插件、子智能体、产物、任务..."],
    ["Search file contents...", "搜索文件内容..."],
    ["Search results", "搜索结果"],
    ["Search Conversations", "搜索会话"],
    ["Pay as you go", "按量计费 (Pay as you go)"],
    ["Strict Mode", "严格安全模式"],
    ["System Prompt", "系统提示词"],
    ["System Instruction", "系统指令"],
    ["Step Details", "步骤详情"],
    ["Step Type", "步骤类型"],
    ["Sunday", "星期日"],
    ["Monday", "星期一"],
    ["Tuesday", "星期二"],
    ["Wednesday", "星期三"],
    ["Thursday", "星期四"],
    ["Friday", "星期五"],
    ["Saturday", "星期六"],
    ["Daily", "每日"],
    ["Hourly", "每小时"],
    ["Weekly", "每周"],
    ["Monthly", "每月"],
    ["Last 7 days", "过去 7 天"],
    ["Last 30 days", "过去 30 天"],
    ["Last 24 hours", "过去 24 小时"],
    ["Open Launchpad", "打开应用启动台"],
    ["Toggle Project Selector", "切换工程项目选择器"],
    ["Toggle Environment Selector", "切换环境选择器"],
    ["Toggle Overview Panel", "切换概览面板"],
    ["New Window", "新建窗口"],
    ["New Tab", "新建标签页"],
    ["Select All", "全选"],
    ["Cut", "剪切"],
    ["Paste", "粘贴"],
    ["Copy Image", "复制图片"],
    ["Allow List Terminal Commands", "终端命令白名单"],
    ["Deny List Terminal Commands", "终端命令黑名单"],
    ["Agent Auto-Fix Lints", "智能体自动修复 Lint 报错"],
    ["Enable Terminal Sandbox", "启用终端沙箱隔离"],
    ["Sandbox Allow Network", "沙箱允许网络访问"],
    ["Enable Shell Integration", "启用 Shell 终端集成"],
    ["Terminal Command Auto Execution", "终端命令自动执行"],
    ["Enable Sounds for Agent", "启用智能体声音提示"],
    ["Enable Notifications for Agent", "启用智能体桌面通知"],
    ["Auto-Expand Changes Overview", "自动展开代码变更概览"],
    ["Auto-Open Edited Files", "自动打开编辑的文件"],
    ["Open Agent on Reload", "重载时自动打开智能体"],
    ["Enable Browser Tools", "启用浏览器自动化工具"],
    ["Chrome Binary Path", "Chrome 可执行文件路径"],
    ["Browser User Profile Path", "浏览器用户配置路径"],
    ["Browser CDP Port", "浏览器 CDP 调试端口"],
    ["Show Selection Actions", "显示选中操作浮条"],
    ["Default Workspace VCS", "默认工作区版本控制系统 (VCS)"],
    ["See more results", "查看更多结果"],
    ["Running", "运行中"],
    ["Starting", "正在启动"],
    ["Overview", "概览"],
    ["Automations", "自动化任务"],
    ["Marketplace", "插件市场"],
    ["Installed", "已安装"],
    ["Discard", "放弃改动"],
    ["Ask first", "执行前询问"],
    ["Always run", "始终运行"],
    ["Allow once", "仅允许一次"],
    ["Undo", "撤销"],
    ["Undo To Here", "撤销至此步骤"],
    ["Open with External Browser", "使用系统默认浏览器打开"],
    ["Create fork in current workspace", "在当前工作区中创建分叉"],
    ["Create fork in shared workspace", "在共享工作区中创建分叉"],
    ["Create fork in new workspace", "在新工作区中创建分叉"],
    ["Ask every time", "每次均询问"],
    ["Standalone Conversations", "独立会话"],
    ["Console logs", "控制台日志"],
    ["Commands", "执行指令"],
    ["Run command", "执行命令"],
    ["File access", "文件访问"],
    ["Open URL", "打开链接"],
    ["Read URL", "读取网页内容"],
    ["MCP tool", "MCP 工具"],
    ["Run JS", "执行 JavaScript"],
    ["Approval", "等待审批"],
    ["About", "关于"],
    ["Toggle Fullscreen", "切换全屏"],
    ["Minimize", "最小化"],
    ["Maximize", "最大化"],
    ["Custom", "自定义"],
    ["Steps", "执行步骤"],
    ["Activity", "活动记录"],
    ["Open Preferences", "打开偏好设置"],
    ["Block", "阻止"],
    ["Standalone", "独立模式"],
    ["OK", "确定"],

    // --- 界面展开/收起/搜索与状态 ---
    ["See All", "查看全部"],
    ["See all", "查看全部"],
    ["See Less", "收起"],
    ["See less", "收起"],
    ["See More", "查看更多"],
    ["See more", "查看更多"],
    ["See more results", "查看更多结果"],
    ["Artifacts", "交付工件"],
    ["Review", "审核"],
    ["Review Policy", "工件审核策略"],
    ["Review Mode", "审核模式"],
    ["Review Comments", "批注审核"],
    ["Collapse All", "全部收起"],
    ["Expand All", "全部展开"],
    ["Collapse All Folders", "收起所有文件夹"],
    ["Expand All Folders", "展开所有文件夹"],
    ["Collapse All Diffs", "收起所有差异对比"],
    ["Expand All Diffs", "展开所有差异对比"],
    ["No Results", "未找到结果"],
    ["No results", "未找到结果"],
    ["No results found", "未找到匹配结果"],
    ["No results found.", "未找到匹配结果。"],
    ["No conversations available", "暂无历史会话"],
    ["No subagents", "暂无子智能体"],
    ["No active terminals", "暂无活动终端"],

    // --- 主题预设 (Theme Presets) ---
    ["Preset", "预设配色"],
    ["Default Light", "默认浅色"],
    ["Default Dark", "默认深色"],
    ["Catppuccin", "Catppuccin 主题"],
    ["One Light", "One Light 亮色"],
    ["One Dark Pro", "One Dark Pro 深色"],
    ["One Dark", "One Dark 暗色"],
    ["Solarized Light", "Solarized 浅色"],
    ["Solarized Dark", "Solarized 深色"],
    ["Dracula", "Dracula 经典紫"],
    ["Monokai", "Monokai 代码高亮"],
    ["Tokyo Night", "Tokyo Night 东京之夜"],
    ["Vesper", "Vesper 暮色黑"],
    ["Nord", "Nord 极光蓝"],
    ["Reset to preset", "重置为预设配色"],
    ["Low contrast ratio", "低对比度"],

    // --- 设置下拉菜单与策略选项 (Settings Dropdowns & Policies) ---
    ["Inherit Global", "继承全局配置"],
    ["Always Ask", "每次询问"],
    ["Always Proceed", "始终执行"],
    ["Request Review", "请求审核 (Request Review)"],
    ["Proceed in Sandbox", "沙箱内直接执行"],
    ["Proceed In Sandbox", "沙箱内直接执行"],
    ["Custom", "自定义"],
    ["Full machine", "完全访问整机"],
    ["Turbo mode", "极速模式 (Turbo)"],
    ["Vetted (Preview)", "安全审核 (预览版)"],
    ["Simplified", "极简模式"],
    ["Fast", "快速"],
    ["Slow", "慢速"],
    ["Narrow", "紧凑窄屏"],
    ["Wide", "加宽"],
    ["Fill", "自适应铺满"],
    ["High Contrast", "高对比度"],
    ["Normal", "标准对比度"],
    ["Inherit Editor", "跟随编辑器"],
    ["Auto (detected)", "自动检测 (Auto)"],
    ["Electron (Desktop)", "Electron 客户端 (桌面)"],
    ["Electron (Gemini App)", "Electron 客户端 (Gemini 应用)"],
    ["Electron (Android Desktop)", "Electron 客户端 (安卓桌面)"],
    ["Web (desktop)", "Web 端 (桌面浏览器)"],
    ["Web (desktop, PWA)", "Web 端 (桌面 PWA)"],
    ["Web (desktop, remote control)", "Web 端 (桌面远程控制)"],
    ["Web (desktop, remote control, PWA)", "Web 端 (桌面远程控制 PWA)"],
    ["Web (mobile)", "Web 端 (手机浏览器)"],
    ["Web (mobile, PWA)", "Web 端 (手机 PWA)"],
    ["Web (mobile, remote control)", "Web 端 (手机远程控制)"],
    ["Web (mobile, remote control, PWA)", "Web 端 (手机远程控制 PWA)"],
    ["Google (internal)", "Google (内部)"],
    ["External", "外部开发者 (External)"],
    ["Enterprise", "企业用户 (Enterprise)"],
    ["Allow", "允许"],
    ["Deny", "拒绝"],
    ["Enabled", "已启用"],
    ["Disabled", "已禁用"],
    ["Disabled by organization policy", "已被组织管理策略禁用"],
    ["Outside of folder access", "工作区外部文件访问"],
    ["Auto-execution policy", "终端命令自动执行策略"],
    ["Enable Sandbox Mode", "启用沙箱隔离模式"],
    ["Artifact Review Policy", "工件审核策略"],
    ["Permission Preset", "权限预设配置"],
    ["Security Preset", "安全预设策略"],
    ["Plan Review Policy", "执行计划审核策略"],
    ["Product Skin", "界面风格与模式"],
    ["Conversation Width", "会话面板宽度"],
    ["Markdown Artifact Width", "Markdown 工件宽度"],
    ["Tab Speed", "Tab 智能补全速度"],
    ["Send Immediately", "立即发送"],
    ["Queue message", "排队发送"],
    ["Send message", "发送消息"],
    ["Queue", "排队等待"],
    ["Queued Messages", "排队发送的消息"],
    ["Piper", "Piper (Google)"],
    ["Piper (g4)", "Piper (g4)"],
    ["JJ", "Jujutsu (JJ)"],
    ["Fig", "Fig 工作区"],
    ["Cog (GoB)", "Cog (GoB)"],
    ["CitC Workspace Type", "CitC 工作区类型"],

    // --- 下拉菜单详细描述与选项说明 ---
    ["Requires manual review for all terminal commands and file accesses outside of the working folders.", "所有终端命令及工作目录外部的文件访问均需手动审核确认。"],
    ["Useful for typical development with an emphasis on security. It prioritizes safety over speed by requiring manual approval for all terminal commands and files outside the project directory.", "适用于强调安全性的常规开发场景。通过要求对所有终端命令及项目外部文件进行手动确认，将安全性置于执行速度之上。"],
    ["All terminal commands require review. The agent can read or write to any file in the machine.", "所有终端命令均需手动审核。智能体可以读写本机任意文件。"],
    ["Useful for tasks that require file access across your full machine. The agent has full read and write access to all local files, but all proposed terminal commands require manual review and approval before running.", "适用于需要跨整机访问文件的任务。智能体对所有本地文件拥有完全读写权限，但所有终端命令执行前必须经过手动审核与批准。"],
    ["Disables all safety barriers for maximal iteration velocity.", "禁用所有安全屏障以获得极致迭代速度。"],
    ["A high-risk mode that disables all safety barriers. The agent operates with full system access, auto-executes all terminal commands, and reads or writes to all local files without review prompts.", "高风险模式，禁用所有安全防护。智能体拥有完整系统权限，自动执行所有终端命令，读写任意本地文件且无任何审核提示。"],
    ["Inherits your Global Permissions when working in this project.", "在当前项目中继承您的全局权限策略配置。"],
    ["Every command requires approval.", "所有命令都需要手动审核批准。"],
    ["Ask before sensitive operations.", "执行敏感操作前进行确认提示。"],
    ["Security agent decides if commands run.", "由内置安全审查智能体判定命令是否执行。"],
    ["Simplified interface without developer tooling.", "隐藏开发者调试工具的极简界面。"],
    ["The full developer experience.", "包含完整开发者工具的高级体验。"],
    ["Agent never asks for review. This maximizes the autonomy of the Agent, but also has the highest risk of the Agent operating over unsafe or injected Artifact content.", "智能体从不请求审核。这极大提升了自主执行速度，但对未知或注入内容存在安全风险。"],
    ["The agent never asks for review. This maximizes the autonomy of the agent, but also has the highest risk of the agent operating over unsafe or injected artifact content.", "智能体从不请求审核。这极大提升了自主执行速度，但对未知或注入内容存在安全风险。"],
    ["Agent always asks for review.", "智能体始终在生成工件后请求手动审核。"],
    ["The agent always asks for review.", "智能体始终在生成工件后请求手动审核。"],
    ["The agent always asks for confirmation before executing terminal commands (except those in the Allow list).", "在执行终端命令前始终请求确认（在允许列表中的命令除外）。"],
    ["Terminal command automatically proceeds if the command runs inside the sandbox. Otherwise, it requests review.", "若终端命令在沙箱内运行则自动执行；离开沙箱的命令将请求手动审核。"],
    ["Terminal commands automatically proceed inside the sandbox. A command that needs to leave the sandbox proceeds only if an agent judges it reversible, and otherwise requests review.", "沙箱内的终端命令自动执行。需要脱离沙箱的命令仅在智能体判定为可逆时自动执行，否则请求手动审核。"],
    ["Block all browser JavaScript execution.", "禁止所有浏览器端 JavaScript 脚本执行。"],
    ["Prompt for approval before running browser scripts.", "运行浏览器脚本前提示审批确认。"],
    ["Allow full browser script execution without prompting.", "允许完整执行浏览器脚本且无需手动提示。"],
    ["Simulate running on a different OS.", "模拟在不同操作系统环境下运行。"],
    ["Simulate a different host (Electron, desktop or mobile Web, Extension, remote control, or installed PWA).", "模拟不同宿主环境（Electron 桌面、移动/桌面 Web、扩展程序、远程控制或 PWA 应用）。"],
    ["Simulate a different user cohort (Google, External, Enterprise).", "模拟不同用户群体环境（Google 内部、外部开发者、企业客户）。"],
    ["Configure when follow-up messages are sent.", "配置后续排队消息的发送时机。"],
    ["Interrupt the agent and send immediately.", "打断智能体当前思考并立即发送。"],
    ["Queue until after the current turn.", "在当前对话轮次结束后排队发送。"],
    ["Choose how technical the interface should be.", "选择界面的技术细节丰富度与展示模式。"],
    ["Configure the maximum width of the conversation panel.", "配置对话面板的最大展示宽度。"],
    ["Configure the default width of markdown artifacts.", "配置 Markdown 交付工件的默认展示宽度。"],
    ["Set the speed of tab suggestions", "设置 Tab 智能代码补全的触发速度。"],
    ["Select the workspace type that will be used for new conversations started with the New Workspace option.", "选择使用“新建工作区”选项启动新会话时将使用的工作区类型。"]
  ]);

  // 建立忽略大小写的查询备份表，消除大小写差异
  const lowerDict = new Map();
  for (const [k, v] of exactDict.entries()) {
    lowerDict.set(k.toLowerCase().trim(), v);
  }

  // 辅助函数：格式化时间与时长描述
  function translateDuration(str) {
    if (!str) return str;
    return str
      .replace(/(\d+)\s*d\b/gi, "$1 天 ")
      .replace(/(\d+)\s*h\b/gi, "$1 小时 ")
      .replace(/(\d+)\s*m\b/gi, "$1 分钟 ")
      .replace(/(\d+)\s*s\b/gi, "$1 秒")
      .replace(/(\d+)\s*days?/gi, "$1 天")
      .replace(/(\d+)\s*hours?/gi, "$1 小时")
      .replace(/(\d+)\s*minutes?/gi, "$1 分钟")
      .replace(/(\d+)\s*seconds?/gi, "$1 秒")
      .replace(/,\s*/g, " ")
      .replace(/\s+/g, " ")
      .trim();
  }

  // 辅助函数：格式化活动与归档会话数量
  function translateConversationCount(str) {
    if (!str) return str;
    const hasDot = str.endsWith(".");
    const res = str
      .replace(/\.$/, "")
      .replace(/(\d+)\s*active conversations?/gi, "$1 个活动会话")
      .replace(/(\d+)\s*archived conversations?/gi, "$1 个归档会话")
      .replace(/\s+and\s+/gi, " 与 ")
      .trim();
    return res + (hasDot ? "。" : "");
  }

  // 辅助函数：格式化探索与执行步骤摘要 (Explored X files, Y tasks, ran Z commands)
  function translateExploredSummary(str) {
    if (!str) return str;
    const m = str.match(/^(Explored|Exploring|Edited|Editing)\s+(.+)$/i);
    if (!m) return str;
    const verb = m[1].toLowerCase();
    const rest = m[2];

    let verbZh = "";
    if (verb === "explored") verbZh = "已探索 ";
    else if (verb === "exploring") verbZh = "正在探索 ";
    else if (verb === "edited") verbZh = "已编辑 ";
    else if (verb === "editing") verbZh = "正在编辑 ";

    const parts = rest.split(/,\s*/);
    const zhParts = parts.map(p => {
      p = p.trim();
      const mRan = p.match(/^(?:ran|Ran)\s+(.+)$/i);
      if (mRan) {
        const cmd = mRan[1];
        const mCmdCount = cmd.match(/^(\d+)\s*commands?$/i);
        if (mCmdCount) return `执行 ${mCmdCount[1]} 条指令`;
        return `执行指令 ${cmd}`;
      }
      const mRunning = p.match(/^(?:running|Running)\s+(.+)$/i);
      if (mRunning) {
        const cmd = mRunning[1];
        const mCmdCount = cmd.match(/^(\d+)\s*commands?$/i);
        if (mCmdCount) return `正在运行 ${mCmdCount[1]} 条指令`;
        return `正在运行指令 ${cmd}`;
      }
      return p
        .replace(/^(\d+)\s*files?$/i, "$1 个文件")
        .replace(/^(\d+)\s*folders?$/i, "$1 个目录")
        .replace(/^(\d+)\s*edits?$/i, "$1 处修改")
        .replace(/^(\d+)\s*artifacts?$/i, "$1 个工件")
        .replace(/^(\d+)\s*search(?:es)?$/i, "$1 次检索")
        .replace(/^(\d+)\s*terminals?$/i, "$1 个终端")
        .replace(/^(\d+)\s*tasks?$/i, "$1 个任务")
        .replace(/^(\d+)\s*webs?$/i, "$1 次网页访问")
        .replace(/^(\d+)\s*browsers?$/i, "$1 个浏览器页面")
        .replace(/^(\d+)\s*images?$/i, "$1 张图片")
        .replace(/^(\d+)\s*actions?$/i, "$1 项操作")
        .replace(/^files$/i, "文件");
    });

    return verbZh + zhParts.join("，");
  }

  // 辅助函数：配额类型转换
  function translateLimitType(str) {
    if (!str) return "";
    const lower = str.toLowerCase().trim();
    if (lower === "weekly" || lower === "the weekly") return "每周";
    if (lower === "5-hour" || lower === "5 hour" || lower === "the 5-hour" || lower === "the 5 hour") return "5 小时";
    if (lower === "daily" || lower === "the daily") return "每日";
    if (lower === "monthly" || lower === "the monthly") return "每月";
    return str.trim();
  }

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

    // 配额刷新时间与达到额度上限 (You have hit your limit...)
    [new RegExp("^You have hit your ([^,]+) limit,\\s*it refreshes in (.+?)\\.\\s*If on a supported paid plan,\\s*you can use AI credits in the interim or upgrade to a higher tier\\.?$", "i"),
      (m, l, t) => `您已达到${translateLimitType(l)}额度上限，将在 ${translateDuration(t)} 后刷新。若订阅了支持的付费计划，期间您可以使用 AI 点数或升级到更高等级。`],
    [new RegExp("^You have hit your ([^,]+) limit,\\s*the ([^,]+) limit does not currently apply\\.\\s*Your ([^,]+) limit will fully refresh in (.+?)\\.?$", "i"),
      (m, l1, l2, l3, t) => `您已达到${translateLimitType(l1)}额度上限，目前不适用${translateLimitType(l2)}额度限制。您的${translateLimitType(l3)}额度将在 ${translateDuration(t)} 后完全刷新。`],
    [new RegExp("^You have hit your ([^,]+) limit,\\s*the ([^,]+) limit does not currently apply\\.?$", "i"),
      (m, l1, l2) => `您已达到${translateLimitType(l1)}额度上限，目前不适用${translateLimitType(l2)}额度限制。`],
    [new RegExp("^You have hit your ([^,]+) limit,\\s*it refreshes in (.+?)\\.?$", "i"),
      (m, l, t) => `您已达到${translateLimitType(l)}额度上限，将在 ${translateDuration(t)} 后刷新。`],
    [new RegExp("^You have hit your ([^,]+) limit,\\s*it will fully refresh in (.+?)\\.?$", "i"),
      (m, l, t) => `您已达到${translateLimitType(l)}额度上限，将在 ${translateDuration(t)} 后完全刷新。`],
    [new RegExp("^Your ([^,]+) limit will fully refresh in (.+?)\\.?$", "i"),
      (m, l, t) => `您的${translateLimitType(l)}额度将在 ${translateDuration(t)} 后完全刷新。`],
    [new RegExp("^the ([^,]+) limit does not currently apply\\.?$", "i"),
      (m, l) => `目前不适用${translateLimitType(l)}额度限制。`],
    [new RegExp("^If on a supported paid plan,\\s*you can use AI credits in the interim or upgrade to a higher tier\\.?$", "i"),
      "若订阅了支持的付费计划，期间您可以使用 AI 点数或升级到更高等级。"],
    [new RegExp("^You have hit your ([^,]+) limit\\.?$", "i"),
      (m, l) => `您已达到${translateLimitType(l)}额度上限。`],

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
    [new RegExp("^it will fully refresh in (.+)$", "i"), (m, t) => `将在 ${translateDuration(t)} 后完全刷新。`],
    [new RegExp("^it refreshes in (.+)$", "i"), (m, t) => `将在 ${translateDuration(t)} 后刷新。`],

    // Token 预算与项目设置
    [new RegExp("^([\\d.]+)%\\s*of the customization budget is available\\.?$", "i"), "可用个性化扩展预算仍有 $1%。"],
    [new RegExp("of the customization budget is available", "i"), "的扩展预算仍可用"],
    [new RegExp("^Show (\\d+) breakdowns?$", "i"), "展开 $1 项明细"],
    [new RegExp("^\\(([\\d,]+) tokens\\)\\s*([\\d.]+)%$", "i"), "($1 Tokens) $2%"],

    // Token 消耗加载与明细
    [new RegExp("^Loading token usage\\.{0,3}$", "i"), "正在加载 Token 消耗数据..."],
    [new RegExp("^There are no customizations enabled\\.?$", "i"), "当前未启用任何个性化扩展。"],
    [new RegExp("^refreshing\\.{0,3}$", "i"), "正在刷新..."],
    [new RegExp("^Show (\\d+) more(?: items?)?$", "i"), "展开另外 $1 项"],
    [new RegExp("^Show fewer(?: items?)?$", "i"), "收起"],
    [new RegExp("^Show all (\\d+) items?$", "i"), "显示全部 $1 项"],

    // 会话数量与项目危险区删除
    [new RegExp("^(?:\\d+\\s*(?:active|archived)\\s*conversations?(?:\\s+and\\s+)?)+\\.?$", "i"), (m) => translateConversationCount(m)],
    [new RegExp("^Permanently delete\\s+(.+?)\\s+including\\s+(.+?)\\.?$", "i"), (m, p, c) => `永久删除 ${p}（包含 ${translateConversationCount(c)}）。`],
    [new RegExp("^Permanently delete\\s+(.+?)\\s*\\.?$", "i"), "永久删除 $1。"],
    [new RegExp("^This will permanently delete\\s+(.+?)\\s+within it\\.?$", "i"), (m, c) => `这将永久删除其包含的 ${translateConversationCount(c)}。`],
    [new RegExp("^Are you sure you want to delete the\\s+(project|workspace)\\s+(.+?)\\??$", "i"), (m, type, name) => `您确定要删除此${type.toLowerCase() === "project" ? "工程项目" : "工作区"} ${name} 吗？`],

    // 项目管理与删除
    [new RegExp("^Agent settings and permissions for conversations outside of (projects|workspaces)\\.?$", "i"), "非$1会话的智能体配置与执行权限。"],
    [new RegExp("^Manage (project|workspace) folders, agent settings, and permissions\\.?$", "i"), "管理$1目录、智能体配置与专属执行权限。"],
    [new RegExp("^Delete (Project|Workspace)$", "i"), "删除$1"],
    [new RegExp("^This (project|workspace) is managed by the (.+?) automation and can only be deleted by deleting that automation\\.?$", "i"), "此$1由 $2 自动化任务管理，只能通过删除该自动化任务来删除。"],
    [new RegExp("^A (project|workspace) with this name already exists\\.?$", "i"), "已存在同名的$1。"],

    // 智能体执行与步数
    [new RegExp("^Worked for\\s+(.+)$", "i"), (m, t) => `运行耗时 ${translateDuration(t)}`],
    [new RegExp("^Stopped after\\s+(.+)$", "i"), (m, t) => `在 ${translateDuration(t)}后停止`],
    [new RegExp("^(?:Explored|Exploring|Edited|Editing)\\s+.+$", "i"), (m) => translateExploredSummary(m)],
    [new RegExp("^Ran (\\d+) commands?$", "i"), "已执行 $1 条指令"],
    [new RegExp("^Running (\\d+) commands?$", "i"), "正在运行 $1 条指令..."],
    [new RegExp("^Ran\\s+(.+)$", "i"), "已执行指令 $1"],
    [new RegExp("^Running\\s+(.+)$", "i"), "正在运行指令 $1..."],
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
    [new RegExp("^Also includes\\s*(?:Global Permissions|全局权限规则)?\\s*when working in this project\\.?$", "i"), "在当前项目中工作时同时继承全局权限。"],
    [new RegExp("^Also includes Global Permissions when working in this project\\. Learn more\\.?$", "i"), "在当前项目中工作时同时继承全局权限。了解更多。"],
    [new RegExp("^Also includes (.+?) when working in this project\\.?$", "i"), "在当前项目中工作时同时包含 $1。"],
    [new RegExp("^Inherits your Global Permissions when working in this project\\.?$", "i"), "在当前项目中工作时继承您的全局权限。"],
    [new RegExp("^when working in this project\\.?$", "i"), "在当前项目中生效。"],
    [new RegExp("^The breakdown below shows token usage from customizations like [^.]*?\\.\\s*If (?:a|the) budget is exceeded[^.]*?\\.?$", "i"), "下方明细展示了规则 (Rules)、技能 (Skills) 及 MCP 等扩展占用的上下文 Token 额度。若超出预算上限，体积较大的规则将被降级为路径指针，体积较大的扩展将被自动排除。"],
    [new RegExp("^(?:Rules and customization|Customization|Rules) token budgets? exceeded\\.\\s*(.+)$", "i"), (m, p1) => `Token 预算已超出上限。${translateString(p1)}`],
    [new RegExp("^Configure default behaviors, skills, and MCP servers\\. Learn more\\.?$", "i"), "配置默认行为策略、技能库 (Skills) 与 MCP 服务节点。了解更多。"],
    [new RegExp("^Configure global allowed and denied resource permissions\\. Learn more\\.?$", "i"), "配置全局允许与拒绝的资源访问权限。了解更多。"],
    [new RegExp("^Browser settings have moved to the Browser section of General settings\\. Go to General settings$", "i"), "浏览器设置已整合至常规偏好设置中的“浏览器”专区。前往常规设置"],
    [new RegExp("^Select project$", "i"), "选择工程项目"],
    [new RegExp("^No matching (.+)$", "i"), "未找到匹配的 $1"],
    [new RegExp("^No (.+) found\\.?$", "i"), "未找到 $1。"],
    [new RegExp("^Working directory:\\s*(.+)$", "i"), "工作目录：$1"],
    [new RegExp("^Page title:\\s*(.+)$", "i"), "页面标题：$1"],

    // 意见反馈
    [new RegExp("^Send feedback as\\s*(.+)$", "i"), "以 $1 身份发送反馈"],
    [new RegExp("^Visit\\s+(?:Legal Help)?\\s+to ask for content changes for legal reasons\\.?$", "i"), "访问 法律援助中心 (Legal Help) 以法律原因为由申请内容变更。"],
    [new RegExp("^We recommend attaching logs\\.\\s*Attaching logs will help the (?:Antigravity|Multigravity) team act on and prioritize your feedback\\.?$", "i"), "建议勾选附加日志，这将帮助开发团队更快定位并优先处理您的反馈。"],

    // 遥测与邮件
    [new RegExp("^When toggled on,\\s*(?:Google\\s*)?(?:Antigravity|Multigravity)\\s*collects usage data to help Google enhance performance and features\\.?$", "i"), "开启后，Multigravity 将收集匿名使用诊断数据，以帮助提升系统性能与体验。"],
    [new RegExp("^(?:Yes,\\s*I'd like to receive|Receive)\\s*product updates,\\s*tips,\\s*and promotions from Google\\s*(?:Antigravity|Multigravity)?\\s*via email\\.?$", "i"), "通过电子邮件接收来自 Multigravity 的产品更新速递、使用技巧与官方资讯。"],

    // 面板与分屏操作
    [new RegExp("^Maximize\\s+(.+)$", "i"), (m, p1) => `最大化 ${translateString(p1)}`],
    [new RegExp("^Restore\\s+(.+)$", "i"), (m, p1) => `恢复 ${translateString(p1)}`],

    // 模型与项目选择下拉 aria-label
    [new RegExp("^Select model,\\s*current:\\s*(.+)$", "i"), (m, p1) => `选择模型，当前为：${translateString(p1)}`],
    [new RegExp("^Select project,\\s*current:\\s*(.+)$", "i"), (m, p1) => `选择项目，当前为：${translateString(p1)}`],
    [new RegExp("^Select Agent$", "i"), "选择智能体"],
    [new RegExp("^Main Agent$", "i"), "主智能体"],
    [new RegExp("^Security Preset\\s+(.+)$", "i"), (m, p1) => `安全预设策略：${translateString(p1)}`],
    [new RegExp("^Plan Review Policy\\s+(.+)$", "i"), (m, p1) => `计划审核策略：${translateString(p1)}`],
    [new RegExp("^Queue until after the current turn\\.?$", "i"), "在当前轮次结束后排队发送。"],
    [new RegExp("^Interrupt the agent and send immediately\\.?$", "i"), "打断智能体并立即发送。"],
    [new RegExp("^Manage Multigravity app settings\\.?$", "i"), "管理 Multigravity 客户端设置与偏好。"],
    [new RegExp("^Manage your plan, credentials, and general preferences\\.?$", "i"), "管理您的计划订阅、身份凭据与通用偏好设置。"],
    [new RegExp("^Replace the default browser right-click menu with quick actions\\.?$", "i"), "使用智能体快捷操作替换浏览器默认的右键菜单。"],
    [new RegExp("^Configure the browser subagent\\.\\s*It requires\\s*(.+?)\\s*to be installed\\.?$", "i"), "配置浏览器智能体。这需要已安装 $1。"],
    [new RegExp("^The browser subagent can be invoked by typing /browser in the conversation input box\\.?$", "i"), "在对话输入框输入 /browser 即可调用浏览器智能体。"],
    [new RegExp("^Type\\s*/\\s*and select\\s*plan\\s*to have the agent generate a plan\\.?$", "i"), "输入 / 并选择 plan 即可让智能体生成执行计划。"],
    [new RegExp("^Sign in to use (.+?)!?$", "i"), "登录以使用 $1 服务！"],
    [new RegExp("^By using this app, you agree to its$", "i"), "使用此应用即表示您同意其"],
    [new RegExp("^When working in this project\\. Learn more\\.?$", "i"), "在当前项目中生效。了解更多。"],
    [new RegExp("^The folder “(.+?)” does not exist\\. Would you like to create it\\?$", "i"), "目录 “$1” 不存在。您是否希望创建它？"],
    [new RegExp("^Directory (.+?) does not exist\\.?$", "i"), "目录 $1 不存在。"],
    [new RegExp("^Failed to read directory (.+?):\\s*(.+)$", "i"), "读取目录 $1 失败：$2"],

    // 展开/收起/搜索结果与动态数量
    [new RegExp("^See all\\s*\\((.+)\\)$", "i"), (m, p1) => `查看全部 (${p1})`],
    [new RegExp("^See less\\s*\\((.+)\\)$", "i"), (m, p1) => `收起 (${p1})`],
    [new RegExp("^Show all\\s*\\((.+)\\)$", "i"), (m, p1) => `查看全部 (${p1})`],
    [new RegExp("^Show less\\s*\\((.+)\\)$", "i"), (m, p1) => `收起 (${p1})`],
    [new RegExp("^Show finished\\s*\\((.+)\\)$", "i"), (m, p1) => `显示已完成 (${p1})`],
    [new RegExp("^See\\s+(\\d+)\\s+more\\s*(.*)$", "i"), (m, p1, p2) => `查看其余 ${p1} 项${p2 ? ' ' + translateString(p2) : ''}`],
    [new RegExp("^See more in\\s*(.+)$", "i"), (m, p1) => `查看 ${translateString(p1)} 中的更多项`],
    [new RegExp("^See less in\\s*(.+)$", "i"), (m, p1) => `收起 ${translateString(p1)} 中的项`],
    [new RegExp("^Collapse All\\s*(.*)$", "i"), (m, p1) => p1 ? `全部收起 ${translateString(p1)}` : "全部收起"],
    [new RegExp("^Expand All\\s*(.*)$", "i"), (m, p1) => p1 ? `全部展开 ${translateString(p1)}` : "全部展开"],
    [new RegExp("^No results found\\.?$", "i"), "未找到匹配结果。"],
    [new RegExp("^Cancel\\s*\\(\\u2303C\\)$", "i"), "取消 (⌃C)"],
    [new RegExp("^Cancel\\s*\\(Ctrl\\+D\\)$", "i"), "取消 (Ctrl+D)"],
    [new RegExp("^Low contrast ratio\\s*\\((.+)\\)\\.?$", "i"), (m, p1) => `低对比度 (${p1})`],
    [new RegExp("^Stop Execution$", "i"), "停止执行"],
    [new RegExp("^Stop execution$", "i"), "停止执行"]
  ];

  // 3. 安全检测：判断是否为不可汉化的代码块、数据或字体图标区域
  const IGNORE_TAGS = new Set(["SCRIPT", "STYLE", "CODE", "PRE", "NOSCRIPT", "PATH", "SVG"]);

  function shouldIgnoreElement(el) {
    if (!el || !el.tagName) return true;
    if (IGNORE_TAGS.has(el.tagName)) return true;

    // 排除编辑器、终端、代码高亮容器与字体图标容器
    const className = typeof el.className === "string" ? el.className : (el.getAttribute ? (el.getAttribute("class") || "") : "");
    if (
      className.includes("monaco-editor") ||
      className.includes("syntax-highlight") ||
      className.includes("xterm") ||
      className.includes("terminal-screen") ||
      className.includes("terminal-container") ||
      className.includes("code-block") ||
      className.includes("language-") ||
      className.includes("google-symbols") ||
      className.includes("material-symbols") ||
      className.includes("material-icons") ||
      className.includes("codicon") ||
      className.includes("markdown-body") ||
      className.includes("rendered-markdown") ||
      className.includes("message-row") ||
      className.includes("chat-stream") ||
      className.includes("agent-bubble") ||
      className.includes("message-stream") ||
      className.includes("messages-stream") ||
      className.includes("conversation-panel") ||
      className.includes("chat-container") ||
      className.includes("chat-history") ||
      className.includes("step-item") ||
      className.includes("bubble") ||
      className.includes("tool-batch") ||
      className.includes("prose")
    ) {
      return true;
    }

    if (el.closest && (
      el.closest(".google-symbols") ||
      el.closest(".material-symbols") ||
      el.closest(".material-icons") ||
      el.closest(".codicon") ||
      el.closest(".markdown-body") ||
      el.closest(".rendered-markdown") ||
      el.closest(".message-stream") ||
      el.closest(".messages-stream") ||
      el.closest(".conversation-panel") ||
      el.closest(".chat-container") ||
      el.closest(".chat-stream") ||
      el.closest(".message-row") ||
      el.closest(".bubble") ||
      el.closest(".agent-thinking-card") ||
      el.closest('[data-testid="message-list"]') ||
      el.closest(".prose")
    )) {
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

    // 忽略大小写查表
    const lower = trimmed.toLowerCase();
    if (lowerDict.has(lower)) {
      const translated = lowerDict.get(lower);
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
      if (parent && parent.closest && (
        parent.closest(".google-symbols") ||
        parent.closest(".material-symbols") ||
        parent.closest(".material-icons") ||
        parent.closest(".codicon")
      )) {
        return;
      }

      // 保护用户自定义的项目名称：如果位于 [data-testid="project-selector-item"] 下的项目名称 span，则跳过文本翻译
      if (parent && parent.tagName === "SPAN" && parent.closest && parent.closest('[data-testid="project-selector-item"]') && !parent.closest("button")) {
        return;
      }

      const original = node.nodeValue;
      if (!original || !original.trim()) return;

      node._agy_translated = true;
      // 如果当前文本与上次翻译的一致，无需重复处理
      if (node._agy_orig === original && node._agy_res === node.nodeValue) {
        return;
      }

      const translated = translateString(original);
      node._agy_orig = original;
      node._agy_res = translated;
      if (translated !== original) {
        node.nodeValue = translated;
      }
      return;
    }

    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node;
      if (!el || !el.tagName || IGNORE_TAGS.has(el.tagName)) return;
      // 快速检查忽略容器，避免遍历无用子树
      if (shouldIgnoreElement(el)) return;

      // 翻译 input / textarea 的 placeholder
      if (el.placeholder) {
        const trPlaceholder = translateString(el.placeholder);
        if (trPlaceholder !== el.placeholder) {
          el.placeholder = trPlaceholder;
          try { el.setAttribute("placeholder", trPlaceholder); } catch (e) {}
        }
      } else if (el.hasAttribute && el.hasAttribute("placeholder")) {
        const attrPh = el.getAttribute("placeholder");
        if (attrPh) {
          const trPh = translateString(attrPh);
          if (trPh !== attrPh) {
            el.setAttribute("placeholder", trPh);
            try { el.placeholder = trPh; } catch (e) {}
          }
        }
      }

      // 翻译 aria-label 提示
      if (el.getAttribute && el.getAttribute("aria-label")) {
        const label = el.getAttribute("aria-label");
        const trLabel = translateString(label);
        if (trLabel !== label) {
          el.setAttribute("aria-label", trLabel);
        }
      }

      // 翻译 aria-description 提示
      if (el.getAttribute && el.getAttribute("aria-description")) {
        const desc = el.getAttribute("aria-description");
        const trDesc = translateString(desc);
        if (trDesc !== desc) {
          el.setAttribute("aria-description", trDesc);
        }
      }

      // 翻译 title 提示
      if (el.title) {
        const trTitle = translateString(el.title);
        if (trTitle !== el.title) {
          el.title = trTitle;
          try { el.setAttribute("title", trTitle); } catch (e) {}
        }
      } else if (el.hasAttribute && el.hasAttribute("title")) {
        const attrTitle = el.getAttribute("title");
        if (attrTitle) {
          const trAttr = translateString(attrTitle);
          if (trAttr !== attrTitle) {
            el.setAttribute("title", trAttr);
            try { el.title = trAttr; } catch (e) {}
          }
        }
      }

      // 翻译各类 tooltip 属性 (react-tooltip / tippy / custom tooltips)
      const tipAttrs = ["data-tooltip-content", "data-tooltip-text", "data-tooltip", "data-tip"];
      for (let i = 0; i < tipAttrs.length; i++) {
        const attr = tipAttrs[i];
        if (el.hasAttribute && el.hasAttribute(attr)) {
          const val = el.getAttribute(attr);
          if (val) {
            const trVal = translateString(val);
            if (trVal !== val) {
              el.setAttribute(attr, trVal);
            }
          }
        }
      }

      // 递归子节点
      for (let child = el.firstChild; child; child = child.nextSibling) {
        translateNode(child);
      }
    }
  }

  // 6. 分片安全扫描（避免单次全量全树遍历卡顿主线程）
  function runLocalization(target) {
    const root = target || (document.body ? document.body : null);
    if (!root || (typeof document.visibilityState === "string" && document.visibilityState === "hidden")) return;

    const queue = [root];
    const maxNodesPerBatch = 250;

    function processBatch() {
      let count = 0;
      while (queue.length > 0 && count < maxNodesPerBatch) {
        const curr = queue.shift();
        count++;
        if (!curr) continue;
        if (curr.nodeType === Node.TEXT_NODE) {
          translateNode(curr);
        } else if (curr.nodeType === Node.ELEMENT_NODE) {
          if (IGNORE_TAGS.has(curr.tagName) || shouldIgnoreElement(curr)) continue;
          translateNode(curr);
          for (let child = curr.firstChild; child; child = child.nextSibling) {
            queue.push(child);
          }
        }
      }
      if (queue.length > 0) {
        if (typeof requestIdleCallback === "function") {
          requestIdleCallback(processBatch, { timeout: 100 });
        } else {
          setTimeout(processBatch, 16);
        }
      }
    }
    processBatch();
  }

  // 监听动态 DOM 变动（带熔断保护与防重入机制）
  let pendingMutations = [];
  let timer = null;
  const MAX_MUTATIONS_BATCH = 300;

  const observer = new MutationObserver((mutations) => {
    for (let i = 0; i < mutations.length; i++) {
      if (pendingMutations.length >= MAX_MUTATIONS_BATCH) break;
      pendingMutations.push(mutations[i]);
    }
    if (timer) return;
    timer = requestAnimationFrame(() => {
      timer = null;
      const batch = pendingMutations;
      pendingMutations = [];
      if (!batch.length) return;

      // 暂停监听以杜绝 DOM 修改引发的连锁级联事件
      observer.disconnect();
      try {
        for (let i = 0; i < batch.length; i++) {
          const m = batch[i];
          if (m.type === "childList") {
            for (let j = 0; j < m.addedNodes.length; j++) {
              translateNode(m.addedNodes[j]);
            }
          } else if (m.type === "characterData") {
            translateNode(m.target);
          } else if (m.type === "attributes") {
            const attr = m.attributeName;
            const el = m.target;
            if (el && el.getAttribute && !shouldIgnoreElement(el)) {
              const current = el.getAttribute(attr);
              if (current) {
                const tr = translateString(current);
                if (tr !== current) {
                  el.setAttribute(attr, tr);
                  if (attr === "placeholder") el.placeholder = tr;
                  if (attr === "title") el.title = tr;
                }
              }
            }
          }
        }
      } finally {
        if (document.body) {
          observer.observe(document.body, observeConfig);
        }
      }
    });
  });

  const observeConfig = {
    childList: true,
    subtree: true,
    characterData: true,
    attributes: true,
    attributeFilter: [
      "aria-label",
      "aria-description",
      "placeholder",
      "title",
      "data-tooltip-content",
      "data-tooltip-text",
      "data-tooltip",
      "data-tip"
    ]
  };

  // 启动观察器
  function init() {
    runLocalization();
    if (document.body) {
      observer.observe(document.body, observeConfig);
    } else {
      document.addEventListener("DOMContentLoaded", () => {
        runLocalization();
        observer.observe(document.body, observeConfig);
      });
    }
    // 周期空闲扫描兜底（改为 30s 兜底，避免 3s 高频空转卡顿）
    setInterval(() => {
      if (typeof requestIdleCallback === "function") {
        requestIdleCallback(() => runLocalization(), { timeout: 1000 });
      } else {
        runLocalization();
      }
    }, 30000);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
