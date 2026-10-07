package com.antigravity.mobile.ui.util

object ToolLocalization {
    fun localizedName(toolName: String?): String {
        val raw = toolName?.trim().orEmpty()
        if (raw.isBlank()) return "工具操作"

        return when (raw.lowercase()) {
            // 命令与任务
            "run_command" -> "运行终端命令"
            "manage_task" -> "管理后台任务"
            "schedule" -> "定时调度"
            "shell_command", "command" -> "运行命令"

            // 文件与代码
            "view_file" -> "查看文件"
            "write_to_file" -> "写入文件"
            "replace_file_content" -> "编辑文件"
            "edit_file" -> "编辑文件"
            "create_file" -> "创建文件"
            "delete_file" -> "删除文件"
            "read_file" -> "读取文件"
            "list_dir", "list_directory" -> "浏览目录"
            "search_code", "grep_search" -> "搜索代码"
            "file_search", "find_by_name" -> "搜索文件"

            // 网络与搜索
            "search_web" -> "搜索网络"
            "read_url_content" -> "读取网页"

            // 子代理与协作
            "invoke_subagent" -> "调用子代理"
            "define_subagent" -> "定义子代理"
            "manage_subagents" -> "管理子代理"
            "send_message" -> "发送消息"
            "browser_subagent" -> "浏览器代理"

            // 人机交互与生成
            "ask_question" -> "询问用户"
            "generate_image" -> "生成图片"

            // MCP 协议工具
            "call_mcp_tool" -> "调用 MCP 工具"
            "list_resources" -> "列出 MCP 资源"
            "read_resource" -> "读取 MCP 资源"

            // 通用兜底
            "thinking" -> "思考中"
            "tool_call" -> "工具操作"
            "action" -> "操作"

            else -> {
                if (raw.startsWith("mcp_")) {
                    "MCP: ${raw.removePrefix("mcp_")}"
                } else {
                    raw
                }
            }
        }
    }
}
