package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.HelpOutline
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Difference
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.InteractionOption
import com.antigravity.mobile.data.model.InteractionQuestion
import com.antigravity.mobile.data.model.PendingInteraction
import com.antigravity.mobile.data.model.QuestionResponse
import com.antigravity.mobile.ui.theme.AntigravityTheme

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun InteractionCard(
    interaction: PendingInteraction,
    onSubmit: (optionId: String, writeInText: String?, questionResponses: List<QuestionResponse>?) -> Unit,
    onSkip: (questionResponses: List<QuestionResponse>?) -> Unit,
    onToggleExpand: ((Boolean) -> Unit)? = null,
    maxAvailableHeight: Dp? = null,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()

    // 动态工作流专有安全审批卡片 (Antigravity 2.32+)
    if (interaction.type == "workflow_approval" || interaction.workflowApproval != null) {
        WorkflowApprovalCard(
            interaction = interaction,
            isSubmitting = false,
            onSubmit = { optId, writeIn ->
                onSubmit(optId, writeIn, null)
            },
            modifier = modifier
        )
        return
    }

    val questions = interaction.questions
    val hasMultipleQuestions = questions != null && questions.size > 1
    val isPermissionType = interaction.type == "permission" || interaction.type == "file_permission"
    val isSingleMultiSelect = !hasMultipleQuestions &&
        (interaction.isMultiSelect || (questions?.firstOrNull()?.isMultiSelect == true))
    val interactionKey = "${interaction.trajectoryId}:${interaction.stepIndex}:${interaction.title.hashCode()}"

    // State for single question (single-select)
    var selectedOptionId by remember(interactionKey) {
        mutableStateOf(
            interaction.defaultOptionId
                ?: interaction.options.firstOrNull()?.id
                ?: if (interaction.hasWriteIn) "__write_in__" else "1"
        )
    }
    var singleWriteInText by remember(interactionKey) { mutableStateOf("") }

    // State for single question (multi-select: starts empty to match Antigravity desktop)
    val singleMultiSelectedIds = remember(interactionKey) { mutableStateListOf<String>() }
    var singleWriteInActive by remember(interactionKey) { mutableStateOf(false) }

    // State for multiple questions: single-select optionId, multi-select optionIds, writeIn active & text
    val multiSelections = remember(interactionKey) {
        val map = mutableStateMapOf<Int, String>()
        questions?.forEachIndexed { idx, q ->
            if (!q.isMultiSelect) {
                map[idx] = q.defaultOptionId
                    ?: q.options.firstOrNull()?.id
                    ?: if (q.hasWriteIn) "__write_in__" else "1"
            }
        }
        map
    }
    val multiQuestionMultiSelections = remember(interactionKey) {
        mutableStateMapOf<Int, Set<String>>()
    }
    val multiWriteInActives = remember(interactionKey) {
        mutableStateMapOf<Int, Boolean>()
    }
    val multiWriteIns = remember(interactionKey) {
        mutableStateMapOf<Int, String>()
    }

    val singleMultiSelectionCount = if (isSingleMultiSelect) {
        singleMultiSelectedIds.size + if (singleWriteInActive && singleWriteInText.isNotBlank()) 1 else 0
    } else 0

    val canSubmit = when {
        hasMultipleQuestions -> {
            questions?.withIndex()?.all { (idx, q) ->
                if (!q.isMultiSelect) {
                    true
                } else {
                    val hasOpts = !multiQuestionMultiSelections[idx].isNullOrEmpty()
                    val hasWriteIn = (multiWriteInActives[idx] == true) && !(multiWriteIns[idx].isNullOrBlank())
                    hasOpts || hasWriteIn
                }
            } ?: true
        }
        isSingleMultiSelect -> singleMultiSelectionCount > 0
        else -> true
    }

    val submitButtonLabel = if (isSingleMultiSelect && singleMultiSelectionCount > 0) {
        "${"提交".loc} ($singleMultiSelectionCount)"
    } else {
        "提交".loc
    }

    val pg = interaction.permissionGrant
    val preview = interaction.approvalPreview
    val isHighRisk = preview?.riskLevel == "high"
    val isMediumRisk = preview?.riskLevel == "medium" && !preview.riskReasons.isNullOrEmpty()
    val riskSummaryText: String? = if (!isHighRisk && !isMediumRisk) {
        null
    } else {
        val labels = (preview?.riskReasons ?: emptyList()).map { code ->
            when (code) {
                "destructive_delete" -> "破坏性删除".loc
                "force_git" -> "Git 强推/硬重置".loc
                "privilege_exec" -> "提权执行".loc
                "remote_pipe_exec" -> "远程脚本执行".loc
                "destructive_sql" -> "高危 SQL 删除".loc
                "system_ops" -> "系统级操作".loc
                "sensitive_path" -> "敏感凭证路径".loc
                "outside_workspace" -> "工作区外路径".loc
                "overwrite_file" -> "覆盖写入".loc
                "external_write" -> "外部写入操作".loc
                "git_push" -> "推送远端".loc
                else -> code
            }
        }
        if (isHighRisk) {
            val prefix = "高危操作".loc
            if (labels.isEmpty()) prefix else "$prefix · ${labels.joinToString(" · ")}"
        } else {
            val prefix = "注意".loc
            "$prefix · ${labels.joinToString(" · ")}"
        }
    }
    val diffLines = remember(preview?.diffPreview) {
        val raw = preview?.diffPreview
        if (raw.isNullOrEmpty()) emptyList() else raw.split("\n")
    }

    val headerIcon = when {
        isHighRisk -> Icons.Default.Warning
        pg != null -> Icons.Default.Lock
        interaction.type == "browser_download" -> Icons.Default.FileDownload
        interaction.type == "approval" -> Icons.Default.CheckCircle
        interaction.type == "mcp" -> Icons.Default.Build
        interaction.type == "elicitation" -> Icons.AutoMirrored.Filled.HelpOutline
        interaction.type == "permission" || interaction.type == "file_permission" -> Icons.Default.Lock
        interaction.type == "ask_question" -> Icons.AutoMirrored.Filled.HelpOutline
        interaction.type == "run_command" -> Icons.Default.Terminal
        else -> Icons.Default.Warning
    }

    val headerColor = when {
        isHighRisk -> colors.accentRed
        pg != null -> colors.accentOrange
        interaction.type == "browser_download" -> colors.accentBlue
        interaction.type == "approval" -> colors.accentGreen
        interaction.type == "mcp" -> colors.accentBlue
        interaction.type == "elicitation" -> colors.accentBlue
        interaction.type == "permission" || interaction.type == "file_permission" -> colors.accentBlue
        interaction.type == "ask_question" -> colors.accentOrange
        interaction.type == "run_command" -> colors.accentOrange
        else -> colors.accentOrange
    }

    val singleQuestionTitle = interaction.title.ifBlank { interaction.prompt ?: "需要用户审批操作".loc }
    val isLongSingleQuestion = !hasMultipleQuestions && (singleQuestionTitle.contains("\n") || singleQuestionTitle.length > 48)
    val compactCategoryTitle = when {
        pg != null -> "权限确认".loc
        interaction.type == "browser_download" -> "下载确认".loc
        interaction.type == "approval" -> "操作确认".loc
        interaction.type == "mcp" -> "MCP 工具确认".loc
        interaction.type == "elicitation" -> "补充信息".loc
        interaction.type == "permission" || interaction.type == "file_permission" -> "权限确认".loc
        interaction.type == "ask_question" -> "询问用户".loc
        interaction.type == "run_command" -> "执行命令".loc
        else -> "需要确认".loc
    }

    // 默认自适应高度，只有超出半屏才截断至半屏，支持一键扩展到会话全屏（严格不超过可用高度，避免遮挡顶栏或挤没底部按钮）
    var isExpanded by remember(interactionKey) { mutableStateOf(false) }
    var isDiffExpanded by remember(interactionKey) { mutableStateOf(false) }
    val screenHeight = LocalConfiguration.current.screenHeightDp.dp
    val effectiveAvailable = maxAvailableHeight ?: (screenHeight * 0.72f)
    val maxAllowedHeight = (effectiveAvailable - 16.dp).coerceAtLeast(200.dp)
    val halfHeight = (effectiveAvailable * 0.56f).coerceIn(240.dp, 420.dp).coerceAtMost(maxAllowedHeight)
    val fullHeight = maxAllowedHeight.coerceAtLeast(halfHeight)

    val scrollState = rememberScrollState()
    LaunchedEffect(interactionKey) {
        scrollState.scrollTo(0)
    }
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    var pendingAutoFocusKey by remember(interactionKey) { mutableStateOf<String?>(null) }
    val dismissCardKeyboard = {
        pendingAutoFocusKey = null
        focusManager.clearFocus()
        keyboardController?.hide()
    }
    LaunchedEffect(scrollState.isScrollInProgress) {
        if (scrollState.isScrollInProgress) {
            dismissCardKeyboard()
        }
    }
    val canExpand = isExpanded || (scrollState.maxValue > 0 && fullHeight > halfHeight + 24.dp)

    val arrowRotation by animateFloatAsState(
        targetValue = if (isExpanded) 180f else 0f,
        animationSpec = spring(
            dampingRatio = 0.82f,
            stiffness = Spring.StiffnessMediumLow
        ),
        label = "interactionArrowRotation"
    )

    Card(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(max = if (isExpanded) fullHeight else halfHeight)
            .animateContentSize(
                animationSpec = spring(
                    dampingRatio = 0.82f,
                    stiffness = Spring.StiffnessMediumLow
                )
            )
            .shadow(
                elevation = 2.5.dp,
                shape = RoundedCornerShape(16.dp),
                ambientColor = Color.Black.copy(alpha = 0.04f),
                spotColor = Color.Black.copy(alpha = 0.08f)
            ),
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = colors.surface),
        border = CardDefaults.outlinedCardBorder().copy(brush = SolidColor(headerColor.copy(alpha = 0.35f)))
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            // Header: Compact pinned header + Expand/Collapse Pill (长问题正文移入统一滚动区，保证顶栏与底部按钮永远不被挤出)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .then(
                        if (canExpand) {
                            Modifier.clickable {
                                dismissCardKeyboard()
                                haptic.light()
                                isExpanded = !isExpanded
                                onToggleExpand?.invoke(isExpanded)
                            }
                        } else Modifier
                    ),
                verticalAlignment = if (hasMultipleQuestions || isLongSingleQuestion) Alignment.CenterVertically else Alignment.Top,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    modifier = Modifier.weight(1f),
                    verticalAlignment = if (hasMultipleQuestions || isLongSingleQuestion) Alignment.CenterVertically else Alignment.Top,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Icon(
                        imageVector = headerIcon,
                        contentDescription = null,
                        tint = headerColor,
                        modifier = Modifier
                            .padding(top = if (hasMultipleQuestions || isLongSingleQuestion) 0.dp else 2.dp)
                            .size(20.dp)
                    )
                    when {
                        hasMultipleQuestions -> {
                            Text(
                                text = "需要确认规格 (${questions?.size} 个问题)".loc,
                                color = colors.textPrimary,
                                fontSize = 14.sp,
                                fontWeight = FontWeight.Bold,
                                lineHeight = 19.sp,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                        isLongSingleQuestion -> {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                Text(
                                    text = compactCategoryTitle,
                                    color = colors.textPrimary,
                                    fontSize = 14.sp,
                                    fontWeight = FontWeight.Bold,
                                    lineHeight = 19.sp,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                if (isSingleMultiSelect) {
                                    MultiSelectBadge()
                                }
                            }
                        }
                        isSingleMultiSelect -> {
                            FlowRow(
                                horizontalArrangement = Arrangement.spacedBy(6.dp),
                                verticalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Text(
                                    text = singleQuestionTitle,
                                    color = colors.textPrimary,
                                    fontSize = 14.sp,
                                    fontWeight = FontWeight.Bold,
                                    lineHeight = 19.sp,
                                    maxLines = 3,
                                    overflow = TextOverflow.Ellipsis
                                )
                                MultiSelectBadge()
                            }
                        }
                        else -> {
                            Text(
                                text = singleQuestionTitle,
                                color = colors.textPrimary,
                                fontSize = 14.sp,
                                fontWeight = FontWeight.Bold,
                                lineHeight = 19.sp,
                                maxLines = 3,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                }

                if (canExpand) {
                    Spacer(modifier = Modifier.width(6.dp))
                    // 一键全屏 / 半屏切换胶囊
                    Row(
                        modifier = Modifier
                            .clip(RoundedCornerShape(50))
                            .background(colors.surfaceVariant.copy(alpha = 0.7f))
                            .padding(horizontal = 9.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(3.dp)
                    ) {
                        Text(
                            text = if (isExpanded) "收起".loc else "展开".loc,
                            color = colors.textSecondary,
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.Medium
                        )
                        Icon(
                            imageVector = Icons.Default.KeyboardArrowUp,
                            contentDescription = if (isExpanded) "收起为半屏".loc else "一键全屏".loc,
                            tint = colors.textSecondary,
                            modifier = Modifier
                                .size(16.dp)
                                .graphicsLayer(rotationZ = arrowRotation)
                        )
                    }
                }
            }

            if (isLongSingleQuestion) {
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.45f),
                    thickness = 0.5.dp
                )
            }

            // Unified Scrollable Content Area: Long Question Text + Target/Permission Box + Options
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f, fill = false)
                    .verticalScroll(scrollState),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (isLongSingleQuestion) {
                    SelectionContainer {
                        Text(
                            text = singleQuestionTitle,
                            color = colors.textPrimary,
                            fontSize = 13.5.sp,
                            fontWeight = FontWeight.Medium,
                            lineHeight = 20.sp,
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }

                // Risk summary 1-line banner (high risk in red, medium risk in subtle amber; silent for low risk)
                if (!riskSummaryText.isNullOrBlank()) {
                    val accent = if (isHighRisk) colors.accentRed else colors.accentOrange
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(8.dp))
                            .background(accent.copy(alpha = if (isHighRisk) 0.12f else 0.10f))
                            .border(0.75.dp, accent.copy(alpha = if (isHighRisk) 0.35f else 0.25f), RoundedCornerShape(8.dp))
                            .padding(horizontal = 10.dp, vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Icon(
                            imageVector = if (isHighRisk) Icons.Default.Warning else Icons.Default.ErrorOutline,
                            contentDescription = null,
                            tint = accent,
                            modifier = Modifier.size(14.dp)
                        )
                        Text(
                            text = riskSummaryText,
                            color = accent,
                            fontSize = 11.5.sp,
                            fontWeight = if (isHighRisk) FontWeight.SemiBold else FontWeight.Medium,
                            lineHeight = 16.sp,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }

                // Structured Permission Grant Preview (Antigravity 2.26+)
                if (pg != null) {
                    val badgeAccent = if (isHighRisk) colors.accentRed else colors.accentOrange
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.surfaceVariant.copy(alpha = 0.7f))
                            .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                            .padding(10.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .clip(RoundedCornerShape(4.dp))
                                    .background(badgeAccent.copy(alpha = 0.2f))
                                    .padding(horizontal = 6.dp, vertical = 2.dp)
                            ) {
                                Text(
                                    text = pg.binary.uppercase(),
                                    color = badgeAccent,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Bold
                                )
                            }

                            Text(
                                text = "操作: ${pg.action}".loc,
                                color = colors.textPrimary,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Medium
                            )
                        }

                        if (!pg.repo.isNullOrBlank() || !pg.org.isNullOrBlank()) {
                            val repoText = if (!pg.org.isNullOrBlank() && !pg.repo.isNullOrBlank()) "${pg.org}/${pg.repo}" else (pg.repo ?: "")
                            Text(
                                text = "仓库: $repoText".loc,
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace
                            )
                        }

                        if (!pg.branch.isNullOrBlank()) {
                            Text(
                                text = "目标分支: ${pg.branch}".loc,
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace
                            )
                        }
                    }
                } else {
                    // Target / Command Monospace Preview Box
                    val displayTarget = if (!preview?.displayPath.isNullOrBlank() && interaction.type != "run_command") {
                        preview?.displayPath
                    } else {
                        interaction.target ?: interaction.command
                    }
                    val cwdText = preview?.cwd
                    if (!displayTarget.isNullOrBlank() || !cwdText.isNullOrBlank()) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.surfaceVariant)
                                .padding(horizontal = 10.dp, vertical = 7.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            if (!displayTarget.isNullOrBlank()) {
                                val styledTarget = remember(displayTarget, isHighRisk, colors) {
                                    androidx.compose.ui.text.buildAnnotatedString {
                                        val slash = if (!displayTarget.contains('\n')) displayTarget.lastIndexOf('/') else -1
                                        if (slash in 0 until displayTarget.length - 1) {
                                            pushStyle(androidx.compose.ui.text.SpanStyle(
                                                color = if (isHighRisk) colors.textPrimary else colors.textSecondary,
                                                fontWeight = FontWeight.Normal
                                            ))
                                            append(displayTarget.substring(0, slash + 1))
                                            pop()
                                            pushStyle(androidx.compose.ui.text.SpanStyle(
                                                color = colors.textPrimary,
                                                fontWeight = FontWeight.Bold
                                            ))
                                            append(displayTarget.substring(slash + 1))
                                            pop()
                                        } else {
                                            pushStyle(androidx.compose.ui.text.SpanStyle(
                                                color = colors.textPrimary,
                                                fontWeight = FontWeight.SemiBold
                                            ))
                                            append(displayTarget)
                                            pop()
                                        }
                                    }
                                }
                                Text(
                                    text = styledTarget,
                                    fontSize = 11.5.sp,
                                    fontFamily = FontFamily.Monospace,
                                    maxLines = if (isExpanded) 12 else 4,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                            if (!cwdText.isNullOrBlank()) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(
                                        imageVector = Icons.Default.Folder,
                                        contentDescription = null,
                                        tint = colors.textSecondary.copy(alpha = 0.7f),
                                        modifier = Modifier.size(12.dp)
                                    )
                                    Text(
                                        text = cwdText,
                                        color = colors.textSecondary.copy(alpha = 0.75f),
                                        fontSize = 10.5.sp,
                                        fontFamily = FontFamily.Monospace,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                }
                            }
                        }
                    }
                }

                // Compact Collapsible Diff Preview (Option A: collapsed by default)
                if (diffLines.isNotEmpty()) {
                    val added = preview?.addedLines ?: 0
                    val removed = preview?.removedLines ?: 0
                    val isNew = (preview?.isNewFile == true) && removed == 0
                    val truncated = preview?.diffTruncated == true

                    Column(
                        modifier = Modifier.fillMaxWidth(),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.surfaceVariant.copy(alpha = 0.75f))
                                .clickable {
                                    haptic.light()
                                    isDiffExpanded = !isDiffExpanded
                                }
                                .padding(horizontal = 10.dp, vertical = 6.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Icon(
                                imageVector = Icons.Default.Difference,
                                contentDescription = null,
                                tint = colors.textSecondary,
                                modifier = Modifier.size(14.dp)
                            )
                            Text(
                                text = if (isDiffExpanded) "收起改动".loc else "预览改动".loc,
                                color = colors.textPrimary,
                                fontSize = 11.5.sp,
                                fontWeight = FontWeight.Medium
                            )
                            if (isNew) {
                                Box(
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(4.dp))
                                        .background(colors.accentBlue.copy(alpha = 0.15f))
                                        .padding(horizontal = 5.dp, vertical = 1.dp)
                                ) {
                                    Text(
                                        text = "新建".loc,
                                        color = colors.accentBlue,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold
                                    )
                                }
                            }
                            if (added > 0) {
                                Text(
                                    text = "+$added",
                                    color = colors.accentGreen,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                            if (removed > 0) {
                                Text(
                                    text = "-$removed",
                                    color = colors.accentRed,
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                            Spacer(modifier = Modifier.weight(1f))
                            Icon(
                                imageVector = if (isDiffExpanded) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                                contentDescription = null,
                                tint = colors.textSecondary,
                                modifier = Modifier.size(16.dp)
                            )
                        }

                        AnimatedVisibility(
                            visible = isDiffExpanded,
                            enter = expandVertically() + fadeIn(),
                            exit = shrinkVertically() + fadeOut()
                        ) {
                            val diffVertScroll = rememberScrollState()
                            val diffHorizScroll = rememberScrollState()
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .heightIn(max = 180.dp)
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(colors.background.copy(alpha = 0.85f))
                                    .border(0.75.dp, colors.border.copy(alpha = 0.6f), RoundedCornerShape(8.dp))
                                    .verticalScroll(diffVertScroll)
                                    .horizontalScroll(diffHorizScroll)
                                    .padding(vertical = 6.dp)
                            ) {
                                Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
                                    diffLines.forEach { line ->
                                        val isAdded = line.startsWith("+ ")
                                        val isRemoved = line.startsWith("- ")
                                        val isHunk = line == "@@"
                                        val fg = when {
                                            isAdded -> colors.accentGreen
                                            isRemoved -> colors.accentRed
                                            isHunk -> colors.textSecondary.copy(alpha = 0.6f)
                                            else -> colors.textSecondary
                                        }
                                        val bg = when {
                                            isAdded -> colors.accentGreen.copy(alpha = 0.11f)
                                            isRemoved -> colors.accentRed.copy(alpha = 0.11f)
                                            else -> Color.Transparent
                                        }
                                        Text(
                                            text = line.ifEmpty { " " },
                                            color = fg,
                                            fontSize = 11.sp,
                                            fontFamily = FontFamily.Monospace,
                                            lineHeight = 15.sp,
                                            softWrap = false,
                                            modifier = Modifier
                                                .background(bg)
                                                .padding(horizontal = 8.dp, vertical = 1.dp)
                                        )
                                    }
                                    if (truncated) {
                                        Text(
                                            text = "仅展示前 40 行改动".loc,
                                            color = colors.textSecondary.copy(alpha = 0.7f),
                                            fontSize = 10.5.sp,
                                            modifier = Modifier.padding(start = 8.dp, end = 8.dp, top = 4.dp)
                                        )
                                    }
                                }
                            }
                        }
                    }
                }

                if (hasMultipleQuestions) {
                    questions?.forEachIndexed { qIdx, q ->
                        val currentSelected = multiSelections[qIdx] ?: q.defaultOptionId ?: "1"
                        val currentMultiSet = multiQuestionMultiSelections[qIdx] ?: emptySet()
                        val currentWriteInActive = multiWriteInActives[qIdx] == true
                        QuestionSection(
                            questionIndex = qIdx,
                            question = q,
                            selectedOptionId = currentSelected,
                            selectedOptionIds = currentMultiSet,
                            writeInActive = currentWriteInActive,
                            onSelectOption = { optId, isWriteInOpt ->
                                haptic.light()
                                if (q.isMultiSelect) {
                                    if (isWriteInOpt) {
                                        if (!currentWriteInActive) {
                                            multiWriteInActives[qIdx] = true
                                            pendingAutoFocusKey = "multi:$qIdx"
                                        }
                                    } else {
                                        val next = currentMultiSet.toMutableSet()
                                        if (next.contains(optId)) next.remove(optId) else next.add(optId)
                                        multiQuestionMultiSelections[qIdx] = next
                                        dismissCardKeyboard()
                                    }
                                } else {
                                    val wasSelected = currentSelected == optId
                                    multiSelections[qIdx] = optId
                                    if (isWriteInOpt) {
                                        if (!wasSelected) {
                                            pendingAutoFocusKey = "multi:$qIdx"
                                        }
                                    } else {
                                        dismissCardKeyboard()
                                    }
                                }
                            },
                            onToggleWriteInOff = {
                                haptic.light()
                                multiWriteInActives[qIdx] = false
                                dismissCardKeyboard()
                            },
                            onHeaderTap = { dismissCardKeyboard() },
                            pendingAutoFocusKey = pendingAutoFocusKey,
                            onConsumeAutoFocus = {
                                if (pendingAutoFocusKey == "multi:$qIdx") {
                                    pendingAutoFocusKey = null
                                }
                            },
                            writeInText = multiWriteIns[qIdx] ?: "",
                            onWriteInChange = { multiWriteIns[qIdx] = it }
                        )
                    }
                } else {
                    val singleHasWriteIn = interaction.hasWriteIn && interaction.options.none { it.id == "__write_in__" || it.isDeny }
                    val displayOptions = remember(interaction) {
                        if (singleHasWriteIn) {
                            val label = resolveWriteInLabel(interaction.writeInLabel)
                            interaction.options + InteractionOption(id = "__write_in__", text = label)
                        } else {
                            interaction.options
                        }
                    }

                    if (displayOptions.isNotEmpty()) {
                        Column(
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            displayOptions.forEachIndexed { optIdx, opt ->
                                val isWriteIn = isOptionWriteIn(opt, interaction.hasWriteIn, isPermissionType)
                                val isSelected = if (isSingleMultiSelect) {
                                    if (isWriteIn) singleWriteInActive else singleMultiSelectedIds.contains(opt.id)
                                } else {
                                    selectedOptionId == opt.id
                                }
                                val placeholderText = resolveWriteInPlaceholder(
                                    interaction.writeInPlaceholder,
                                    isDeny = opt.isDeny || (isPermissionType && opt.id == "5")
                                )
                                OptionRow(
                                    option = opt,
                                    isSelected = isSelected,
                                    isMultiSelect = isSingleMultiSelect,
                                    onClick = {
                                        if (isSingleMultiSelect && isWriteIn && isSelected) {
                                            pendingAutoFocusKey = "single"
                                            return@OptionRow
                                        }
                                        haptic.light()
                                        if (isSingleMultiSelect) {
                                            if (isWriteIn) {
                                                singleWriteInActive = true
                                                pendingAutoFocusKey = "single"
                                            } else {
                                                if (singleMultiSelectedIds.contains(opt.id)) {
                                                    singleMultiSelectedIds.remove(opt.id)
                                                } else {
                                                    singleMultiSelectedIds.add(opt.id)
                                                }
                                                dismissCardKeyboard()
                                            }
                                        } else {
                                            val wasSelected = isSelected
                                            selectedOptionId = opt.id
                                            if (isWriteIn) {
                                                if (!wasSelected) {
                                                    pendingAutoFocusKey = "single"
                                                }
                                            } else {
                                                dismissCardKeyboard()
                                            }
                                        }
                                    },
                                    onToggleWriteInOff = if (isSingleMultiSelect && isWriteIn && isSelected) {
                                        {
                                            haptic.light()
                                            singleWriteInActive = false
                                            dismissCardKeyboard()
                                        }
                                    } else null,
                                    displayIndex = optIdx,
                                    isWriteIn = isWriteIn,
                                    shouldAutoFocus = pendingAutoFocusKey == "single",
                                    onConsumeAutoFocus = {
                                        if (pendingAutoFocusKey == "single") {
                                            pendingAutoFocusKey = null
                                        }
                                    },
                                    writeInText = singleWriteInText,
                                    onWriteInChange = { singleWriteInText = it },
                                    placeholderText = placeholderText
                                )
                            }
                        }
                    }
                }
            }

            if (scrollState.maxValue > 0) {
                HorizontalDivider(
                    color = colors.separator.copy(alpha = 0.45f),
                    thickness = 0.5.dp
                )
            }

            // Bottom Action Bar: Skip & Submit (Always pinned at the bottom)
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                OutlinedButton(
                    onClick = {
                        dismissCardKeyboard()
                        haptic.medium()
                        if (hasMultipleQuestions) {
                            val responses = questions?.mapIndexed { idx, _ ->
                                QuestionResponse(questionIndex = idx, selectedOptionIds = emptyList(), writeInResponse = "", skipped = true)
                            }
                            onSkip(responses)
                        } else {
                            onSkip(null)
                        }
                    },
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.outlinedButtonColors(
                        contentColor = colors.textSecondary
                    ),
                    shape = RoundedCornerShape(10.dp)
                ) {
                    Text("跳过".loc)
                }

                Button(
                    onClick = {
                        if (!canSubmit) return@Button
                        dismissCardKeyboard()
                        haptic.heavy()
                        if (hasMultipleQuestions) {
                            val responses = questions?.mapIndexed { idx, q ->
                                if (q.isMultiSelect) {
                                    val selectedSet = multiQuestionMultiSelections[idx] ?: emptySet()
                                    val orderedIds = q.options.map { it.id }.filter { selectedSet.contains(it) }
                                    val writeIn = if (multiWriteInActives[idx] == true) {
                                        (multiWriteIns[idx] ?: "").trim()
                                    } else ""
                                    QuestionResponse(
                                        questionIndex = idx,
                                        selectedOptionIds = orderedIds,
                                        writeInResponse = writeIn.ifBlank { null },
                                        skipped = false
                                    )
                                } else {
                                    val optId = multiSelections[idx] ?: q.defaultOptionId ?: q.options.firstOrNull()?.id ?: "1"
                                    val isWriteIn = optId == "__write_in__" || (q.options.find { it.id == optId }?.isDeny == true)
                                    val writeIn = if (isWriteIn) (multiWriteIns[idx] ?: "").trim() else ""
                                    QuestionResponse(
                                        questionIndex = idx,
                                        selectedOptionIds = if (isWriteIn) emptyList() else listOf(optId),
                                        writeInResponse = writeIn.ifBlank { null },
                                        skipped = false
                                    )
                                }
                            }
                            val primaryOptId = responses?.firstOrNull()?.selectedOptionIds?.firstOrNull() ?: ""
                            onSubmit(primaryOptId, null, responses)
                        } else if (isSingleMultiSelect) {
                            val orderedIds = interaction.options.map { it.id }.filter { singleMultiSelectedIds.contains(it) }
                            val trimmedWriteIn = if (singleWriteInActive) singleWriteInText.trim() else ""
                            val response = QuestionResponse(
                                questionIndex = 0,
                                selectedOptionIds = orderedIds,
                                writeInResponse = trimmedWriteIn.ifBlank { null },
                                skipped = false
                            )
                            val primaryOptId = orderedIds.firstOrNull() ?: if (trimmedWriteIn.isNotEmpty()) "__write_in__" else ""
                            onSubmit(primaryOptId, trimmedWriteIn.ifBlank { null }, listOf(response))
                        } else {
                            val selectedOpt = interaction.options.find { it.id == selectedOptionId }
                            val isWriteInOrDeny = selectedOptionId == "__write_in__" ||
                                (selectedOpt?.isDeny == true) ||
                                (isPermissionType && selectedOptionId == "5")
                            onSubmit(selectedOptionId, if (isWriteInOrDeny) singleWriteInText.ifBlank { null } else null, null)
                        }
                    },
                    enabled = canSubmit,
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = colors.accentBlue,
                        contentColor = Color.White,
                        disabledContainerColor = colors.accentBlue.copy(alpha = 0.38f),
                        disabledContentColor = Color.White.copy(alpha = 0.78f)
                    ),
                    shape = RoundedCornerShape(10.dp)
                ) {
                    Text(submitButtonLabel)
                }
            }
        }
    }
}

@Composable
private fun MultiSelectBadge() {
    val colors = AntigravityTheme.colors
    Box(
        modifier = Modifier
            .clip(RoundedCornerShape(50))
            .background(colors.accentBlue.copy(alpha = 0.12f))
            .padding(horizontal = 7.dp, vertical = 2.dp),
        contentAlignment = Alignment.Center
    ) {
        Text(
            text = "多选".loc,
            color = colors.accentBlue,
            fontSize = 11.sp,
            fontWeight = FontWeight.SemiBold,
            lineHeight = 14.sp,
            maxLines = 1
        )
    }
}

private fun isOptionWriteIn(
    opt: InteractionOption,
    hasWriteIn: Boolean,
    isPermissionType: Boolean
): Boolean {
    if (opt.id == "__write_in__") return true
    if (!hasWriteIn) return false
    if (opt.isDeny) return true
    if (isPermissionType && opt.id == "5") return true
    return false
}

@Composable
private fun QuestionSection(
    questionIndex: Int,
    question: InteractionQuestion,
    selectedOptionId: String,
    selectedOptionIds: Set<String> = emptySet(),
    writeInActive: Boolean = false,
    onSelectOption: (String, Boolean) -> Unit,
    onToggleWriteInOff: () -> Unit = {},
    onHeaderTap: () -> Unit = {},
    pendingAutoFocusKey: String? = null,
    onConsumeAutoFocus: () -> Unit = {},
    writeInText: String,
    onWriteInChange: (String) -> Unit
) {
    val colors = AntigravityTheme.colors
    val isMulti = question.isMultiSelect

    val hasWriteIn = question.hasWriteIn && question.options.none { it.id == "__write_in__" || it.isDeny }
    val displayOptions = remember(question) {
        if (hasWriteIn) {
            val label = resolveWriteInLabel(question.writeInLabel)
            question.options + InteractionOption(id = "__write_in__", text = label)
        } else {
            question.options
        }
    }

    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        // Question title with Q[index] badge + optional Multi-select pill
        Row(
            verticalAlignment = Alignment.Top,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onHeaderTap)
        ) {
            Box(
                modifier = Modifier
                    .clip(RoundedCornerShape(4.dp))
                    .background(colors.accentOrange)
                    .padding(horizontal = 5.dp, vertical = 2.dp)
            ) {
                Text(
                    text = "Q${questionIndex + 1}",
                    color = Color.White,
                    fontSize = 10.sp,
                    fontWeight = FontWeight.Bold,
                    fontFamily = FontFamily.Monospace
                )
            }

            Text(
                text = question.question,
                color = colors.textPrimary,
                fontSize = 12.5.sp,
                fontWeight = FontWeight.SemiBold,
                modifier = Modifier.weight(1f)
            )

            if (isMulti) {
                MultiSelectBadge()
            }
        }

        // Options for this question
        Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
            displayOptions.forEachIndexed { optIdx, opt ->
                val isWriteIn = isOptionWriteIn(opt, question.hasWriteIn, isPermissionType = false)
                val isSelected = if (isMulti) {
                    if (isWriteIn) writeInActive else selectedOptionIds.contains(opt.id)
                } else {
                    selectedOptionId == opt.id
                }
                val placeholderText = resolveWriteInPlaceholder(question.writeInPlaceholder, isDeny = false)
                OptionRow(
                    option = opt,
                    isSelected = isSelected,
                    isMultiSelect = isMulti,
                    onClick = { onSelectOption(opt.id, isWriteIn) },
                    onToggleWriteInOff = if (isMulti && isWriteIn && isSelected) onToggleWriteInOff else null,
                    displayIndex = optIdx,
                    isWriteIn = isWriteIn,
                    shouldAutoFocus = pendingAutoFocusKey == "multi:$questionIndex",
                    onConsumeAutoFocus = onConsumeAutoFocus,
                    writeInText = writeInText,
                    onWriteInChange = onWriteInChange,
                    placeholderText = placeholderText
                )
            }
        }
    }
}

@Composable
private fun OptionRow(
    option: InteractionOption,
    isSelected: Boolean,
    isMultiSelect: Boolean = false,
    onClick: () -> Unit,
    onToggleWriteInOff: (() -> Unit)? = null,
    displayIndex: Int = -1,
    isWriteIn: Boolean = false,
    shouldAutoFocus: Boolean = false,
    onConsumeAutoFocus: () -> Unit = {},
    writeInText: String = "",
    onWriteInChange: (String) -> Unit = {},
    placeholderText: String = "输入自定义回答...".loc
) {
    val colors = AntigravityTheme.colors
    val focusRequester = remember { FocusRequester() }

    LaunchedEffect(isSelected, isWriteIn, shouldAutoFocus) {
        if (isSelected && isWriteIn && shouldAutoFocus) {
            onConsumeAutoFocus()
            focusRequester.requestFocus()
        }
    }

    Row(
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(if (isSelected) colors.accentBlue.copy(alpha = 0.08f) else colors.surfaceVariant.copy(alpha = 0.5f))
            .border(
                width = 1.dp,
                color = if (isSelected) colors.accentBlue.copy(alpha = 0.5f) else Color.Transparent,
                shape = RoundedCornerShape(8.dp)
            )
            .clickable(onClick = onClick)
            .padding(horizontal = 9.dp, vertical = 8.dp)
    ) {
        val badgeText = when {
            option.id == "__write_in__" -> if (displayIndex >= 0) "${displayIndex + 1}" else "✎"
            option.id.length <= 3 -> option.id
            else -> option.id.take(2)
        }

        // Number badge [1], [2], etc.
        Box(
            modifier = Modifier
                .padding(top = 1.dp)
                .size(20.dp)
                .clip(RoundedCornerShape(5.dp))
                .background(if (isSelected) colors.accentBlue else colors.surfaceVariant)
                .then(if (onToggleWriteInOff != null) Modifier.clickable(onClick = onToggleWriteInOff) else Modifier),
            contentAlignment = Alignment.Center
        ) {
            Text(
                text = badgeText,
                color = if (isSelected) Color.White else colors.textSecondary,
                fontSize = if (badgeText == "✎") 12.sp else 11.sp,
                fontWeight = FontWeight.Bold,
                fontFamily = FontFamily.Monospace
            )
        }

        // Multi-line Option Text or Inline TextField when writeIn is selected
        if (isWriteIn && isSelected) {
            BasicTextField(
                value = writeInText,
                onValueChange = onWriteInChange,
                textStyle = TextStyle(
                    color = colors.textPrimary,
                    fontSize = 12.5.sp,
                    fontWeight = FontWeight.Normal,
                    lineHeight = 17.sp
                ),
                cursorBrush = SolidColor(colors.accentBlue),
                modifier = Modifier
                    .weight(1f)
                    .padding(vertical = 1.dp)
                    .focusRequester(focusRequester),
                decorationBox = { innerTextField ->
                    if (writeInText.isEmpty()) {
                        Text(
                            text = placeholderText,
                            color = colors.textSecondary.copy(alpha = 0.7f),
                            fontSize = 12.5.sp,
                            fontWeight = FontWeight.Normal,
                            lineHeight = 17.sp
                        )
                    }
                    innerTextField()
                }
            )
        } else {
            Text(
                text = if (isWriteIn && writeInText.isNotBlank()) writeInText else option.text,
                color = if (isSelected) colors.textPrimary else colors.textSecondary,
                fontSize = 12.5.sp,
                fontWeight = if (isSelected) FontWeight.Medium else FontWeight.Normal,
                lineHeight = 17.sp,
                modifier = Modifier
                    .weight(1f)
                    .padding(vertical = 1.dp)
            )
        }

        // Checkbox (multi-select) or Radio (single-select) indicator
        if (isMultiSelect) {
            Box(
                modifier = Modifier
                    .padding(top = 1.5.dp)
                    .size(17.dp)
                    .clip(RoundedCornerShape(4.dp))
                    .background(if (isSelected) colors.accentBlue else Color.Transparent)
                    .border(
                        width = 1.5.dp,
                        color = if (isSelected) colors.accentBlue else colors.textSecondary.copy(alpha = 0.45f),
                        shape = RoundedCornerShape(4.dp)
                    )
                    .then(if (onToggleWriteInOff != null) Modifier.clickable(onClick = onToggleWriteInOff) else Modifier),
                contentAlignment = Alignment.Center
            ) {
                if (isSelected) {
                    Icon(
                        imageVector = Icons.Default.Check,
                        contentDescription = null,
                        tint = Color.White,
                        modifier = Modifier.size(12.dp)
                    )
                }
            }
        } else {
            Box(
                modifier = Modifier
                    .padding(top = 2.dp)
                    .size(16.dp)
                    .clip(CircleShape)
                    .border(
                        width = 1.5.dp,
                        color = if (isSelected) colors.accentBlue else colors.textSecondary.copy(alpha = 0.4f),
                        shape = CircleShape
                    ),
                contentAlignment = Alignment.Center
            ) {
                if (isSelected) {
                    Box(
                        modifier = Modifier
                            .size(8.dp)
                            .clip(CircleShape)
                            .background(colors.accentBlue)
                    )
                }
            }
        }
    }
}

/**
 * Backward compatibility overload for legacy approve/reject call sites.
 */
@Composable
fun InteractionCard(
    interaction: PendingInteraction,
    onApprove: () -> Unit,
    onReject: () -> Unit,
    modifier: Modifier = Modifier
) {
    InteractionCard(
        interaction = interaction,
        onSubmit = { optId, _, _ ->
            if (optId == "5" || optId == "__write_in__") {
                onReject()
            } else {
                onApprove()
            }
        },
        onSkip = { onReject() },
        modifier = modifier
    )
}

@Composable
fun ProceedBanner(
    onProceed: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors

    Card(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(14.dp)),
        colors = CardDefaults.cardColors(
            containerColor = colors.accentBlue.copy(alpha = 0.12f)
        ),
        border = CardDefaults.outlinedCardBorder().copy(brush = androidx.compose.ui.graphics.SolidColor(colors.accentBlue.copy(alpha = 0.3f))),
        shape = RoundedCornerShape(14.dp)
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(14.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Column(modifier = Modifier.weight(1f).padding(end = 8.dp)) {
                Text(
                    text = "📋 实施方案已就绪".loc,
                    color = colors.accentBlue,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold
                )
                Text(
                    text = "点击 Proceed 开始自动化代码实施".loc,
                    color = colors.textSecondary,
                    fontSize = 12.sp
                )
            }

            Button(
                onClick = onProceed,
                colors = ButtonDefaults.buttonColors(
                    containerColor = colors.accentBlue,
                    contentColor = Color.White
                ),
                shape = RoundedCornerShape(10.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.PlayArrow,
                    contentDescription = "Proceed",
                    modifier = Modifier.size(16.dp)
                )
                Spacer(modifier = Modifier.width(4.dp))
                Text("Proceed")
            }
        }
    }
}

private fun resolveWriteInLabel(raw: String?): String {
    if (raw.isNullOrBlank()) return "自定义输入".loc
    val lower = raw.trim().lowercase()
    return when {
        lower == "other" || lower == "write in" || lower == "write in..." -> "自定义输入".loc
        else -> raw
    }
}

private fun resolveWriteInPlaceholder(raw: String?, isDeny: Boolean): String {
    if (raw.isNullOrBlank()) {
        return if (isDeny) "告诉 Agent 应该怎么做...".loc else "输入自定义回答...".loc
    }
    val lower = raw.trim().lowercase()
    return when {
        lower.contains("write in your response") || lower.contains("write in") -> "输入自定义回答...".loc
        lower.contains("tell the agent") || lower.contains("what to do instead") -> "告诉 Agent 应该怎么做...".loc
        else -> raw
    }
}
