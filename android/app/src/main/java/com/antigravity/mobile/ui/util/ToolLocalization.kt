package com.antigravity.mobile.ui.util

object ToolLocalization {
    fun localizedName(toolName: String?): String {
        val raw = toolName?.trim().orEmpty()
        if (raw.isBlank()) return "工具操作".loc

        return when (raw.lowercase()) {
            // 命令与任务
            "run_command" -> "运行终端命令".loc
            "manage_task" -> "管理后台任务".loc
            "schedule" -> "定时调度".loc
            "shell_command", "command" -> "运行命令".loc

            // 文件与代码
            "view_file" -> "查看文件".loc
            "write_to_file" -> "写入文件".loc
            "replace_file_content" -> "编辑文件".loc
            "edit_file" -> "编辑文件".loc
            "create_file" -> "创建文件".loc
            "delete_file" -> "删除文件".loc
            "read_file" -> "读取文件".loc
            "list_dir", "list_directory" -> "浏览目录".loc
            "search_code", "grep_search" -> "搜索代码".loc
            "file_search", "find_by_name" -> "搜索文件".loc

            // 网络与搜索
            "search_web" -> "搜索网络".loc
            "read_url_content" -> "读取网页".loc

            // 子代理与协作
            "invoke_subagent" -> "调用子代理".loc
            "define_subagent" -> "定义子代理".loc
            "manage_subagents" -> "管理子代理".loc
            "send_message" -> "发送消息".loc
            "browser_subagent" -> "浏览器代理".loc

            // 人机交互与生成
            "ask_question" -> "询问用户".loc
            "generate_image" -> "生成图片".loc

            // MCP 协议工具
            "call_mcp_tool" -> "调用 MCP 工具".loc
            "list_resources" -> "列出 MCP 资源".loc
            "read_resource" -> "读取 MCP 资源".loc

            // 通用兜底
            "thinking" -> "思考中".loc
            "tool_call" -> "工具操作".loc
            "action" -> "操作".loc

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
