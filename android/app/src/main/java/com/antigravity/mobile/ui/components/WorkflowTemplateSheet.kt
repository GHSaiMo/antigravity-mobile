package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import android.content.Context
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.DragHandle
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.zIndex
import com.antigravity.mobile.data.model.WorkflowTemplateDefaults
import com.antigravity.mobile.data.model.WorkflowTemplateRecord
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityColors
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlin.math.roundToInt

internal data class WorkflowIconOption(
    val key: String,
    val pathData: String,
    val label: String
)

internal data class WorkflowColorOption(
    val key: String,
    val label: String
)

/**
 * 统一自绘 24×24 矢量图标路径（与 iOS / Web 端像素级对齐，替代 star 为 Xcode 编译锤子，Bug 对齐昆虫甲虫）。
 */
internal object WorkflowStyleCatalog {
    val icons = listOf(
        WorkflowIconOption(
            "workflow",
            "M 6 3.3 C 7.5 3.3 8.7 4.5 8.7 6 C 8.7 7.5 7.5 8.7 6 8.7 C 4.5 8.7 3.3 7.5 3.3 6 C 3.3 4.5 4.5 3.3 6 3.3 Z M 18 3.3 C 19.5 3.3 20.7 4.5 20.7 6 C 20.7 7.5 19.5 8.7 18 8.7 C 16.5 8.7 15.3 7.5 15.3 6 C 15.3 4.5 16.5 3.3 18 3.3 Z M 12 15.3 C 13.5 15.3 14.7 16.5 14.7 18 C 14.7 19.5 13.5 20.7 12 20.7 C 10.5 20.7 9.3 19.5 9.3 18 C 9.3 16.5 10.5 15.3 12 15.3 Z M 8.7 6 L 15.3 6 M 7.3 8.4 L 10.7 15.6 M 16.7 8.4 L 13.3 15.6",
            "工作流".loc
        ),
        WorkflowIconOption(
            "security",
            "M 12 2.5 L 19.5 5.8 L 19.5 11.5 C 19.5 16.5 16.2 20.0 12 21.5 C 7.8 20.0 4.5 16.5 4.5 11.5 L 4.5 5.8 L 12 2.5 Z M 9 12 L 11.2 14.2 L 15.4 9.8",
            "安全审查".loc
        ),
        WorkflowIconOption(
            "bug",
            "M 9 7.5 L 9 6.5 C 9 4.84 10.34 3.5 12 3.5 C 13.66 3.5 15 4.84 15 6.5 L 15 7.5 M 8.2 2.5 L 10 4.2 M 15.8 2.5 L 14 4.2 M 8.5 7.5 L 15.5 7.5 C 16.88 7.5 18 8.62 18 10 L 18 14 C 18 17.31 15.31 20 12 20 C 8.69 20 6 17.31 6 14 L 6 10 C 6 8.62 7.12 7.5 8.5 7.5 Z M 12 11 L 12 20 M 6 9.5 L 3.2 7.5 M 6 13.2 L 2.5 13.2 M 6.4 16.8 L 3.2 19.2 M 18 9.5 L 20.8 7.5 M 18 13.2 L 21.5 13.2 M 17.6 16.8 L 20.8 19.2",
            "测试修复".loc
        ),
        WorkflowIconOption(
            "search",
            "M 10.8 4 C 14.56 4 17.6 7.04 17.6 10.8 C 17.6 14.56 14.56 17.6 10.8 17.6 C 7.04 17.6 4 14.56 4 10.8 C 4 7.04 7.04 4 10.8 4 Z M 15.8 15.8 L 20.5 20.5",
            "调研分析".loc
        ),
        WorkflowIconOption(
            "bolt",
            "M 13.2 2.5 L 4.5 13.5 L 11.5 13.5 L 10.2 21.5 L 19.5 10.5 L 12.5 10.5 L 13.2 2.5 Z",
            "快捷执行".loc
        ),
        WorkflowIconOption(
            "code",
            "M 7.5 7.5 L 3 12 L 7.5 16.5 M 16.5 7.5 L 21 12 L 16.5 16.5 M 13.8 5 L 10.2 19",
            "代码开发".loc
        ),
        WorkflowIconOption(
            "terminal",
            "M 5.5 4 L 18.5 4 C 19.88 4 21 5.12 21 6.5 L 21 17.5 C 21 18.88 19.88 20 18.5 20 L 5.5 20 C 4.12 20 3 18.88 3 17.5 L 3 6.5 C 3 5.12 4.12 4 5.5 4 Z M 7 9.5 L 10.5 12.5 L 7 15.5 M 12.5 15.5 L 17 15.5",
            "终端脚本".loc
        ),
        WorkflowIconOption(
            "doc",
            "M 14 2.5 L 6.5 2.5 C 5.4 2.5 4.5 3.4 4.5 4.5 L 4.5 19.5 C 4.5 20.6 5.4 21.5 6.5 21.5 L 17.5 21.5 C 18.6 21.5 19.5 20.6 19.5 19.5 L 19.5 8 L 14 2.5 Z M 14 2.5 L 14 8 L 19.5 8 M 8.5 13 L 15.5 13 M 8.5 17 L 13.5 17",
            "文档总结".loc
        ),
        WorkflowIconOption(
            "build",
            "M 9 3.2 C 12.0 2.7 14.8 3.7 16.8 5.7 L 21 9.9 L 18.2 12.7 L 14 8.5 C 12.5 7.0 10.8 6.4 9 6.7 L 9 3.2 Z M 13.5 8.1 L 3.4 18.2 C 2.6 19.0 2.6 20.3 3.4 21.1 C 4.2 21.9 5.5 21.9 6.3 21.1 L 16.4 11.0",
            "编译构建".loc
        ),
        WorkflowIconOption(
            "sparkles",
            "M 11 4 L 12.8 9.7 L 18.5 11.5 L 12.8 13.3 L 11 19 L 9.2 13.3 L 3.5 11.5 L 9.2 9.7 L 11 4 Z M 18.5 2.8 L 18.5 7.2 M 16.3 5 L 20.7 5",
            "AI 增强".loc
        ),
        WorkflowIconOption(
            "git",
            "M 6 3.5 C 7.38 3.5 8.5 4.62 8.5 6 C 8.5 7.38 7.38 8.5 6 8.5 C 4.62 8.5 3.5 7.38 3.5 6 C 3.5 4.62 4.62 3.5 6 3.5 Z M 6 15.5 C 7.38 15.5 8.5 16.62 8.5 18 C 8.5 19.38 7.38 20.5 6 20.5 C 4.62 20.5 3.5 19.38 3.5 18 C 3.5 16.62 4.62 15.5 6 15.5 Z M 18 5.5 C 19.38 5.5 20.5 6.62 20.5 8 C 20.5 9.38 19.38 10.5 18 10.5 C 16.62 10.5 15.5 9.38 15.5 8 C 15.5 6.62 16.62 5.5 18 5.5 Z M 6 8.5 L 6 15.5 M 18 10.5 C 18 14.5 14.5 15.5 8.5 15.5",
            "Git 分支".loc
        ),
        WorkflowIconOption(
            "merge",
            "M 6 3.5 C 7.38 3.5 8.5 4.62 8.5 6 C 8.5 7.38 7.38 8.5 6 8.5 C 4.62 8.5 3.5 7.38 3.5 6 C 3.5 4.62 4.62 3.5 6 3.5 Z M 6 15.5 C 7.38 15.5 8.5 16.62 8.5 18 C 8.5 19.38 7.38 20.5 6 20.5 C 4.62 20.5 3.5 19.38 3.5 18 C 3.5 16.62 4.62 15.5 6 15.5 Z M 18 15.5 C 19.38 15.5 20.5 16.62 20.5 18 C 20.5 19.38 19.38 20.5 18 20.5 C 16.62 20.5 15.5 19.38 15.5 18 C 15.5 16.62 16.62 15.5 18 15.5 Z M 6 8.5 L 6 15.5 M 6 8.5 C 6 13.5 11 18 15.5 18",
            "分支合并".loc
        ),
        WorkflowIconOption(
            "pipeline",
            "M 4.5 11 C 5.0 6.8 8.6 3.8 12.8 4.0 C 15.8 4.1 18.4 5.8 19.6 8.5 M 20 4.5 L 20 8.8 L 15.7 8.8 M 19.5 13 C 19.0 17.2 15.4 20.2 11.2 20.0 C 8.2 19.9 5.6 18.2 4.4 15.5 M 4 19.5 L 4 15.2 L 8.3 15.2",
            "流水线".loc
        ),
        WorkflowIconOption(
            "checklist",
            "M 3.5 7.5 L 5.8 9.8 L 9.8 5.5 M 13 7.8 L 20.5 7.8 M 3.5 16.5 L 5.8 18.8 L 9.8 14.5 M 13 16.8 L 20.5 16.8",
            "流程清单".loc
        ),
        WorkflowIconOption(
            "report",
            "M 3.5 4 L 3.5 20 L 20.5 20 M 8 16.5 L 8 12 M 12.8 16.5 L 12.8 8.5 M 17.6 16.5 L 17.6 5.5",
            "分析报告".loc
        )
    )

    val colors = listOf(
        WorkflowColorOption("indigo", "靛蓝".loc),
        WorkflowColorOption("green", "翠绿".loc),
        WorkflowColorOption("blue", "湛蓝".loc),
        WorkflowColorOption("purple", "亮紫".loc),
        WorkflowColorOption("orange", "活力橙".loc),
        WorkflowColorOption("red", "珊瑚红".loc),
        WorkflowColorOption("teal", "青碧".loc)
    )

    private val pathCache = ConcurrentHashMap<String, Path>()

    fun normalizeIconKey(key: String): String {
        if (key == "star") return "build"
        return if (icons.any { it.key == key }) key else "workflow"
    }

    fun getPath(iconKey: String): Path {
        val resolved = normalizeIconKey(iconKey)
        return pathCache.computeIfAbsent(resolved) { k ->
            val d = icons.firstOrNull { it.key == k }?.pathData ?: icons.first().pathData
            PathParser().parsePathString(d).toPath()
        }
    }

    fun resolveColor(key: String, themeColors: AntigravityColors): Color = when (key) {
        "green" -> themeColors.accentGreen
        "blue" -> themeColors.accentBlue
        "purple" -> themeColors.accentPurple
        "orange" -> themeColors.accentOrange
        "red" -> themeColors.accentRed
        "teal" -> Color(0xFF30B0C7)
        else -> themeColors.accentIndigo
    }
}

@Composable
internal fun WorkflowVectorIcon(
    iconKey: String,
    tint: Color,
    size: Dp = 20.dp,
    modifier: Modifier = Modifier
) {
    val path = remember(iconKey) { WorkflowStyleCatalog.getPath(iconKey) }
    Canvas(modifier = modifier.size(size)) {
        val s = this.size.minDimension / 24f
        scale(scaleX = s, scaleY = s, pivot = Offset.Zero) {
            drawPath(
                path = path,
                color = tint,
                style = Stroke(
                    width = 2.1f,
                    cap = StrokeCap.Round,
                    join = StrokeJoin.Round
                )
            )
        }
    }
}

internal object WorkflowTemplateLocalCache {
    private const val PREF_NAME = "agy_workflow_templates"
    private const val KEY_LIST = "templates_json_v1"
    private const val KEY_GUIDE_DISMISSED = "agy_wf_quick_guide_dismissed_v1"
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    fun load(context: Context): List<WorkflowTemplateRecord> {
        val prefs = context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        val raw = prefs.getString(KEY_LIST, null) ?: return WorkflowTemplateDefaults.partition(WorkflowTemplateDefaults.all)
        return runCatching {
            WorkflowTemplateDefaults.partition(json.decodeFromString<List<WorkflowTemplateRecord>>(raw))
        }.getOrElse { WorkflowTemplateDefaults.partition(WorkflowTemplateDefaults.all) }
    }

    fun save(context: Context, list: List<WorkflowTemplateRecord>) {
        val partitioned = WorkflowTemplateDefaults.partition(list)
        val prefs = context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        prefs.edit().putString(KEY_LIST, json.encodeToString(partitioned)).apply()
    }

    fun isGuideDismissed(context: Context): Boolean =
        context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
            .getBoolean(KEY_GUIDE_DISMISSED, false)

    fun setGuideDismissed(context: Context, dismissed: Boolean) {
        context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
            .edit().putBoolean(KEY_GUIDE_DISMISSED, dismissed).apply()
    }
}

private data class WorkflowEditorDraft(
    val isNew: Boolean,
    val record: WorkflowTemplateRecord
)

/**
 * 动态多智能体工作流模板选择、自定义与跨区排序抽屉 (WorkflowTemplateSheet)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorkflowTemplateSheet(
    onDismissRequest: () -> Unit,
    onSelectTemplate: (String) -> Unit,
    initialEditing: Boolean = false,
    onTemplatesChanged: (List<WorkflowTemplateRecord>) -> Unit = {}
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    val apiClient = remember(context) {
        ApiClient.defaultInstance ?: ApiClient(
            context.applicationContext,
            PreferencesManager(context.applicationContext)
        )
    }

    var templates by remember { mutableStateOf(WorkflowTemplateLocalCache.load(context)) }
    var isEditing by remember(initialEditing) { mutableStateOf(initialEditing) }
    var editorDraft by remember { mutableStateOf<WorkflowEditorDraft?>(null) }
    var syncError by remember { mutableStateOf<String?>(null) }
    var showResetDialog by remember { mutableStateOf(false) }
    var swipedId by remember { mutableStateOf<String?>(null) }
    var guideDismissed by remember { mutableStateOf(WorkflowTemplateLocalCache.isGuideDismissed(context)) }

    // Cross-section drag-and-drop state
    val density = LocalDensity.current
    val rowSpacingPx = with(density) { 10.dp.toPx() }
    var draggedId by remember { mutableStateOf<String?>(null) }
    var dragStartIndex by remember { mutableStateOf<Int?>(null) }
    var dragStartCenterY by remember { mutableStateOf<Float?>(null) }
    var dragTargetInQuickMenu by remember { mutableStateOf<Boolean?>(null) }
    var dragInsertInTargetSection by remember { mutableIntStateOf(0) }
    var dragOffsetY by remember { mutableFloatStateOf(0f) }

    val itemHeights = remember { mutableStateMapOf<String, Int>() }
    var scrollContentCoords by remember { mutableStateOf<LayoutCoordinates?>(null) }
    var moreHeaderCoords by remember { mutableStateOf<LayoutCoordinates?>(null) }
    val rowCoords = remember { mutableStateMapOf<String, LayoutCoordinates>() }

    fun resetDragState() {
        draggedId = null
        dragStartIndex = null
        dragStartCenterY = null
        dragTargetInQuickMenu = null
        dragInsertInTargetSection = 0
        dragOffsetY = 0f
    }

    fun itemCenterYInScroll(id: String): Float? {
        val root = scrollContentCoords ?: return null
        val child = rowCoords[id] ?: return null
        if (!root.isAttached || !child.isAttached) return null
        return root.localPositionOf(child, Offset.Zero).y + child.size.height / 2f
    }

    fun moreBoundaryYInScroll(): Float? {
        val root = scrollContentCoords ?: return null
        val header = moreHeaderCoords ?: return null
        if (!root.isAttached || !header.isAttached) return null
        return root.localPositionOf(header, Offset.Zero).y
    }

    fun updateDragTarget(startIdx: Int, offsetY: Float) {
        if (startIdx !in templates.indices) return
        val startItem = templates[startIdx]
        val baseCenterY = dragStartCenterY ?: itemCenterYInScroll(startItem.id) ?: return
        val currentMidY = baseCenterY + offsetY
        val boundary = moreBoundaryYInScroll()
        val nextInQuick = if (boundary != null) {
            currentMidY < boundary
        } else {
            dragTargetInQuickMenu ?: startItem.inQuickMenu
        }

        val destOthers = templates.withIndex().filter {
            it.index != startIdx && it.value.inQuickMenu == nextInQuick
        }
        var insertAt = 0
        for (entry in destOthers) {
            val mid = itemCenterYInScroll(entry.value.id) ?: continue
            if (currentMidY > mid) {
                insertAt++
            } else {
                break
            }
        }

        if (nextInQuick != dragTargetInQuickMenu || insertAt != dragInsertInTargetSection) {
            dragTargetInQuickMenu = nextInQuick
            dragInsertInTargetSection = insertAt
            haptic.light()
        }
    }

    fun siblingDisplacementY(index: Int): Float {
        val activeId = draggedId ?: return 0f
        val start = dragStartIndex ?: return 0f
        val targetQuick = dragTargetInQuickMenu ?: return 0f
        if (index == start || start !in templates.indices || index !in templates.indices) return 0f

        val startItem = templates[start]
        val thisItem = templates[index]
        val draggedStep = ((itemHeights[activeId] ?: 160).toFloat()) + rowSpacingPx

        return if (startItem.inQuickMenu == targetQuick) {
            if (thisItem.inQuickMenu != targetQuick) return 0f
            val sectionOthers = templates.withIndex().filter {
                it.index != start && it.value.inQuickMenu == targetQuick
            }
            val startPosInSection = templates.subList(0, start).count { it.inQuickMenu == targetQuick }
            val thisPosAmongOthers = sectionOthers.indexOfFirst { it.index == index }
            if (thisPosAmongOthers < 0) return 0f
            val origPos = if (thisPosAmongOthers >= startPosInSection) thisPosAmongOthers + 1 else thisPosAmongOthers
            when {
                dragInsertInTargetSection > startPosInSection && origPos in (startPosInSection + 1)..dragInsertInTargetSection -> -draggedStep
                dragInsertInTargetSection < startPosInSection && origPos in dragInsertInTargetSection until startPosInSection -> draggedStep
                else -> 0f
            }
        } else {
            when (thisItem.inQuickMenu) {
                startItem.inQuickMenu -> if (index > start) -draggedStep else 0f
                targetQuick -> {
                    val destOthers = templates.withIndex().filter {
                        it.index != start && it.value.inQuickMenu == targetQuick
                    }
                    val posInDest = destOthers.indexOfFirst { it.index == index }
                    if (posInDest >= 0 && posInDest >= dragInsertInTargetSection) draggedStep else 0f
                }
                else -> 0f
            }
        }
    }

    fun persistAndSync(nextList: List<WorkflowTemplateRecord>) {
        val partitioned = WorkflowTemplateDefaults.partition(nextList)
        templates = partitioned
        WorkflowTemplateLocalCache.save(context, partitioned)
        onTemplatesChanged(partitioned)
        syncError = null
        scope.launch {
            apiClient.saveWorkflowTemplates(partitioned)
                .onSuccess { resp ->
                    val serverPartitioned = WorkflowTemplateDefaults.partition(resp.templates)
                    if (serverPartitioned != templates) {
                        templates = serverPartitioned
                        WorkflowTemplateLocalCache.save(context, serverPartitioned)
                        onTemplatesChanged(serverPartitioned)
                    }
                }
                .onFailure { err ->
                    syncError = "已保存本机缓存，网关同步失败：%s".loc(err.message ?: "未知错误".loc)
                }
        }
    }

    LaunchedEffect(Unit) {
        apiClient.fetchWorkflowTemplates()
            .onSuccess { resp ->
                val partitioned = WorkflowTemplateDefaults.partition(resp.templates)
                if (partitioned != templates) {
                    templates = partitioned
                    WorkflowTemplateLocalCache.save(context, partitioned)
                    onTemplatesChanged(partitioned)
                }
                syncError = null
            }
    }

    val quickEntries = remember(templates) {
        templates.withIndex().filter { it.value.inQuickMenu }
    }
    val moreEntries = remember(templates) {
        templates.withIndex().filter { !it.value.inQuickMenu }
    }
    val draggingInQuickMenu = dragStartIndex?.let { templates.getOrNull(it)?.inQuickMenu }

    @Composable
    fun RenderCardEntry(entry: IndexedValue<WorkflowTemplateRecord>) {
        val index = entry.index
        val item = entry.value
        key(item.id) {
            val isDragging = draggedId == item.id
            val isAnyDragging = draggedId != null
            val displacementTarget = siblingDisplacementY(index)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .zIndex(if (isDragging) 10f else 0f)
                    .onGloballyPositioned { rowCoords[item.id] = it }
            ) {
                WorkflowSwipeableCard(
                    item = item,
                    isEditing = isEditing,
                    isDragging = isDragging,
                    isAnyDragging = isAnyDragging,
                    dragOffsetY = if (isDragging) dragOffsetY else 0f,
                    siblingDisplacementY = displacementTarget,
                    isSwiped = swipedId == item.id,
                    colors = colors,
                    onSwipeOpen = { swipedId = item.id },
                    onSwipeClose = { if (swipedId == item.id) swipedId = null },
                    onSized = { h -> itemHeights[item.id] = h },
                    onSelect = {
                        swipedId = null
                        haptic.medium()
                        onSelectTemplate(item.templatePrompt)
                        onDismissRequest()
                    },
                    onEdit = {
                        swipedId = null
                        editorDraft = WorkflowEditorDraft(isNew = false, record = item)
                    },
                    onDelete = {
                        swipedId = null
                        haptic.medium()
                        persistAndSync(templates.filter { it.id != item.id })
                    },
                    dragHandleModifier = Modifier.pointerInput(item.id, templates) {
                        detectDragGestures(
                            onDragStart = {
                                swipedId = null
                                val idx = templates.indexOfFirst { it.id == item.id }
                                if (idx >= 0) {
                                    draggedId = item.id
                                    dragStartIndex = idx
                                    dragStartCenterY = itemCenterYInScroll(item.id)
                                    dragTargetInQuickMenu = item.inQuickMenu
                                    dragInsertInTargetSection = templates.subList(0, idx).count {
                                        it.inQuickMenu == item.inQuickMenu
                                    }
                                    dragOffsetY = 0f
                                    haptic.light()
                                }
                            },
                            onDragEnd = {
                                val start = dragStartIndex
                                val targetQuick = dragTargetInQuickMenu
                                val insertSlot = dragInsertInTargetSection
                                val updatedList = if (start != null && targetQuick != null && start in templates.indices) {
                                    val mutable = templates.toMutableList()
                                    val removed = mutable.removeAt(start)
                                    val oldQuick = removed.inQuickMenu
                                    val updatedItem = removed.copy(inQuickMenu = targetQuick)
                                    val quickOthersCount = mutable.count { it.inQuickMenu }
                                    val destGlobalIdx = if (targetQuick) {
                                        insertSlot.coerceIn(0, quickOthersCount)
                                    } else {
                                        (quickOthersCount + insertSlot).coerceIn(quickOthersCount, mutable.size)
                                    }
                                    mutable.add(destGlobalIdx, updatedItem)
                                    if (targetQuick != oldQuick || destGlobalIdx != start) mutable else null
                                } else {
                                    null
                                }
                                resetDragState()
                                if (updatedList != null) {
                                    persistAndSync(updatedList)
                                }
                            },
                            onDragCancel = {
                                resetDragState()
                            },
                            onDrag = { change, dragAmount ->
                                change.consume()
                                val start = dragStartIndex ?: return@detectDragGestures
                                dragOffsetY += dragAmount.y
                                updateDragTarget(start, dragOffsetY)
                            }
                        )
                    }
                )
            }
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismissRequest,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = Modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.surface, onDismiss = onDismissRequest) {
            Column(modifier = Modifier.fillMaxSize()) {
                // Floating grab handle hinting pull-down dismissal (matching AccountQuotaSheet 38x5)
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 8.dp, bottom = 10.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Box(
                        modifier = Modifier
                            .size(width = 38.dp, height = 5.dp)
                            .clip(CircleShape)
                            .background(colors.textMuted.copy(alpha = 0.35f))
                    )
                }

                val currentDraft = editorDraft
                if (currentDraft != null) {
                    WorkflowTemplateEditorContent(
                        draft = currentDraft,
                        colors = colors,
                        onBack = { editorDraft = null },
                        onSave = { saved ->
                            val next = if (currentDraft.isNew) {
                                templates + saved
                            } else {
                                templates.map { if (it.id == saved.id) saved else it }
                            }
                            editorDraft = null
                            persistAndSync(next)
                        },
                        modifier = Modifier.weight(1f)
                    )
                } else {
                    // Top Bar: Icon Edit/Done toggle on left, Title in center, + icon only on right
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(bottom = 10.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        IconButton(
                            onClick = {
                                haptic.light()
                                swipedId = null
                                isEditing = !isEditing
                            },
                            modifier = Modifier
                                .align(Alignment.CenterStart)
                                .padding(start = 8.dp)
                                .size(34.dp)
                        ) {
                            Icon(
                                imageVector = if (isEditing) Icons.Default.Check else Icons.Default.Edit,
                                contentDescription = if (isEditing) "完成".loc else "编辑".loc,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(19.dp)
                            )
                        }

                        Text(
                            text = "Workflow 模板".loc,
                            fontWeight = FontWeight.Bold,
                            fontSize = 17.sp,
                            lineHeight = 20.sp,
                            color = colors.textPrimary,
                            textAlign = TextAlign.Center,
                            modifier = Modifier
                                .fillMaxWidth()
                                .align(Alignment.Center)
                        )

                        IconButton(
                            onClick = {
                                haptic.light()
                                swipedId = null
                                editorDraft = WorkflowEditorDraft(
                                    isNew = true,
                                    record = WorkflowTemplateRecord(
                                        id = "wf_${UUID.randomUUID().toString().lowercase()}",
                                        title = "",
                                        subtitle = "",
                                        icon = "workflow",
                                        color = "indigo",
                                        templatePrompt = "",
                                        inQuickMenu = false
                                    )
                                )
                            },
                            modifier = Modifier
                                .align(Alignment.CenterEnd)
                                .padding(end = 8.dp)
                                .size(34.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Add,
                                contentDescription = "新增".loc,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(19.dp)
                            )
                        }
                    }

                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f)
                            .verticalScroll(rememberScrollState(), enabled = draggedId == null)
                            .onGloballyPositioned { scrollContentCoords = it }
                            .padding(horizontal = 16.dp)
                            .padding(bottom = 28.dp)
                            .navigationBarsPadding(),
                        verticalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        if (!guideDismissed) {
                            WorkflowQuickGuideCard(
                                isEditing = isEditing,
                                colors = colors,
                                onEnterEdit = {
                                    haptic.light()
                                    isEditing = true
                                },
                                onDismiss = {
                                    haptic.light()
                                    WorkflowTemplateLocalCache.setGuideDismissed(context, true)
                                    guideDismissed = true
                                }
                            )
                        }

                        syncError?.let { err ->
                            Surface(
                                shape = RoundedCornerShape(10.dp),
                                color = colors.accentRed.copy(alpha = 0.12f),
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text(
                                    text = err,
                                    color = colors.accentRed,
                                    style = MaterialTheme.typography.bodySmall,
                                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp)
                                )
                            }
                        }

                        if (templates.isEmpty()) {
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 28.dp),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(12.dp)
                            ) {
                                Text(
                                    text = "暂无 Workflow 模板".loc,
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = colors.textSecondary
                                )
                                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                                    Button(
                                        onClick = {
                                            editorDraft = WorkflowEditorDraft(
                                                isNew = true,
                                                record = WorkflowTemplateRecord(
                                                    id = "wf_${UUID.randomUUID().toString().lowercase()}",
                                                    icon = "workflow",
                                                    color = "indigo"
                                                )
                                            )
                                        },
                                        colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                                    ) {
                                        Text("+ 新增模板".loc)
                                    }
                                    OutlinedButton(onClick = { showResetDialog = true }) {
                                        Text("恢复默认".loc)
                                    }
                                }
                            }
                        } else {
                            // Section 1: 长按快捷菜单
                            WorkflowSectionHeader(
                                title = "长按快捷菜单".loc,
                                subtitle = if (isEditing) "拖拽至此置入长按上滑菜单".loc else "长按聊天栏 Workflow 上滑直达".loc,
                                count = quickEntries.size,
                                colors = colors
                            )

                            if (quickEntries.isEmpty()) {
                                val highlightDrop = draggedId != null && dragTargetInQuickMenu == true
                                EmptyQuickMenuDropZone(
                                    isEditing = isEditing,
                                    isHighlighted = highlightDrop,
                                    colors = colors,
                                    onTapToEdit = {
                                        haptic.light()
                                        isEditing = true
                                    }
                                )
                            } else {
                                Column(
                                    modifier = Modifier.zIndex(if (draggingInQuickMenu == true) 10f else 0f),
                                    verticalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    quickEntries.forEach { entry ->
                                        RenderCardEntry(entry)
                                    }
                                }
                            }

                            // Section 2: 更多模板
                            WorkflowSectionHeader(
                                title = "更多模板".loc,
                                subtitle = if (isEditing) "点击卡片编辑，按住 ≡ 可拖入上方快捷菜单".loc else "点击模板直接填入输入框".loc,
                                count = moreEntries.size,
                                colors = colors,
                                modifier = Modifier
                                    .padding(top = 4.dp)
                                    .onGloballyPositioned { moreHeaderCoords = it }
                            )

                            if (moreEntries.isEmpty()) {
                                if (isEditing) {
                                    val highlightDrop = draggedId != null && dragTargetInQuickMenu == false
                                    EmptyMoreTemplatesDropZone(
                                        isHighlighted = highlightDrop,
                                        colors = colors
                                    )
                                } else {
                                    Text(
                                        text = "全部模板已加入长按快捷菜单".loc,
                                        fontSize = 12.5.sp,
                                        color = colors.textSecondary,
                                        textAlign = TextAlign.Center,
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .padding(vertical = 16.dp)
                                    )
                                }
                            } else {
                                Column(
                                    modifier = Modifier.zIndex(if (draggingInQuickMenu == false) 10f else 0f),
                                    verticalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    moreEntries.forEach { entry ->
                                        RenderCardEntry(entry)
                                    }
                                }
                            }
                        }

                        if (isEditing) {
                            TextButton(
                                onClick = { showResetDialog = true },
                                modifier = Modifier.align(Alignment.CenterHorizontally)
                            ) {
                                Text(
                                    text = "恢复默认模板".loc,
                                    color = colors.accentRed,
                                    fontSize = 13.5.sp,
                                    fontWeight = FontWeight.Medium
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    if (showResetDialog) {
        AlertDialog(
            onDismissRequest = { showResetDialog = false },
            containerColor = colors.surface,
            title = { Text("恢复默认模板".loc) },
            text = { Text("将恢复为系统初始的 4 个模板，当前自定义模板、快捷菜单配置和排序会被替换。".loc) },
            confirmButton = {
                TextButton(
                    onClick = {
                        showResetDialog = false
                        val defaults = WorkflowTemplateDefaults.partition(WorkflowTemplateDefaults.all)
                        templates = defaults
                        WorkflowTemplateLocalCache.save(context, defaults)
                        onTemplatesChanged(defaults)
                        syncError = null
                        scope.launch {
                            apiClient.resetWorkflowTemplates()
                                .onSuccess { resp ->
                                    val partitioned = WorkflowTemplateDefaults.partition(resp.templates)
                                    templates = partitioned
                                    WorkflowTemplateLocalCache.save(context, partitioned)
                                    onTemplatesChanged(partitioned)
                                }
                                .onFailure { err ->
                                    syncError = "已重置本机列表，网关同步失败：%s".loc(err.message ?: "未知错误".loc)
                                }
                        }
                    }
                ) {
                    Text("确认恢复".loc, color = colors.accentRed)
                }
            },
            dismissButton = {
                TextButton(onClick = { showResetDialog = false }) {
                    Text("取消".loc)
                }
            }
        )
    }
}

@Composable
private fun WorkflowQuickGuideCard(
    isEditing: Boolean,
    colors: AntigravityColors,
    onEnterEdit: () -> Unit,
    onDismiss: () -> Unit
) {
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = colors.surfaceVariant.copy(alpha = 0.72f),
        border = BorderStroke(0.8.dp, colors.accentIndigo.copy(alpha = 0.25f)),
        modifier = Modifier.fillMaxWidth()
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            // Left: Micro-gesture schematic
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(3.dp),
                modifier = Modifier
                    .clip(RoundedCornerShape(9.dp))
                    .background(colors.accentIndigo.copy(alpha = 0.10f))
                    .padding(6.dp)
            ) {
                Column(
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                    modifier = Modifier
                        .clip(RoundedCornerShape(5.dp))
                        .background(colors.surface.copy(alpha = 0.9f))
                        .border(0.7.dp, colors.border, RoundedCornerShape(5.dp))
                        .padding(4.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(width = 44.dp, height = 6.dp)
                            .clip(RoundedCornerShape(2.5.dp))
                            .background(colors.textPrimary.copy(alpha = 0.18f))
                    )
                    Box(
                        modifier = Modifier
                            .size(width = 44.dp, height = 7.dp)
                            .clip(RoundedCornerShape(2.5.dp))
                            .background(colors.accentIndigo.copy(alpha = 0.55f))
                    )
                }

                Icon(
                    imageVector = Icons.Default.ArrowUpward,
                    contentDescription = null,
                    tint = colors.accentIndigo,
                    modifier = Modifier.size(10.dp)
                )

                Box(
                    modifier = Modifier
                        .clip(RoundedCornerShape(4.dp))
                        .background(colors.surface)
                        .border(0.7.dp, colors.border, RoundedCornerShape(4.dp))
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                ) {
                    Text(
                        text = "Workflow",
                        fontSize = 7.5.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.textPrimary
                    )
                }
            }

            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.Top
                ) {
                    Text(
                        text = "长按上滑，快捷输入常用模板".loc,
                        fontSize = 13.5.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary,
                        modifier = Modifier.weight(1f)
                    )
                    Box(
                        modifier = Modifier
                            .size(22.dp)
                            .clickable(onClick = onDismiss),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Close,
                            contentDescription = "关闭引导".loc,
                            tint = colors.textMuted,
                            modifier = Modifier.size(14.dp)
                        )
                    }
                }

                Text(
                    text = "聊天栏长按 Workflow 上滑松手即可填入；编辑后按住 ≡ 可拖入「长按快捷菜单」".loc,
                    fontSize = 11.5.sp,
                    lineHeight = 15.5.sp,
                    color = colors.textSecondary
                )

                if (!isEditing) {
                    Box(
                        modifier = Modifier
                            .padding(top = 2.dp)
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.14f))
                            .clickable(onClick = onEnterEdit)
                            .padding(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Text(
                            text = "去配置 ›".loc,
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.accentIndigo
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun WorkflowSectionHeader(
    title: String,
    subtitle: String,
    count: Int,
    colors: AntigravityColors,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Text(
            text = title,
            fontSize = 13.5.sp,
            fontWeight = FontWeight.Bold,
            color = colors.textPrimary,
            maxLines = 1,
            softWrap = false
        )
        Box(
            modifier = Modifier
                .clip(CircleShape)
                .background(colors.surfaceVariant)
                .padding(horizontal = 7.dp, vertical = 1.dp)
        ) {
            Text(
                text = "$count",
                fontSize = 11.5.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textSecondary
            )
        }
        Text(
            text = subtitle,
            fontSize = 11.sp,
            color = colors.textSecondary,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.End,
            modifier = Modifier.weight(1f)
        )
    }
}

@Composable
private fun EmptyQuickMenuDropZone(
    isEditing: Boolean,
    isHighlighted: Boolean,
    colors: AntigravityColors,
    onTapToEdit: () -> Unit
) {
    val strokeColor = if (isHighlighted) colors.accentIndigo else colors.border
    val strokeWidth = if (isHighlighted) 1.5.dp else 1.dp
    val bgColor = if (isHighlighted) colors.accentIndigo.copy(alpha = 0.10f) else colors.surfaceVariant.copy(alpha = 0.45f)

    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(5.dp),
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(bgColor)
            .drawBehind {
                drawRoundRect(
                    color = strokeColor,
                    cornerRadius = CornerRadius(12.dp.toPx(), 12.dp.toPx()),
                    style = Stroke(
                        width = strokeWidth.toPx(),
                        pathEffect = PathEffect.dashPathEffect(floatArrayOf(14f, 10f), 0f)
                    )
                )
            }
            .then(if (!isEditing) Modifier.clickable(onClick = onTapToEdit) else Modifier)
            .padding(horizontal = 14.dp, vertical = 18.dp)
    ) {
        Text(
            text = if (isEditing) "按住下方模板左侧 ≡ 向上拖入此处".loc else "暂未添加快捷模板".loc,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            color = if (isHighlighted) colors.accentIndigo else colors.textPrimary
        )
        Text(
            text = if (isEditing) "拖入后即可在聊天栏长按 Workflow 上滑直达".loc else "点击进入编辑，或编辑单个模板开启「加入长按快捷菜单」".loc,
            fontSize = 11.5.sp,
            color = colors.textSecondary,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
private fun EmptyMoreTemplatesDropZone(
    isHighlighted: Boolean,
    colors: AntigravityColors
) {
    val strokeColor = if (isHighlighted) colors.accentIndigo else colors.border
    val strokeWidth = if (isHighlighted) 1.5.dp else 1.dp
    val bgColor = if (isHighlighted) colors.accentIndigo.copy(alpha = 0.08f) else Color.Transparent

    Box(
        contentAlignment = Alignment.Center,
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(bgColor)
            .drawBehind {
                drawRoundRect(
                    color = strokeColor,
                    cornerRadius = CornerRadius(12.dp.toPx(), 12.dp.toPx()),
                    style = Stroke(
                        width = strokeWidth.toPx(),
                        pathEffect = PathEffect.dashPathEffect(floatArrayOf(14f, 10f), 0f)
                    )
                )
            }
            .padding(horizontal = 14.dp, vertical = 16.dp)
    ) {
        Text(
            text = "拖至此处可移出长按快捷菜单".loc,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
            color = if (isHighlighted) colors.accentIndigo else colors.textSecondary
        )
    }
}

@Composable
private fun WorkflowSwipeableCard(
    item: WorkflowTemplateRecord,
    isEditing: Boolean,
    isDragging: Boolean,
    isAnyDragging: Boolean,
    dragOffsetY: Float,
    siblingDisplacementY: Float,
    isSwiped: Boolean,
    colors: AntigravityColors,
    onSwipeOpen: () -> Unit,
    onSwipeClose: () -> Unit,
    onSized: (Int) -> Unit,
    onSelect: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
    dragHandleModifier: Modifier
) {
    val density = LocalDensity.current
    val scope = rememberCoroutineScope()
    val maxRevealPx = with(density) { 68.dp.toPx() }
    val offsetX = remember { Animatable(0f) }
    val animatedDisplacementY = remember { Animatable(0f) }
    val haptic = rememberHaptic()

    LaunchedEffect(isSwiped) {
        if (!isSwiped && offsetX.value != 0f) {
            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
        }
    }

    LaunchedEffect(isAnyDragging, siblingDisplacementY) {
        if (!isAnyDragging) {
            animatedDisplacementY.snapTo(0f)
        } else {
            animatedDisplacementY.animateTo(
                siblingDisplacementY,
                spring(stiffness = Spring.StiffnessMedium)
            )
        }
    }

    val itemColor = WorkflowStyleCatalog.resolveColor(item.color, colors)
    val displaySub = item.subtitle.ifBlank { item.templatePrompt }

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .zIndex(if (isDragging) 2f else 0f)
            .graphicsLayer {
                translationY = if (isDragging) {
                    dragOffsetY
                } else if (isAnyDragging) {
                    animatedDisplacementY.value
                } else {
                    0f
                }
                scaleX = if (isDragging) 1.015f else 1f
                scaleY = if (isDragging) 1.015f else 1f
            }
            .onSizeChanged { onSized(it.height) }
            .clip(RoundedCornerShape(12.dp)),
        contentAlignment = Alignment.Center
    ) {
        // Right-side trailing red circle delete action (revealed on left swipe)
        if (offsetX.value < -1f) {
            Box(
                modifier = Modifier
                    .matchParentSize()
                    .padding(end = 14.dp),
                contentAlignment = Alignment.CenterEnd
            ) {
                Box(
                    modifier = Modifier
                        .size(38.dp)
                        .clip(CircleShape)
                        .background(colors.accentRed)
                        .clickable {
                            haptic.medium()
                            onDelete()
                        },
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Delete,
                        contentDescription = "删除".loc,
                        tint = Color.White,
                        modifier = Modifier.size(18.dp)
                    )
                }
            }
        }

        // Foreground card
        Surface(
            shape = RoundedCornerShape(12.dp),
            color = colors.surfaceVariant.copy(alpha = if (isDragging) 0.95f else 0.72f),
            shadowElevation = if (isDragging) 6.dp else 0.dp,
            modifier = Modifier
                .fillMaxWidth()
                .pointerInput(item.id) {
                    detectHorizontalDragGestures(
                        onDragStart = {
                            if (!isSwiped) onSwipeOpen()
                        },
                        onDragEnd = {
                            scope.launch {
                                if (offsetX.value < -maxRevealPx * 0.45f) {
                                    offsetX.animateTo(-maxRevealPx, spring(stiffness = Spring.StiffnessMediumLow))
                                    onSwipeOpen()
                                } else {
                                    offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                                    onSwipeClose()
                                }
                            }
                        },
                        onDragCancel = {
                            scope.launch {
                                offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                                onSwipeClose()
                            }
                        },
                        onHorizontalDrag = { change, dragAmount ->
                            change.consume()
                            val next = (offsetX.value + dragAmount).coerceIn(-maxRevealPx, 0f)
                            scope.launch { offsetX.snapTo(next) }
                        }
                    )
                }
                .offset { IntOffset(offsetX.value.roundToInt(), 0) }
                .clickable {
                    if (offsetX.value < -10f) {
                        scope.launch {
                            offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                            onSwipeClose()
                        }
                    } else if (isEditing) {
                        haptic.light()
                        onEdit()
                    } else {
                        onSelect()
                    }
                }
        ) {
            Row(
                modifier = Modifier.padding(12.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (isEditing) {
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .then(dragHandleModifier),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.DragHandle,
                            contentDescription = "拖拽排序".loc,
                            tint = colors.textMuted,
                            modifier = Modifier.size(20.dp)
                        )
                    }
                }

                Box(
                    modifier = Modifier
                        .size(40.dp)
                        .clip(RoundedCornerShape(10.dp))
                        .background(itemColor.copy(alpha = 0.15f)),
                    contentAlignment = Alignment.Center
                ) {
                    WorkflowVectorIcon(
                        iconKey = item.icon,
                        tint = itemColor,
                        size = 20.dp
                    )
                }

                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = item.title,
                        style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                        color = colors.textPrimary,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Spacer(modifier = Modifier.height(2.dp))
                    Text(
                        text = displaySub,
                        style = MaterialTheme.typography.bodySmall,
                        color = colors.textSecondary,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                if (!isEditing) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                        contentDescription = null,
                        tint = colors.textMuted,
                        modifier = Modifier.size(16.dp)
                    )
                }
            }
        }
    }
}

@Composable
private fun WorkflowTemplateEditorContent(
    draft: WorkflowEditorDraft,
    colors: AntigravityColors,
    onBack: () -> Unit,
    onSave: (WorkflowTemplateRecord) -> Unit,
    modifier: Modifier = Modifier
) {
    var title by remember(draft) { mutableStateOf(draft.record.title) }
    var subtitle by remember(draft) { mutableStateOf(draft.record.subtitle) }
    var icon by remember(draft) { mutableStateOf(WorkflowStyleCatalog.normalizeIconKey(draft.record.icon)) }
    var colorKey by remember(draft) { mutableStateOf(draft.record.color.ifBlank { "indigo" }) }
    var promptValue by remember(draft) { mutableStateOf(TextFieldValue(draft.record.templatePrompt)) }
    var inQuickMenu by remember(draft) { mutableStateOf(draft.record.inQuickMenu) }
    val promptFocusRequester = remember { FocusRequester() }

    val canSave = title.trim().isNotEmpty() && promptValue.text.trim().isNotEmpty()
    val selectedColor = WorkflowStyleCatalog.resolveColor(colorKey, colors)

    Column(modifier = modifier.fillMaxSize()) {
        // Editor Header: icon-only Back on left, centered Title, icon-only Checkmark Save on right
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 10.dp),
            contentAlignment = Alignment.Center
        ) {
            IconButton(
                onClick = onBack,
                modifier = Modifier
                    .align(Alignment.CenterStart)
                    .padding(start = 8.dp)
                    .size(34.dp)
            ) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = "返回".loc,
                    tint = colors.textPrimary,
                    modifier = Modifier.size(19.dp)
                )
            }

            Text(
                text = if (draft.isNew) "新增 Workflow".loc else "编辑 Workflow".loc,
                fontWeight = FontWeight.Bold,
                fontSize = 17.sp,
                lineHeight = 20.sp,
                color = colors.textPrimary,
                textAlign = TextAlign.Center,
                modifier = Modifier
                    .fillMaxWidth()
                    .align(Alignment.Center)
            )

            IconButton(
                onClick = {
                    onSave(
                        WorkflowTemplateRecord(
                            id = draft.record.id,
                            title = title.trim(),
                            subtitle = subtitle.trim(),
                            icon = icon,
                            color = colorKey,
                            templatePrompt = promptValue.text.trim(),
                            inQuickMenu = inQuickMenu
                        )
                    )
                },
                enabled = canSave,
                modifier = Modifier
                    .align(Alignment.CenterEnd)
                    .padding(end = 8.dp)
                    .size(34.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Check,
                    contentDescription = "保存".loc,
                    tint = if (canSave) colors.accentIndigo else colors.textMuted,
                    modifier = Modifier.size(19.dp)
                )
            }
        }

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp)
                .padding(bottom = 28.dp)
                .navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            OutlinedTextField(
                value = title,
                onValueChange = { title = it },
                label = { Text("标题（必填）".loc) },
                placeholder = { Text("例如：每日代码巡检".loc) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )

            OutlinedTextField(
                value = subtitle,
                onValueChange = { subtitle = it },
                label = { Text("简介（可选）".loc) },
                placeholder = { Text("简短描述用途，方便快速辨识".loc) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth()
            )

            Surface(
                shape = RoundedCornerShape(12.dp),
                color = colors.surfaceVariant.copy(alpha = 0.6f),
                modifier = Modifier.fillMaxWidth()
            ) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { inQuickMenu = !inQuickMenu }
                        .padding(horizontal = 14.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = "加入长按快捷菜单".loc,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.Medium,
                            color = colors.textPrimary
                        )
                        Text(
                            text = "开启后，在聊天栏长按 Workflow 上滑即可快速填入".loc,
                            fontSize = 11.5.sp,
                            color = colors.textSecondary
                        )
                    }
                    Switch(
                        checked = inQuickMenu,
                        onCheckedChange = { inQuickMenu = it }
                    )
                }
            }

            Text(
                text = "主题色".loc,
                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
                color = colors.textSecondary
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                WorkflowStyleCatalog.colors.forEach { opt ->
                    val c = WorkflowStyleCatalog.resolveColor(opt.key, colors)
                    val active = colorKey == opt.key
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(CircleShape)
                            .background(c)
                            .then(
                                if (active) {
                                    Modifier.border(2.5.dp, colors.textPrimary, CircleShape)
                                } else {
                                    Modifier
                                }
                            )
                            .clickable { colorKey = opt.key }
                    )
                }
            }

            Text(
                text = "图标".loc,
                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
                color = colors.textSecondary
            )
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                WorkflowStyleCatalog.icons.chunked(5).forEach { rowIcons ->
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        rowIcons.forEach { ic ->
                            val active = icon == ic.key
                            Box(
                                modifier = Modifier
                                    .weight(1f)
                                    .height(42.dp)
                                    .clip(RoundedCornerShape(10.dp))
                                    .background(
                                        if (active) selectedColor.copy(alpha = 0.16f)
                                        else colors.surfaceVariant.copy(alpha = 0.5f)
                                    )
                                    .border(
                                        width = if (active) 1.5.dp else 0.5.dp,
                                        color = if (active) selectedColor else colors.border,
                                        shape = RoundedCornerShape(10.dp)
                                    )
                                    .clickable { icon = ic.key },
                                contentAlignment = Alignment.Center
                            ) {
                                WorkflowVectorIcon(
                                    iconKey = ic.key,
                                    tint = if (active) selectedColor else colors.textSecondary,
                                    size = 20.dp
                                )
                            }
                        }
                    }
                }
            }

            OutlinedTextField(
                value = promptValue,
                onValueChange = { promptValue = it },
                label = { Text("提示词 / 命令模板（必填）".loc) },
                placeholder = { Text("支持任意提示词或 /workflow、/plan 等命令，可用 [占位符] 提示补充参数".loc) },
                minLines = 4,
                maxLines = 8,
                modifier = Modifier
                    .fillMaxWidth()
                    .focusRequester(promptFocusRequester)
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Text(
                    text = "占位符在编辑框中可以直接选中".loc,
                    fontSize = 12.sp,
                    color = colors.textSecondary,
                    modifier = Modifier.weight(1f)
                )
                Spacer(modifier = Modifier.width(8.dp))
                OutlinedButton(
                    onClick = {
                        val current = promptValue
                        val start = current.selection.min.coerceIn(0, current.text.length)
                        val end = current.selection.max.coerceIn(start, current.text.length)
                        val token = "[占位符]".loc
                        val nextText = buildString {
                            append(current.text.substring(0, start))
                            append(token)
                            append(current.text.substring(end))
                        }
                        promptValue = TextFieldValue(
                            text = nextText,
                            selection = TextRange(start + 1, start + 1 + "占位符".loc.length)
                        )
                        promptFocusRequester.requestFocus()
                    },
                    shape = RoundedCornerShape(10.dp),
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                    border = BorderStroke(1.dp, colors.accentIndigo.copy(alpha = 0.4f))
                ) {
                    Icon(
                        imageVector = Icons.Default.Add,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(16.dp)
                    )
                    Spacer(modifier = Modifier.width(4.dp))
                    Text(
                        text = "插入占位符".loc,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Medium,
                        color = colors.accentIndigo
                    )
                }
            }
        }
    }
}
