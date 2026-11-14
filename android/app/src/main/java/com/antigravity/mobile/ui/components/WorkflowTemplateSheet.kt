package com.antigravity.mobile.ui.components

import android.content.Context
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.foundation.BorderStroke
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
import androidx.compose.material.icons.automirrored.filled.CallSplit
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Assessment
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.BugReport
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Checklist
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.DragHandle
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Merge
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.pointer.pointerInput
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
import kotlin.math.roundToInt

private data class WorkflowIconOption(
    val key: String,
    val icon: ImageVector,
    val label: String
)

private data class WorkflowColorOption(
    val key: String,
    val label: String
)

private object WorkflowStyleCatalog {
    val icons = listOf(
        WorkflowIconOption("workflow", Icons.Default.AccountTree, "工作流"),
        WorkflowIconOption("security", Icons.Default.Security, "安全审查"),
        WorkflowIconOption("bug", Icons.Default.BugReport, "测试修复"),
        WorkflowIconOption("search", Icons.Default.Search, "调研分析"),
        WorkflowIconOption("bolt", Icons.Default.Bolt, "快捷执行"),
        WorkflowIconOption("code", Icons.Default.Code, "代码开发"),
        WorkflowIconOption("terminal", Icons.Default.Terminal, "终端脚本"),
        WorkflowIconOption("doc", Icons.Default.Description, "文档总结"),
        WorkflowIconOption("star", Icons.Default.Star, "常用收藏"),
        WorkflowIconOption("sparkles", Icons.Default.AutoAwesome, "AI 增强"),
        WorkflowIconOption("git", Icons.AutoMirrored.Filled.CallSplit, "Git 分支"),
        WorkflowIconOption("merge", Icons.Default.Merge, "分支合并"),
        WorkflowIconOption("pipeline", Icons.Default.Autorenew, "流水线"),
        WorkflowIconOption("checklist", Icons.Default.Checklist, "流程清单"),
        WorkflowIconOption("report", Icons.Default.Assessment, "分析报告")
    )

    val colors = listOf(
        WorkflowColorOption("indigo", "靛蓝"),
        WorkflowColorOption("green", "翠绿"),
        WorkflowColorOption("blue", "湛蓝"),
        WorkflowColorOption("purple", "亮紫"),
        WorkflowColorOption("orange", "活力橙"),
        WorkflowColorOption("red", "珊瑚红"),
        WorkflowColorOption("teal", "青碧")
    )

    fun resolveIcon(key: String): ImageVector =
        icons.firstOrNull { it.key == key }?.icon ?: Icons.Default.AccountTree

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

private object WorkflowTemplateLocalCache {
    private const val PREF_NAME = "agy_workflow_templates"
    private const val KEY_LIST = "templates_json_v1"
    private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }

    fun load(context: Context): List<WorkflowTemplateRecord> {
        val prefs = context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        val raw = prefs.getString(KEY_LIST, null) ?: return WorkflowTemplateDefaults.all
        return runCatching {
            json.decodeFromString<List<WorkflowTemplateRecord>>(raw)
        }.getOrElse { WorkflowTemplateDefaults.all }
    }

    fun save(context: Context, list: List<WorkflowTemplateRecord>) {
        val prefs = context.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE)
        prefs.edit().putString(KEY_LIST, json.encodeToString(list)).apply()
    }
}

private data class WorkflowEditorDraft(
    val isNew: Boolean,
    val record: WorkflowTemplateRecord
)

/**
 * 动态多智能体工作流模板选择、自定义与排序抽屉 (WorkflowTemplateSheet)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorkflowTemplateSheet(
    onDismissRequest: () -> Unit,
    onSelectTemplate: (String) -> Unit
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = false)
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
    var isEditing by remember { mutableStateOf(false) }
    var editorDraft by remember { mutableStateOf<WorkflowEditorDraft?>(null) }
    var syncError by remember { mutableStateOf<String?>(null) }
    var showResetDialog by remember { mutableStateOf(false) }
    var swipedId by remember { mutableStateOf<String?>(null) }

    // Visual-only drag-and-drop state in edit mode (list commits once on drop to prevent flicker)
    val density = LocalDensity.current
    val rowSpacingPx = with(density) { 10.dp.toPx() }
    var draggedId by remember { mutableStateOf<String?>(null) }
    var dragStartIndex by remember { mutableStateOf<Int?>(null) }
    var dragTargetIndex by remember { mutableStateOf<Int?>(null) }
    var dragOffsetY by remember { mutableStateOf(0f) }
    val itemHeights = remember { mutableStateMapOf<String, Int>() }

    fun resetDragState() {
        draggedId = null
        dragStartIndex = null
        dragTargetIndex = null
        dragOffsetY = 0f
    }

    fun targetIndexForOffset(startIdx: Int, offsetY: Float): Int {
        if (templates.isEmpty() || startIdx !in templates.indices) return startIdx
        var remaining = offsetY
        var target = startIdx
        if (offsetY > 0f) {
            while (target < templates.lastIndex) {
                val nextH = (itemHeights[templates[target + 1].id] ?: 160).toFloat()
                val step = (nextH + rowSpacingPx) * 0.55f
                if (remaining >= step) {
                    remaining -= (nextH + rowSpacingPx)
                    target += 1
                } else {
                    break
                }
            }
        } else if (offsetY < 0f) {
            while (target > 0) {
                val prevH = (itemHeights[templates[target - 1].id] ?: 160).toFloat()
                val step = (prevH + rowSpacingPx) * 0.55f
                if (-remaining >= step) {
                    remaining += (prevH + rowSpacingPx)
                    target -= 1
                } else {
                    break
                }
            }
        }
        return target
    }

    fun siblingDisplacementY(index: Int): Float {
        val activeId = draggedId ?: return 0f
        val start = dragStartIndex ?: return 0f
        val target = dragTargetIndex ?: return 0f
        if (index == start) return 0f
        val draggedStep = ((itemHeights[activeId] ?: 160).toFloat()) + rowSpacingPx
        return when {
            start < target && index in (start + 1)..target -> -draggedStep
            start > target && index in target until start -> draggedStep
            else -> 0f
        }
    }

    fun persistAndSync(nextList: List<WorkflowTemplateRecord>) {
        templates = nextList
        WorkflowTemplateLocalCache.save(context, nextList)
        syncError = null
        scope.launch {
            apiClient.saveWorkflowTemplates(nextList)
                .onSuccess { resp ->
                    if (resp.templates != templates) {
                        templates = resp.templates
                        WorkflowTemplateLocalCache.save(context, resp.templates)
                    }
                }
                .onFailure { err ->
                    syncError = "已保存本机缓存，网关同步失败：${err.message ?: "未知错误"}"
                }
        }
    }

    LaunchedEffect(Unit) {
        apiClient.fetchWorkflowTemplates()
            .onSuccess { resp ->
                if (resp.templates != templates) {
                    templates = resp.templates
                    WorkflowTemplateLocalCache.save(context, resp.templates)
                }
                syncError = null
            }
    }

    LaunchedEffect(editorDraft != null) {
        if (editorDraft != null) {
            sheetState.expand()
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
                    // Top Bar: Icon Edit/Done toggle on left, Title in center, + icon only on right (matching AccountQuotaSheet)
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
                                contentDescription = if (isEditing) "完成" else "编辑",
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(19.dp)
                            )
                        }

                        Text(
                            text = "Workflow 模板",
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
                                        templatePrompt = ""
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
                                contentDescription = "新增",
                                tint = colors.accentIndigo,
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

                // Header Banner
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(36.dp)
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.18f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.AccountTree,
                            contentDescription = null,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(20.dp)
                        )
                    }

                    Column {
                        Text(
                            text = "Antigravity 动态工作流",
                            style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.Bold),
                            color = colors.textPrimary
                        )
                        Text(
                            text = if (isEditing) "按住最左侧手柄拖拽排序，左滑可删除模板" else "选择常用模板快速填充，左滑可删除",
                            style = MaterialTheme.typography.bodySmall,
                            color = colors.textSecondary
                        )
                    }
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
                            text = "暂无 Workflow 模板",
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
                                Text("+ 新增模板")
                            }
                            OutlinedButton(
                                onClick = { showResetDialog = true }
                            ) {
                                Text("恢复默认")
                            }
                        }
                    }
                } else {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        templates.forEachIndexed { index, item ->
                            key(item.id) {
                                val isDragging = draggedId == item.id
                                val isAnyDragging = draggedId != null
                                val displacementTarget = siblingDisplacementY(index)
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
                                    dragHandleModifier = Modifier.pointerInput(item.id, templates.size) {
                                        detectDragGestures(
                                            onDragStart = {
                                                swipedId = null
                                                val idx = templates.indexOfFirst { it.id == item.id }
                                                if (idx >= 0) {
                                                    draggedId = item.id
                                                    dragStartIndex = idx
                                                    dragTargetIndex = idx
                                                    dragOffsetY = 0f
                                                    haptic.light()
                                                }
                                            },
                                            onDragEnd = {
                                                val start = dragStartIndex
                                                val target = dragTargetIndex
                                                val moved = if (start != null && target != null && start != target &&
                                                    start in templates.indices && target in templates.indices
                                                ) {
                                                    val mutable = templates.toMutableList()
                                                    val itemToMove = mutable.removeAt(start)
                                                    mutable.add(target, itemToMove)
                                                    mutable
                                                } else {
                                                    null
                                                }
                                                resetDragState()
                                                if (moved != null) {
                                                    persistAndSync(moved)
                                                }
                                            },
                                            onDragCancel = {
                                                resetDragState()
                                            },
                                            onDrag = { change, dragAmount ->
                                                change.consume()
                                                val start = dragStartIndex ?: return@detectDragGestures
                                                dragOffsetY += dragAmount.y
                                                val nextTarget = targetIndexForOffset(start, dragOffsetY)
                                                if (nextTarget != dragTargetIndex) {
                                                    dragTargetIndex = nextTarget
                                                    haptic.light()
                                                }
                                            }
                                        )
                                    }
                                )
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
                            text = "恢复默认模板",
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
            title = { Text("恢复默认模板") },
            text = { Text("将恢复为系统初始的 4 个模板，当前自定义模板和排序会被替换。") },
            confirmButton = {
                TextButton(
                    onClick = {
                        showResetDialog = false
                        val defaults = WorkflowTemplateDefaults.all
                        templates = defaults
                        WorkflowTemplateLocalCache.save(context, defaults)
                        syncError = null
                        scope.launch {
                            apiClient.resetWorkflowTemplates()
                                .onSuccess { resp ->
                                    templates = resp.templates
                                    WorkflowTemplateLocalCache.save(context, resp.templates)
                                }
                                .onFailure { err ->
                                    syncError = "已重置本机列表，网关同步失败：${err.message ?: "未知错误"}"
                                }
                        }
                    }
                ) {
                    Text("确认恢复", color = colors.accentRed)
                }
            },
            dismissButton = {
                TextButton(onClick = { showResetDialog = false }) {
                    Text("取消")
                }
            }
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
    val itemIcon = WorkflowStyleCatalog.resolveIcon(item.icon)
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
                        contentDescription = "删除",
                        tint = Color.White,
                        modifier = Modifier.size(18.dp)
                    )
                }
            }
        }

        // Foreground card (horizontal swipe pointerInput placed before offset to avoid coordinate feedback jitter)
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
                .then(
                    if (!isEditing) {
                        Modifier.clickable {
                            if (offsetX.value < -10f) {
                                scope.launch {
                                    offsetX.animateTo(0f, spring(stiffness = Spring.StiffnessMediumLow))
                                    onSwipeClose()
                                }
                            } else {
                                onSelect()
                            }
                        }
                    } else {
                        Modifier
                    }
                )
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
                            contentDescription = "拖拽排序",
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
                    Icon(
                        imageVector = itemIcon,
                        contentDescription = null,
                        tint = itemColor,
                        modifier = Modifier.size(20.dp)
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

                if (isEditing) {
                    IconButton(
                        onClick = onEdit,
                        modifier = Modifier.size(32.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Edit,
                            contentDescription = "编辑",
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(18.dp)
                        )
                    }
                } else {
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
    var icon by remember(draft) { mutableStateOf(draft.record.icon.ifBlank { "workflow" }) }
    var colorKey by remember(draft) { mutableStateOf(draft.record.color.ifBlank { "indigo" }) }
    var promptValue by remember(draft) { mutableStateOf(TextFieldValue(draft.record.templatePrompt)) }
    val promptFocusRequester = remember { FocusRequester() }

    val canSave = title.trim().isNotEmpty() && promptValue.text.trim().isNotEmpty()
    val selectedColor = WorkflowStyleCatalog.resolveColor(colorKey, colors)

    Column(modifier = modifier.fillMaxSize()) {
        // Editor Header: icon-only Back on left, centered Title, icon-only Checkmark Save on right (matching AccountQuotaSheet)
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
                    contentDescription = "返回",
                    tint = colors.textPrimary,
                    modifier = Modifier.size(19.dp)
                )
            }

            Text(
                text = if (draft.isNew) "新增 Workflow" else "编辑 Workflow",
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
                            templatePrompt = promptValue.text.trim()
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
                    contentDescription = "保存",
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
            label = { Text("标题（必填）") },
            placeholder = { Text("例如：每日代码巡检") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth()
        )

        OutlinedTextField(
            value = subtitle,
            onValueChange = { subtitle = it },
            label = { Text("简介（可选）") },
            placeholder = { Text("简短描述用途，方便快速辨识") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth()
        )

        Text(
            text = "主题色",
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
            text = "图标",
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
                            Icon(
                                imageVector = ic.icon,
                                contentDescription = ic.label,
                                tint = if (active) selectedColor else colors.textSecondary,
                                modifier = Modifier.size(20.dp)
                            )
                        }
                    }
                }
            }
        }

        OutlinedTextField(
            value = promptValue,
            onValueChange = { promptValue = it },
            label = { Text("提示词 / 命令模板（必填）") },
            placeholder = { Text("支持任意提示词或 /workflow、/plan 等命令，可用 [占位符] 提示补充参数") },
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
                text = "占位符在编辑框中可以直接选中",
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
                    val token = "[占位符]"
                    val nextText = buildString {
                        append(current.text.substring(0, start))
                        append(token)
                        append(current.text.substring(end))
                    }
                    // Select "占位符" inside [...] so user can type to replace it directly
                    promptValue = TextFieldValue(
                        text = nextText,
                        selection = TextRange(start + 1, start + 1 + "占位符".length)
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
                    text = "插入占位符",
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium,
                    color = colors.accentIndigo
                )
            }
        }
        }
    }
}
