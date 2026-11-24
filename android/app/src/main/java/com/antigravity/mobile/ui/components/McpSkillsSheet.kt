package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.createAutomation
import com.antigravity.mobile.data.service.deleteAutomation
import com.antigravity.mobile.data.service.deleteMemory
import com.antigravity.mobile.data.service.fetchMemories
import com.antigravity.mobile.data.service.refreshMcpServers
import com.antigravity.mobile.data.service.toggleMcpServer
import com.antigravity.mobile.data.service.upsertMemory
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.async
import kotlinx.coroutines.launch

enum class McpSkillsTab(val index: Int) {
    MCP(0),
    SKILLS(1),
    PLUGINS(2),
    AUTOMATIONS(3),
    TASKS(4);

    val label: String
        get() = when (this) {
            MCP -> "MCP"
            SKILLS -> "技能".loc
            PLUGINS -> "插件".loc
            AUTOMATIONS -> "定时".loc
            TASKS -> "任务".loc
        }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun McpSkillsSheet(
    onDismiss: () -> Unit,
    initialTab: McpSkillsTab = McpSkillsTab.MCP,
    currentConversationId: String? = null,
    hasActiveSwarm: Boolean = false,
    onSelectConversation: ((cascadeId: String, title: String) -> Unit)? = null,
    onTasksChanged: ((List<GlobalBackgroundTaskItem>) -> Unit)? = null,
    onOpenSwarm: (() -> Unit)? = null,
    modifier: Modifier = Modifier
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val scope = rememberCoroutineScope()
    val prefs = remember { PreferencesManager(context.applicationContext) }
    val apiClient = remember {
        ApiClient.defaultInstance ?: ApiClient(
            context.applicationContext,
            prefs
        )
    }

    var selectedTab by remember(initialTab) { mutableStateOf(initialTab) }
    var searchText by remember { mutableStateOf("") }
    var isLoading by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    var mcpServers by remember { mutableStateOf(prefs.loadCachedMcpServers()) }
    var skills by remember { mutableStateOf(prefs.loadCachedSkills()) }
    var plugins by remember { mutableStateOf<List<CascadePluginItem>>(emptyList()) }
    var pluginProviders by remember { mutableStateOf<List<PluginAuthProviderItem>>(emptyList()) }
    var pluginPermissionGrants by remember { mutableStateOf<List<PluginPermissionGrantItem>>(emptyList()) }
    val revokingGrantKeys = remember { mutableStateListOf<String>() }
    var automations by remember { mutableStateOf<List<AutomationSidecarItem>>(emptyList()) }
    var automationEvents by remember { mutableStateOf<List<AutomationEventItem>>(emptyList()) }
    var backgroundTasks by remember { mutableStateOf<List<GlobalBackgroundTaskItem>>(emptyList()) }
    var expandedServers by remember { mutableStateOf<Set<String>>(emptySet()) }
    var expandedTools by remember { mutableStateOf<Set<String>>(emptySet()) }
    var viewingSkill by remember { mutableStateOf<SkillInfo?>(null) }
    val installingPluginIds = remember { mutableStateListOf<String>() }
    val managingAutomationNames = remember { mutableStateListOf<String>() }

    // Phase 3: Memories CRUD, Automation Create dialog, MCP refresh state
    var memories by remember { mutableStateOf<List<MemoryRuleItem>>(emptyList()) }
    var showMemoriesPanel by remember { mutableStateOf(false) }
    var editingMemory by remember { mutableStateOf<MemoryRuleItem?>(null) }
    var pendingDeleteMemory by remember { mutableStateOf<MemoryRuleItem?>(null) }
    var showNewAutomationDialog by remember { mutableStateOf(false) }
    var selectedAutomationId by remember { mutableStateOf<String?>(null) }
    var automationActionError by remember { mutableStateOf<String?>(null) }
    var isRefreshingMcp by remember { mutableStateOf(false) }

    var expandedLogTaskIds by remember { mutableStateOf<Set<String>>(emptySet()) }
    var taskLogs by remember { mutableStateOf<Map<String, String>>(emptyMap()) }
    var loadingLogTaskIds by remember { mutableStateOf<Set<String>>(emptySet()) }
    var stoppingTaskIds by remember { mutableStateOf<Set<String>>(emptySet()) }
    var copiedTaskId by remember { mutableStateOf<String?>(null) }

    fun applyDemoData() {
        mcpServers = listOf(
            McpServerState(
                spec = McpServerSpec(serverName = "ifind", command = "python3 -m ifind_mcp.server run"),
                status = "MCP_SERVER_STATUS_READY",
                tools = listOf(
                    McpToolDefinition(
                        name = "get_realtime_quotation",
                        description = "查询证券实时行情、最新价、涨跌幅、盘口及资金流".loc,
                        serverName = "ifind",
                        jsonSchemaString = "{\"properties\":{\"codes\":{\"type\":\"string\",\"description\":\"证券代码，如 600519.SH\"}},\"required\":[\"codes\"]}".loc
                    ),
                    McpToolDefinition(
                        name = "get_history_quotation",
                        description = "查询历史K线行情与序列数据".loc,
                        serverName = "ifind",
                        jsonSchemaString = "{\"properties\":{\"codes\":{\"type\":\"string\"},\"period\":{\"type\":\"string\",\"default\":\"D\"}},\"required\":[\"codes\"]}"
                    )
                ),
                serverInfo = McpServerInfoDetail(name = "ifind-mcp", version = "4.0.0"),
                instructions = "iFinD 金融数据终端服务接口".loc
            ),
            McpServerState(
                spec = McpServerSpec(serverName = "weather", command = "uv run weather-server"),
                status = "MCP_SERVER_STATUS_READY",
                tools = listOf(
                    McpToolDefinition(
                        name = "get_current_weather",
                        description = "获取指定城市的当前实时天气情况".loc,
                        serverName = "weather",
                        jsonSchemaString = "{\"properties\":{\"city\":{\"type\":\"string\",\"description\":\"城市名称\"}},\"required\":[\"city\"]}".loc
                    )
                ),
                serverInfo = McpServerInfoDetail(name = "weather-service", version = "1.2.0"),
                instructions = "全球实时与未来天气查询".loc
            ),
            McpServerState(
                spec = McpServerSpec(serverName = "message_hub", command = "node message_hub.js"),
                status = "MCP_SERVER_STATUS_READY",
                tools = listOf(
                    McpToolDefinition(
                        name = "send_message",
                        description = "统一向微信或元宝渠道发送通知与长图".loc,
                        serverName = "message_hub",
                        jsonSchemaString = "{\"properties\":{\"target\":{\"type\":\"string\"},\"text\":{\"type\":\"string\"}},\"required\":[\"target\",\"text\"]}"
                    )
                ),
                serverInfo = McpServerInfoDetail(name = "message-hub", version = "2.1.0"),
                instructions = "统一消息与多端通信路由".loc
            )
        )
        skills = listOf(
            SkillInfo(name = "chart-generator", description = "自动化图表生成与多维度可视化分析".loc, path = "builtin/skills/chart-generator/SKILL.md", content = "# Chart Generator\n自动根据输入数据渲染交互图表".loc),
            SkillInfo(name = "sql-inspector", description = "SQL 查询调优与执行计划诊断".loc, path = "builtin/skills/sql-inspector/SKILL.md", content = "# SQL Inspector\n分析 SQL 慢查询并提出索引优化方案".loc),
            SkillInfo(name = "jiuge-market-analysis", description = "首席宏观研判与市场逻辑观点体系".loc, path = "config/skills/jiuge-market-analysis/SKILL.md", content = "# Jiuge Market Analysis\n宏观与大宗商品核心驱动逻辑".loc)
        )
    }

    fun loadData() {
        scope.launch {
            isLoading = true
            errorMessage = null
            try {
                val tasksDeferred = async { apiClient.fetchGlobalBackgroundTasks() }
                val mcpDeferred = async { apiClient.fetchMcpServerStates() }
                val skillsDeferred = async { apiClient.fetchAllSkills() }
                val pluginsDeferred = async { apiClient.fetchPluginsCatalog() }
                val automationsDeferred = async { apiClient.fetchAutomationsData() }
                val memoriesDeferred = async { apiClient.fetchMemories() }

                tasksDeferred.await().onSuccess {
                    backgroundTasks = it
                    onTasksChanged?.invoke(it)
                }
                pluginsDeferred.await().onSuccess {
                    plugins = it.plugins
                    pluginProviders = it.providers
                    pluginPermissionGrants = it.permissionGrants
                }
                automationsDeferred.await().onSuccess {
                    automations = it.automations
                    automationEvents = it.events
                }
                memoriesDeferred.await().onSuccess {
                    memories = it
                }
                val mcpRes = mcpDeferred.await()
                val skillsRes = skillsDeferred.await()

                mcpRes.onSuccess {
                    mcpServers = it
                    prefs.saveCachedMcpServers(it)
                }
                skillsRes.onSuccess {
                    skills = it
                    prefs.saveCachedSkills(it)
                }
                if (mcpRes.isFailure && skillsRes.isFailure && mcpServers.isEmpty() && skills.isEmpty()) {
                    applyDemoData()
                }
            } catch (_: Exception) {
                if (mcpServers.isEmpty() && skills.isEmpty()) {
                    applyDemoData()
                }
            } finally {
                isLoading = false
            }
        }
    }

    fun fetchTaskLog(task: GlobalBackgroundTaskItem) {
        scope.launch {
            loadingLogTaskIds = loadingLogTaskIds + task.uniqueId
            apiClient.fetchBackgroundTaskLog(
                conversationId = task.conversationId,
                taskId = task.taskId,
                logUri = task.logUri
            ).onSuccess { resp ->
                taskLogs = taskLogs + (task.uniqueId to resp.content)
            }
            loadingLogTaskIds = loadingLogTaskIds - task.uniqueId
        }
    }

    fun stopSingleTask(task: GlobalBackgroundTaskItem) {
        scope.launch {
            stoppingTaskIds = stoppingTaskIds + task.uniqueId
            apiClient.stopTask(task.conversationId, task.taskId, task.stepIndex)
            stoppingTaskIds = stoppingTaskIds - task.uniqueId
            val updated = backgroundTasks.filterNot { it.uniqueId == task.uniqueId }
            backgroundTasks = updated
            onTasksChanged?.invoke(updated)
            apiClient.fetchGlobalBackgroundTasks().onSuccess {
                backgroundTasks = it
                onTasksChanged?.invoke(it)
            }
        }
    }

    fun stopAllTasks(tasks: List<GlobalBackgroundTaskItem>) {
        scope.launch {
            stoppingTaskIds = stoppingTaskIds + tasks.map { it.uniqueId }
            for (t in tasks) {
                apiClient.stopTask(t.conversationId, t.taskId, t.stepIndex)
            }
            stoppingTaskIds = emptySet()
            backgroundTasks = emptyList()
            onTasksChanged?.invoke(emptyList())
            apiClient.fetchGlobalBackgroundTasks().onSuccess {
                backgroundTasks = it
                onTasksChanged?.invoke(it)
            }
        }
    }

    LaunchedEffect(Unit) {
        loadData()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.background, onDismiss = onDismiss) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .fillMaxHeight()
            ) {
                // Floating grab handle hinting pull-down dismissal (matching iOS Capsule 38x5 / NewConversationSheet)
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 10.dp, bottom = 18.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Box(
                        modifier = Modifier
                            .size(width = 38.dp, height = 5.dp)
                            .clip(CircleShape)
                            .background(colors.textMuted.copy(alpha = 0.35f))
                    )
                }

                // Header title: Centered, no icon, aligned with NewConversationSheet
                Text(
                    text = "工具与任务看板".loc,
                    fontSize = 18.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 12.dp)
                )

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(horizontal = 16.dp)
                        .padding(top = 12.dp)
                ) {

            // Tab Row
            TabRow(
                selectedTabIndex = selectedTab.index,
                containerColor = colors.surface,
                contentColor = colors.accentIndigo,
                modifier = Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .padding(bottom = 8.dp)
            ) {
                McpSkillsTab.entries.forEach { tab ->
                    val tabText = if (tab == McpSkillsTab.TASKS && backgroundTasks.isNotEmpty()) {
                        "${tab.label}(${backgroundTasks.size})"
                    } else {
                        tab.label
                    }
                    Tab(
                        selected = selectedTab == tab,
                        selectedContentColor = colors.accentIndigo,
                        unselectedContentColor = colors.textSecondary,
                        onClick = {
                            haptic.light()
                            selectedTab = tab
                        },
                        text = {
                            Text(
                                text = tabText,
                                fontWeight = if (selectedTab == tab) FontWeight.Bold else FontWeight.Normal,
                                fontSize = 12.sp,
                                maxLines = 1,
                                softWrap = false
                            )
                        }
                    )
                }
            }

            // Content Area with Bottom Floating Search Bar
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
            ) {
                // Content List
            if (isLoading && mcpServers.isEmpty() && skills.isEmpty()) {
                Box(
                    modifier = Modifier.fillMaxSize(),
                    contentAlignment = Alignment.Center
                ) {
                    CircularProgressIndicator(
                        color = colors.accentIndigo,
                        modifier = Modifier.size(32.dp)
                    )
                }
            } else if (selectedTab == McpSkillsTab.MCP) {
                // MCP Tab
                val query = searchText.trim().lowercase()
                val filteredServers = remember(mcpServers, query) {
                    if (query.isEmpty()) mcpServers
                    else mcpServers.filter { s ->
                        s.spec.serverName.lowercase().contains(query) ||
                        (s.serverInfo?.name?.lowercase()?.contains(query) == true) ||
                        (s.instructions?.lowercase()?.contains(query) == true) ||
                        (s.tools?.any { t ->
                            t.name.lowercase().contains(query) ||
                            (t.description?.lowercase()?.contains(query) == true)
                        } == true)
                    }
                }

                if (filteredServers.isEmpty()) {
                    Column(
                        modifier = Modifier.fillMaxSize(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.Center
                    ) {
                        Text(
                            text = "未找到匹配的 MCP 服务或工具".loc,
                            fontSize = 14.sp,
                            color = colors.textSecondary
                        )
                        Spacer(Modifier.height(10.dp))
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    isRefreshingMcp = true
                                    apiClient.refreshMcpServers()
                                    loadData()
                                    isRefreshingMcp = false
                                }
                            },
                            enabled = !isRefreshingMcp
                        ) {
                            Text(if (isRefreshingMcp) "正在刷新...".loc else "刷新 MCP 状态".loc, fontSize = 13.sp)
                        }
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                        contentPadding = PaddingValues(top = 4.dp, bottom = 84.dp)
                    ) {
                        item(key = "mcp_refresh_bar") {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = "已配置 ${filteredServers.size} 个服务".loc,
                                    fontSize = 12.sp,
                                    color = colors.textMuted
                                )
                                TextButton(
                                    enabled = !isRefreshingMcp,
                                    onClick = {
                                        scope.launch {
                                            isRefreshingMcp = true
                                            apiClient.refreshMcpServers()
                                            loadData()
                                            isRefreshingMcp = false
                                        }
                                    },
                                    contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                                ) {
                                    Text(if (isRefreshingMcp) "刷新中...".loc else "↻ 刷新 MCP".loc, fontSize = 12.sp, color = colors.accentIndigo)
                                }
                            }
                        }
                        items(filteredServers, key = { it.spec.serverName }) { server ->
                            McpServerCard(
                                server = server,
                                isExpanded = expandedServers.contains(server.spec.serverName) || query.isNotEmpty(),
                                onToggleExpand = {
                                    haptic.light()
                                    expandedServers = if (expandedServers.contains(server.spec.serverName)) {
                                        expandedServers - server.spec.serverName
                                    } else {
                                        expandedServers + server.spec.serverName
                                    }
                                },
                                expandedTools = expandedTools,
                                onToggleTool = { toolId ->
                                    haptic.light()
                                    expandedTools = if (expandedTools.contains(toolId)) {
                                        expandedTools - toolId
                                    } else {
                                        expandedTools + toolId
                                    }
                                },
                                onToggleEnabled = { enabled ->
                                    haptic.light()
                                    scope.launch {
                                        apiClient.toggleMcpServer(server.spec.serverName, enabled)
                                        loadData()
                                    }
                                }
                            )
                        }
                    }
                }
            } else if (selectedTab == McpSkillsTab.SKILLS) {
                // Skills Tab + embedded Agent Memories accordion
                val query = searchText.trim().lowercase()
                val filteredSkills = remember(skills, query) {
                    if (query.isEmpty()) skills
                    else skills.filter { sk ->
                        sk.name.lowercase().contains(query) ||
                        (sk.description?.lowercase()?.contains(query) == true)
                    }
                }
                val filteredMemories = remember(memories, query) {
                    if (query.isEmpty()) memories
                    else memories.filter { m ->
                        m.title.lowercase().contains(query) || m.content.lowercase().contains(query)
                    }
                }

                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    contentPadding = PaddingValues(top = 4.dp, bottom = 84.dp)
                ) {
                    item(key = "agent_memories_accordion") {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(12.dp))
                                .background(colors.surface)
                                .border(0.5.dp, colors.border, RoundedCornerShape(12.dp))
                                .padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable { haptic.light(); showMemoriesPanel = !showMemoriesPanel },
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = "🧠 Agent 记忆库与规则 (${filteredMemories.size})".loc,
                                    fontSize = 14.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textPrimary,
                                    modifier = Modifier.weight(1f)
                                )
                                TextButton(
                                    onClick = { editingMemory = MemoryRuleItem(id = "", scope = "memory", title = "", content = "") },
                                    contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                                ) {
                                    Text("+ 新增".loc, fontSize = 12.sp, color = colors.accentIndigo)
                                }
                                Icon(
                                    imageVector = if (showMemoriesPanel) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                                    contentDescription = null,
                                    tint = colors.textMuted,
                                    modifier = Modifier.size(18.dp)
                                )
                            }
                            if (showMemoriesPanel) {
                                if (filteredMemories.isEmpty()) {
                                    Text("暂无记忆或规则条目".loc, fontSize = 12.5.sp, color = colors.textMuted)
                                } else {
                                    filteredMemories.forEach { mem ->
                                        Row(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .clip(RoundedCornerShape(8.dp))
                                                .background(colors.surfaceVariant.copy(alpha = 0.5f))
                                                .clickable { editingMemory = mem }
                                                .padding(horizontal = 10.dp, vertical = 8.dp),
                                            verticalAlignment = Alignment.CenterVertically
                                        ) {
                                            Column(modifier = Modifier.weight(1f)) {
                                                Text(
                                                    text = mem.title.ifBlank { mem.id },
                                                    fontSize = 13.sp,
                                                    fontWeight = FontWeight.Medium,
                                                    color = colors.textPrimary,
                                                    maxLines = 1,
                                                    overflow = TextOverflow.Ellipsis
                                                )
                                                Text(
                                                    text = mem.content,
                                                    fontSize = 11.5.sp,
                                                    color = colors.textSecondary,
                                                    maxLines = 2,
                                                    overflow = TextOverflow.Ellipsis
                                                )
                                            }
                                            if (!mem.readOnly) {
                                                IconButton(
                                                    onClick = { pendingDeleteMemory = mem },
                                                    modifier = Modifier.size(28.dp)
                                                ) {
                                                    Icon(Icons.Default.Delete, contentDescription = "删除记忆".loc, tint = Color(0xFFF44336), modifier = Modifier.size(16.dp))
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }

                    if (filteredSkills.isEmpty()) {
                        item(key = "skills_empty") {
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 40.dp),
                                contentAlignment = Alignment.Center
                            ) {
                                Text(
                                    text = "未找到匹配的技能".loc,
                                    fontSize = 14.sp,
                                    color = colors.textSecondary
                                )
                            }
                        }
                    } else {
                        items(filteredSkills, key = { it.name }) { skill ->
                            SkillCard(
                                skill = skill,
                                onClick = {
                                    haptic.light()
                                    viewingSkill = skill
                                }
                            )
                        }
                    }
                }
            } else if (selectedTab == McpSkillsTab.PLUGINS) {
                // Plugins Tab
                val query = searchText.trim().lowercase()
                val filteredPlugins = remember(plugins, query) {
                    if (query.isEmpty()) plugins
                    else plugins.filter { p ->
                        p.name.lowercase().contains(query) ||
                        p.effectiveId.lowercase().contains(query) ||
                        p.description.lowercase().contains(query)
                    }
                }

                if (filteredPlugins.isEmpty() && pluginProviders.isEmpty() && pluginPermissionGrants.isEmpty()) {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "暂无插件或未找到匹配项".loc,
                            fontSize = 14.sp,
                            color = colors.textSecondary
                        )
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                        contentPadding = PaddingValues(top = 4.dp, bottom = 84.dp)
                    ) {
                        if (pluginProviders.isNotEmpty()) {
                            item(key = "plugin_providers_header") {
                                Text(
                                    text = "插件账号与认证状态 (${pluginProviders.size})".loc,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(top = 2.dp, bottom = 2.dp)
                                )
                            }
                            itemsIndexed(pluginProviders, key = { index, it -> "prov_${index}_${it.id}_${it.name}" }) { _, prov ->
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(colors.surface)
                                        .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                                        .padding(horizontal = 12.dp, vertical = 9.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.SpaceBetween
                                ) {
                                    Text(
                                        text = prov.name.ifBlank { prov.id },
                                        color = colors.textPrimary,
                                        fontSize = 13.5.sp,
                                        fontWeight = FontWeight.Medium
                                    )
                                    Text(
                                        text = if (prov.signedIn) "已连接".loc else "未登录".loc,
                                        color = if (prov.signedIn) colors.accentGreen else colors.textSecondary,
                                        fontSize = 11.5.sp,
                                        fontWeight = FontWeight.SemiBold
                                    )
                                }
                            }
                        }

                        if (pluginPermissionGrants.isNotEmpty()) {
                            item(key = "plugin_grants_header") {
                                Text(
                                    text = "已授权插件权限 (${pluginPermissionGrants.size})".loc,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(top = 6.dp, bottom = 2.dp)
                                )
                            }
                            itemsIndexed(pluginPermissionGrants, key = { index, it -> "grant_${index}_${it.uniqueKey}" }) { _, item ->
                                val isRevoking = revokingGrantKeys.contains(item.uniqueKey)
                                val scopeLabel = when (item.scope) {
                                    "PLUGIN_PERMISSION_GRANT_SCOPE_GLOBAL" -> "全局".loc
                                    "PLUGIN_PERMISSION_GRANT_SCOPE_PROJECT" -> item.projectName?.takeIf { it.isNotBlank() } ?: "项目".loc
                                    "PLUGIN_PERMISSION_GRANT_SCOPE_DEFAULT" -> "默认".loc
                                    else -> "已授权".loc
                                }
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(colors.surface)
                                        .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                                        .padding(horizontal = 12.dp, vertical = 9.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                                        Row(
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                                        ) {
                                            Text(
                                                text = item.displayTitle,
                                                color = colors.textPrimary,
                                                fontSize = 13.sp,
                                                fontWeight = FontWeight.SemiBold,
                                                maxLines = 2,
                                                overflow = TextOverflow.Ellipsis,
                                                modifier = Modifier.weight(1f, fill = false)
                                            )
                                            Box(
                                                modifier = Modifier
                                                    .clip(RoundedCornerShape(6.dp))
                                                    .background(colors.accentIndigo.copy(alpha = 0.12f))
                                                    .padding(horizontal = 6.dp, vertical = 2.dp)
                                            ) {
                                                Text(
                                                    text = scopeLabel,
                                                    color = colors.accentIndigo,
                                                    fontSize = 10.5.sp,
                                                    fontWeight = FontWeight.Medium
                                                )
                                            }
                                        }
                                        val resLabel = item.resourceName?.takeIf { it.isNotBlank() }
                                            ?: if (item.anyResource) "全部资源".loc else ""
                                        if (resLabel.isNotBlank()) {
                                            Text(
                                                text = resLabel,
                                                color = colors.textSecondary,
                                                fontSize = 11.sp,
                                                fontFamily = FontFamily.Monospace,
                                                maxLines = 1,
                                                overflow = TextOverflow.Ellipsis
                                            )
                                        }
                                    }
                                    TextButton(
                                        enabled = !isRevoking,
                                        onClick = {
                                            scope.launch {
                                                revokingGrantKeys.add(item.uniqueKey)
                                                apiClient.revokePluginPermissionGrant(item).onSuccess {
                                                    pluginPermissionGrants = pluginPermissionGrants.filterNot { it.uniqueKey == item.uniqueKey }
                                                }
                                                revokingGrantKeys.remove(item.uniqueKey)
                                            }
                                        },
                                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                                    ) {
                                        if (isRevoking) {
                                            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 1.8.dp)
                                        } else {
                                            Text("撤销".loc, color = colors.accentRed, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
                                        }
                                    }
                                }
                            }
                        }

                        if (filteredPlugins.isNotEmpty()) {
                            item(key = "plugins_header") {
                                Text(
                                    text = "已安装 / 可用插件 (${filteredPlugins.size})".loc,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(top = 6.dp, bottom = 2.dp)
                                )
                            }
                            itemsIndexed(filteredPlugins, key = { index, it -> "plugin_${index}_${it.effectiveId}" }) { _, plugin ->
                                val pid = plugin.effectiveId
                                val isSwarmPlugin = pid.equals("swarm", ignoreCase = true) || plugin.name.equals("swarm", ignoreCase = true)
                                PluginCard(
                                    plugin = plugin,
                                    isInstalling = installingPluginIds.contains(pid),
                                    onInstall = {
                                        scope.launch {
                                            installingPluginIds.add(pid)
                                            apiClient.installPlugin(pid, currentConversationId, autoResume = true)
                                            apiClient.fetchPluginsCatalog().onSuccess {
                                                plugins = it.plugins
                                                pluginProviders = it.providers
                                                pluginPermissionGrants = it.permissionGrants
                                            }
                                            installingPluginIds.remove(pid)
                                        }
                                    },
                                    onOpenSwarm = if (isSwarmPlugin && hasActiveSwarm && onOpenSwarm != null) {
                                        {
                                            onDismiss()
                                            onOpenSwarm()
                                        }
                                    } else null
                                )
                            }
                        }
                    }
                }
            } else if (selectedTab == McpSkillsTab.AUTOMATIONS) {
                // Automations Tab
                val query = searchText.trim().lowercase()
                val filteredAutomations = remember(automations, query) {
                    if (query.isEmpty()) automations
                    else automations.filter { a ->
                        a.name.lowercase().contains(query) ||
                        (a.description?.lowercase()?.contains(query) == true) ||
                        (a.cronExpression?.lowercase()?.contains(query) == true) ||
                        (a.prompt?.lowercase()?.contains(query) == true)
                    }
                }
                val filteredEvents = remember(automationEvents, query) {
                    if (query.isEmpty()) automationEvents
                    else automationEvents.filter { ev ->
                        ev.scheduleId.lowercase().contains(query) ||
                        (ev.message?.lowercase()?.contains(query) == true)
                    }
                }

                if (filteredAutomations.isEmpty() && filteredEvents.isEmpty()) {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center
                    ) {
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(10.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Schedule,
                                contentDescription = null,
                                tint = colors.textSecondary,
                                modifier = Modifier.size(32.dp)
                            )
                            Text(
                                text = "暂无定时自动化任务或未找到匹配项".loc,
                                fontSize = 14.sp,
                                color = colors.textSecondary
                            )
                            FilledTonalButton(onClick = { showNewAutomationDialog = true }) {
                                Text("+ 新建定时任务".loc, fontSize = 13.sp)
                            }
                        }
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                        contentPadding = PaddingValues(top = 4.dp, bottom = 84.dp)
                    ) {
                        item(key = "automations_top_bar") {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = "定时计划 (${filteredAutomations.size})".loc,
                                    fontSize = 13.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(start = 2.dp)
                                )
                                FilledTonalButton(
                                    onClick = { showNewAutomationDialog = true },
                                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                                    modifier = Modifier.height(30.dp)
                                ) {
                                    Text("+ 新建定时任务".loc, fontSize = 12.sp)
                                }
                            }
                        }
                        if (filteredAutomations.isNotEmpty()) {
                            items(filteredAutomations, key = { it.targetSidecarId }) { sidecar ->
                                val sid = sidecar.targetSidecarId
                                AutomationCard(
                                    sidecar = sidecar,
                                    isManaging = managingAutomationNames.contains(sid),
                                    onOpenDetail = { selectedAutomationId = sid },
                                    onToggle = {
                                        scope.launch {
                                            val action = if (sidecar.isRunning) "stop" else "start"
                                            managingAutomationNames.add(sid)
                                            apiClient.manageAutomation(action, sid)
                                                .onFailure { automationActionError = it.message }
                                            apiClient.fetchAutomationsData().onSuccess {
                                                automations = it.automations
                                                automationEvents = it.events
                                            }
                                            managingAutomationNames.remove(sid)
                                        }
                                    },
                                    onRunNow = {
                                        if (!sidecar.isRunning) return@AutomationCard
                                        scope.launch {
                                            managingAutomationNames.add(sid)
                                            apiClient.manageAutomation("run_now", sid)
                                                .onFailure { automationActionError = it.message }
                                            apiClient.fetchAutomationsData().onSuccess {
                                                automations = it.automations
                                                automationEvents = it.events
                                            }
                                            managingAutomationNames.remove(sid)
                                        }
                                    }
                                )
                            }
                        }

                        if (filteredEvents.isNotEmpty()) {
                            item(key = "automation_events_header") {
                                Text(
                                    text = "最近触发记录".loc,
                                    fontSize = 13.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(top = 14.dp, bottom = 2.dp, start = 2.dp)
                                )
                            }
                            item(key = "automation_events_group") {
                                Column(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(14.dp))
                                        .background(colors.surface)
                                        .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                                ) {
                                    filteredEvents.forEachIndexed { index, ev ->
                                        if (index > 0) {
                                            HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.6f))
                                        }
                                        Column(
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .clickable(enabled = !ev.conversationId.isNullOrBlank()) {
                                                    val cid = ev.conversationId
                                                    if (!cid.isNullOrBlank()) {
                                                        onDismiss()
                                                        onSelectConversation?.invoke(cid, ev.scheduleId.ifBlank { "自动化任务记录".loc })
                                                    }
                                                }
                                                .padding(horizontal = 14.dp, vertical = 11.dp),
                                            verticalArrangement = Arrangement.spacedBy(4.dp)
                                        ) {
                                            Row(
                                                verticalAlignment = Alignment.CenterVertically,
                                                horizontalArrangement = Arrangement.SpaceBetween,
                                                modifier = Modifier.fillMaxWidth()
                                            ) {
                                                Text(
                                                    text = ev.scheduleId.ifBlank { "自动化执行".loc },
                                                    color = colors.textPrimary,
                                                    fontSize = 13.5.sp,
                                                    fontWeight = FontWeight.SemiBold,
                                                    modifier = Modifier.weight(1f, fill = false),
                                                    maxLines = 1,
                                                    overflow = TextOverflow.Ellipsis
                                                )
                                                if (ev.timestamp.isNotBlank()) {
                                                    Spacer(modifier = Modifier.width(8.dp))
                                                    Text(
                                                        text = ev.timestamp,
                                                        color = colors.textSecondary,
                                                        fontSize = 11.sp
                                                    )
                                                }
                                            }
                                            val msg = ev.message.takeIf { !it.isNullOrBlank() }
                                            if (msg != null) {
                                                Text(
                                                    text = msg,
                                                    color = colors.textSecondary,
                                                    fontSize = 12.sp,
                                                    lineHeight = 16.sp,
                                                    maxLines = 2,
                                                    overflow = TextOverflow.Ellipsis
                                                )
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                    } else {
                // Background Tasks Tab (McpSkillsTab.TASKS)
                val query = searchText.trim().lowercase()
                val filteredTasks = remember(backgroundTasks, query) {
                    if (query.isEmpty()) backgroundTasks
                    else backgroundTasks.filter { t ->
                        t.displayTitle.lowercase().contains(query) ||
                        t.commandLine.lowercase().contains(query) ||
                        t.conversationTitle.lowercase().contains(query) ||
                        (t.workspaceName?.lowercase()?.contains(query) == true)
                    }
                }

                if (isLoading && backgroundTasks.isEmpty()) {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center
                    ) {
                        CircularProgressIndicator(
                            color = colors.accentIndigo,
                            modifier = Modifier.size(32.dp)
                        )
                    }
                } else if (filteredTasks.isEmpty()) {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(horizontal = 24.dp),
                        verticalArrangement = Arrangement.Center,
                        horizontalAlignment = Alignment.CenterHorizontally
                    ) {
                        Text(
                            text = ">_",
                            fontSize = 26.sp,
                            fontFamily = FontFamily.Monospace,
                            fontWeight = FontWeight.Bold,
                            color = colors.textMuted
                        )
                        Spacer(modifier = Modifier.height(10.dp))
                        Text(
                            text = if (query.isEmpty()) "当前没有正在运行的后台任务".loc else "未找到匹配的后台任务".loc,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.Medium,
                            color = colors.textSecondary
                        )
                        if (query.isEmpty()) {
                            Spacer(modifier = Modifier.height(4.dp))
                            Text(
                                text = "各会话中执行的后台终端命令、常驻开发服务器与定时任务会实时汇总在此处".loc,
                                fontSize = 12.sp,
                                color = colors.textMuted,
                                textAlign = TextAlign.Center
                            )
                        }
                    }
                } else {
                    val currentTasks = remember(filteredTasks, currentConversationId) {
                        filteredTasks.filter { it.conversationId == currentConversationId }
                    }
                    val otherTasks = remember(filteredTasks, currentConversationId) {
                        filteredTasks.filter { it.conversationId != currentConversationId }
                    }

                    LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                        contentPadding = PaddingValues(top = 4.dp, bottom = 84.dp)
                    ) {
                        if (filteredTasks.size > 1) {
                            item(key = "stop_all_tasks") {
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(10.dp))
                                        .background(colors.accentRed.copy(alpha = 0.12f))
                                        .border(0.8.dp, colors.accentRed.copy(alpha = 0.35f), RoundedCornerShape(10.dp))
                                        .clickable {
                                            haptic.medium()
                                            stopAllTasks(filteredTasks)
                                        }
                                        .padding(horizontal = 12.dp, vertical = 10.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.StopCircle,
                                        contentDescription = null,
                                        tint = colors.accentRed,
                                        modifier = Modifier.size(18.dp)
                                    )
                                    Text(
                                        text = "一键停止全部后台任务 (${filteredTasks.size})".loc,
                                        fontSize = 13.5.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentRed
                                    )
                                }
                            }
                        }

                        if (currentTasks.isNotEmpty()) {
                            item(key = "header_current_tasks") {
                                Text(
                                    text = "当前会话任务 (${currentTasks.size})".loc,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(top = 4.dp, bottom = 2.dp)
                                )
                            }
                            items(currentTasks, key = { it.uniqueId }) { task ->
                                GlobalBackgroundTaskCard(
                                    task = task,
                                    isCurrentSession = true,
                                    isStopping = stoppingTaskIds.contains(task.uniqueId),
                                    isLogExpanded = expandedLogTaskIds.contains(task.uniqueId),
                                    isLoadingLog = loadingLogTaskIds.contains(task.uniqueId),
                                    logContent = taskLogs[task.uniqueId],
                                    isCopied = copiedTaskId == task.uniqueId,
                                    onCopyCommand = {
                                        val cm = context.getSystemService(android.content.Context.CLIPBOARD_SERVICE) as? android.content.ClipboardManager
                                        cm?.setPrimaryClip(android.content.ClipData.newPlainText("Command", task.commandLine))
                                        haptic.light()
                                        copiedTaskId = task.uniqueId
                                    },
                                    onToggleLog = {
                                        haptic.light()
                                        if (expandedLogTaskIds.contains(task.uniqueId)) {
                                            expandedLogTaskIds = expandedLogTaskIds - task.uniqueId
                                        } else {
                                            expandedLogTaskIds = expandedLogTaskIds + task.uniqueId
                                            fetchTaskLog(task)
                                        }
                                    },
                                    onRefreshLog = {
                                        haptic.light()
                                        fetchTaskLog(task)
                                    },
                                    onStop = {
                                        haptic.medium()
                                        stopSingleTask(task)
                                    },
                                    onSelectSession = null
                                )
                            }
                        }

                        if (otherTasks.isNotEmpty()) {
                            item(key = "header_other_tasks") {
                                Text(
                                    text = if (currentConversationId != null) "其他会话任务 (${otherTasks.size})".loc else "正在运行的后台任务 (${otherTasks.size})".loc,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(top = 4.dp, bottom = 2.dp)
                                )
                            }
                            items(otherTasks, key = { it.uniqueId }) { task ->
                                GlobalBackgroundTaskCard(
                                    task = task,
                                    isCurrentSession = false,
                                    isStopping = stoppingTaskIds.contains(task.uniqueId),
                                    isLogExpanded = expandedLogTaskIds.contains(task.uniqueId),
                                    isLoadingLog = loadingLogTaskIds.contains(task.uniqueId),
                                    logContent = taskLogs[task.uniqueId],
                                    isCopied = copiedTaskId == task.uniqueId,
                                    onCopyCommand = {
                                        val cm = context.getSystemService(android.content.Context.CLIPBOARD_SERVICE) as? android.content.ClipboardManager
                                        cm?.setPrimaryClip(android.content.ClipData.newPlainText("Command", task.commandLine))
                                        haptic.light()
                                        copiedTaskId = task.uniqueId
                                    },
                                    onToggleLog = {
                                        haptic.light()
                                        if (expandedLogTaskIds.contains(task.uniqueId)) {
                                            expandedLogTaskIds = expandedLogTaskIds - task.uniqueId
                                        } else {
                                            expandedLogTaskIds = expandedLogTaskIds + task.uniqueId
                                            fetchTaskLog(task)
                                        }
                                    },
                                    onRefreshLog = {
                                        haptic.light()
                                        fetchTaskLog(task)
                                    },
                                    onStop = {
                                        haptic.medium()
                                        stopSingleTask(task)
                                    },
                                    onSelectSession = onSelectConversation?.let { cb ->
                                        {
                                            haptic.light()
                                            onDismiss()
                                            cb(task.conversationId, task.conversationTitle)
                                        }
                                    }
                                )
                            }
                        }
                    }
                }
            }

            // iOS-Style Floating Bottom Search Bar
            Box(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .navigationBarsPadding()
                    .padding(bottom = 12.dp)
                    .shadow(
                        elevation = 6.dp,
                        shape = RoundedCornerShape(26.dp),
                        spotColor = Color.Black.copy(alpha = 0.2f)
                    )
                    .clip(RoundedCornerShape(26.dp))
                    .background(colors.surface)
                    .border(0.8.dp, colors.border, RoundedCornerShape(26.dp))
                    .padding(horizontal = 16.dp, vertical = 11.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.Search,
                        contentDescription = "Search",
                        tint = colors.textSecondary,
                        modifier = Modifier.size(18.dp)
                    )
                    BasicTextField(
                        value = searchText,
                        onValueChange = { searchText = it },
                        singleLine = true,
                        textStyle = TextStyle(
                            color = colors.textPrimary,
                            fontSize = 14.5.sp
                        ),
                        cursorBrush = SolidColor(colors.accentIndigo),
                        decorationBox = { innerTextField ->
                            if (searchText.isEmpty()) {
                                Text(
                                    text = when (selectedTab) {
                                        McpSkillsTab.MCP -> "搜索 MCP 服务或工具名称".loc
                                        McpSkillsTab.SKILLS -> "搜索技能名称或描述".loc
                                        McpSkillsTab.PLUGINS -> "搜索插件名称或描述".loc
                                        McpSkillsTab.AUTOMATIONS -> "搜索定时任务或表达式".loc
                                        McpSkillsTab.TASKS -> "搜索命令、会话标题或工作区".loc
                                    },
                                    color = colors.textMuted,
                                    fontSize = 14.5.sp
                                )
                            }
                            innerTextField()
                        },
                        modifier = Modifier.weight(1f)
                    )
                    if (searchText.isNotEmpty()) {
                        Icon(
                            imageVector = Icons.Default.Close,
                            contentDescription = "Clear",
                            tint = colors.textMuted,
                            modifier = Modifier
                                .size(18.dp)
                                .clip(CircleShape)
                                .clickable { searchText = "" }
                        )
                    }
                }
            }
        } // Close Content Area Box
        }
    }
}
}

    // Skill detail dialog
    viewingSkill?.let { skill ->
        AlertDialog(
            onDismissRequest = { viewingSkill = null },
            confirmButton = {
                TextButton(onClick = { viewingSkill = null }) {
                    Text("关闭".loc, color = colors.accentIndigo)
                }
            },
            title = {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        text = skill.name,
                        fontSize = 17.sp,
                        fontWeight = FontWeight.Bold,
                        color = colors.textPrimary
                    )
                    Text(
                        text = skill.locationCategory,
                        fontSize = 10.5.sp,
                        color = if (skill.isBuiltin) colors.textSecondary else colors.accentIndigo,
                        modifier = Modifier
                            .clip(RoundedCornerShape(4.dp))
                            .background(if (skill.isBuiltin) colors.surfaceVariant else colors.accentIndigo.copy(alpha = 0.12f))
                            .padding(horizontal = 5.dp, vertical = 2.dp)
                    )
                }
            },
            text = {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    if (!skill.description.isNullOrEmpty()) {
                        Text(
                            text = skill.description,
                            fontSize = 13.5.sp,
                            color = colors.textPrimary
                        )
                    }
                    if (!skill.path.isNullOrEmpty()) {
                        Text(
                            text = skill.path,
                            fontSize = 11.sp,
                            fontFamily = FontFamily.Monospace,
                            color = colors.textMuted
                        )
                    }
                    if (!skill.content.isNullOrEmpty()) {
                        HorizontalDivider(color = colors.border)
                        Text(
                            text = "规则定义 (SKILL.md)：".loc,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.textSecondary
                        )
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.codeBlockBg)
                                .padding(10.dp)
                        ) {
                            Text(
                                text = skill.content,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textPrimary
                            )
                        }
                    }
                }
            },
            containerColor = colors.surface
        )
    }

    editingMemory?.let { initialMem ->
        var memTitle by remember(initialMem) { mutableStateOf(initialMem.title) }
        var memContent by remember(initialMem) { mutableStateOf(initialMem.content) }
        AlertDialog(
            onDismissRequest = { editingMemory = null },
            title = { Text(if (initialMem.id.isBlank()) "新增 Agent 记忆".loc else "编辑记忆 / 规则".loc, fontSize = 16.sp, fontWeight = FontWeight.SemiBold) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = memTitle,
                        onValueChange = { memTitle = it },
                        label = { Text("标题".loc) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                    OutlinedTextField(
                        value = memContent,
                        onValueChange = { memContent = it },
                        label = { Text("内容".loc) },
                        minLines = 3,
                        maxLines = 8,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                TextButton(
                    enabled = memContent.isNotBlank() && !initialMem.readOnly,
                    onClick = {
                        val t = memTitle.trim().ifBlank { "未命名记忆".loc }
                        val c = memContent.trim()
                        editingMemory = null
                        scope.launch {
                            apiClient.upsertMemory(
                                scope = initialMem.scope.ifBlank { "memory" },
                                title = t,
                                content = c,
                                path = initialMem.path.ifBlank { null }
                            )
                            apiClient.fetchMemories().onSuccess { memories = it }
                        }
                    }
                ) { Text("保存".loc) }
            },
            dismissButton = {
                TextButton(onClick = { editingMemory = null }) { Text("关闭".loc) }
            },
            containerColor = colors.surface
        )
    }

    pendingDeleteMemory?.let { targetMem ->
        AlertDialog(
            onDismissRequest = { pendingDeleteMemory = null },
            title = { Text("确认删除记忆条目？".loc, fontSize = 16.sp, fontWeight = FontWeight.SemiBold) },
            text = {
                Text(
                    text = "确定要删除「${targetMem.title.ifBlank { targetMem.id }}」吗？这会移除对应的本地规则/记忆文件（并在 ~/.gemini/backups/deleted_memories 保留备份）。".loc,
                    fontSize = 13.5.sp,
                    color = colors.textSecondary
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        pendingDeleteMemory = null
                        scope.launch {
                            apiClient.deleteMemory(targetMem.path)
                            apiClient.fetchMemories().onSuccess { memories = it }
                        }
                    }
                ) { Text("删除".loc, color = Color(0xFFF44336)) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteMemory = null }) { Text("取消".loc) }
            },
            containerColor = colors.surface
        )
    }

    if (showNewAutomationDialog) {
        var autoName by remember { mutableStateOf("") }
        var autoCron by remember { mutableStateOf("0 9 * * *") }
        var autoPrompt by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { showNewAutomationDialog = false },
            title = { Text("新建定时自动化任务".loc, fontSize = 16.sp, fontWeight = FontWeight.SemiBold) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = autoName,
                        onValueChange = { autoName = it },
                        label = { Text("任务名称".loc) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                    OutlinedTextField(
                        value = autoCron,
                        onValueChange = { autoCron = it },
                        label = { Text("Cron 表达式（如 0 9 * * *）".loc) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                    OutlinedTextField(
                        value = autoPrompt,
                        onValueChange = { autoPrompt = it },
                        label = { Text("执行 Prompt".loc) },
                        minLines = 2,
                        maxLines = 5,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            },
            confirmButton = {
                TextButton(
                    enabled = autoName.isNotBlank() && autoCron.isNotBlank() && autoPrompt.isNotBlank(),
                    onClick = {
                        val n = autoName.trim(); val s = autoCron.trim(); val p = autoPrompt.trim()
                        showNewAutomationDialog = false
                        scope.launch {
                            apiClient.createAutomation(n, s, p)
                            apiClient.fetchAutomationsData().onSuccess {
                                automations = it.automations
                                automationEvents = it.events
                            }
                        }
                    }
                ) { Text("创建".loc) }
            },
            dismissButton = {
                TextButton(onClick = { showNewAutomationDialog = false }) { Text("取消".loc) }
            },
            containerColor = colors.surface
        )
    }

    val selectedAuto = selectedAutomationId?.let { id ->
        automations.firstOrNull { it.targetSidecarId == id }
    }
    if (selectedAuto != null) {
        AutomationDetailDialog(
            sidecar = selectedAuto,
            allEvents = automationEvents,
            apiClient = apiClient,
            prefs = prefs,
            onRefresh = {
                apiClient.fetchAutomationsData().onSuccess {
                    automations = it.automations
                    automationEvents = it.events
                }
            },
            onDismiss = { selectedAutomationId = null },
            onSelectConversation = { cid, title ->
                selectedAutomationId = null
                onDismiss()
                onSelectConversation?.invoke(cid, title)
            }
        )
    }

    if (automationActionError != null) {
        AlertDialog(
            onDismissRequest = { automationActionError = null },
            title = { Text("操作失败".loc) },
            text = { Text(automationActionError.orEmpty()) },
            confirmButton = {
                TextButton(onClick = { automationActionError = null }) { Text("确定".loc) }
            },
            containerColor = colors.surface
        )
    }
}

@Composable
private fun McpServerCard(
    server: McpServerState,
    isExpanded: Boolean,
    onToggleExpand: () -> Unit,
    expandedTools: Set<String>,
    onToggleTool: (String) -> Unit,
    onToggleEnabled: ((Boolean) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(1.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(12.dp)
    ) {
        // Server header
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable { onToggleExpand() },
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = server.spec.serverName,
                fontSize = 15.5.sp,
                fontWeight = FontWeight.Bold,
                color = colors.textPrimary
            )
            server.serverInfo?.version?.let { ver ->
                Spacer(modifier = Modifier.width(6.dp))
                Text(
                    text = "v$ver",
                    fontSize = 10.5.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textSecondary,
                    modifier = Modifier
                        .clip(RoundedCornerShape(4.dp))
                        .background(colors.surfaceVariant)
                        .padding(horizontal = 4.dp, vertical = 1.dp)
                )
            }
            Spacer(modifier = Modifier.weight(1f))
            // Status badge
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .clip(RoundedCornerShape(10.dp))
                    .background(if (server.isReady) colors.accentGreen.copy(alpha = 0.12f) else colors.accentOrange.copy(alpha = 0.12f))
                    .padding(horizontal = 6.dp, vertical = 2.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(6.dp)
                        .clip(CircleShape)
                        .background(if (server.isReady) colors.accentGreen else colors.accentOrange)
                )
                Spacer(modifier = Modifier.width(4.dp))
                Text(
                    text = server.statusTitle,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium,
                    color = if (server.isReady) colors.accentGreen else colors.accentOrange
                )
            }
            Spacer(modifier = Modifier.width(6.dp))
            if (onToggleEnabled != null) {
                val isEnabled = !server.spec.disabled && !server.status.contains("STOPPED")
                Switch(
                    checked = isEnabled,
                    onCheckedChange = { onToggleEnabled(it) },
                    modifier = Modifier.padding(end = 4.dp)
                )
            }
            Icon(
                imageVector = if (isExpanded) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(18.dp)
            )
        }

        if (!server.instructions.isNullOrEmpty()) {
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = server.instructions,
                fontSize = 12.5.sp,
                color = colors.textSecondary,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )
        }

        // Tools list
        AnimatedVisibility(visible = isExpanded) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 10.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                HorizontalDivider(color = colors.border.copy(alpha = 0.5f))
                Text(
                    text = "可用工具清单 (${server.toolCount})".loc,
                    fontSize = 12.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.accentIndigo
                )

                server.tools?.forEach { tool ->
                    McpToolItem(
                        tool = tool,
                        isExpanded = expandedTools.contains(tool.id),
                        onToggle = { onToggleTool(tool.id) }
                    )
                }
            }
        }
    }
}

@Composable
private fun McpToolItem(
    tool: McpToolDefinition,
    isExpanded: Boolean,
    onToggle: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val params = remember(tool) { tool.parseParameters() }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(colors.surfaceVariant.copy(alpha = 0.5f))
            .clickable { onToggle() }
            .padding(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = tool.name,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
                fontFamily = FontFamily.Monospace,
                color = colors.textPrimary
            )
            if (params.isNotEmpty()) {
                Spacer(modifier = Modifier.width(6.dp))
                Text(
                    text = "${params.size} 参数".loc,
                    fontSize = 10.sp,
                    color = colors.accentIndigo,
                    modifier = Modifier
                        .clip(RoundedCornerShape(4.dp))
                        .background(colors.accentIndigo.copy(alpha = 0.12f))
                        .padding(horizontal = 4.dp, vertical = 1.dp)
                )
            }
            Spacer(modifier = Modifier.weight(1f))
            Icon(
                imageVector = if (isExpanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(16.dp)
            )
        }

        if (!tool.description.isNullOrEmpty()) {
            Spacer(modifier = Modifier.height(2.dp))
            Text(
                text = tool.description,
                fontSize = 11.5.sp,
                color = colors.textSecondary,
                maxLines = if (isExpanded) Int.MAX_VALUE else 2,
                overflow = TextOverflow.Ellipsis
            )
        }

        // Expanded parameters
        if (isExpanded) {
            Spacer(modifier = Modifier.height(6.dp))
            if (params.isEmpty()) {
                Text(
                    text = "无输入参数（直接调用）".loc,
                    fontSize = 11.sp,
                    color = colors.textMuted
                )
            } else {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(6.dp))
                        .background(colors.surface)
                        .padding(8.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Text(
                        text = "参数定义：".loc,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.textSecondary
                    )
                    params.forEach { param ->
                        Column(modifier = Modifier.fillMaxWidth()) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    text = param.name,
                                    fontSize = 11.5.sp,
                                    fontFamily = FontFamily.Monospace,
                                    fontWeight = FontWeight.Medium,
                                    color = colors.textPrimary
                                )
                                if (param.isRequired) {
                                    Spacer(modifier = Modifier.width(4.dp))
                                    Text(
                                        text = "必填".loc,
                                        fontSize = 9.sp,
                                        fontWeight = FontWeight.Bold,
                                        color = colors.accentRed,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(3.dp))
                                            .background(colors.accentRed.copy(alpha = 0.12f))
                                            .padding(horizontal = 3.dp, vertical = 0.5.dp)
                                    )
                                }
                                Spacer(modifier = Modifier.width(4.dp))
                                Text(
                                    text = "(${param.type})",
                                    fontSize = 10.5.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textSecondary
                                )
                                param.defaultValue?.let { def ->
                                    Spacer(modifier = Modifier.weight(1f))
                                    Text(
                                        text = "默认: $def".loc,
                                        fontSize = 10.sp,
                                        color = colors.textMuted
                                    )
                                }
                            }
                            if (param.description.isNotEmpty()) {
                                Text(
                                    text = param.description,
                                    fontSize = 11.sp,
                                    color = colors.textSecondary,
                                    modifier = Modifier.padding(start = 6.dp, top = 1.dp)
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SkillCard(
    skill: SkillInfo,
    onClick: () -> Unit
) {
    val colors = AntigravityTheme.colors
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surface)
            .border(1.dp, colors.border, RoundedCornerShape(10.dp))
            .clickable { onClick() }
            .padding(12.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = skill.name,
                fontSize = 14.5.sp,
                fontWeight = FontWeight.Bold,
                color = colors.textPrimary
            )
            Spacer(modifier = Modifier.width(6.dp))
            Text(
                text = skill.locationCategory,
                fontSize = 10.sp,
                color = if (skill.isBuiltin) colors.textSecondary else colors.accentIndigo,
                modifier = Modifier
                    .clip(RoundedCornerShape(4.dp))
                    .background(if (skill.isBuiltin) colors.surfaceVariant else colors.accentIndigo.copy(alpha = 0.12f))
                    .padding(horizontal = 4.dp, vertical = 1.5.dp)
            )
            Spacer(modifier = Modifier.weight(1f))
            Icon(
                imageVector = Icons.Default.ChevronRight,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(16.dp)
            )
        }

        if (!skill.description.isNullOrEmpty()) {
            Spacer(modifier = Modifier.height(3.dp))
            Text(
                text = skill.description,
                fontSize = 12.sp,
                color = colors.textSecondary,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )
        }

        if (!skill.path.isNullOrEmpty()) {
            Spacer(modifier = Modifier.height(2.dp))
            Text(
                text = skill.path,
                fontSize = 10.sp,
                fontFamily = FontFamily.Monospace,
                color = colors.textMuted,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@Composable
private fun GlobalBackgroundTaskCard(
    task: GlobalBackgroundTaskItem,
    isCurrentSession: Boolean,
    isStopping: Boolean,
    isLogExpanded: Boolean,
    isLoadingLog: Boolean,
    logContent: String?,
    isCopied: Boolean,
    onCopyCommand: () -> Unit,
    onToggleLog: () -> Unit,
    onRefreshLog: () -> Unit,
    onStop: () -> Unit,
    onSelectSession: (() -> Unit)?
) {
    val colors = AntigravityTheme.colors
    val badgeTitle = when {
        task.isDaemon -> "常驻守护".loc
        task.toolName == "schedule" -> "定时任务".loc
        else -> "后台命令".loc
    }
    val badgeColor = when {
        task.isDaemon -> Color(0xFFAF52DE)
        task.toolName == "schedule" -> colors.accentOrange
        else -> colors.accentIndigo
    }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(1.dp, colors.border, RoundedCornerShape(12.dp))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        // 1. Header Row: Badge + Title + Elapsed + Stop Button
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(
                text = badgeTitle,
                fontSize = 10.5.sp,
                fontWeight = FontWeight.SemiBold,
                color = badgeColor,
                modifier = Modifier
                    .clip(CircleShape)
                    .background(badgeColor.copy(alpha = 0.14f))
                    .padding(horizontal = 7.dp, vertical = 2.dp)
            )
            Text(
                text = task.displayTitle,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textPrimary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
            task.elapsedText?.let { elapsed ->
                Text(
                    text = elapsed,
                    fontSize = 11.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textSecondary,
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(colors.surfaceVariant)
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                )
            }
            if (isStopping) {
                CircularProgressIndicator(
                    color = colors.accentRed,
                    strokeWidth = 2.dp,
                    modifier = Modifier.size(18.dp)
                )
            } else {
                Icon(
                    imageVector = Icons.Default.StopCircle,
                    contentDescription = "停止任务".loc,
                    tint = colors.accentRed,
                    modifier = Modifier
                        .size(22.dp)
                        .clip(CircleShape)
                        .clickable { onStop() }
                )
            }
        }

        // 2. Session & Workspace Row
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(8.dp))
                .background(colors.surfaceVariant.copy(alpha = 0.7f))
                .let { mod ->
                    if (!isCurrentSession && onSelectSession != null) {
                        mod.clickable { onSelectSession() }
                    } else mod
                }
                .padding(horizontal = 8.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(5.dp)
        ) {
            Icon(
                imageVector = if (isCurrentSession) Icons.Default.ChatBubble else Icons.Default.Folder,
                contentDescription = null,
                tint = colors.accentIndigo,
                modifier = Modifier.size(13.dp)
            )
            if (!task.workspaceName.isNullOrBlank()) {
                Text(
                    text = task.workspaceName,
                    fontSize = 11.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    color = colors.accentIndigo,
                    maxLines = 1
                )
                Text(
                    text = "·",
                    fontSize = 11.5.sp,
                    color = colors.textMuted
                )
            }
            Text(
                text = task.conversationTitle,
                fontSize = 12.sp,
                color = colors.textSecondary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
            if (!isCurrentSession && onSelectSession != null) {
                Text(
                    text = "跳转会话 ›".loc,
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium,
                    color = colors.accentIndigo
                )
            }
        }

        // 3. Command Line + Action Buttons
        if (task.commandLine.isNotBlank()) {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(8.dp))
                        .background(colors.codeBlockBg)
                        .padding(8.dp)
                ) {
                    Text(
                        text = task.commandLine,
                        fontSize = 11.5.sp,
                        fontFamily = FontFamily.Monospace,
                        color = colors.textPrimary,
                        maxLines = 3,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    Row(
                        modifier = Modifier
                            .clip(RoundedCornerShape(4.dp))
                            .clickable { onCopyCommand() }
                            .padding(vertical = 2.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Icon(
                            imageVector = if (isCopied) Icons.Default.Check else Icons.Default.ContentCopy,
                            contentDescription = null,
                            tint = if (isCopied) colors.accentGreen else colors.textSecondary,
                            modifier = Modifier.size(13.dp)
                        )
                        Text(
                            text = if (isCopied) "已复制命令".loc else "复制命令".loc,
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.Medium,
                            color = if (isCopied) colors.accentGreen else colors.textSecondary
                        )
                    }

                    if (!task.logUri.isNullOrBlank()) {
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .clickable { onToggleLog() }
                                .padding(vertical = 2.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Subject,
                                contentDescription = null,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(13.dp)
                            )
                            Text(
                                text = if (isLogExpanded) "收起终端日志".loc else "查看最新终端日志".loc,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Medium,
                                color = colors.accentIndigo
                            )
                        }
                    }

                    if (isLogExpanded) {
                        Spacer(modifier = Modifier.weight(1f))
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .clickable { onRefreshLog() }
                                .padding(vertical = 2.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(3.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Refresh,
                                contentDescription = null,
                                tint = colors.textSecondary,
                                modifier = Modifier.size(12.dp)
                            )
                            Text(
                                text = "刷新".loc,
                                fontSize = 11.sp,
                                color = colors.textSecondary
                            )
                        }
                    }
                }
            }
        }

        // 4. Expanded Log Output Tail
        if (isLogExpanded) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 160.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .background(colors.codeBlockBg)
                    .padding(8.dp)
            ) {
                if (isLoadingLog && logContent == null) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        CircularProgressIndicator(
                            color = colors.accentIndigo,
                            strokeWidth = 1.8.dp,
                            modifier = Modifier.size(14.dp)
                        )
                        Text(
                            text = "正在读取日志尾部...".loc,
                            fontSize = 11.5.sp,
                            color = colors.textSecondary
                        )
                    }
                } else {
                    val trimmed = logContent?.trim().orEmpty()
                    Text(
                        text = trimmed.ifEmpty { "（暂无终端日志输出）".loc },
                        fontSize = 11.sp,
                        fontFamily = FontFamily.Monospace,
                        color = colors.textPrimary,
                        modifier = Modifier.verticalScroll(rememberScrollState())
                    )
                }
            }
        }
    }
}

@Composable
private fun PluginCard(
    plugin: CascadePluginItem,
    isInstalling: Boolean,
    onInstall: () -> Unit,
    onOpenSwarm: (() -> Unit)? = null
) {
    val colors = AntigravityTheme.colors

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp)
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
            modifier = Modifier.fillMaxWidth()
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.weight(1f)
            ) {
                Text(
                    text = plugin.name,
                    color = colors.textPrimary,
                    fontSize = 14.5.sp,
                    fontWeight = FontWeight.Bold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                if (plugin.installed) {
                    Box(
                        modifier = Modifier
                            .clip(CircleShape)
                            .background(if (plugin.enabled) colors.accentGreen.copy(alpha = 0.12f) else colors.surfaceVariant)
                            .padding(horizontal = 6.dp, vertical = 2.dp)
                    ) {
                        Text(
                            text = if (plugin.enabled) "已启用".loc else "已停用".loc,
                            color = if (plugin.enabled) colors.accentGreen else colors.textSecondary,
                            fontSize = 10.5.sp,
                            fontWeight = FontWeight.Medium
                        )
                    }
                }
            }

            if (!plugin.installed) {
                Button(
                    onClick = onInstall,
                    enabled = !isInstalling,
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                    modifier = Modifier.height(28.dp)
                ) {
                    if (isInstalling) {
                        CircularProgressIndicator(modifier = Modifier.size(12.dp), strokeWidth = 1.5.dp)
                    } else {
                        Text("安装".loc, fontSize = 11.5.sp)
                    }
                }
            } else if (onOpenSwarm != null) {
                Button(
                    onClick = onOpenSwarm,
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                    modifier = Modifier.height(28.dp)
                ) {
                    Text("打开看板".loc, fontSize = 11.5.sp)
                }
            }
        }

        Text(
            text = plugin.effectiveId,
            color = colors.textSecondary,
            fontSize = 11.sp,
            fontFamily = FontFamily.Monospace
        )

        if (plugin.description.isNotBlank()) {
            Text(
                text = plugin.description,
                color = colors.textSecondary,
                fontSize = 12.5.sp,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}

@Composable
private fun AutomationCard(
    sidecar: AutomationSidecarItem,
    isManaging: Boolean,
    onOpenDetail: () -> Unit,
    onToggle: () -> Unit,
    onRunNow: (() -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    val isRunning = sidecar.isRunning

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp))
            .background(colors.surface)
            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
            .clickable { onOpenDetail() }
            .padding(14.dp),
        verticalArrangement = Arrangement.spacedBy(7.dp)
    ) {
        // Line 1: Title + Status Capsule + Action Buttons
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
            modifier = Modifier.fillMaxWidth()
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.weight(1f)
            ) {
                Text(
                    text = sidecar.name.ifBlank { sidecar.targetSidecarId },
                    color = colors.textPrimary,
                    fontSize = 15.sp,
                    fontWeight = FontWeight.Bold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
                Box(
                    modifier = Modifier
                        .clip(CircleShape)
                        .background(if (isRunning) colors.accentGreen.copy(alpha = 0.12f) else colors.surfaceVariant)
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                ) {
                    Text(
                        text = if (isRunning) "运行中".loc else "已暂停".loc,
                        color = if (isRunning) colors.accentGreen else colors.textSecondary,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium
                    )
                }
            }

            Spacer(modifier = Modifier.width(6.dp))

            // Action Buttons (Run Now only shown when enabled, matching desktop)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                if (isRunning && onRunNow != null) {
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(CircleShape)
                            .background(colors.accentIndigo.copy(alpha = 0.12f))
                            .clickable(enabled = !isManaging, onClick = onRunNow),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.PlayArrow,
                            contentDescription = "立即运行".loc,
                            tint = colors.accentIndigo,
                            modifier = Modifier.size(16.dp)
                        )
                    }
                }
                Box(
                    modifier = Modifier
                        .height(28.dp)
                        .clip(CircleShape)
                        .background(if (isRunning) colors.surfaceVariant else colors.accentIndigo.copy(alpha = 0.14f))
                        .clickable(enabled = !isManaging, onClick = onToggle)
                        .padding(horizontal = 12.dp),
                    contentAlignment = Alignment.Center
                ) {
                    if (isManaging) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(12.dp),
                            strokeWidth = 1.5.dp,
                            color = colors.accentIndigo
                        )
                    } else {
                        Text(
                            text = if (isRunning) "停止".loc else "启动".loc,
                            color = if (isRunning) colors.textPrimary else colors.accentIndigo,
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                    }
                }
                Icon(
                    imageVector = Icons.Default.ChevronRight,
                    contentDescription = "任务详情".loc,
                    tint = colors.textSecondary.copy(alpha = 0.7f),
                    modifier = Modifier.size(18.dp)
                )
            }
        }

        // Line 2: Human-readable schedule summary
        if (!sidecar.cronExpression.isNullOrBlank()) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.Schedule,
                    contentDescription = null,
                    tint = colors.textSecondary,
                    modifier = Modifier.size(11.dp)
                )
                Text(
                    text = AutomationCronHelper.humanSummary(sidecar.cronExpression),
                    color = colors.textSecondary,
                    fontSize = 11.5.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }

        // Line 3: Next & Last Run Times
        if ((isRunning && !sidecar.nextRunAt.isNullOrBlank()) || !sidecar.lastRunAt.isNullOrBlank()) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (isRunning && !sidecar.nextRunAt.isNullOrBlank()) {
                    Text(
                        text = "下次: ${sidecar.nextRunAt}".loc,
                        color = colors.textSecondary,
                        fontSize = 11.sp
                    )
                }
                if (!sidecar.lastRunAt.isNullOrBlank()) {
                    Text(
                        text = "上次: ${sidecar.lastRunAt}".loc,
                        color = colors.textMuted,
                        fontSize = 11.sp
                    )
                }
            }
        }

        // Line 4: Prompt / Description
        val desc = sidecar.description.takeIf { !it.isNullOrBlank() } ?: sidecar.prompt
        if (!desc.isNullOrBlank()) {
            Text(
                text = desc,
                color = colors.textSecondary,
                fontSize = 12.5.sp,
                lineHeight = 17.sp,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )
        }
    }
}
