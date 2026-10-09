package com.antigravity.mobile.data.model

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** 网关用这些 ID 标记「依赖的 language_server 接口缺失」的功能；改名需与网关 capabilities.go 同步。 */
object GatewayFeature {
    const val SEARCH = "search"
    const val EXPORT = "export"
    const val CHANGES = "changes"
    const val REVERT = "revert"
    const val SLASH = "slash"
}

/**
 * 升级自检结果（/gateway/status 的 compat 字段）。
 *
 * 原则：拿不到、解析不了、网关版本旧没有这个字段、网关无法检测（checked=false）时一律「放行」，
 * 宁可保留入口、出错时再报错，也不误隐藏可用功能。
 */
@Serializable
data class GatewayCompat(
    val version: String? = null,
    val checked: Boolean = false,
    val coreOk: Boolean = true,
    val unavailable: List<String> = emptyList(),
    val missingRpcs: List<String> = emptyList()
) {
    fun isAvailable(featureId: String): Boolean = !(checked && unavailable.contains(featureId))

    /** 基础能力（会话列表/读取/发送/流式/审批）缺失：整个 App 基本不可用，需要升级网关。 */
    val isIncompatible: Boolean get() = checked && !coreOk

    /** 首页顶部提示条文案。 */
    val bannerText: String
        get() {
            val ver = version?.takeIf { it.isNotBlank() }?.let { "（$it）" } ?: ""
            return "当前 Antigravity 版本$ver 与网关不兼容，部分基础功能可能无法使用。请升级网关（mgy）。"
        }

    companion object {
        private val parser = Json { ignoreUnknownKeys = true; coerceInputValues = true }

        /** 从 /gateway/status 响应体取 compat；缺失或格式不对返回放行的默认值。 */
        fun fromStatusJson(body: String): GatewayCompat {
            return try {
                val root = parser.parseToJsonElement(body)
                val compat = (root as? kotlinx.serialization.json.JsonObject)?.get("compat") ?: return GatewayCompat()
                parser.decodeFromJsonElement(GatewayCompat.serializer(), compat)
            } catch (_: Exception) {
                GatewayCompat()
            }
        }
    }
}

/** 全应用共享的最新自检结果；首页刷新会话列表时更新，其余界面只读。 */
object GatewayCompatStore {
    private val _state = MutableStateFlow(GatewayCompat())
    val state: StateFlow<GatewayCompat> = _state.asStateFlow()

    fun update(value: GatewayCompat) { _state.value = value }
    fun isAvailable(featureId: String): Boolean = _state.value.isAvailable(featureId)
    internal fun resetForTest() { _state.value = GatewayCompat() }
}
