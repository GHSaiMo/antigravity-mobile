package com.antigravity.mobile.ui.components

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.ui.graphics.Brush
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
import com.antigravity.mobile.data.model.ProjectItem
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.createAutomation
import com.antigravity.mobile.data.service.deleteAutomation
import com.antigravity.mobile.data.service.fetchAutomationLogs
import com.antigravity.mobile.data.service.updateAutomation
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.L10n
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Locale
import java.util.TimeZone

internal object AutomationSlugHelper {
    fun slug(raw: String): String {
        val lower = raw.trim().lowercase(Locale.US)
        if (lower.isEmpty()) return ""
        val sb = StringBuilder()
        var prevDash = false
        for (ch in lower) {
            if ((ch in 'a'..'z') || (ch in '0'..'9')) {
                sb.append(ch)
                prevDash = false
            } else if (!prevDash && sb.isNotEmpty()) {
                sb.append('-')
                prevDash = true
            }
        }
        return sb.toString().trim('-')
    }
}

internal object AutomationDataSourceExtractor {
    private val patterns = listOf(
        Regex("\\bgcalendar\\b|calendar", RegexOption.IGNORE_CASE) to "Calendar",
        Regex("\\bgmail\\b", RegexOption.IGNORE_CASE) to "Gmail",
        Regex("\\bgchat\\b|google chat", RegexOption.IGNORE_CASE) to "Chat",
        Regex("\\bgdocs\\b|google docs", RegexOption.IGNORE_CASE) to "Docs",
        Regex("\\bgdrive\\b|google drive", RegexOption.IGNORE_CASE) to "Drive",
        Regex("\\bgsheets\\b", RegexOption.IGNORE_CASE) to "Sheets",
        Regex("\\bgh\\s+|github", RegexOption.IGNORE_CASE) to "GitHub"
    )

    fun extract(prompt: String, accessGrants: List<String>): Pair<List<String>, Boolean> {
        val corpus = (listOf(prompt) + accessGrants).joinToString("\n")
        val found = patterns.mapNotNull { (re, label) -> if (re.containsMatchIn(corpus)) label else null }
        val readOnly = accessGrants.isNotEmpty() && accessGrants.all {
            it.startsWith("read_file(") || it.startsWith("read_url(")
        }
        return found to readOnly
    }
}

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

    fun shortWeekday(day: Int): String = when (day) {
        1 -> "周一".loc
        2 -> "周二".loc
        3 -> "周三".loc
        4 -> "周四".loc
        5 -> "周五".loc
        6 -> "周六".loc
        else -> "周日".loc
    }

    fun parseSortMillis(raw: String?): Long {
        if (raw.isNullOrBlank()) return Long.MAX_VALUE
        val clean = raw.trim()
        val patterns = listOf(
            "yyyy-MM-dd HH:mm:ss",
            "yyyy-MM-dd'T'HH:mm:ss.SSS'Z'",
            "yyyy-MM-dd'T'HH:mm:ss'Z'"
        )
        for (pat in patterns) {
            try {
                val fmt = SimpleDateFormat(pat, Locale.US)
                if (pat.endsWith("'Z'")) fmt.timeZone = TimeZone.getTimeZone("UTC")
                fmt.parse(clean)?.time?.let { return it }
            } catch (_: Exception) {
            }
        }
        return Long.MAX_VALUE
    }

    fun humanSummary(raw: String): String {
        val (tz, cron) = splitTimeZone(raw)
        val parts = cron.split(Regex("\\s+")).filter { it.isNotEmpty() }
        var base = cron.ifBlank { "未配置调度".loc }
        if (parts.size == 5) {
            val (min, hour, dom, mon, dow) = parts
            if (min == "0" && hour == "*" && dom == "*" && mon == "*" && dow == "*") {
                base = "每小时整点".loc
            } else if (min == "0" && hour.startsWith("*/") && dom == "*" && mon == "*" && dow == "*") {
                val step = hour.removePrefix("*/").toIntOrNull()
                if (step != null && step > 0) {
                    base = "每 %d 小时".loc(step)
                }
            } else {
                val m = min.toIntOrNull()
                val h = hour.toIntOrNull()
                if (m != null && h != null && m in 0..59 && h in 0..23 && mon == "*") {
                    val hm = String.format(Locale.US, "%02d:%02d", h, m)
                    if (dom == "*") {
                        when {
                            dow == "*" -> base = "每天 %s".loc(hm)
                            dow == "1-5" -> base = "工作日 %s".loc(hm)
                            dow.toIntOrNull() in 0..7 -> {
                                val d = dow.toInt()
                                val name = shortWeekday(if (d == 7) 0 else d)
                                base = "每%1\$s %2\$s".loc(name, hm)
                            }
                            dow.contains(",") -> {
                                val days = dow.split(",")
                                    .mapNotNull { it.toIntOrNull() }
                                    .filter { it in 0..7 }
                                    .map { shortWeekday(if (it == 7) 0 else it) }
                                if (days.isNotEmpty()) {
                                    val sep = if (L10n.isEnglish) ", " else "、"
                                    base = "${days.joinToString(sep)} $hm"
                                }
                            }
                        }
                    } else if (dow == "*") {
                        val dayNum = dom.toIntOrNull()
                        if (dayNum != null && dayNum in 1..31) {
                            base = "每月 %1\$d 日 %2\$s".loc(dayNum, hm)
                        }
                    }
                }
            }
        }
        return if (!tz.isNullOrBlank()) "$base ($tz)" else base
    }
}

internal enum class AndroidScheduleFrequency(val label: String) {
    HOURLY("每小时"),
    DAILY("每天"),
    WEEKDAYS("工作日"),
    WEEKLY("每周"),
    CUSTOM("自定义");

    fun localizedLabel(): String = when (this) {
        WEEKLY -> when (L10n.currentLang) {
            L10n.AppLang.JA -> "毎週"
            else -> label.loc
        }
        else -> label.loc
    }
}

internal enum class AndroidCustomScheduleUnit(val label: String) {
    EVERY_HOURS("每隔几小时"),
    MULTI_WEEKDAY("每周多选"),
    MONTHLY("每月"),
    RAW_CRON("Cron 表达式");

    fun localizedLabel(): String = label.loc
}

internal data class AndroidScheduleDraft(
    val frequency: AndroidScheduleFrequency = AndroidScheduleFrequency.DAILY,
    val hour: Int = 9,
    val minute: Int = 0,
    val weekday: Int = 1,
    val customUnit: AndroidCustomScheduleUnit = AndroidCustomScheduleUnit.EVERY_HOURS,
    val everyHours: Int = 2,
    val multiWeekdays: Set<Int> = setOf(1, 3, 5),
    val dayOfMonth: Int = 1,
    val rawCron: String = "0 9 * * *",
    val timeZone: String = ""
) {
    fun resolvedCron(): String = when (frequency) {
        AndroidScheduleFrequency.HOURLY -> AutomationCronHelper.combine(timeZone, "0 * * * *")
        AndroidScheduleFrequency.DAILY -> AutomationCronHelper.combine(timeZone, "$minute $hour * * *")
        AndroidScheduleFrequency.WEEKDAYS -> AutomationCronHelper.combine(timeZone, "$minute $hour * * 1-5")
        AndroidScheduleFrequency.WEEKLY -> AutomationCronHelper.combine(timeZone, "$minute $hour * * $weekday")
        AndroidScheduleFrequency.CUSTOM -> when (customUnit) {
            AndroidCustomScheduleUnit.EVERY_HOURS -> AutomationCronHelper.combine(timeZone, "0 */$everyHours * * *")
            AndroidCustomScheduleUnit.MULTI_WEEKDAY -> {
                val ordered = listOf(1, 2, 3, 4, 5, 6, 0).filter { it in multiWeekdays }
                val days = if (ordered.isEmpty()) "1" else ordered.joinToString(",")
                AutomationCronHelper.combine(timeZone, "$minute $hour * * $days")
            }
            AndroidCustomScheduleUnit.MONTHLY -> AutomationCronHelper.combine(timeZone, "$minute $hour $dayOfMonth * *")
            AndroidCustomScheduleUnit.RAW_CRON -> rawCron.trim()
        }
    }

    companion object {
        val everyHourChoices = listOf(2, 3, 4, 6, 8, 12)
        val orderedWeekdays = listOf(1, 2, 3, 4, 5, 6, 0)

        fun from(cronExpression: String, fallbackTimeZone: String? = null): AndroidScheduleDraft {
            val (tz, cron) = AutomationCronHelper.splitTimeZone(cronExpression)
            val resolvedTz = tz ?: (fallbackTimeZone ?: "")
            val parts = cron.split(Regex("\\s+")).filter { it.isNotEmpty() }
            if (parts.size != 5) {
                return AndroidScheduleDraft(
                    frequency = AndroidScheduleFrequency.CUSTOM,
                    customUnit = AndroidCustomScheduleUnit.RAW_CRON,
                    rawCron = cronExpression.trim(),
                    timeZone = resolvedTz
                )
            }
            val (minStr, hourStr, domStr, monStr, dowStr) = parts
            if (minStr == "0" && hourStr == "*" && domStr == "*" && monStr == "*" && dowStr == "*") {
                return AndroidScheduleDraft(
                    frequency = AndroidScheduleFrequency.HOURLY,
                    rawCron = cronExpression.trim(),
                    timeZone = resolvedTz
                )
            }
            if (minStr == "0" && hourStr.startsWith("*/") && domStr == "*" && monStr == "*" && dowStr == "*") {
                val step = hourStr.removePrefix("*/").toIntOrNull()
                if (step != null && step in everyHourChoices) {
                    return AndroidScheduleDraft(
                        frequency = AndroidScheduleFrequency.CUSTOM,
                        customUnit = AndroidCustomScheduleUnit.EVERY_HOURS,
                        everyHours = step,
                        rawCron = cronExpression.trim(),
                        timeZone = resolvedTz
                    )
                }
            }
            val m = minStr.toIntOrNull()
            val h = hourStr.toIntOrNull()
            if (m != null && m in 0..59 && h != null && h in 0..23 && monStr == "*") {
                if (domStr == "*") {
                    when {
                        dowStr == "*" -> return AndroidScheduleDraft(
                            frequency = AndroidScheduleFrequency.DAILY,
                            hour = h,
                            minute = m,
                            rawCron = cronExpression.trim(),
                            timeZone = resolvedTz
                        )
                        dowStr == "1-5" -> return AndroidScheduleDraft(
                            frequency = AndroidScheduleFrequency.WEEKDAYS,
                            hour = h,
                            minute = m,
                            rawCron = cronExpression.trim(),
                            timeZone = resolvedTz
                        )
                        dowStr.toIntOrNull() in 0..7 -> {
                            val d = dowStr.toInt()
                            return AndroidScheduleDraft(
                                frequency = AndroidScheduleFrequency.WEEKLY,
                                hour = h,
                                minute = m,
                                weekday = if (d == 7) 0 else d,
                                rawCron = cronExpression.trim(),
                                timeZone = resolvedTz
                            )
                        }
                        dowStr.contains(",") -> {
                            val days = dowStr.split(",")
                                .mapNotNull { it.toIntOrNull() }
                                .filter { it in 0..7 }
                                .map { if (it == 7) 0 else it }
                                .toSet()
                            if (days.isNotEmpty()) {
                                return AndroidScheduleDraft(
                                    frequency = AndroidScheduleFrequency.CUSTOM,
                                    customUnit = AndroidCustomScheduleUnit.MULTI_WEEKDAY,
                                    hour = h,
                                    minute = m,
                                    multiWeekdays = days,
                                    rawCron = cronExpression.trim(),
                                    timeZone = resolvedTz
                                )
                            }
                        }
                    }
                } else if (dowStr == "*") {
                    val dom = domStr.toIntOrNull()
                    if (dom != null && dom in 1..28) {
                        return AndroidScheduleDraft(
                            frequency = AndroidScheduleFrequency.CUSTOM,
                            customUnit = AndroidCustomScheduleUnit.MONTHLY,
                            hour = h,
                            minute = m,
                            dayOfMonth = dom,
                            rawCron = cronExpression.trim(),
                            timeZone = resolvedTz
                        )
                    }
                }
            }
            return AndroidScheduleDraft(
                frequency = AndroidScheduleFrequency.CUSTOM,
                customUnit = AndroidCustomScheduleUnit.RAW_CRON,
                rawCron = cronExpression.trim(),
                timeZone = resolvedTz
            )
        }
    }
}

@Composable
internal fun AutomationScheduleEditorSection(
    draft: AndroidScheduleDraft,
    onDraftChange: (AndroidScheduleDraft) -> Unit
) {
    val colors = AntigravityTheme.colors
    val needsTime = draft.frequency == AndroidScheduleFrequency.DAILY ||
        draft.frequency == AndroidScheduleFrequency.WEEKDAYS ||
        draft.frequency == AndroidScheduleFrequency.WEEKLY ||
        (draft.frequency == AndroidScheduleFrequency.CUSTOM &&
            (draft.customUnit == AndroidCustomScheduleUnit.MULTI_WEEKDAY || draft.customUnit == AndroidCustomScheduleUnit.MONTHLY))

    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        // Frequency selector pills (horizontally scrollable for compact EN/JA/ZH labels)
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            AndroidScheduleFrequency.entries.forEach { freq ->
                val selected = draft.frequency == freq
                Box(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(if (selected) colors.accentIndigo else colors.surfaceVariant)
                        .border(
                            width = if (selected) 0.dp else 0.5.dp,
                            color = if (selected) Color.Transparent else colors.border,
                            shape = RoundedCornerShape(8.dp)
                        )
                        .clickable { onDraftChange(draft.copy(frequency = freq)) }
                        .padding(horizontal = 10.dp, vertical = 7.dp),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        text = freq.localizedLabel(),
                        color = if (selected) Color.White else colors.textPrimary,
                        fontSize = 12.sp,
                        fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
                        maxLines = 1,
                        softWrap = false
                    )
                }
            }
        }

        when (draft.frequency) {
            AndroidScheduleFrequency.HOURLY -> {
                Text(
                    text = "每小时整点自动执行（0 * * * *）".loc,
                    color = colors.textSecondary,
                    fontSize = 12.5.sp
                )
            }
            AndroidScheduleFrequency.WEEKLY -> {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    AndroidScheduleDraft.orderedWeekdays.forEach { d ->
                        val sel = draft.weekday == d
                        Box(
                            modifier = Modifier
                                .weight(1f)
                                .clip(RoundedCornerShape(8.dp))
                                .background(if (sel) colors.accentIndigo else colors.surfaceVariant)
                                .clickable { onDraftChange(draft.copy(weekday = d)) }
                                .padding(vertical = 6.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            Text(
                                text = AutomationCronHelper.shortWeekday(d),
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
            AndroidScheduleFrequency.CUSTOM -> {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    AndroidCustomScheduleUnit.entries.forEach { unit ->
                        val sel = draft.customUnit == unit
                        Box(
                            modifier = Modifier
                                .clip(RoundedCornerShape(8.dp))
                                .background(if (sel) colors.accentIndigo.copy(alpha = 0.16f) else colors.surfaceVariant)
                                .border(
                                    0.5.dp,
                                    if (sel) colors.accentIndigo else colors.border,
                                    RoundedCornerShape(8.dp)
                                )
                                .clickable { onDraftChange(draft.copy(customUnit = unit)) }
                                .padding(horizontal = 9.dp, vertical = 5.dp)
                        ) {
                            Text(
                                text = unit.localizedLabel(),
                                color = if (sel) colors.accentIndigo else colors.textPrimary,
                                fontSize = 11.5.sp,
                                fontWeight = if (sel) FontWeight.SemiBold else FontWeight.Normal,
                                maxLines = 1,
                                softWrap = false
                            )
                        }
                    }
                }

                when (draft.customUnit) {
                    AndroidCustomScheduleUnit.EVERY_HOURS -> {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            AndroidScheduleDraft.everyHourChoices.forEach { step ->
                                val sel = draft.everyHours == step
                                Box(
                                    modifier = Modifier
                                        .weight(1f)
                                        .clip(RoundedCornerShape(8.dp))
                                        .background(if (sel) colors.accentIndigo else colors.surfaceVariant)
                                        .clickable { onDraftChange(draft.copy(everyHours = step)) }
                                        .padding(vertical = 6.dp),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Text(
                                        text = "${step}h",
                                        color = if (sel) Color.White else colors.textPrimary,
                                        fontSize = 12.sp,
                                        fontWeight = FontWeight.SemiBold
                                    )
                                }
                            }
                        }
                    }
                    AndroidCustomScheduleUnit.MULTI_WEEKDAY -> {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            AndroidScheduleDraft.orderedWeekdays.forEach { d ->
                                val sel = d in draft.multiWeekdays
                                Box(
                                    modifier = Modifier
                                        .weight(1f)
                                        .clip(RoundedCornerShape(8.dp))
                                        .background(if (sel) colors.accentIndigo else colors.surfaceVariant)
                                        .clickable {
                                            val next = if (sel && draft.multiWeekdays.size > 1) {
                                                draft.multiWeekdays - d
                                            } else {
                                                draft.multiWeekdays + d
                                            }
                                            onDraftChange(draft.copy(multiWeekdays = next))
                                        }
                                        .padding(vertical = 6.dp),
                                    contentAlignment = Alignment.Center
                                ) {
                                    Text(
                                        text = AutomationCronHelper.shortWeekday(d),
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
                    AndroidCustomScheduleUnit.MONTHLY -> {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Text("每月日期".loc, fontSize = 13.sp, color = colors.textSecondary)
                            IconButton(
                                onClick = { onDraftChange(draft.copy(dayOfMonth = if (draft.dayOfMonth <= 1) 28 else draft.dayOfMonth - 1)) },
                                modifier = Modifier.size(30.dp)
                            ) {
                                Icon(Icons.Default.Remove, contentDescription = null)
                            }
                            Text(
                                text = "每月 %d 日".loc(draft.dayOfMonth),
                                fontSize = 14.sp,
                                fontWeight = FontWeight.SemiBold,
                                color = colors.textPrimary
                            )
                            IconButton(
                                onClick = { onDraftChange(draft.copy(dayOfMonth = if (draft.dayOfMonth >= 28) 1 else draft.dayOfMonth + 1)) },
                                modifier = Modifier.size(30.dp)
                            ) {
                                Icon(Icons.Default.Add, contentDescription = null)
                            }
                        }
                    }
                    AndroidCustomScheduleUnit.RAW_CRON -> {
                        OutlinedTextField(
                            value = draft.rawCron,
                            onValueChange = { onDraftChange(draft.copy(rawCron = it)) },
                            singleLine = true,
                            label = { Text("Cron 表达式".loc) },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }
            }
            else -> Unit
        }

        if (needsTime) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Text("触发时间".loc, fontSize = 13.sp, color = colors.textSecondary)
                Spacer(modifier = Modifier.weight(1f))
                // Hour stepper
                IconButton(
                    onClick = { onDraftChange(draft.copy(hour = (draft.hour + 23) % 24)) },
                    modifier = Modifier.size(28.dp)
                ) {
                    Icon(Icons.Default.Remove, contentDescription = null, modifier = Modifier.size(16.dp))
                }
                Text(
                    text = "%02d".format(draft.hour),
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textPrimary
                )
                IconButton(
                    onClick = { onDraftChange(draft.copy(hour = (draft.hour + 1) % 24)) },
                    modifier = Modifier.size(28.dp)
                ) {
                    Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                }
                Text(":", fontSize = 15.sp, fontWeight = FontWeight.Bold, color = colors.textSecondary)
                // Minute stepper (5-min increments for quick mobile dialing)
                IconButton(
                    onClick = { onDraftChange(draft.copy(minute = (draft.minute + 55) % 60)) },
                    modifier = Modifier.size(28.dp)
                ) {
                    Icon(Icons.Default.Remove, contentDescription = null, modifier = Modifier.size(16.dp))
                }
                Text(
                    text = "%02d".format(draft.minute),
                    fontSize = 15.sp,
                    fontWeight = FontWeight.SemiBold,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textPrimary
                )
                IconButton(
                    onClick = { onDraftChange(draft.copy(minute = (draft.minute + 5) % 60)) },
                    modifier = Modifier.size(28.dp)
                ) {
                    Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                }
            }
        }

        if (draft.timeZone.isNotBlank() && !(draft.frequency == AndroidScheduleFrequency.CUSTOM && draft.customUnit == AndroidCustomScheduleUnit.RAW_CRON)) {
            Text(
                text = "时区：%s".loc(draft.timeZone),
                color = colors.textSecondary,
                fontSize = 11.5.sp
            )
        }
    }
}

// MARK: - Full-Screen Prompt Editor Dialog

@Composable
fun AutomationPromptEditorDialog(
    taskName: String,
    initialPrompt: String,
    onDismiss: () -> Unit,
    onSave: suspend (String) -> Result<Unit>
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    var draft by remember(initialPrompt) { mutableStateOf(initialPrompt) }
    var isSaving by remember { mutableStateOf(false) }
    var saveError by remember { mutableStateOf<String?>(null) }

    val trimmed = draft.trim()
    val isDirty = trimmed != initialPrompt.trim()

    Dialog(
        onDismissRequest = { if (!isSaving) onDismiss() },
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = Modifier
                .fillMaxSize()
                .padding(top = 20.dp),
            shape = RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp),
            color = colors.background
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 8.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    TextButton(enabled = !isSaving, onClick = onDismiss) {
                        Text("取消".loc, color = colors.textSecondary)
                    }
                    Text(
                        text = "编辑执行指令".loc,
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier.weight(1f),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Button(
                        enabled = !isSaving && trimmed.isNotEmpty() && isDirty,
                        onClick = {
                            scope.launch {
                                isSaving = true
                                saveError = null
                                onSave(trimmed)
                                    .onSuccess { onDismiss() }
                                    .onFailure { saveError = it.message ?: "保存失败".loc }
                                isSaving = false
                            }
                        },
                        contentPadding = PaddingValues(horizontal = 14.dp, vertical = 6.dp)
                    ) {
                        Text(if (isSaving) "保存中...".loc else "保存".loc, fontSize = 13.sp)
                    }
                }

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(colors.surface)
                        .padding(horizontal = 16.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        text = taskName,
                        color = colors.textSecondary,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f)
                    )
                    Text(
                        text = "${trimmed.length} ${"字符".loc}",
                        color = colors.textSecondary,
                        fontSize = 11.5.sp,
                        fontFamily = FontFamily.Monospace
                    )
                }

                HorizontalDivider(thickness = 0.5.dp, color = colors.border)

                if (saveError != null) {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .background(Color(0xFFF44336).copy(alpha = 0.1f))
                            .padding(horizontal = 16.dp, vertical = 8.dp)
                    ) {
                        Text(saveError.orEmpty(), color = Color(0xFFF44336), fontSize = 12.sp)
                    }
                }

                OutlinedTextField(
                    value = draft,
                    onValueChange = { draft = it },
                    placeholder = { Text("输入每次触发时交给 Agent 执行的完整指令…".loc, fontSize = 14.sp) },
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(14.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = colors.accentIndigo,
                        unfocusedBorderColor = colors.border
                    )
                )
            }
        }
    }
}

// MARK: - Structured Create Automation Dialog (Desktop `tBd` equivalent)

@Composable
fun CreateAutomationDialog(
    existingIds: Set<String>,
    apiClient: ApiClient,
    prefs: PreferencesManager,
    onCreated: suspend () -> Unit,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()

    var name by remember { mutableStateOf("") }
    var scheduleDraft by remember {
        mutableStateOf(AndroidScheduleDraft(timeZone = TimeZone.getDefault().id))
    }
    var selectedModel by remember { mutableStateOf("") }
    var selectedProjectId by remember { mutableStateOf("") }
    var prompt by remember { mutableStateOf("") }
    var isCreating by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    var modelOptions by remember {
        mutableStateOf(
            prefs.cachedModelCatalog?.models?.takeIf { it.isNotEmpty() } ?: ModelDefaults.fallbackOptions()
        )
    }
    var projects by remember { mutableStateOf<List<ProjectItem>>(emptyList()) }
    var modelMenuExpanded by remember { mutableStateOf(false) }
    var projectMenuExpanded by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        apiClient.getModels(refresh = false).onSuccess { resp ->
            if (resp.models.isNotEmpty()) modelOptions = resp.models
        }
        apiClient.fetchProjects().onSuccess { list ->
            projects = list.filter { !it.isPureChat && !it.rawId.isNullOrBlank() }
        }
    }

    val slug = remember(name) { AutomationSlugHelper.slug(name) }
    val isDuplicate = slug.isNotEmpty() && (slug in existingIds || "schedule-$slug" in existingIds)
    val cronPreview = scheduleDraft.resolvedCron()
    val canSubmit = !isCreating && name.trim().isNotEmpty() && !isDuplicate && cronPreview.isNotEmpty() && prompt.trim().isNotEmpty()

    Dialog(
        onDismissRequest = { if (!isCreating) onDismiss() },
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = Modifier
                .fillMaxSize()
                .padding(top = 20.dp),
            shape = RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp),
            color = colors.background
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 8.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    TextButton(enabled = !isCreating, onClick = onDismiss) {
                        Text("取消".loc, color = colors.textSecondary)
                    }
                    Text(
                        text = "新建定时任务".loc,
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        modifier = Modifier.weight(1f)
                    )
                    Button(
                        enabled = canSubmit,
                        onClick = {
                            scope.launch {
                                isCreating = true
                                errorMessage = null
                                apiClient.createAutomation(
                                    name = name.trim(),
                                    schedule = cronPreview,
                                    prompt = prompt.trim(),
                                    model = selectedModel,
                                    projectId = selectedProjectId
                                ).onSuccess {
                                    onCreated()
                                    onDismiss()
                                }.onFailure { err ->
                                    errorMessage = err.message ?: "创建失败".loc
                                }
                                isCreating = false
                            }
                        },
                        contentPadding = PaddingValues(horizontal = 14.dp, vertical = 6.dp)
                    ) {
                        Text(if (isCreating) "创建中...".loc else "创建".loc, fontSize = 13.sp)
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
                    if (errorMessage != null) {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(Color(0xFFF44336).copy(alpha = 0.1f))
                                .padding(10.dp)
                        ) {
                            Text(errorMessage.orEmpty(), color = Color(0xFFF44336), fontSize = 12.5.sp)
                        }
                    }

                    // Name card
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Text("任务名称".loc, color = colors.textSecondary, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)
                        OutlinedTextField(
                            value = name,
                            onValueChange = { name = it },
                            singleLine = true,
                            placeholder = { Text("例如：晨间 PR 汇总".loc) },
                            isError = isDuplicate,
                            modifier = Modifier.fillMaxWidth()
                        )
                        if (isDuplicate) {
                            Text(
                                text = "已存在同名任务（ID: %s），请更换名称".loc(slug),
                                color = Color(0xFFF44336),
                                fontSize = 11.5.sp
                            )
                        } else if (slug.isNotEmpty()) {
                            Text(
                                text = "ID: $slug",
                                color = colors.textSecondary,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace
                            )
                        }
                    }

                    // Schedule card
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Text("执行频率与时间".loc, color = colors.textSecondary, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)
                        AutomationScheduleEditorSection(draft = scheduleDraft, onDraftChange = { scheduleDraft = it })
                        if (cronPreview.isNotBlank()) {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Icon(Icons.Default.Schedule, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(14.dp))
                                Text(
                                    text = AutomationCronHelper.humanSummary(cronPreview),
                                    color = colors.textPrimary,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.Medium
                                )
                            }
                        }
                    }

                    // Runtime (Model + Project) card
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp)
                    ) {
                        Text("运行环境".loc, color = colors.textSecondary, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)

                        Box(modifier = Modifier.fillMaxWidth()) {
                            val label = if (selectedModel.isBlank()) "默认模型".loc else ModelDefaults.displayName(selectedModel, modelOptions)
                            OutlinedButton(onClick = { modelMenuExpanded = true }, modifier = Modifier.fillMaxWidth()) {
                                Text("${"执行模型".loc}: $label", maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                            DropdownMenu(
                                expanded = modelMenuExpanded,
                                onDismissRequest = { modelMenuExpanded = false },
                                containerColor = colors.surface
                            ) {
                                DropdownMenuItem(
                                    text = { Text("默认模型".loc) },
                                    onClick = { selectedModel = ""; modelMenuExpanded = false }
                                )
                                modelOptions.forEach { opt ->
                                    DropdownMenuItem(
                                        text = { Text(opt.name) },
                                        onClick = { selectedModel = opt.id; modelMenuExpanded = false }
                                    )
                                }
                            }
                        }

                        if (projects.isNotEmpty()) {
                            Box(modifier = Modifier.fillMaxWidth()) {
                                val projLabel = projects.firstOrNull { it.rawId == selectedProjectId }?.displayName ?: "默认 / 不指定".loc
                                OutlinedButton(onClick = { projectMenuExpanded = true }, modifier = Modifier.fillMaxWidth()) {
                                    Text("${"绑定项目".loc}: $projLabel", maxLines = 1, overflow = TextOverflow.Ellipsis)
                                }
                                DropdownMenu(
                                    expanded = projectMenuExpanded,
                                    onDismissRequest = { projectMenuExpanded = false },
                                    containerColor = colors.surface
                                ) {
                                    DropdownMenuItem(
                                        text = { Text("默认 / 不指定".loc) },
                                        onClick = { selectedProjectId = ""; projectMenuExpanded = false }
                                    )
                                    projects.forEach { proj ->
                                        val pid = proj.rawId ?: return@forEach
                                        DropdownMenuItem(
                                            text = { Text(proj.displayName) },
                                            onClick = { selectedProjectId = pid; projectMenuExpanded = false }
                                        )
                                    }
                                }
                            }
                        }
                    }

                    // Prompt card
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .padding(14.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Text("执行指令".loc, color = colors.textSecondary, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold)
                        OutlinedTextField(
                            value = prompt,
                            onValueChange = { prompt = it },
                            minLines = 5,
                            maxLines = 14,
                            placeholder = { Text("描述每次触发时 Agent 需要完成的任务、数据源和输出格式…".loc) },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }

                    Spacer(modifier = Modifier.height(24.dp))
                }
            }
        }
    }
}

// MARK: - Automation Detail Dialog (Desktop `TAd` equivalent)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AutomationDetailDialog(
    sidecar: AutomationSidecarItem,
    allEvents: List<AutomationEventItem>,
    apiClient: ApiClient,
    prefs: PreferencesManager,
    initialEditPrompt: Boolean = false,
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
    var projects by remember { mutableStateOf<List<ProjectItem>>(emptyList()) }

    LaunchedEffect(Unit) {
        apiClient.getModels(refresh = false).onSuccess { resp ->
            if (resp.models.isNotEmpty()) modelOptions = resp.models
        }
        apiClient.fetchProjects().onSuccess { list ->
            projects = list.filter { !it.isPureChat && !it.rawId.isNullOrBlank() }
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

    // Schedule / Model / Project inline editor
    var isEditingSchedule by remember { mutableStateOf(false) }
    var scheduleDraft by remember { mutableStateOf(AndroidScheduleDraft()) }
    var draftModel by remember { mutableStateOf(sidecar.model ?: "") }
    var draftProjectId by remember { mutableStateOf(sidecar.projectId ?: "") }
    var modelMenuExpanded by remember { mutableStateOf(false) }
    var projectMenuExpanded by remember { mutableStateOf(false) }

    // Prompt view + full-screen editor dialog
    var isPromptExpanded by remember { mutableStateOf(false) }
    var showPromptEditor by remember { mutableStateOf(initialEditPrompt) }

    // Logs dialog (supports multi-file selector)
    var showLogsDialog by remember { mutableStateOf(false) }
    var logsLoading by remember { mutableStateOf(false) }
    var logsText by remember { mutableStateOf("") }
    var logsError by remember { mutableStateOf<String?>(null) }
    var logsTruncated by remember { mutableStateOf(false) }
    var logFiles by remember { mutableStateOf<List<String>>(emptyList()) }
    var selectedLogFile by remember { mutableStateOf("sidecar.log") }

    fun beginEditSchedule() {
        scheduleDraft = AndroidScheduleDraft.from(
            cronExpression = sidecar.cronExpression ?: "0 9 * * *",
            fallbackTimeZone = sidecar.timeZone
        )
        draftModel = sidecar.model ?: ""
        draftProjectId = sidecar.projectId ?: ""
        isEditingSchedule = true
    }

    fun loadLogs(file: String = selectedLogFile) {
        scope.launch {
            logsLoading = true
            logsError = null
            apiClient.fetchAutomationLogs(sid, tail = 200, file = file)
                .onSuccess { res ->
                    logsText = res.logs
                    logsTruncated = res.truncated
                    if (res.files.isNotEmpty()) logFiles = res.files
                    if (res.filename.isNotBlank()) selectedLogFile = res.filename
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
    val boundProjectDisplay = remember(sidecar.projectName, sidecar.projectId, sidecar.workspaceUri, projects) {
        sidecar.projectName?.takeIf { it.isNotBlank() }
            ?: sidecar.projectId?.let { pid -> projects.firstOrNull { it.rawId == pid }?.displayName }
            ?: sidecar.workspaceUri?.takeIf { it.isNotBlank() }?.let { uri ->
                val cleaned = uri.removePrefix("file://").trimEnd('/')
                cleaned.substringAfterLast('/').ifBlank { cleaned }
            }
    }
    val readsSummary = remember(sidecar.prompt, sidecar.accessGrants) {
        val (sources, readOnly) = AutomationDataSourceExtractor.extract(
            sidecar.prompt.orEmpty(),
            sidecar.accessGrants
        )
        if (sources.isEmpty()) null else {
            val joined = sources.joinToString(", ")
            if (readOnly) "$joined (${"只读".loc})" else joined
        }
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
                                text = { Text("编辑执行指令".loc) },
                                leadingIcon = { Icon(Icons.Default.Edit, contentDescription = null) },
                                onClick = {
                                    overflowExpanded = false
                                    showPromptEditor = true
                                }
                            )
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
                                    selectedLogFile = "sidecar.log"
                                    showLogsDialog = true
                                    loadLogs("sidecar.log")
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
                    // Header Card
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
                                Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(15.dp))
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
                                Text(sidecar.lastError, color = Color(0xFFF44336), fontSize = 12.sp)
                            }
                        }
                    }

                    // Schedule, Model & Project Pipeline Card
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(14.dp))
                            .background(colors.surface)
                            .border(0.5.dp, colors.border, RoundedCornerShape(14.dp))
                            .let { mod -> if (!isEditingSchedule) mod.clickable { beginEditSchedule() } else mod }
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
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(3.dp),
                                    modifier = Modifier.clickable { beginEditSchedule() }
                                ) {
                                    Icon(Icons.Default.Edit, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(13.dp))
                                    Text(
                                        text = "编辑".loc,
                                        color = colors.accentIndigo,
                                        fontSize = 12.5.sp,
                                        fontWeight = FontWeight.SemiBold
                                    )
                                }
                            }
                        }

                        if (isEditingSchedule) {
                            AutomationScheduleEditorSection(
                                draft = scheduleDraft,
                                onDraftChange = { scheduleDraft = it }
                            )

                            HorizontalDivider(thickness = 0.5.dp, color = colors.border)

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
                                        onClick = { draftModel = ""; modelMenuExpanded = false }
                                    )
                                    modelOptions.forEach { opt ->
                                        DropdownMenuItem(
                                            text = { Text(opt.name) },
                                            onClick = { draftModel = opt.id; modelMenuExpanded = false }
                                        )
                                    }
                                }
                            }

                            if (projects.isNotEmpty()) {
                                Box(modifier = Modifier.fillMaxWidth()) {
                                    val projLabel = projects.firstOrNull { it.rawId == draftProjectId }?.displayName
                                        ?: (if (draftProjectId.isNotBlank()) boundProjectDisplay ?: draftProjectId else "默认 / 不指定".loc)
                                    OutlinedButton(
                                        onClick = { projectMenuExpanded = true },
                                        modifier = Modifier.fillMaxWidth()
                                    ) {
                                        Text("${"绑定项目".loc}: $projLabel", maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    }
                                    DropdownMenu(
                                        expanded = projectMenuExpanded,
                                        onDismissRequest = { projectMenuExpanded = false },
                                        containerColor = colors.surface
                                    ) {
                                        DropdownMenuItem(
                                            text = { Text("默认 / 不指定".loc) },
                                            onClick = { draftProjectId = ""; projectMenuExpanded = false }
                                        )
                                        projects.forEach { proj ->
                                            val pid = proj.rawId ?: return@forEach
                                            DropdownMenuItem(
                                                text = { Text(proj.displayName) },
                                                onClick = { draftProjectId = pid; projectMenuExpanded = false }
                                            )
                                        }
                                    }
                                }
                            }

                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Button(
                                    enabled = !isBusy && scheduleDraft.resolvedCron().isNotEmpty(),
                                    onClick = {
                                        scope.launch {
                                            isBusy = true
                                            val origProject = sidecar.projectId ?: ""
                                            val changedProject = if (draftProjectId != origProject) draftProjectId else null
                                            apiClient.updateAutomation(
                                                sidecarId = sid,
                                                cron = scheduleDraft.resolvedCron(),
                                                model = draftModel,
                                                projectId = changedProject
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
                            if (boundProjectDisplay != null) {
                                DetailInfoRow("绑定工作区".loc, boundProjectDisplay)
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

                    // Instructions / Prompt Section (Markdown rendered + collapsible + full-screen sheet editor)
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
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(3.dp),
                                modifier = Modifier
                                    .clip(RoundedCornerShape(6.dp))
                                    .clickable { showPromptEditor = true }
                                    .padding(horizontal = 4.dp, vertical = 2.dp)
                            ) {
                                Icon(Icons.Default.Edit, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(13.dp))
                                Text(
                                    text = "编辑".loc,
                                    color = colors.accentIndigo,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.SemiBold
                                )
                            }
                        }

                        if (!sidecar.description.isNullOrBlank()) {
                            Text(
                                text = sidecar.description,
                                color = colors.textSecondary,
                                fontSize = 12.5.sp,
                                lineHeight = 18.sp
                            )
                            HorizontalDivider(thickness = 0.5.dp, color = colors.border.copy(alpha = 0.6f))
                        }

                        if (readsSummary != null) {
                            Text(
                                text = "读取范围：%s".loc(readsSummary),
                                color = colors.textSecondary,
                                fontSize = 12.sp
                            )
                        }

                        val promptText = sidecar.prompt?.trim().orEmpty()
                        if (promptText.isEmpty()) {
                            Text(
                                text = "暂无执行指令，点击编写".loc,
                                color = colors.accentIndigo,
                                fontSize = 13.5.sp,
                                modifier = Modifier.clickable { showPromptEditor = true }
                            )
                        } else {
                            val isLong = promptText.length > 480
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .let { mod -> if (isLong && !isPromptExpanded) mod.heightIn(max = 200.dp).clip(RoundedCornerShape(6.dp)) else mod }
                            ) {
                                MarkdownContentView(content = promptText)
                                if (isLong && !isPromptExpanded) {
                                    Box(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .height(44.dp)
                                            .align(Alignment.BottomCenter)
                                            .background(
                                                Brush.verticalGradient(
                                                    colors = listOf(colors.surface.copy(alpha = 0f), colors.surface)
                                                )
                                            )
                                    )
                                }
                            }
                            if (isLong) {
                                Text(
                                    text = if (isPromptExpanded) "收起".loc else "展开全部".loc,
                                    color = colors.accentIndigo,
                                    fontSize = 12.5.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    modifier = Modifier.clickable { isPromptExpanded = !isPromptExpanded }
                                )
                            }
                        }
                    }

                    // Run History Section (Desktop `qzd` live runStatus + conversationTitle)
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
                                val runState = ev.effectiveRunStatus
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
                                                    onSelectConversation?.invoke(cid, ev.conversationTitle?.takeIf { it.isNotBlank() } ?: sidecar.name.ifBlank { sid })
                                                }
                                            }
                                        }
                                        .padding(vertical = 6.dp),
                                    verticalAlignment = Alignment.Top,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    when (runState) {
                                        "running" -> CircularProgressIndicator(
                                            modifier = Modifier.size(15.dp).padding(top = 2.dp),
                                            strokeWidth = 1.8.dp,
                                            color = colors.accentIndigo
                                        )
                                        "needs_input" -> Box(
                                            modifier = Modifier
                                                .padding(top = 4.dp)
                                                .size(10.dp)
                                                .clip(CircleShape)
                                                .background(Color(0xFFFF9800))
                                        )
                                        "failed" -> Icon(
                                            imageVector = Icons.Default.Error,
                                            contentDescription = null,
                                            tint = Color(0xFFF44336),
                                            modifier = Modifier.size(16.dp).padding(top = 2.dp)
                                        )
                                        else -> Icon(
                                            imageVector = Icons.Default.CheckCircle,
                                            contentDescription = null,
                                            tint = colors.accentGreen,
                                            modifier = Modifier.size(16.dp).padding(top = 2.dp)
                                        )
                                    }

                                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                        Row(
                                            verticalAlignment = Alignment.CenterVertically,
                                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                                        ) {
                                            Text(
                                                text = ev.timestamp,
                                                color = colors.textPrimary,
                                                fontSize = 12.sp,
                                                fontFamily = FontFamily.Monospace,
                                                fontWeight = FontWeight.Medium
                                            )
                                            if (runState == "running" || runState == "needs_input") {
                                                val tagColor = if (runState == "needs_input") Color(0xFFFF9800) else colors.accentIndigo
                                                Box(
                                                    modifier = Modifier
                                                        .clip(CircleShape)
                                                        .background(tagColor.copy(alpha = 0.14f))
                                                        .padding(horizontal = 6.dp, vertical = 1.dp)
                                                ) {
                                                    Text(
                                                        text = if (runState == "needs_input") "等待确认".loc else "运行中".loc,
                                                        color = tagColor,
                                                        fontSize = 10.sp,
                                                        fontWeight = FontWeight.SemiBold
                                                    )
                                                }
                                            }
                                        }
                                        val summaryText = when {
                                            runState == "failed" -> ev.message?.takeIf { it.isNotBlank() } ?: "执行失败".loc
                                            !ev.conversationTitle.isNullOrBlank() &&
                                                !ev.conversationTitle.equals(sidecar.name.trim(), ignoreCase = true) -> ev.conversationTitle
                                            else -> ev.message
                                        }
                                        if (!summaryText.isNullOrBlank()) {
                                            Text(
                                                text = summaryText,
                                                color = if (runState == "failed") Color(0xFFF44336) else colors.textSecondary,
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

    if (showPromptEditor) {
        AutomationPromptEditorDialog(
            taskName = sidecar.name.ifBlank { sid },
            initialPrompt = sidecar.prompt ?: "",
            onDismiss = { showPromptEditor = false },
            onSave = { updated ->
                apiClient.updateAutomation(sid, prompt = updated).onSuccess {
                    onRefresh()
                    Toast.makeText(context, "执行指令已保存".loc, Toast.LENGTH_SHORT).show()
                }
            }
        )
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
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (logFiles.size > 1) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .horizontalScroll(rememberScrollState()),
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            logFiles.forEach { file ->
                                val active = file == selectedLogFile
                                Box(
                                    modifier = Modifier
                                        .clip(CircleShape)
                                        .background(if (active) colors.accentIndigo.copy(alpha = 0.16f) else colors.surfaceVariant)
                                        .clickable {
                                            selectedLogFile = file
                                            loadLogs(file)
                                        }
                                        .padding(horizontal = 10.dp, vertical = 4.dp)
                                ) {
                                    Text(
                                        text = file,
                                        fontFamily = FontFamily.Monospace,
                                        fontSize = 11.sp,
                                        color = if (active) colors.accentIndigo else colors.textSecondary
                                    )
                                }
                            }
                        }
                    }
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
