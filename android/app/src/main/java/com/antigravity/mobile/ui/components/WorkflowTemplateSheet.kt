package com.antigravity.mobile.ui.components

import android.content.Context
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.BugReport
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.DragHandle
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Security
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
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
        WorkflowIconOption("sparkles", Icons.Default.AutoAwesome, "AI 增强")
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
    var isEditing by remember { mutableStateOf(false) }
    var editorDraft by remember { mutableStateOf<WorkflowEditorDraft?>(null) }
    var syncError by remember { mutableStateOf<String?>(null) }
    var showResetDialog by remember { mutableStateOf(false) }

    // Drag-and-drop state in edit mode
    var draggedId by remember { mutableStateOf<String?>(null) }
    var dragOffsetY by remember { mutableStateOf(0f) }
    val itemHeights = remember { mutableStateMapOf<String, Int>() }

    fun persistAndSync(nextList: List<WorkflowTemplateRecord>) {
        templates = nextList
        WorkflowTemplateLocalCache.save(context, nextList)
        syncError = null
        scope.launch {
            apiClient.saveWorkflowTemplates(nextList)
                .onSuccess { resp ->
                    templates = resp.templates
                    WorkflowTemplateLocalCache.save(context, resp.templates)
                }
                .onFailure { err ->
                    syncError = "已保存本机缓存，网关同步失败：${err.message ?: "未知错误"}"
                }
        }
    }

    fun moveItem(fromIndex: Int, toIndex: Int) {
        if (fromIndex == toIndex || fromIndex !in templates.indices || toIndex !in templates.indices) return
        val mutable = templates.toMutableList()
        val moved = mutable.removeAt(fromIndex)
        mutable.add(toIndex, moved)
        haptic.light()
        persistAndSync(mutable)
    }

    LaunchedEffect(Unit) {
        apiClient.fetchWorkflowTemplates()
            .onSuccess { resp ->
                templates = resp.templates
                WorkflowTemplateLocalCache.save(context, resp.templates)
                syncError = null
            }
    }

    ModalBottomSheet(
        onDismissRequest = onDismissRequest,
        sheetState = sheetState,
        containerColor = colors.surface
    ) {
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
                }
            )
        } else {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 28.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                // Top Bar: Edit toggle + Title + Add button
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    TextButton(
                        onClick = {
                            haptic.light()
                            isEditing = !isEditing
                        },
                        contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp)
                    ) {
                        Text(
                            text = if (isEditing) "完成" else "编辑",
                            color = colors.accentIndigo,
                            fontWeight = if (isEditing) FontWeight.Bold else FontWeight.Medium
                        )
                    }

                    Text(
                        text = "Workflow 模板",
                        style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                        color = colors.textPrimary
                    )

                    TextButton(
                        onClick = {
                            haptic.light()
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
                        contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Add,
                            contentDescription = "新增",
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(18.dp)
                        )
                        Spacer(modifier = Modifier.width(2.dp))
                        Text(
                            text = "新增",
                            color = colors.accentIndigo,
                            fontWeight = FontWeight.SemiBold
                        )
                    }
                }

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
                            text = if (isEditing) "按住左侧手柄拖拽或点箭头排序，支持编辑与删除" else "选择常用模板快速填充，支持自定义与排序",
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
                                val itemColor = WorkflowStyleCatalog.resolveColor(item.color, colors)
                                val itemIcon = WorkflowStyleCatalog.resolveIcon(item.icon)
                                val displaySub = item.subtitle.ifBlank { item.templatePrompt }

                                Surface(
                                    shape = RoundedCornerShape(12.dp),
                                    color = colors.surfaceVariant.copy(alpha = if (isDragging) 0.95f else 0.6f),
                                    shadowElevation = if (isDragging) 6.dp else 0.dp,
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .zIndex(if (isDragging) 1f else 0f)
                                        .graphicsLayer {
                                            translationY = if (isDragging) dragOffsetY else 0f
                                        }
                                        .onSizeChanged { size ->
                                            itemHeights[item.id] = size.height
                                        }
                                        .then(
                                            if (!isEditing) {
                                                Modifier.clickable {
                                                    haptic.medium()
                                                    onSelectTemplate(item.templatePrompt)
                                                    onDismissRequest()
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
                                                    .pointerInput(item.id, templates.size) {
                                                        detectDragGestures(
                                                            onDragStart = {
                                                                draggedId = item.id
                                                                dragOffsetY = 0f
                                                                haptic.light()
                                                            },
                                                            onDragEnd = {
                                                                draggedId = null
                                                                dragOffsetY = 0f
                                                                persistAndSync(templates)
                                                            },
                                                            onDragCancel = {
                                                                draggedId = null
                                                                dragOffsetY = 0f
                                                            },
                                                            onDrag = { change, dragAmount ->
                                                                change.consume()
                                                                dragOffsetY += dragAmount.y
                                                                val currentIdx = templates.indexOfFirst { it.id == item.id }
                                                                if (currentIdx >= 0) {
                                                                    val threshold = ((itemHeights[item.id] ?: 160) * 0.6f).coerceAtLeast(48f)
                                                                    if (dragOffsetY > threshold && currentIdx < templates.lastIndex) {
                                                                        val mutable = templates.toMutableList()
                                                                        val moving = mutable.removeAt(currentIdx)
                                                                        mutable.add(currentIdx + 1, moving)
                                                                        templates = mutable
                                                                        dragOffsetY -= threshold
                                                                        haptic.light()
                                                                    } else if (dragOffsetY < -threshold && currentIdx > 0) {
                                                                        val mutable = templates.toMutableList()
                                                                        val moving = mutable.removeAt(currentIdx)
                                                                        mutable.add(currentIdx - 1, moving)
                                                                        templates = mutable
                                                                        dragOffsetY += threshold
                                                                        haptic.light()
                                                                    }
                                                                }
                                                            }
                                                        )
                                                    },
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
                                            Row(
                                                verticalAlignment = Alignment.CenterVertically,
                                                horizontalArrangement = Arrangement.spacedBy(2.dp)
                                            ) {
                                                IconButton(
                                                    onClick = { moveItem(index, index - 1) },
                                                    enabled = index > 0,
                                                    modifier = Modifier.size(30.dp)
                                                ) {
                                                    Icon(
                                                        imageVector = Icons.Default.KeyboardArrowUp,
                                                        contentDescription = "上移",
                                                        tint = if (index > 0) colors.textPrimary else colors.textMuted.copy(alpha = 0.35f),
                                                        modifier = Modifier.size(18.dp)
                                                    )
                                                }
                                                IconButton(
                                                    onClick = { moveItem(index, index + 1) },
                                                    enabled = index < templates.lastIndex,
                                                    modifier = Modifier.size(30.dp)
                                                ) {
                                                    Icon(
                                                        imageVector = Icons.Default.KeyboardArrowDown,
                                                        contentDescription = "下移",
                                                        tint = if (index < templates.lastIndex) colors.textPrimary else colors.textMuted.copy(alpha = 0.35f),
                                                        modifier = Modifier.size(18.dp)
                                                    )
                                                }
                                                IconButton(
                                                    onClick = {
                                                        editorDraft = WorkflowEditorDraft(isNew = false, record = item)
                                                    },
                                                    modifier = Modifier.size(30.dp)
                                                ) {
                                                    Icon(
                                                        imageVector = Icons.Default.Edit,
                                                        contentDescription = "编辑",
                                                        tint = colors.accentIndigo,
                                                        modifier = Modifier.size(16.dp)
                                                    )
                                                }
                                                IconButton(
                                                    onClick = {
                                                        haptic.light()
                                                        persistAndSync(templates.filter { it.id != item.id })
                                                    },
                                                    modifier = Modifier.size(30.dp)
                                                ) {
                                                    Icon(
                                                        imageVector = Icons.Default.Delete,
                                                        contentDescription = "删除",
                                                        tint = colors.accentRed,
                                                        modifier = Modifier.size(16.dp)
                                                    )
                                                }
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
private fun WorkflowTemplateEditorContent(
    draft: WorkflowEditorDraft,
    colors: AntigravityColors,
    onBack: () -> Unit,
    onSave: (WorkflowTemplateRecord) -> Unit
) {
    var title by remember(draft) { mutableStateOf(draft.record.title) }
    var subtitle by remember(draft) { mutableStateOf(draft.record.subtitle) }
    var icon by remember(draft) { mutableStateOf(draft.record.icon.ifBlank { "workflow" }) }
    var colorKey by remember(draft) { mutableStateOf(draft.record.color.ifBlank { "indigo" }) }
    var prompt by remember(draft) { mutableStateOf(draft.record.templatePrompt) }

    val canSave = title.trim().isNotEmpty() && prompt.trim().isNotEmpty()
    val selectedColor = WorkflowStyleCatalog.resolveColor(colorKey, colors)

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp)
            .padding(bottom = 28.dp)
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Editor Header
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            TextButton(onClick = onBack) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = "返回",
                    tint = colors.textPrimary,
                    modifier = Modifier.size(18.dp)
                )
                Spacer(modifier = Modifier.width(4.dp))
                Text("返回", color = colors.textPrimary)
            }

            Text(
                text = if (draft.isNew) "新增 Workflow" else "编辑 Workflow",
                style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                color = colors.textPrimary
            )

            TextButton(
                onClick = {
                    onSave(
                        WorkflowTemplateRecord(
                            id = draft.record.id,
                            title = title.trim(),
                            subtitle = subtitle.trim(),
                            icon = icon,
                            color = colorKey,
                            templatePrompt = prompt.trim()
                        )
                    )
                },
                enabled = canSave
            ) {
                Text(
                    text = "保存",
                    color = if (canSave) colors.accentIndigo else colors.textMuted,
                    fontWeight = FontWeight.Bold
                )
            }
        }

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
            value = prompt,
            onValueChange = { prompt = it },
            label = { Text("提示词 / 命令模板（必填）") },
            placeholder = { Text("支持任意提示词或 /workflow、/plan 等命令，可用 [占位符] 提示补充参数") },
            minLines = 4,
            maxLines = 8,
            modifier = Modifier.fillMaxWidth()
        )
    }
}
