package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable
import org.json.JSONObject

@Serializable
data class McpServerSpec(
    val serverName: String = "",
    val command: String? = null,
    val args: List<String>? = null,
    val env: Map<String, String>? = null
)

@Serializable
data class McpServerInfoDetail(
    val name: String? = null,
    val version: String? = null
)

data class McpToolParamInfo(
    val name: String,
    val type: String,
    val description: String,
    val isRequired: Boolean,
    val defaultValue: String?
)

@Serializable
data class McpToolDefinition(
    val name: String = "",
    val description: String? = null,
    val serverName: String? = null,
    val jsonSchemaString: String? = null
) {
    val id: String get() = "${serverName.orEmpty()}_$name"

    fun parseParameters(): List<McpToolParamInfo> {
        val schemaStr = jsonSchemaString ?: return emptyList()
        return try {
            val json = JSONObject(schemaStr)
            val reqArray = json.optJSONArray("required")
            val requiredSet = mutableSetOf<String>()
            if (reqArray != null) {
                for (i in 0 until reqArray.length()) {
                    requiredSet.add(reqArray.optString(i))
                }
            }
            val propsObj = json.optJSONObject("properties") ?: return emptyList()
            val list = mutableListOf<McpToolParamInfo>()
            val keys = propsObj.keys()
            while (keys.hasNext()) {
                val key = keys.next()
                val prop = propsObj.optJSONObject(key)
                if (prop != null) {
                    val type = prop.optString("type", "any")
                    val desc = prop.optString("description", "")
                    val defVal = if (prop.has("default") && !prop.isNull("default")) prop.opt("default")?.toString() else null
                    list.add(
                        McpToolParamInfo(
                            name = key,
                            type = type,
                            description = desc,
                            isRequired = requiredSet.contains(key),
                            defaultValue = defVal
                        )
                    )
                }
            }
            list.sortedWith(compareBy({ !it.isRequired }, { it.name }))
        } catch (_: Exception) {
            emptyList()
        }
    }
}

@Serializable
data class McpServerState(
    val spec: McpServerSpec = McpServerSpec(),
    val status: String = "",
    val tools: List<McpToolDefinition>? = null,
    val serverInfo: McpServerInfoDetail? = null,
    val instructions: String? = null,
    val toolErrors: List<String>? = null
) {
    val isReady: Boolean get() = status.contains("READY")
    val statusTitle: String get() = when {
        status.contains("READY") -> "就绪"
        status.contains("STARTING") -> "启动中"
        status.contains("ERROR") -> "异常"
        status.contains("STOPPED") -> "已停止"
        else -> status
    }
    val toolCount: Int get() = tools?.size ?: 0
}

@Serializable
data class GetMcpServerStatesResponse(
    val states: List<McpServerState>? = null
)

@Serializable
data class SkillInfo(
    val name: String = "",
    val description: String? = null,
    val path: String? = null,
    val content: String? = null
) {
    val isBuiltin: Boolean get() = path?.contains("builtin") == true
    val locationCategory: String get() = when {
        isBuiltin -> "内置技能"
        path?.contains(".gemini/config") == true -> "用户技能"
        else -> "工作区技能"
    }
}

@Serializable
data class GetAllSkillsResponse(
    val skills: List<SkillInfo>? = null
)
