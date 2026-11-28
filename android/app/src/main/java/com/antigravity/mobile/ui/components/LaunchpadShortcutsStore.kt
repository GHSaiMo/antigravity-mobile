package com.antigravity.mobile.ui.components

import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AddCircleOutline
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.ChevronLeft
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.LaunchpadShortcutItem
import com.antigravity.mobile.data.model.LaunchpadShortcutsResponse
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.data.model.WorkflowTemplateRecord
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

data class LaunchpadShortcutsState(
    val customized: Boolean = false,
    val items: List<LaunchpadShortcutItem> = emptyList()
)

enum class LaunchpadPinResult {
    ADDED,
    ALREADY_PINNED,
    FULL
}

/**
 * 新建对话 2×2 快捷指令本地缓存 + Go 网关 `/gateway/launchpad-shortcuts` 跨端同步仓库。
 */
object LaunchpadShortcutsStore {
    const val MAX_SLOTS = 4
    private const val PREF_NAME = "mgy_launchpad_shortcuts_v1"
    private const val KEY_PAYLOAD = "payload_json"

    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    @Serializable
    private data class CachedPayload(
        val customized: Boolean = false,
        val items: List<LaunchpadShortcutItem> = emptyList()
    )

    private val _state = MutableStateFlow(LaunchpadShortcutsState())
    val state: StateFlow<LaunchpadShortcutsState> = _state.asStateFlow()

    @Volatile
    private var loadedFromDisk = false

    fun ensureLoaded(context: Context): LaunchpadShortcutsState {
        if (!loadedFromDisk) {
            synchronized(this) {
                if (!loadedFromDisk) {
                    val prefs = context.applicationContext.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
                    val raw = prefs.getString(KEY_PAYLOAD, null)
                    if (!raw.isNullOrBlank()) {
                        runCatching { json.decodeFromString<CachedPayload>(raw) }.onSuccess { cached ->
                            val clean = cached.items.map { it.normalized() }.take(MAX_SLOTS)
                            _state.value = LaunchpadShortcutsState(
                                customized = cached.customized && clean.isNotEmpty(),
                                items = if (cached.customized && clean.isNotEmpty()) clean else emptyList()
                            )
                        }
                    }
                    loadedFromDisk = true
                }
            }
        }
        return _state.value
    }

    private fun saveCache(context: Context, next: LaunchpadShortcutsState) {
        val prefs = context.applicationContext.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        val encoded = json.encodeToString(CachedPayload(next.customized, next.items))
        prefs.edit().putString(KEY_PAYLOAD, encoded).apply()
    }

    private fun resolveApiClient(context: Context): ApiClient {
        return ApiClient.defaultInstance ?: ApiClient(
            context.applicationContext,
            PreferencesManager(context.applicationContext)
        )
    }

    fun isPinned(kind: String, ref: String): Boolean {
        val targetId = LaunchpadShortcutItem(kind = kind, ref = ref).stableId()
        return _state.value.items.any { it.stableId() == targetId }
    }

    suspend fun refresh(context: Context) {
        ensureLoaded(context)
        resolveApiClient(context).fetchLaunchpadShortcuts().onSuccess { resp: LaunchpadShortcutsResponse ->
            val clean = resp.items.map { it.normalized() }.take(MAX_SLOTS)
            val effectiveCustom = resp.customized && clean.isNotEmpty()
            val next = LaunchpadShortcutsState(
                customized = effectiveCustom,
                items = if (effectiveCustom) clean else emptyList()
            )
            if (next != _state.value) {
                _state.value = next
                saveCache(context, next)
            }
        }
    }

    suspend fun applyAndSync(context: Context, raw: List<LaunchpadShortcutItem>, customized: Boolean) {
        val dedup = mutableListOf<LaunchpadShortcutItem>()
        val seen = mutableSetOf<String>()
        for (item in raw) {
            val norm = item.normalized()
            if (norm.ref.isBlank()) continue
            if (seen.add(norm.stableId())) {
                dedup.add(norm)
                if (dedup.size == MAX_SLOTS) break
            }
        }
        val effectiveCustom = customized && dedup.isNotEmpty()
        val next = LaunchpadShortcutsState(
            customized = effectiveCustom,
            items = if (effectiveCustom) dedup else emptyList()
        )
        _state.value = next
        saveCache(context, next)

        val api = resolveApiClient(context)
        val result = if (effectiveCustom) {
            api.saveLaunchpadShortcuts(next.items, true)
        } else {
            api.resetLaunchpadShortcuts()
        }
        result.onSuccess { resp ->
            val serverItems = resp.items.map { it.normalized() }.take(MAX_SLOTS)
            val serverCustom = resp.customized && serverItems.isNotEmpty()
            val synced = LaunchpadShortcutsState(
                customized = serverCustom,
                items = if (serverCustom) serverItems else emptyList()
            )
            if (synced != _state.value) {
                _state.value = synced
                saveCache(context, synced)
            }
        }
    }

    suspend fun pin(context: Context, item: LaunchpadShortcutItem): LaunchpadPinResult {
        ensureLoaded(context)
        val norm = item.normalized()
        val current = _state.value.items
        if (current.any { it.stableId() == norm.stableId() }) return LaunchpadPinResult.ALREADY_PINNED
        if (current.size >= MAX_SLOTS) return LaunchpadPinResult.FULL
        applyAndSync(context, current + norm, customized = true)
        return LaunchpadPinResult.ADDED
    }

    suspend fun unpin(context: Context, kind: String, ref: String) {
        ensureLoaded(context)
        val targetId = LaunchpadShortcutItem(kind = kind, ref = ref).stableId()
        val next = _state.value.items.filterNot { it.stableId() == targetId }
        applyAndSync(context, next, customized = next.isNotEmpty())
    }

    suspend fun removeAt(context: Context, index: Int) {
        ensureLoaded(context)
        val current = _state.value.items
        if (index !in current.indices) return
        val next = current.toMutableList().apply { removeAt(index) }
        applyAndSync(context, next, customized = next.isNotEmpty())
    }

    suspend fun replaceAt(context: Context, index: Int, item: LaunchpadShortcutItem) {
        ensureLoaded(context)
        val norm = item.normalized()
        val current = _state.value.items
        if (index !in current.indices) return
        val next = current.filterNot { it.stableId() == norm.stableId() }.toMutableList()
        val safeIdx = index.coerceIn(0, next.size)
        if (next.size >= MAX_SLOTS && safeIdx < next.size) {
            next[safeIdx] = norm
        } else if (next.size < MAX_SLOTS) {
            next.add(safeIdx, norm)
        }
        applyAndSync(context, next.take(MAX_SLOTS), customized = next.isNotEmpty())
    }

    suspend fun move(context: Context, fromIndex: Int, toIndex: Int) {
        ensureLoaded(context)
        val current = _state.value.items
        if (fromIndex !in current.indices || toIndex !in current.indices || fromIndex == toIndex) return
        val next = current.toMutableList()
        val moved = next.removeAt(fromIndex)
        next.add(toIndex, moved)
        applyAndSync(context, next, customized = true)
    }

    suspend fun resetToSmartDefaults(context: Context) {
        applyAndSync(context, emptyList(), customized = false)
    }
}

private enum class ConfigPickerTab {
    WORKFLOW, SLASH, SKILL, PROMPT
}

private data class CandidateEntry(
    val shortcut: LaunchpadShortcutItem,
    val svgKey: String,
    val tint: Color,
    val title: String,
    val subtitle: String,
    val badge: String? = null
)

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun LaunchpadShortcutConfiguratorSheet(
    workflowTemplates: List<WorkflowTemplateRecord>,
    slashCommands: List<SlashCommandOption>,
    pendingPinItem: LaunchpadShortcutItem?,
    onDismiss: () -> Unit,
    onOpenWorkflowManager: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val scope = rememberCoroutineScope()
    val currentLangCode by L10n.currentLangFlow.collectAsState()
    val currentLang = remember(currentLangCode) { L10n.currentLang }
    val storeState by LaunchpadShortcutsStore.state.collectAsState()

    var activeTab by remember { mutableStateOf(ConfigPickerTab.WORKFLOW) }
    var searchQuery by remember { mutableStateOf("") }
    var activePending by remember(pendingPinItem) { mutableStateOf(pendingPinItem) }

    LaunchedEffect(Unit) {
        LaunchpadShortcutsStore.refresh(context)
    }

    fun t3(zh: String, en: String, ja: String): String = when (currentLang) {
        L10n.AppLang.EN -> en
        L10n.AppLang.JA -> ja
        L10n.AppLang.ZH -> zh
    }

    fun resolveDisplay(item: LaunchpadShortcutItem): Triple<String, Color, Pair<String, String>> {
        val norm = item.normalized()
        return when (norm.kind) {
            "workflow" -> {
                val wf = workflowTemplates.firstOrNull { it.id == norm.ref }
                if (wf != null) {
                    Triple(
                        "wf:${WorkflowStyleCatalog.normalizeIconKey(wf.icon)}",
                        launchpadSafeTemplateTint(wf.color),
                        wf.title to wf.subtitle.ifBlank { t3("自定义 Workflow", "Custom Workflow", "カスタム Workflow") }
                    )
                } else {
                    val fallbackKey = if ((norm.svgKey ?: "").startsWith("wf:")) norm.svgKey!! else "wf:workflow"
                    Triple(
                        fallbackKey,
                        launchpadSafeTemplateTint(norm.tint ?: "indigo"),
                        (norm.title ?: norm.ref) to (norm.subtitle ?: t3("Workflow 模板", "Workflow template", "Workflow テンプレート"))
                    )
                }
            }
            "slash", "skill" -> {
                val core = LAUNCHPAD_CORE_SLASH_POOL.firstOrNull { it.name == norm.ref }
                if (core != null) {
                    Triple(core.svgKey, core.tint, t3(core.titleZh, core.titleEn, core.titleJa) to t3(core.subZh, core.subEn, core.subJa))
                } else {
                    val live = slashCommands.firstOrNull { it.name.equals(norm.ref, ignoreCase = true) }
                    if (live != null) {
                        val sub = live.description.trim().ifEmpty { t3("已安装技能指令", "Installed skill", "スキルコマンド") }
                        Triple("slash_skill", LP_TEAL, "/${live.name}" to sub)
                    } else {
                        Triple(
                            norm.svgKey ?: "slash_skill",
                            launchpadTintByName(norm.tint ?: "teal"),
                            (norm.title ?: "/${norm.ref}") to (norm.subtitle ?: t3("斜杠 / 技能", "Slash / Skill", "コマンド"))
                        )
                    }
                }
            }
            "prompt" -> {
                val spec = LAUNCHPAD_ALL_PROMPT_POOL.firstOrNull { it.id == norm.ref }
                if (spec != null) {
                    Triple(spec.svgKey, spec.tint, t3(spec.titleZh, spec.titleEn, spec.titleJa) to t3(spec.subZh, spec.subEn, spec.subJa))
                } else {
                    Triple(
                        norm.svgKey ?: "prompt_draft",
                        launchpadTintByName(norm.tint ?: "blue"),
                        (norm.title ?: norm.ref) to (norm.subtitle ?: t3("场景提示词模板", "Prompt template", "テンプレート"))
                    )
                }
            }
            else -> Triple("slash_skill", LP_INDIGO, (norm.title ?: norm.ref) to (norm.subtitle ?: ""))
        }
    }

    fun toggleCandidate(item: LaunchpadShortcutItem) {
        scope.launch {
            if (LaunchpadShortcutsStore.isPinned(item.kind, item.ref)) {
                haptic.light()
                LaunchpadShortcutsStore.unpin(context, item.kind, item.ref)
            } else {
                when (LaunchpadShortcutsStore.pin(context, item)) {
                    LaunchpadPinResult.ADDED -> {
                        haptic.light()
                        val count = LaunchpadShortcutsStore.state.value.items.size
                        Toast.makeText(context, t3("已固定到 2×2 ($count/4)", "Pinned to 2×2 ($count/4)", "2×2 に固定 ($count/4)"), Toast.LENGTH_SHORT).show()
                    }
                    LaunchpadPinResult.ALREADY_PINNED -> Unit
                    LaunchpadPinResult.FULL -> {
                        haptic.medium()
                        Toast.makeText(context, t3("已选满 4 项，请先在上方移除一项", "4/4 slots full — remove one above first", "4枠が満枠です。上から1つ削除してください"), Toast.LENGTH_SHORT).show()
                    }
                }
            }
        }
    }

    val localCounts = remember(context) { SlashUsageLocalStore.counts(context) }
    val gatewayByName = remember(slashCommands) { slashCommands.associateBy { it.name.lowercase() } }
    val q = searchQuery.trim().lowercase()

    val candidates = remember(activeTab, q, workflowTemplates, slashCommands, currentLang) {
        when (activeTab) {
            ConfigPickerTab.WORKFLOW -> workflowTemplates
                .filter { q.isEmpty() || it.title.lowercase().contains(q) || it.subtitle.lowercase().contains(q) }
                .map { tpl ->
                    val wfIconKey = "wf:${WorkflowStyleCatalog.normalizeIconKey(tpl.icon)}"
                    CandidateEntry(
                        shortcut = LaunchpadShortcutItem(
                            kind = "workflow",
                            ref = tpl.id,
                            title = tpl.title,
                            subtitle = tpl.subtitle,
                            svgKey = wfIconKey,
                            tint = launchpadSafeTintName(tpl.color)
                        ),
                        svgKey = wfIconKey,
                        tint = launchpadSafeTemplateTint(tpl.color),
                        title = tpl.title,
                        subtitle = tpl.subtitle.ifBlank { tpl.templatePrompt },
                        badge = if (tpl.inQuickMenu) t3("快选", "Quick", "クイック") else null
                    )
                }

            ConfigPickerTab.SLASH -> {
                val coreNames = LAUNCHPAD_CORE_SLASH_POOL.map { it.name }.toSet()
                val coreItems = LAUNCHPAD_CORE_SLASH_POOL
                    .filter { spec ->
                        q.isEmpty() || spec.name.contains(q) || spec.titleZh.lowercase().contains(q) ||
                            spec.titleEn.lowercase().contains(q) || spec.subZh.lowercase().contains(q)
                    }
                    .map { spec ->
                        val count = maxOf(gatewayByName[spec.name]?.usageCount ?: 0, localCounts[spec.name] ?: 0)
                        val title = t3(spec.titleZh, spec.titleEn, spec.titleJa)
                        val sub = t3(spec.subZh, spec.subEn, spec.subJa)
                        CandidateEntry(
                            shortcut = LaunchpadShortcutItem("slash", spec.name, title, sub, spec.svgKey, spec.tintName),
                            svgKey = spec.svgKey,
                            tint = spec.tint,
                            title = title,
                            subtitle = sub,
                            badge = if (count > 0) t3("用过 $count 次", "${count}×", "${count}回") else null
                        )
                    }
                val extraSystem = slashCommands
                    .filter { cmd ->
                        val lower = cmd.name.lowercase()
                        cmd.kind.equals("system", ignoreCase = true) && lower !in coreNames && lower !in LAUNCHPAD_MID_CHAT_EXCLUDED &&
                            (q.isEmpty() || lower.contains(q) || cmd.description.lowercase().contains(q))
                    }
                    .map { cmd ->
                        val count = maxOf(cmd.usageCount, localCounts[cmd.name.lowercase()] ?: 0)
                        CandidateEntry(
                            shortcut = LaunchpadShortcutItem("slash", cmd.name, "/${cmd.name}", cmd.description, "slash_skill", "blue"),
                            svgKey = "slash_skill",
                            tint = LP_BLUE,
                            title = "/${cmd.name}",
                            subtitle = cmd.description.ifBlank { t3("系统斜杠指令", "System slash command", "システムコマンド") },
                            badge = if (count > 0) t3("用过 $count 次", "${count}×", "${count}回") else null
                        )
                    }
                coreItems + extraSystem
            }

            ConfigPickerTab.SKILL -> {
                val slashExcludedNames = LAUNCHPAD_CORE_SLASH_POOL.map { it.name }.toSet() + LAUNCHPAD_MID_CHAT_EXCLUDED
                slashCommands
                    .filter { cmd ->
                        val lower = cmd.name.lowercase()
                        !cmd.kind.equals("system", ignoreCase = true) && lower !in slashExcludedNames
                    }
                    .filter { cmd ->
                        q.isEmpty() || cmd.name.lowercase().contains(q) || cmd.title.lowercase().contains(q) || cmd.description.lowercase().contains(q)
                    }
                    .sortedWith(
                        compareByDescending<SlashCommandOption> { maxOf(it.usageCount, localCounts[it.name.lowercase()] ?: 0) }
                            .thenBy { it.name.lowercase() }
                    )
                    .map { cmd ->
                        val count = maxOf(cmd.usageCount, localCounts[cmd.name.lowercase()] ?: 0)
                        CandidateEntry(
                            shortcut = LaunchpadShortcutItem("skill", cmd.name, "/${cmd.name}", cmd.description, "slash_skill", "teal"),
                            svgKey = "slash_skill",
                            tint = LP_TEAL,
                            title = "/${cmd.name}",
                            subtitle = cmd.description.ifBlank { t3("已安装技能", "Installed skill", "スキル") },
                            badge = if (count > 0) t3("用过 $count 次", "${count}×", "${count}回") else null
                        )
                    }
            }

            ConfigPickerTab.PROMPT -> LAUNCHPAD_ALL_PROMPT_POOL
                .filter { spec ->
                    q.isEmpty() || spec.titleZh.lowercase().contains(q) || spec.titleEn.lowercase().contains(q) || spec.subZh.lowercase().contains(q)
                }
                .map { spec ->
                    val title = t3(spec.titleZh, spec.titleEn, spec.titleJa)
                    val sub = t3(spec.subZh, spec.subEn, spec.subJa)
                    val groupBadge = if (spec.id.startsWith("pure_")) t3("纯对话", "Chat", "チャット") else t3("代码区", "Code", "コード")
                    CandidateEntry(
                        shortcut = LaunchpadShortcutItem("prompt", spec.id, title, sub, spec.svgKey, spec.tintName),
                        svgKey = spec.svgKey,
                        tint = spec.tint,
                        title = title,
                        subtitle = sub,
                        badge = groupBadge
                    )
                }
        }
    }

    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = colors.background,
        dragHandle = { BottomSheetDefaults.DragHandle(color = colors.border) }
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight(0.88f)
                .padding(horizontal = 16.dp)
        ) {
            // Header
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = t3("自定义 2×2 快捷指令", "Customize 2×2 Shortcuts", "2×2 ショートカット設定"),
                    color = colors.textPrimary,
                    fontSize = 17.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f)
                )
                TextButton(onClick = onDismiss) {
                    Text(t3("完成", "Done", "完了"), color = colors.accentIndigo, fontWeight = FontWeight.SemiBold)
                }
            }

            Spacer(modifier = Modifier.height(6.dp))

            // Optional swap banner when triggered via long-press at 4/4 capacity
            activePending?.let { pending ->
                val (svgKey, tint, titleSub) = resolveDisplay(pending)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(13.dp))
                        .background(LP_AMBER.copy(alpha = 0.12f))
                        .border(1.dp, LP_AMBER.copy(alpha = 0.35f), RoundedCornerShape(13.dp))
                        .padding(11.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(26.dp)
                            .clip(RoundedCornerShape(7.dp))
                            .background(tint.copy(alpha = 0.16f)),
                        contentAlignment = Alignment.Center
                    ) {
                        LaunchpadSvgIcon(svgKey = svgKey, tint = tint, size = 14.dp)
                    }
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = t3("2×2 已满，点选下方槽位替换为：", "2×2 full — tap a slot below to replace with:", "満枠です。下の枠をタップして置換："),
                            color = colors.textSecondary,
                            fontSize = 11.5.sp
                        )
                        Text(
                            text = titleSub.first,
                            color = colors.textPrimary,
                            fontSize = 13.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                    Text(
                        text = t3("取消", "Cancel", "取消"),
                        color = colors.textSecondary,
                        fontSize = 12.5.sp,
                        modifier = Modifier
                            .clickable { activePending = null }
                            .padding(horizontal = 6.dp, vertical = 4.dp)
                    )
                }
                Spacer(modifier = Modifier.height(10.dp))
            }

            // 2x2 Slot Preview Section
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = t3("当前已固定 (${storeState.items.size}/4)", "Pinned Slots (${storeState.items.size}/4)", "固定済み (${storeState.items.size}/4)"),
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f)
                )
                if (storeState.customized) {
                    Text(
                        text = t3("恢复智能推荐", "Smart Default", "おすすめに戻す"),
                        color = colors.accentIndigo,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                        modifier = Modifier
                            .clickable {
                                haptic.light()
                                scope.launch { LaunchpadShortcutsStore.resetToSmartDefaults(context) }
                            }
                            .padding(horizontal = 4.dp, vertical = 2.dp)
                    )
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            val isSwapMode = activePending != null
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                for (row in 0..1) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        for (col in 0..1) {
                            val idx = row * 2 + col
                            if (idx < storeState.items.size) {
                                val item = storeState.items[idx]
                                val (svgKey, tint, titleSub) = resolveDisplay(item)
                                Column(
                                    modifier = Modifier
                                        .weight(1f)
                                        .clip(RoundedCornerShape(12.dp))
                                        .background(colors.surface)
                                        .border(
                                            width = if (isSwapMode) 1.2.dp else 0.7.dp,
                                            color = if (isSwapMode) LP_AMBER.copy(alpha = 0.6f) else colors.border,
                                            shape = RoundedCornerShape(12.dp)
                                        )
                                        .clickable(enabled = isSwapMode) {
                                            val pending = activePending ?: return@clickable
                                            haptic.medium()
                                            scope.launch {
                                                LaunchpadShortcutsStore.replaceAt(context, idx, pending)
                                                activePending = null
                                            }
                                        }
                                        .padding(horizontal = 10.dp, vertical = 9.dp),
                                    verticalArrangement = Arrangement.spacedBy(5.dp)
                                ) {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                                    ) {
                                        Box(
                                            modifier = Modifier
                                                .size(20.dp)
                                                .clip(RoundedCornerShape(5.5.dp))
                                                .background(tint.copy(alpha = 0.14f)),
                                            contentAlignment = Alignment.Center
                                        ) {
                                            LaunchpadSvgIcon(svgKey = svgKey, tint = tint, size = 12.dp)
                                        }
                                        Text(
                                            text = titleSub.first,
                                            color = colors.textPrimary,
                                            fontSize = 12.5.sp,
                                            fontWeight = FontWeight.SemiBold,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis,
                                            modifier = Modifier.weight(1f)
                                        )
                                        if (!isSwapMode) {
                                            Icon(
                                                imageVector = Icons.Default.Close,
                                                contentDescription = null,
                                                tint = colors.textSecondary,
                                                modifier = Modifier
                                                    .size(15.dp)
                                                    .clickable {
                                                        haptic.light()
                                                        scope.launch { LaunchpadShortcutsStore.removeAt(context, idx) }
                                                    }
                                            )
                                        }
                                    }
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                                    ) {
                                        Text(
                                            text = if (isSwapMode) t3("点此替换", "Tap to replace", "タップで置換") else titleSub.second,
                                            color = if (isSwapMode) LP_AMBER else colors.textSecondary,
                                            fontSize = 10.5.sp,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis,
                                            modifier = Modifier.weight(1f)
                                        )
                                        if (!isSwapMode && storeState.items.size > 1) {
                                            if (idx > 0) {
                                                Icon(
                                                    imageVector = Icons.Default.ChevronLeft,
                                                    contentDescription = null,
                                                    tint = colors.textSecondary,
                                                    modifier = Modifier
                                                        .size(16.dp)
                                                        .clip(CircleShape)
                                                        .background(colors.textPrimary.copy(alpha = 0.06f))
                                                        .clickable {
                                                            haptic.light()
                                                            scope.launch { LaunchpadShortcutsStore.move(context, idx, idx - 1) }
                                                        }
                                                )
                                            }
                                            if (idx < storeState.items.size - 1) {
                                                Icon(
                                                    imageVector = Icons.Default.ChevronRight,
                                                    contentDescription = null,
                                                    tint = colors.textSecondary,
                                                    modifier = Modifier
                                                        .size(16.dp)
                                                        .clip(CircleShape)
                                                        .background(colors.textPrimary.copy(alpha = 0.06f))
                                                        .clickable {
                                                            haptic.light()
                                                            scope.launch { LaunchpadShortcutsStore.move(context, idx, idx + 1) }
                                                        }
                                                )
                                            }
                                        }
                                    }
                                }
                            } else {
                                Column(
                                    modifier = Modifier
                                        .weight(1f)
                                        .height(58.dp)
                                        .clip(RoundedCornerShape(12.dp))
                                        .background(colors.surface.copy(alpha = 0.5f))
                                        .border(0.8.dp, colors.border.copy(alpha = 0.7f), RoundedCornerShape(12.dp)),
                                    horizontalAlignment = Alignment.CenterHorizontally,
                                    verticalArrangement = Arrangement.Center
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.Add,
                                        contentDescription = null,
                                        tint = colors.textSecondary.copy(alpha = 0.7f),
                                        modifier = Modifier.size(14.dp)
                                    )
                                    Text(
                                        text = t3("空位 ${idx + 1} · 自动补位", "Slot ${idx + 1} · Auto", "枠 ${idx + 1} · 自動"),
                                        color = colors.textSecondary,
                                        fontSize = 10.5.sp
                                    )
                                }
                            }
                        }
                    }
                }
            }

            Text(
                text = t3("未填满的空位将自动用场景模板与随机斜杠补齐，配置跨端同步。", "Unfilled slots auto-fill with prompt templates & random slash commands, synced across devices.", "空き枠はテンプレートとランダムなコマンドで自動補完され、全端末に同期されます。"),
                color = colors.textSecondary,
                fontSize = 11.5.sp
            )

            Spacer(modifier = Modifier.height(10.dp))

            // 4-Tab Bar
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .padding(3.dp),
                horizontalArrangement = Arrangement.spacedBy(3.dp)
            ) {
                val tabs = listOf(
                    ConfigPickerTab.WORKFLOW to "Workflow",
                    ConfigPickerTab.SLASH to t3("斜杠", "Slash", "スラッシュ"),
                    ConfigPickerTab.SKILL to t3("技能", "Skills", "スキル"),
                    ConfigPickerTab.PROMPT to t3("模板", "Prompts", "テンプレート")
                )
                tabs.forEach { (tab, label) ->
                    val selected = tab == activeTab
                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .clip(RoundedCornerShape(8.dp))
                            .background(if (selected) colors.accentIndigo.copy(alpha = 0.16f) else Color.Transparent)
                            .clickable { activeTab = tab }
                            .padding(vertical = 7.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = label,
                            color = if (selected) colors.accentIndigo else colors.textSecondary,
                            fontSize = 12.5.sp,
                            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
                            maxLines = 1
                        )
                    }
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            // Search Box
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(0.7.dp, colors.border, RoundedCornerShape(10.dp))
                    .padding(horizontal = 11.dp, vertical = 9.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Search,
                    contentDescription = null,
                    tint = colors.textSecondary,
                    modifier = Modifier.size(16.dp)
                )
                Box(modifier = Modifier.weight(1f)) {
                    if (searchQuery.isEmpty()) {
                        Text(
                            text = t3("搜索指令、技能或模板...", "Search commands, skills or prompts...", "検索..."),
                            color = colors.textSecondary,
                            fontSize = 13.5.sp
                        )
                    }
                    BasicTextField(
                        value = searchQuery,
                        onValueChange = { searchQuery = it },
                        singleLine = true,
                        textStyle = TextStyle(color = colors.textPrimary, fontSize = 13.5.sp),
                        cursorBrush = SolidColor(colors.accentIndigo),
                        modifier = Modifier.fillMaxWidth()
                    )
                }
                if (searchQuery.isNotEmpty()) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = null,
                        tint = colors.textSecondary,
                        modifier = Modifier
                            .size(15.dp)
                            .clickable { searchQuery = "" }
                    )
                }
            }

            Spacer(modifier = Modifier.height(8.dp))

            // Candidate List
            LazyColumn(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(7.dp),
                contentPadding = PaddingValues(bottom = 24.dp)
            ) {
                items(candidates, key = { it.shortcut.stableId() }) { entry ->
                    val pinned = storeState.items.any { it.stableId() == entry.shortcut.stableId() }
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .background(colors.surface)
                            .border(
                                width = 0.8.dp,
                                color = if (pinned) colors.accentIndigo.copy(alpha = 0.4f) else colors.border,
                                shape = RoundedCornerShape(12.dp)
                            )
                            .clickable { toggleCandidate(entry.shortcut) }
                            .padding(horizontal = 12.dp, vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(30.dp)
                                .clip(RoundedCornerShape(8.dp))
                                .background(entry.tint.copy(alpha = 0.14f)),
                            contentAlignment = Alignment.Center
                        ) {
                            LaunchpadSvgIcon(svgKey = entry.svgKey, tint = entry.tint, size = 15.dp)
                        }
                        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text(
                                    text = entry.title,
                                    color = colors.textPrimary,
                                    fontSize = 13.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f, fill = false)
                                )
                                entry.badge?.let { b ->
                                    Text(
                                        text = b,
                                        color = LP_TEAL,
                                        fontSize = 10.5.sp,
                                        fontWeight = FontWeight.Medium,
                                        modifier = Modifier
                                            .clip(CircleShape)
                                            .background(LP_TEAL.copy(alpha = 0.12f))
                                            .padding(horizontal = 6.dp, vertical = 1.dp)
                                    )
                                }
                            }
                            Text(
                                text = entry.subtitle,
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        Icon(
                            imageVector = if (pinned) Icons.Default.CheckCircle else Icons.Default.AddCircleOutline,
                            contentDescription = null,
                            tint = if (pinned) colors.accentIndigo else colors.textSecondary.copy(alpha = 0.65f),
                            modifier = Modifier.size(20.dp)
                        )
                    }
                }

                if (activeTab == ConfigPickerTab.WORKFLOW) {
                    item(key = "manage_workflow_btn") {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.surface)
                                .clickable {
                                    onDismiss()
                                    onOpenWorkflowManager()
                                }
                                .padding(vertical = 12.dp),
                            horizontalArrangement = Arrangement.Center,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Icon(
                                imageVector = Icons.Default.Tune,
                                contentDescription = null,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(15.dp)
                            )
                            Spacer(modifier = Modifier.width(6.dp))
                            Text(
                                text = t3("新建或编辑 Workflow 模板", "Create or edit Workflow templates", "Workflow テンプレートを管理"),
                                color = colors.accentIndigo,
                                fontSize = 13.sp,
                                fontWeight = FontWeight.Medium
                            )
                        }
                    }
                }
            }
        }
    }
}
