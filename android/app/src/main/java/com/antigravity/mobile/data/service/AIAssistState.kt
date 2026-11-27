package com.antigravity.mobile.data.service

import com.antigravity.mobile.ui.util.L10n
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.Serializable
import java.util.Locale

@Serializable
data class AIAssistPrefsPayload(
    val titleTranslateEnabled: Boolean = false,
    val titleTranslateLang: String = "",
    val titleTranslateLangName: String = "",
    val stepPlanTranslateEnabled: Boolean = false,
    val focusTranslateEnabled: Boolean = false,
    val nextPromptEnabled: Boolean = false,
    val postCompactRehydrateEnabled: Boolean = false
)

@Serializable
data class AIAssistStatusPayload(
    val prefs: AIAssistPrefsPayload = AIAssistPrefsPayload(),
    val modelId: String = "",
    val modelName: String = "Gemini 3.5 Flash Lite",
    val resolvedLang: String = "",
    val resolvedLangName: String = "",
    val available: Boolean = true,
    val rehydrateHookInstalled: Boolean = false
)

/**
 * Resolves the user's true device/app language for adaptive Smart Translation UI:
 * - English (`en*`): Smart Translation module is hidden.
 * - Chinese (`zh-Hans` / `zh-Hant`) & Japanese (`ja`): native localized menu.
 * - Other non-English languages (`pt-BR`, `ko`, `es`, `fr`, ...): English UI dynamically naming the language.
 */
data class DeviceTitleTargetLanguage(
    val kind: Kind,
    val bcp47Tag: String,
    val englishDisplayName: String
) {
    enum class Kind {
        ENGLISH,
        SIMPLIFIED_CHINESE,
        TRADITIONAL_CHINESE,
        JAPANESE,
        OTHER_NON_ENGLISH
    }

    val isEnglish: Boolean get() = kind == Kind.ENGLISH
    val shouldShowSmartTranslation: Boolean get() = !isEnglish

    companion object {
        fun current(): DeviceTitleTargetLanguage {
            val appPref = L10n.getLanguagePreference()
            when (appPref) {
                "en" -> return DeviceTitleTargetLanguage(Kind.ENGLISH, "en", "English")
                "zh", "zh-CN" -> return DeviceTitleTargetLanguage(Kind.SIMPLIFIED_CHINESE, "zh-Hans", "Simplified Chinese")
                "zh-Hant", "zh-TW", "zh-HK" -> return DeviceTitleTargetLanguage(Kind.TRADITIONAL_CHINESE, "zh-Hant", "Traditional Chinese")
                "ja" -> return DeviceTitleTargetLanguage(Kind.JAPANESE, "ja", "Japanese")
            }

            val locale = Locale.getDefault()
            val rawTag = locale.toLanguageTag().replace('_', '-').trim()
            val lower = rawTag.lowercase(Locale.ROOT)
            if (lower == "en" || lower.startsWith("en-")) {
                return DeviceTitleTargetLanguage(Kind.ENGLISH, "en", "English")
            }
            if (lower.startsWith("zh-hant") || lower.startsWith("zh-tw") || lower.startsWith("zh-hk") || lower.startsWith("zh-mo")) {
                return DeviceTitleTargetLanguage(Kind.TRADITIONAL_CHINESE, "zh-Hant", "Traditional Chinese")
            }
            if (lower == "zh" || lower.startsWith("zh-")) {
                return DeviceTitleTargetLanguage(Kind.SIMPLIFIED_CHINESE, "zh-Hans", "Simplified Chinese")
            }
            if (lower == "ja" || lower.startsWith("ja-")) {
                return DeviceTitleTargetLanguage(Kind.JAPANESE, "ja", "Japanese")
            }

            val display = locale.getDisplayLanguage(Locale.ENGLISH).trim().ifEmpty { rawTag }
            val capitalized = display.replaceFirstChar { if (it.isLowerCase()) it.titlecase(Locale.ENGLISH) else it.toString() }
            return DeviceTitleTargetLanguage(
                kind = Kind.OTHER_NON_ENGLISH,
                bcp47Tag = rawTag.ifEmpty { locale.language },
                englishDisplayName = capitalized
            )
        }
    }
}

object AIAssistCache {
    private val _statusFlow = MutableStateFlow(AIAssistStatusPayload())
    val statusFlow: StateFlow<AIAssistStatusPayload> = _statusFlow.asStateFlow()

    fun update(status: AIAssistStatusPayload) {
        _statusFlow.value = status
    }

    fun updatePrefs(prefs: AIAssistPrefsPayload) {
        _statusFlow.value = _statusFlow.value.copy(prefs = prefs)
    }

    suspend fun refresh(apiClient: ApiClient?): Result<AIAssistStatusPayload> {
        val client = apiClient ?: return Result.failure(IllegalStateException("ApiClient unavailable"))
        return client.fetchAIAssistStatus().onSuccess { update(it) }
    }
}
