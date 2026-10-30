package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun McpSkillsSheet(
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = false)
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val haptic = rememberHaptic()
    val scope = rememberCoroutineScope()
    val apiClient = remember {
        ApiClient.defaultInstance ?: ApiClient(
            context.applicationContext,
            PreferencesManager(context.applicationContext)
        )
    }

    var selectedTab by remember { mutableStateOf(0) } // 0: MCP, 1: Skills
    var searchText by remember { mutableStateOf("") }
    var isLoading by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    var mcpServers by remember { mutableStateOf<List<McpServerState>>(emptyList()) }
    var skills by remember { mutableStateOf<List<SkillInfo>>(emptyList()) }
    var expandedServers by remember { mutableStateOf<Set<String>>(emptySet()) }
    var expandedTools by remember { mutableStateOf<Set<String>>(emptySet()) }
    var viewingSkill by remember { mutableStateOf<SkillInfo?>(null) }

    fun applyDemoData() {
        mcpServers = listOf(
            McpServerState(
                spec = McpServerSpec(serverName = "ifind", command = "python3 -m ifind_mcp.server run"),
                status = "MCP_SERVER_STATUS_READY",
                tools = listOf(
                    McpToolDefinition(
                        name = "get_realtime_quotation",
                        description = "查询证券实时行情、最新价、涨跌幅、盘口及资金流",
                        serverName = "ifind",
                        jsonSchemaString = "{\"properties\":{\"codes\":{\"type\":\"string\",\"description\":\"证券代码，如 600519.SH\"}},\"required\":[\"codes\"]}"
                    ),
                    McpToolDefinition(
                        name = "get_history_quotation",
                        description = "查询历史K线行情与序列数据",
                        serverName = "ifind",
                        jsonSchemaString = "{\"properties\":{\"codes\":{\"type\":\"string\"},\"period\":{\"type\":\"string\",\"default\":\"D\"}},\"required\":[\"codes\"]}"
                    )
                ),
                serverInfo = McpServerInfoDetail(name = "ifind-mcp", version = "4.0.0"),
                instructions = "iFinD 金融数据终端服务接口"
            ),
            McpServerState(
                spec = McpServerSpec(serverName = "weather", command = "uv run weather-server"),
                status = "MCP_SERVER_STATUS_READY",
                tools = listOf(
                    McpToolDefinition(
                        name = "get_current_weather",
                        description = "获取指定城市的当前实时天气情况",
                        serverName = "weather",
                        jsonSchemaString = "{\"properties\":{\"city\":{\"type\":\"string\",\"description\":\"城市名称\"}},\"required\":[\"city\"]}"
                    )
                ),
                serverInfo = McpServerInfoDetail(name = "weather-service", version = "1.2.0"),
                instructions = "全球实时与未来天气查询"
            ),
            McpServerState(
                spec = McpServerSpec(serverName = "message_hub", command = "node message_hub.js"),
                status = "MCP_SERVER_STATUS_READY",
                tools = listOf(
                    McpToolDefinition(
                        name = "send_message",
                        description = "统一向微信或元宝渠道发送通知与长图",
                        serverName = "message_hub",
                        jsonSchemaString = "{\"properties\":{\"target\":{\"type\":\"string\"},\"text\":{\"type\":\"string\"}},\"required\":[\"target\",\"text\"]}"
                    )
                ),
                serverInfo = McpServerInfoDetail(name = "message-hub", version = "2.1.0"),
                instructions = "统一消息与多端通信路由"
            )
        )
        skills = listOf(
            SkillInfo(name = "chart-generator", description = "自动化图表生成与多维度可视化分析", path = "builtin/skills/chart-generator/SKILL.md", content = "# Chart Generator\n自动根据输入数据渲染交互图表"),
            SkillInfo(name = "sql-inspector", description = "SQL 查询调优与执行计划诊断", path = "builtin/skills/sql-inspector/SKILL.md", content = "# SQL Inspector\n分析 SQL 慢查询并提出索引优化方案"),
            SkillInfo(name = "jiuge-market-analysis", description = "首席宏观研判与市场逻辑观点体系", path = "config/skills/jiuge-market-analysis/SKILL.md", content = "# Jiuge Market Analysis\n宏观与大宗商品核心驱动逻辑")
        )
        expandedServers = mcpServers.map { it.spec.serverName }.toSet()
    }

    fun loadData() {
        scope.launch {
            isLoading = true
            errorMessage = null
            try {
                val mcpRes = apiClient.fetchMcpServerStates()
                val skillsRes = apiClient.fetchAllSkills()
                if (mcpRes.isSuccess && skillsRes.isSuccess) {
                    mcpServers = mcpRes.getOrNull() ?: emptyList()
                    skills = skillsRes.getOrNull() ?: emptyList()
                    expandedServers = mcpServers.map { it.spec.serverName }.toSet()
                } else {
                    applyDemoData()
                }
            } catch (_: Exception) {
                applyDemoData()
            } finally {
                isLoading = false
            }
        }
    }

    LaunchedEffect(Unit) {
        loadData()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = colors.background,
        dragHandle = { BottomSheetDefaults.DragHandle() },
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .fillMaxHeight(0.9f)
                .padding(horizontal = 16.dp)
        ) {
            // Header bar
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Icon(
                    imageVector = Icons.Default.Build,
                    contentDescription = null,
                    tint = colors.accentIndigo,
                    modifier = Modifier.size(20.dp)
                )
                Spacer(modifier = Modifier.width(8.dp))
                Text(
                    text = "工具与技能看板",
                    fontSize = 17.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary
                )
                Spacer(modifier = Modifier.weight(1f))
                IconButton(onClick = onDismiss) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = "Close",
                        tint = colors.textSecondary
                    )
                }
            }

            // Tab Row
            TabRow(
                selectedTabIndex = selectedTab,
                containerColor = colors.surface,
                contentColor = colors.accentIndigo,
                modifier = Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .padding(bottom = 8.dp)
            ) {
                Tab(
                    selected = selectedTab == 0,
                    onClick = {
                        haptic.light()
                        selectedTab = 0
                    },
                    text = {
                        Text(
                            text = "MCP 服务 (${mcpServers.size})",
                            fontWeight = if (selectedTab == 0) FontWeight.Bold else FontWeight.Normal,
                            fontSize = 13.5.sp
                        )
                    }
                )
                Tab(
                    selected = selectedTab == 1,
                    onClick = {
                        haptic.light()
                        selectedTab = 1
                    },
                    text = {
                        Text(
                            text = "技能清单 (${skills.size})",
                            fontWeight = if (selectedTab == 1) FontWeight.Bold else FontWeight.Normal,
                            fontSize = 13.5.sp
                        )
                    }
                )
            }

            // Summary row
            val readyCount = mcpServers.count { it.isReady }
            val totalTools = mcpServers.sumOf { it.toolCount }
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(7.dp)
                            .clip(CircleShape)
                            .background(if (readyCount > 0) colors.accentGreen else colors.accentOrange)
                    )
                    Spacer(modifier = Modifier.width(5.dp))
                    Text(
                        text = "$readyCount/${mcpServers.size} 服务就绪",
                        fontSize = 11.5.sp,
                        fontWeight = FontWeight.Medium,
                        color = colors.textPrimary
                    )
                }

                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.Handyman,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(12.dp)
                    )
                    Spacer(modifier = Modifier.width(4.dp))
                    Text(
                        text = "$totalTools 个工具",
                        fontSize = 11.5.sp,
                        color = colors.textSecondary
                    )
                }

                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surface)
                        .padding(horizontal = 8.dp, vertical = 4.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.AutoAwesome,
                        contentDescription = null,
                        tint = colors.accentPurple,
                        modifier = Modifier.size(12.dp)
                    )
                    Spacer(modifier = Modifier.width(4.dp))
                    Text(
                        text = "${skills.size} 个技能",
                        fontSize = 11.5.sp,
                        color = colors.textSecondary
                    )
                }
            }

            // Search Bar
            OutlinedTextField(
                value = searchText,
                onValueChange = { searchText = it },
                placeholder = {
                    Text(
                        if (selectedTab == 0) "搜索 MCP 服务或工具名称" else "搜索技能名称或描述",
                        fontSize = 13.sp,
                        color = colors.textMuted
                    )
                },
                leadingIcon = {
                    Icon(
                        imageVector = Icons.Default.Search,
                        contentDescription = null,
                        tint = colors.textSecondary,
                        modifier = Modifier.size(18.dp)
                    )
                },
                trailingIcon = {
                    if (searchText.isNotEmpty()) {
                        IconButton(onClick = { searchText = "" }) {
                            Icon(
                                imageVector = Icons.Default.Clear,
                                contentDescription = "Clear",
                                tint = colors.textSecondary,
                                modifier = Modifier.size(16.dp)
                            )
                        }
                    }
                },
                singleLine = true,
                shape = RoundedCornerShape(10.dp),
                colors = OutlinedTextFieldDefaults.colors(
                    focusedContainerColor = colors.surface,
                    unfocusedContainerColor = colors.surface,
                    focusedBorderColor = colors.accentIndigo,
                    unfocusedBorderColor = colors.border
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 8.dp)
            )

            // Content List
            if (isLoading && mcpServers.isEmpty() && skills.isEmpty()) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f),
                    contentAlignment = Alignment.Center
                ) {
                    CircularProgressIndicator(
                        color = colors.accentIndigo,
                        modifier = Modifier.size(32.dp)
                    )
                }
            } else if (selectedTab == 0) {
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
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "未找到匹配的 MCP 服务或工具",
                            fontSize = 14.sp,
                            color = colors.textSecondary
                        )
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                        contentPadding = PaddingValues(bottom = 24.dp)
                    ) {
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
                                }
                            )
                        }
                    }
                }
            } else {
                // Skills Tab
                val query = searchText.trim().lowercase()
                val filteredSkills = remember(skills, query) {
                    if (query.isEmpty()) skills
                    else skills.filter { sk ->
                        sk.name.lowercase().contains(query) ||
                        (sk.description?.lowercase()?.contains(query) == true)
                    }
                }

                if (filteredSkills.isEmpty()) {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f),
                        contentAlignment = Alignment.Center
                    ) {
                        Text(
                            text = "未找到匹配的技能",
                            fontSize = 14.sp,
                            color = colors.textSecondary
                        )
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                        contentPadding = PaddingValues(bottom = 24.dp)
                    ) {
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
            }
        }
    }

    // Skill detail dialog
    viewingSkill?.let { skill ->
        AlertDialog(
            onDismissRequest = { viewingSkill = null },
            confirmButton = {
                TextButton(onClick = { viewingSkill = null }) {
                    Text("关闭", color = colors.accentIndigo)
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
                            text = "规则定义 (SKILL.md)：",
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
}

@Composable
private fun McpServerCard(
    server: McpServerState,
    isExpanded: Boolean,
    onToggleExpand: () -> Unit,
    expandedTools: Set<String>,
    onToggleTool: (String) -> Unit
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
                    text = "可用工具 (${server.toolCount})",
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
                    text = "${params.size} 参数",
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
                    text = "无输入参数（直接调用）",
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
                        text = "参数定义：",
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
                                        text = "必填",
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
                                        text = "默认: $def",
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
