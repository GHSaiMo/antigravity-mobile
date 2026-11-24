package com.antigravity.mobile.ui.components

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
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
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.data.model.AutomationEventItem
import com.antigravity.mobile.data.model.AutomationSidecarItem
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelOption
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.deleteAutomation
import com.antigravity.mobile.data.service.fetchAutomationLogs
import com.antigravity.mobile.data.service.updateAutomation
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.launch

internal object AutomationCronHelper {
    fun splitTimeZone(raw: String): Pair<String?, String> {
        val trimmed = raw.trim()
        for (prefix in listOf("CRON_TZ=", "TZ=")) {
            if (trimmed.startsWith(prefix)) {
                val rest = trimmed.removePrefix(prefix)
                val idx = rest.indexOfFirst { it.isWhitespace() }
                if (idx > 0) {
                    return rest.substring(0, idx) to rest.substring(idx).trim()
                }
            }
        }
        return null to trimmed
    }

    fun combine(timeZone: String, cron: String): String {
        val tz = timeZone.trim()
        val c = cron.trim()
        return if (tz.isEmpty()) c else "CRON_TZ=$tz $c"
    }

    fun humanSummary(raw: String): String {
        val (tz, cron) = splitTimeZone(raw)
        val parts = cron.split(Regex("\\s+")).filter { it.isNotEmpty() }
        var base = cron.ifBlank { "未配置调度".loc }
        if (parts.size == 5) {
            val (min, hour, dom, mon, dow) = parts
            if (min == "0" && hour == "*" && dom == "*" && mon == "*" && dow == "*") {
                base = "每小时整点".loc
            } else {
                val m = min.toIntOrNull()
                val h = hour.toIntOrNull()
                if (m != null && h != null && m in 0..59 && h in 0..23 && dom == "*" && mon == "*") {
                    val hm = "%02d:%02d".format(h, m)
                    if (dow == "*") {
                        base = "每天 %s".loc(hm)
                    } else {
                        val d = dow.toIntOrNull()
                        if (d != null && d in 0..7) {
                            val names = listOf("周日", "周一", "周二", "周三", "周四", "周五", "周六", "周日")
                            base = "每%1\$s %2\$s".loc(names[d].loc, hm)
                        }
                    }
                }
            }
        }
        return if (!tz.isNullOrBlank()) "$base ($tz)" else base
    }
}

private enum class AndroidSchedulePreset(val label: String) {
    HOURLY("每小时"),
    DAILY("每天"),
    WEEKLY("每周"),
    CUSTOM("自定义");

    fun localizedLabel(): String = when (this) {
        WEEKLY -> when (com.antigravity.mobile.ui.util.L10n.currentLang) {
            com.antigravity.mobile.ui.util.L10n.AppLang.JA -> "毎週"
            else -> label.loc
        }
        else -> label.loc
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AutomationDetailDialog(
    sidecar: AutomationSidecarItem,
    allEvents: List<AutomationEventItem>,
    apiClient: ApiClient,
    prefs: PreferencesManager,
    onRefresh: suspend () -> Unit,
    onDismiss: () -> Unit,
    onSelectConversation: ((cascadeId: String, title: String) -> Unit)? = null
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    val sid = sidecar.targetSidecarId
    val taskEvents = remember(allEvents, sid, sidecar.name) {
        allEvents.filter { it.matches(sidecar) }
    }

    var modelOptions by remember {
        mutableStateOf(
            prefs.cachedModelCatalog?.models?.takeIf { it.isNotEmpty() } ?: ModelDefaults.fallbackOptions()
        )
    }
    LaunchedEffect(Unit) {
        apiClient.getModels(refresh = false).onSuccess { resp ->
            if (resp.models.isNotEmpty()) {
                modelOptions = resp.models
            }
        }
    }

    var isBusy by remember { mutableStateOf(false) }
    var overflowExpanded by remember { mutableStateOf(false) }
    var showDeleteConfirm by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    var showAllEvents by remember { mutableStateOf(false) }

    // Title inline editor
    var isEditingTitle by remember { mutableStateOf(false) }
    var draftTitle by remember(sidecar.name) { mutableStateOf(sidecar.name) }

    // Schedule & model inline editor
    var isEditingSchedule by remember { mutableStateOf(false) }
    var draftPreset by remember { mutableStateOf(AndroidSchedulePreset.CUSTOM) }
    var draftHour by remember { mutableIntStateOf(9) }
    var draftWeekday by remember { mutableIntStateOf(1) }
    var draftCustomCron by remember { mutableStateOf(sidecar.cronExpression ?: "0 9 * * *") }
    var draftTimeZone by remember { mutableStateOf(sidecar.timeZone ?: "") }
    var draftModel by remember { mutableStateOf(sidecar.model ?: "") }
    var modelMenuExpanded by remember { mutableStateOf(false) }

    // Prompt inline editor
    var isEditingPrompt by remember { mutableStateOf(false) }
    var draftPrompt by remember(sidecar.prompt) { mutableStateOf(sidecar.prompt ?: "") }

    // Logs dialog
    var showLogsDialog by remember { mutableStateOf(false) }
    var logsLoading by remember { mutableStateOf(false) }
    var logsText by remember { mutableStateOf("") }
    var logsError by remember { mutableStateOf<String?>(null) }
    var logsTruncated by remember { mutableStateOf(false) }

    fun resolvedDraftCron(): String = when (draftPreset) {
        AndroidSchedulePreset.CUSTOM -> draftCustomCron.trim()
        AndroidSchedulePreset.HOURLY -> AutomationCronHelper.combine(draftTimeZone, "0 * * * *")
        AndroidSchedulePreset.DAILY -> AutomationCronHelper.combine(draftTimeZone, "0 $draftHour * * *")
        AndroidSchedulePreset.WEEKLY -> AutomationCronHelper.combine(draftTimeZone, "0 $draftHour * * $draftWeekday")
    }

    fun beginEditSchedule() {
        val (tz, cron) = AutomationCronHelper.splitTimeZone(sidecar.cronExpression ?: "")
        draftTimeZone = tz ?: (sidecar.timeZone ?: "")
        draftCustomCron = sidecar.cronExpression ?: "0 9 * * *"
        draftModel = sidecar.model ?: ""
        val parts = cron.split(Regex("\\s+")).filter { it.isNotEmpty() }
        if (parts.size == 5 && parts[0] == "0" && parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*") {
            draftPreset = AndroidSchedulePreset.HOURLY
        } else if (parts.size == 5 && parts[0] == "0" && parts[1].toIntOrNull() in 0..23 && parts[2] == "*" && parts[3] == "*" && parts[4] == "*") {
            draftPreset = AndroidSchedulePreset.DAILY
            draftHour = parts[1].toInt()
        } else if (parts.size == 5 && parts[0] == "0" && parts[1].toIntOrNull() in 0..23 && parts[2] == "*" && parts[3] == "*" && parts[4].toIntOrNull() in 0..7) {
            draftPreset = AndroidSchedulePreset.WEEKLY
            draftHour = parts[1].toInt()
            val d = parts[4].toInt()
            draftWeekday = if (d == 7) 0 else d
        } else {
            draftPreset = AndroidSchedulePreset.CUSTOM
        }
        isEditingSchedule = true
    }

    fun loadLogs() {
        scope.launch {
            logsLoading = true
            logsError = null
            apiClient.fetchAutomationLogs(sid, tail = 200)
                .onSuccess { res ->
                    logsText = res.logs
                    logsTruncated = res.truncated
                }
                .onFailure { err ->
                    logsError = err.message ?: "获取运行日志失败".loc
                }
            logsLoading = false
        }
    }

    val currentModelDisplay = remember(sidecar.model, modelOptions) {
        val m = sidecar.model
        if (m.isNullOrBlank()) "默认模型".loc else ModelDefaults.displayName(m, modelOptions)
    }
    val workspaceDisplay = remember(sidecar.workspaceUri) {
        val uri = sidecar.workspaceUri?.takeIf { it.isNotBlank() } ?: return@remember null
        val cleaned = uri.removePrefix("file://").trimEnd('/')
        cleaned.substringAfterLast('/').ifBlank { cleaned }
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = Modifier
                .fillMaxSize()
                .padding(top = 24.dp),
            shape = RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp),
            color = colors.background
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                // Top Bar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 8.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    IconButton(onClick = onDismiss) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "返回".loc,
                            tint = colors.textPrimary
                        )
                    }
                    Text(
                        text = "任务详情".loc,
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier.weight(1f)
                    )
                    Box {
                        IconButton(onClick = { overflowExpanded = true }) {
                            Icon(
                                imageVector = Icons.Default.MoreVert,
                                contentDescription = "更多操作".loc,
                                tint = colors.textPrimary
                            )
                        }
                        DropdownMenu(
                            expanded = overflowExpanded,
                            onDismissRequest = { overflowExpanded = false },
                            containerColor = colors.surface
                        ) {
                            DropdownMenuItem(
                                text = { Text("重启任务".loc) },
                                enabled = sidecar.isRunning && !isBusy,
                                leadingIcon = { Icon(Icons.Default.Refresh, contentDescription = null) },
                                onClick = {
                                    overflowExpanded = false
                                    scope.launch {
                                        isBusy = true
                                        apiClient.manageAutomation("restart", sid)
                                            .onSuccess {
                                                onRefresh()
                                                Toast.makeText(context, "已重启任务".loc, Toast.LENGTH_SHORT).show()
                                            }
                                            .onFailure { errorMessage = it.message }
                                        isBusy = false
                                    }
                                }
                            )
                            DropdownMenuItem(
                                text = { Text("查看日志".loc) },
                                leadingIcon = { Icon(Icons.Default.Description, contentDescription = null) },
                                onClick = {
                                    overflowExpanded = false
                                    showLogsDialog = true
                                    loadLogs()
                                }
                            )
                            HorizontalDivider()
                            DropdownMenuItem(
                                text = { Text("删除任务".loc, color = Color(0xFFF44336)) },
                                enabled = !isBusy,
                                leadingIcon = {
                                    Icon(Icons.Default.Delete, contentDescription = null, tint = Color(0xFFF44336))
                                },
                                onClick = {
                                    overflowExpanded = false
                                    showDeleteConfirm = true
                                }
                            )
                        }
                    }
                }

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                Column(
                    modifier = Modifier
                        .weight(1f)
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp, vertical = 14.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    // Header Card: Title + Status + Run Now + Toggle
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        if (isEditingTitle) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                OutlinedTextField(
                                    value = draftTitle,
                                    onValueChange = { draftTitle = it },
                                    singleLine = true,
                                    modifier = Modifier.weight(1f),
                                    label = { Text("任务名称".loc) }
                                )
                                TextButton(
                                    enabled = !isBusy && draftTitle.trim().isNotEmpty(),
                                    onClick = {
                                        scope.launch {
                                            isBusy = true
                                            apiClient.updateAutomation(sid, name = draftTitle.trim())
                                                .onSuccess {
                                                    onRefresh()
                                                    isEditingTitle = false
                                                }
                                                .onFailure { errorMessage = it.message }
                                            isBusy = false
                                        }
                                    }
                                ) { Text("保存".loc) }
                                TextButton(onClick = { isEditingTitle = false }) { Text("取消".loc) }
                            }
                        } else {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text(
                                    text = sidecar.name.ifBlank { sid },
                                    color = colors.textPrimary,
                                    fontSize = 18.sp,
                                    fontWeight = FontWeight.Bold,
                                    modifier = Modifier.weight(1f)
                                )
                                IconButton(
                                    onClick = {
                                        draftTitle = sidecar.name
                                        isEditingTitle = true
                                    },
                                    modifier = Modifier.size(30.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.Edit,
                                        contentDescription = "编辑任务名称".loc,
                                        tint = colors.textSecondary,
                                        modifier = Modifier.size(16.dp)
                                    )
                                }
                            }
                        }

                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            val isErr = sidecar.status.equals("ERROR", ignoreCase = true)
                            val badgeColor = when {
                                isErr -> Color(0xFFF44336)
                                sidecar.isRunning -> colors.accentGreen
                                else -> colors.textSecondary
                            }
                            val badgeText = when {
                                isErr -> "异常".loc
                                sidecar.isRunning -> "运行中".loc
                                else -> "已暂停".loc
                            }
                            Box(
                                modifier = Modifier
                                    .clip(CircleShape)
                                    .background(badgeColor.copy(alpha = 0.14f))
                                    .padding(horizontal = 10.dp, vertical = 4.dp)
                            ) {
                                Text(
                                    text = badgeText,
                                    color = badgeColor,
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }

                            Spacer(modifier = Modifier.weight(1f))

                            // Run Now button (disabled when paused, matching desktop)
                            FilledTonalButton(
                                enabled = sidecar.isRunning && !isBusy,
                                onClick = {
                                    scope.launch {
                                        isBusy = true
                                        apiClient.manageAutomation("run_now", sid)
                                            .onSuccess {
                                                onRefresh()
                                                Toast.makeText(context, "已触发立即运行".loc, Toast.LENGTH_SHORT).show()
                                            }
                                            .onFailure { errorMessage = it.message }
                                        isBusy = false
                                    }
                                },
                                contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                                modifier = Modifier.height(32.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.PlayArrow,
                                    contentDescription = null,
                                    modifier = Modifier.size(15.dp)
                                )
                                Spacer(modifier = Modifier.width(3.dp))
                                Text("立即运行".loc, fontSize = 12.sp, maxLines = 1, softWrap = false)
                            }

                            Spacer(modifier = Modifier.width(10.dp))

                            Switch(
                                checked = sidecar.isRunning,
                                enabled = !isBusy,
                                onCheckedChange = { enable ->
                                    scope.launch {
                                        isBusy = true
                                        apiClient.manageAutomation(if (enable) "start" else "stop", sid)
                                            .onSuccess {
                                                onRefresh()
                                                Toast.makeText(
                                                    context,
                                                    if (enable) "已启用定时任务".loc else "已暂停定时任务".loc,
                                                    Toast.LENGTH_SHORT
                                                ).show()
                                            }
                                            .onFailure { errorMessage = it.message }
                                        isBusy = false
                                    }
                                }
                            )
                        }

                        if (!sidecar.lastError.isNullOrBlank()) {
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(Color(0xFFF44336).copy(alpha = 0.1f))
                                    .padding(10.dp)
                            ) {
                                Text(
                                    text = sidecar.lastError,
                                    color = Color(0xFFF44336),
                                    fontSize = 12.sp
                                )
                            }
                        }
                    }

                    // Schedule & Model Section
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text(
                                text = "调度与配置".loc,
                                color = colors.textSecondary,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            if (!isEditingSchedule) {
                                Text(
                                    text = "编辑".loc,
                                    color = colors.accentIndigo,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    modifier = Modifier.clickable { beginEditSchedule() }
                                )
                            }
                        }

                        if (isEditingSchedule) {
                            // Frequency preset pills (compact so EN/JA labels never clip)
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                AndroidSchedulePreset.entries.forEach { preset ->
                                    val selected = draftPreset == preset
                                    Box(
                                        modifier = Modifier
                                            .weight(1f)
                                            .clip(RoundedCornerShape(8.dp))
                                            .background(if (selected) colors.accentIndigo else colors.surfaceVariant)
                                            .border(
                                                width = if (selected) 0.dp else 0.5.dp,
                                                color = if (selected) Color.Transparent else colors.border,
                                                shape = RoundedCornerShape(8.dp)
                                            )
                                            .clickable { draftPreset = preset }
                                            .padding(horizontal = 4.dp, vertical = 7.dp),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Text(
                                            text = preset.localizedLabel(),
                                            color = if (selected) Color.White else colors.textPrimary,
                                            fontSize = 11.5.sp,
                                            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
                                    }
                                }
                            }

                            if (draftPreset == AndroidSchedulePreset.WEEKLY) {
                                val weekdays = listOf(1 to "周一", 2 to "周二", 3 to "周三", 4 to "周四", 5 to "周五", 6 to "周六", 0 to "周日")
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    weekdays.forEach { (d, lbl) ->
                                        val sel = draftWeekday == d
                                        Box(
                                            modifier = Modifier
                                                .weight(1f)
                                                .clip(RoundedCornerShape(8.dp))
                                                .background(if (sel) colors.accentIndigo else colors.surfaceVariant)
                                                .clickable { draftWeekday = d }
                                                .padding(vertical = 6.dp),
                                            contentAlignment = Alignment.Center
                                        ) {
                                            Text(
                                                text = lbl.loc,
                                                color = if (sel) Color.White else colors.textPrimary,
                                                fontSize = 11.sp,
                                                fontWeight = FontWeight.Medium,
                                                maxLines = 1,
                                                softWrap = false
                                            )
                                        }
                                    }
                                }
                            }

                            if (draftPreset == AndroidSchedulePreset.DAILY || draftPreset == AndroidSchedulePreset.WEEKLY) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Text("触发小时".loc, fontSize = 13.sp, color = colors.textSecondary)
                                    IconButton(onClick = { draftHour = (draftHour + 23) % 24 }, modifier = Modifier.size(30.dp)) {
                                        Icon(Icons.Default.Remove, contentDescription = null)
                                    }
                                    Text(
                                        text = "%02d:00".format(draftHour),
                                        fontSize = 15.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        fontFamily = FontFamily.Monospace,
                                        color = colors.textPrimary
                                    )
                                    IconButton(onClick = { draftHour = (draftHour + 1) % 24 }, modifier = Modifier.size(30.dp)) {
                                        Icon(Icons.Default.Add, contentDescription = null)
                                    }
                                }
                            } else if (draftPreset == AndroidSchedulePreset.CUSTOM) {
                                OutlinedTextField(
                                    value = draftCustomCron,
                                    onValueChange = { draftCustomCron = it },
                                    singleLine = true,
                                    label = { Text("Cron 表达式".loc) },
                                    modifier = Modifier.fillMaxWidth()
                                )
                            } else {
                                Text(
                                    text = "每小时整点自动执行（0 * * * *）".loc,
                                    color = colors.textSecondary,
                                    fontSize = 12.5.sp
                                )
                            }

                            // Model Dropdown
                            Box(modifier = Modifier.fillMaxWidth()) {
                                val selectedModelLabel = if (draftModel.isBlank()) "默认模型".loc else ModelDefaults.displayName(draftModel, modelOptions)
                                OutlinedButton(
                                    onClick = { modelMenuExpanded = true },
                                    modifier = Modifier.fillMaxWidth()
                                ) {
                                    Text("${"执行模型".loc}: $selectedModelLabel", maxLines = 1, overflow = TextOverflow.Ellipsis)
                                }
                                DropdownMenu(
                                    expanded = modelMenuExpanded,
                                    onDismissRequest = { modelMenuExpanded = false },
                                    containerColor = colors.surface
                                ) {
                                    DropdownMenuItem(
                                        text = { Text("默认模型".loc) },
                                        onClick = {
                                            draftModel = ""
                                            modelMenuExpanded = false
                                        }
                                    )
                                    modelOptions.forEach { opt ->
                                        DropdownMenuItem(
                                            text = { Text(opt.name) },
                                            onClick = {
                                                draftModel = opt.id
                                                modelMenuExpanded = false
                                            }
                                        )
                                    }
                                }
                            }

                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Button(
                                    enabled = !isBusy && resolvedDraftCron().isNotEmpty(),
                                    onClick = {
                                        scope.launch {
                                            isBusy = true
                                            apiClient.updateAutomation(
                                                sidecarId = sid,
                                                cron = resolvedDraftCron(),
                                                model = draftModel
                                            ).onSuccess {
                                                onRefresh()
                                                isEditingSchedule = false
                                                Toast.makeText(context, "调度配置已保存".loc, Toast.LENGTH_SHORT).show()
                                            }.onFailure { errorMessage = it.message }
                                            isBusy = false
                                        }
                                    }
                                ) { Text("保存配置".loc) }
                                TextButton(onClick = { isEditingSchedule = false }) { Text("取消".loc) }
                            }
                        } else {
                            DetailInfoRow("调度频率".loc, AutomationCronHelper.humanSummary(sidecar.cronExpression ?: ""))
                            if (!sidecar.cronExpression.isNullOrBlank()) {
                                DetailInfoRow("Cron", sidecar.cronExpression, monospace = true)
                            }
                            DetailInfoRow("执行模型".loc, currentModelDisplay)
                            if (workspaceDisplay != null) {
                                DetailInfoRow("绑定工作区".loc, workspaceDisplay)
                            }
                            DetailInfoRow(
                                "下次运行".loc,
                                when {
                                    !sidecar.isRunning -> "已暂停".loc
                                    !sidecar.nextRunAt.isNullOrBlank() -> sidecar.nextRunAt
                                    else -> "等待调度".loc
                                },
                                monospace = sidecar.isRunning && !sidecar.nextRunAt.isNullOrBlank()
                            )
                            if (!sidecar.lastRunAt.isNullOrBlank()) {
                                DetailInfoRow("上次运行".loc, sidecar.lastRunAt, monospace = true)
                            }
                        }
                    }

                    // Instructions Section
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text(
                                text = "执行指令".loc,
                                color = colors.textSecondary,
                                fontSize = 12.5.sp,
                                fontWeight = FontWeight.SemiBold
                            )
                            if (!isEditingPrompt) {
                                Text(
                                    text = "编辑".loc,
                                    color = colors.accentIndigo,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    modifier = Modifier.clickable {
                                        draftPrompt = sidecar.prompt ?: ""
                                        isEditingPrompt = true
                                    }
                                )
                            }
                        }

                        if (isEditingPrompt) {
                            OutlinedTextField(
                                value = draftPrompt,
                                onValueChange = { draftPrompt = it },
                                minLines = 5,
                                maxLines = 14,
                                modifier = Modifier.fillMaxWidth()
                            )
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Button(
                                    enabled = !isBusy && draftPrompt.trim().isNotEmpty(),
                                    onClick = {
                                        scope.launch {
                                            isBusy = true
                                            apiClient.updateAutomation(sid, prompt = draftPrompt.trim())
                                                .onSuccess {
                                                    onRefresh()
                                                    isEditingPrompt = false
                                                    Toast.makeText(context, "执行指令已保存".loc, Toast.LENGTH_SHORT).show()
                                                }
                                                .onFailure { errorMessage = it.message }
                                            isBusy = false
                                        }
                                    }
                                ) { Text("保存指令".loc) }
                                TextButton(onClick = { isEditingPrompt = false }) { Text("取消".loc) }
                            }
                        } else {
                            if (!sidecar.description.isNullOrBlank()) {
                                Text(
                                    text = sidecar.description,
                                    color = colors.textSecondary,
                                    fontSize = 12.5.sp,
                                    lineHeight = 18.sp
                                )
                                HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.6f))
                            }
                            SelectionContainer {
                                Text(
                                    text = sidecar.prompt?.takeIf { it.isNotBlank() } ?: "暂无执行指令".loc,
                                    color = if (sidecar.prompt.isNullOrBlank()) colors.textSecondary else colors.textPrimary,
                                    fontSize = 13.5.sp,
                                    lineHeight = 20.sp
                                )
                            }
                        }
                    }

                    // Run History Section
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Text(
                            text = "${"运行历史".loc} (${taskEvents.size})",
                            color = colors.textSecondary,
                            fontSize = 12.5.sp,
                            fontWeight = FontWeight.SemiBold
                        )
                        if (taskEvents.isEmpty()) {
                            Text(
                                text = if (sidecar.isRunning) "暂无运行记录".loc else "任务已暂停，启用后触发记录将显示在此处".loc,
                                color = colors.textSecondary,
                                fontSize = 13.sp
                            )
                        } else {
                            val visible = if (showAllEvents || taskEvents.size <= 8) taskEvents else taskEvents.take(8)
                            visible.forEachIndexed { idx, ev ->
                                if (idx > 0) {
                                    HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.5f))
                                }
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clickable(enabled = !ev.conversationId.isNullOrBlank()) {
                                            val cid = ev.conversationId
                                            if (!cid.isNullOrBlank()) {
                                                if (ev.conversationExists == false) {
                                                    Toast.makeText(context, "该触发记录对应的会话已删除".loc, Toast.LENGTH_SHORT).show()
                                                } else {
                                                    onDismiss()
                                                    onSelectConversation?.invoke(cid, sidecar.name.ifBlank { sid })
                                                }
                                            }
                                        }
                                        .padding(vertical = 6.dp),
                                    verticalAlignment = Alignment.Top,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    val isErr = ev.status.equals("error", ignoreCase = true)
                                    Icon(
                                        imageVector = if (isErr) Icons.Default.Error else Icons.Default.CheckCircle,
                                        contentDescription = null,
                                        tint = if (isErr) Color(0xFFF44336) else colors.accentGreen,
                                        modifier = Modifier.size(16.dp).padding(top = 2.dp)
                                    )
                                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                        Text(
                                            text = ev.timestamp,
                                            color = colors.textPrimary,
                                            fontSize = 12.sp,
                                            fontFamily = FontFamily.Monospace,
                                            fontWeight = FontWeight.Medium
                                        )
                                        if (!ev.message.isNullOrBlank()) {
                                            Text(
                                                text = ev.message,
                                                color = if (isErr) Color(0xFFF44336) else colors.textSecondary,
                                                fontSize = 12.sp,
                                                maxLines = 2,
                                                overflow = TextOverflow.Ellipsis
                                            )
                                        }
                                    }
                                    if (!ev.conversationId.isNullOrBlank()) {
                                        Icon(
                                            imageVector = Icons.AutoMirrored.Filled.KeyboardArrowRight,
                                            contentDescription = null,
                                            tint = colors.textSecondary,
                                            modifier = Modifier.size(16.dp)
                                        )
                                    }
                                }
                            }
                            if (taskEvents.size > 8) {
                                TextButton(
                                    onClick = { showAllEvents = !showAllEvents },
                                    modifier = Modifier.align(Alignment.CenterHorizontally)
                                ) {
                                    Text(if (showAllEvents) "收起记录".loc else "${"查看全部".loc} (${taskEvents.size})")
                                }
                            }
                        }
                    }

                    Spacer(modifier = Modifier.height(24.dp))
                }
            }
        }
    }

    if (showDeleteConfirm) {
        AlertDialog(
            onDismissRequest = { showDeleteConfirm = false },
            containerColor = colors.surface,
            title = { Text("确认删除定时任务？".loc) },
            text = { Text("确定要删除定时任务「%s」吗？删除后将停止调度并移除该任务配置。".loc(sidecar.name)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        showDeleteConfirm = false
                        scope.launch {
                            isBusy = true
                            apiClient.deleteAutomation(sid)
                                .onSuccess {
                                    onRefresh()
                                    onDismiss()
                                }
                                .onFailure { errorMessage = it.message }
                            isBusy = false
                        }
                    }
                ) { Text("删除".loc, color = Color(0xFFF44336)) }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteConfirm = false }) { Text("取消".loc) }
            }
        )
    }

    if (errorMessage != null) {
        AlertDialog(
            onDismissRequest = { errorMessage = null },
            containerColor = colors.surface,
            title = { Text("操作失败".loc) },
            text = { Text(errorMessage.orEmpty()) },
            confirmButton = {
                TextButton(onClick = { errorMessage = null }) { Text("确定".loc) }
            }
        )
    }

    if (showLogsDialog) {
        AlertDialog(
            onDismissRequest = { showLogsDialog = false },
            containerColor = colors.surface,
            title = {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text("运行日志".loc, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
                    Row {
                        if (logsText.isNotBlank()) {
                            IconButton(
                                onClick = {
                                    val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
                                    cm?.setPrimaryClip(ClipData.newPlainText("sidecar.log", logsText))
                                    Toast.makeText(context, "已复制日志".loc, Toast.LENGTH_SHORT).show()
                                },
                                modifier = Modifier.size(32.dp)
                            ) {
                                Icon(Icons.Default.ContentCopy, contentDescription = "复制".loc, modifier = Modifier.size(16.dp))
                            }
                        }
                        IconButton(onClick = { loadLogs() }, enabled = !logsLoading, modifier = Modifier.size(32.dp)) {
                            Icon(Icons.Default.Refresh, contentDescription = "刷新".loc, modifier = Modifier.size(16.dp))
                        }
                    }
                }
            },
            text = {
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = 160.dp, max = 420.dp)
                        .verticalScroll(rememberScrollState())
                ) {
                    when {
                        logsLoading && logsText.isBlank() -> {
                            CircularProgressIndicator(modifier = Modifier.align(Alignment.Center).size(24.dp))
                        }
                        logsError != null -> {
                            Text(logsError.orEmpty(), color = Color(0xFFF44336), fontSize = 12.5.sp)
                        }
                        logsText.isBlank() -> {
                            Text("暂无日志输出".loc, color = colors.textSecondary, fontSize = 13.sp)
                        }
                        else -> {
                            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                                if (logsTruncated) {
                                    Text("仅展示最近 200 行日志".loc, color = colors.textSecondary, fontSize = 11.sp)
                                }
                                SelectionContainer {
                                    Text(
                                        text = logsText,
                                        fontFamily = FontFamily.Monospace,
                                        fontSize = 11.sp,
                                        lineHeight = 15.sp,
                                        color = colors.textPrimary
                                    )
                                }
                            }
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = { showLogsDialog = false }) { Text("关闭".loc) }
            }
        )
    }
}

@Composable
private fun DetailInfoRow(
    label: String,
    value: String,
    monospace: Boolean = false
) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Text(
            text = label,
            color = colors.textSecondary,
            fontSize = 13.sp,
            modifier = Modifier.padding(end = 8.dp)
        )
        Text(
            text = value,
            color = colors.textPrimary,
            fontSize = if (monospace) 12.sp else 13.sp,
            fontFamily = if (monospace) FontFamily.Monospace else FontFamily.Default,
            fontWeight = FontWeight.Medium,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f, fill = false)
        )
    }
}
