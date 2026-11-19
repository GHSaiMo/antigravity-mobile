package com.antigravity.mobile.ui.components

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
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.FileDownload
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

    // State for single question
    var selectedOptionId by remember(interaction) {
        mutableStateOf(
            interaction.defaultOptionId
                ?: interaction.options.firstOrNull()?.id
                ?: if (interaction.hasWriteIn) "__write_in__" else "1"
        )
    }
    var singleWriteInText by remember(interaction) { mutableStateOf("") }

    // State for multiple questions: questionIndex -> optionId
    val multiSelections = remember(interaction) {
        val map = mutableStateMapOf<Int, String>()
        questions?.forEachIndexed { idx, q ->
            map[idx] = q.defaultOptionId
                ?: q.options.firstOrNull()?.id
                ?: if (q.hasWriteIn) "__write_in__" else "1"
        }
        map
    }
    val multiWriteIns = remember(interaction) {
        mutableStateMapOf<Int, String>()
    }

    val pg = interaction.permissionGrant
    val headerIcon = when {
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

    val singleQuestionTitle = interaction.title.ifBlank { interaction.prompt ?: "需要用户审批操作" }
    val isLongSingleQuestion = !hasMultipleQuestions && (singleQuestionTitle.contains("\n") || singleQuestionTitle.length > 48)
    val compactCategoryTitle = when {
        pg != null -> "权限确认"
        interaction.type == "browser_download" -> "下载确认"
        interaction.type == "approval" -> "操作确认"
        interaction.type == "mcp" -> "MCP 工具确认"
        interaction.type == "elicitation" -> "补充信息"
        interaction.type == "permission" || interaction.type == "file_permission" -> "权限确认"
        interaction.type == "ask_question" -> "询问用户"
        interaction.type == "run_command" -> "执行命令"
        else -> "需要确认"
    }

    // 默认自适应高度，只有超出半屏才截断至半屏，支持一键扩展到会话全屏（严格不超过可用高度，避免遮挡顶栏或挤没底部按钮）
    val interactionKey = "${interaction.trajectoryId}:${interaction.stepIndex}:${interaction.title.hashCode()}"
    var isExpanded by remember(interactionKey) { mutableStateOf(false) }
    val screenHeight = LocalConfiguration.current.screenHeightDp.dp
    val effectiveAvailable = maxAvailableHeight ?: (screenHeight * 0.72f)
    val maxAllowedHeight = (effectiveAvailable - 16.dp).coerceAtLeast(200.dp)
    val halfHeight = (effectiveAvailable * 0.56f).coerceIn(240.dp, 420.dp).coerceAtMost(maxAllowedHeight)
    val fullHeight = maxAllowedHeight.coerceAtLeast(halfHeight)

    val scrollState = rememberScrollState()
    LaunchedEffect(interactionKey) {
        scrollState.scrollTo(0)
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
                    Text(
                        text = when {
                            hasMultipleQuestions -> "需要确认规格 (${questions?.size} 个问题)"
                            isLongSingleQuestion -> compactCategoryTitle
                            else -> singleQuestionTitle
                        },
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Bold,
                        lineHeight = 19.sp,
                        maxLines = if (hasMultipleQuestions || isLongSingleQuestion) 1 else 3,
                        overflow = TextOverflow.Ellipsis
                    )
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
                            text = if (isExpanded) "收起" else "展开",
                            color = colors.textSecondary,
                            fontSize = 11.5.sp,
                            fontWeight = FontWeight.Medium
                        )
                        Icon(
                            imageVector = Icons.Default.KeyboardArrowUp,
                            contentDescription = if (isExpanded) "收起为半屏" else "一键全屏",
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

                // Structured Permission Grant Preview (Antigravity 2.26+)
                if (pg != null) {
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
                                    .background(colors.accentOrange.copy(alpha = 0.2f))
                                    .padding(horizontal = 6.dp, vertical = 2.dp)
                            ) {
                                Text(
                                    text = pg.binary.uppercase(),
                                    color = colors.accentOrange,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Bold
                                )
                            }

                            Text(
                                text = "操作: ${pg.action}",
                                color = colors.textPrimary,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.Medium
                            )
                        }

                        if (!pg.repo.isNullOrBlank() || !pg.org.isNullOrBlank()) {
                            val repoText = if (!pg.org.isNullOrBlank() && !pg.repo.isNullOrBlank()) "${pg.org}/${pg.repo}" else (pg.repo ?: "")
                            Text(
                                text = "仓库: $repoText",
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace
                            )
                        }

                        if (!pg.branch.isNullOrBlank()) {
                            Text(
                                text = "目标分支: ${pg.branch}",
                                color = colors.textSecondary,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace
                            )
                        }
                    }
                } else {
                    // Target / Command Monospace Preview Box
                    val targetText = interaction.target ?: interaction.command
                    if (!targetText.isNullOrBlank()) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(colors.surfaceVariant)
                                .padding(8.dp)
                        ) {
                            Text(
                                text = targetText,
                                color = colors.textPrimary,
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace,
                                maxLines = if (isExpanded) 12 else 4,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                }

                if (hasMultipleQuestions) {
                    questions?.forEachIndexed { qIdx, q ->
                        QuestionSection(
                            questionIndex = qIdx,
                            question = q,
                            selectedOptionId = multiSelections[qIdx] ?: q.defaultOptionId ?: "1",
                            onSelectOption = { optId ->
                                haptic.light()
                                multiSelections[qIdx] = optId
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
                                val isSelected = selectedOptionId == opt.id
                                val isWriteIn = opt.id == "__write_in__" || (interaction.hasWriteIn && (opt.isDeny || opt.id == "5"))
                                val placeholderText = resolveWriteInPlaceholder(
                                    interaction.writeInPlaceholder,
                                    isDeny = opt.isDeny || opt.id == "5"
                                )
                                OptionRow(
                                    option = opt,
                                    isSelected = isSelected,
                                    onClick = {
                                        haptic.light()
                                        selectedOptionId = opt.id
                                    },
                                    displayIndex = optIdx,
                                    isWriteIn = isWriteIn,
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
                    Text("跳过")
                }

                Button(
                    onClick = {
                        haptic.heavy()
                        if (hasMultipleQuestions) {
                            val responses = questions?.mapIndexed { idx, q ->
                                val optId = multiSelections[idx] ?: q.defaultOptionId ?: q.options.firstOrNull()?.id ?: "1"
                                val writeIn = multiWriteIns[idx] ?: ""
                                QuestionResponse(
                                    questionIndex = idx,
                                    selectedOptionIds = listOf(optId),
                                    writeInResponse = writeIn,
                                    skipped = false
                                )
                            }
                            onSubmit(selectedOptionId, null, responses)
                        } else {
                            onSubmit(selectedOptionId, singleWriteInText.ifBlank { null }, null)
                        }
                    },
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = colors.accentBlue,
                        contentColor = Color.White
                    ),
                    shape = RoundedCornerShape(10.dp)
                ) {
                    Text("提交")
                }
            }
        }
    }
}

@Composable
private fun QuestionSection(
    questionIndex: Int,
    question: InteractionQuestion,
    selectedOptionId: String,
    onSelectOption: (String) -> Unit,
    writeInText: String,
    onWriteInChange: (String) -> Unit
) {
    val colors = AntigravityTheme.colors

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
        // Question title with Q[index] badge
        Row(
            verticalAlignment = Alignment.Top,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
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
        }

        // Options for this question
        Column(verticalArrangement = Arrangement.spacedBy(5.dp)) {
            displayOptions.forEachIndexed { optIdx, opt ->
                val isSelected = selectedOptionId == opt.id
                val isWriteIn = opt.id == "__write_in__" || (question.hasWriteIn && (opt.isDeny || opt.id == "5" || opt.id.equals("other", ignoreCase = true)))
                val placeholderText = resolveWriteInPlaceholder(question.writeInPlaceholder, isDeny = false)
                OptionRow(
                    option = opt,
                    isSelected = isSelected,
                    onClick = { onSelectOption(opt.id) },
                    displayIndex = optIdx,
                    isWriteIn = isWriteIn,
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
    onClick: () -> Unit,
    displayIndex: Int = -1,
    isWriteIn: Boolean = false,
    writeInText: String = "",
    onWriteInChange: (String) -> Unit = {},
    placeholderText: String = "输入自定义回答..."
) {
    val colors = AntigravityTheme.colors
    val focusRequester = remember { FocusRequester() }

    LaunchedEffect(isSelected, isWriteIn) {
        if (isSelected && isWriteIn) {
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
                .background(if (isSelected) colors.accentBlue else colors.surfaceVariant),
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

        // Radio indicator
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
                    text = "📋 实施方案已就绪",
                    color = colors.accentBlue,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.Bold
                )
                Text(
                    text = "点击 Proceed 开始自动化代码实施",
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
    if (raw.isNullOrBlank()) return "自定义输入"
    val lower = raw.trim().lowercase()
    return when {
        lower == "other" || lower == "write in" || lower == "write in..." -> "自定义输入"
        else -> raw
    }
}

private fun resolveWriteInPlaceholder(raw: String?, isDeny: Boolean): String {
    if (raw.isNullOrBlank()) {
        return if (isDeny) "告诉 Agent 应该怎么做..." else "输入自定义回答..."
    }
    val lower = raw.trim().lowercase()
    return when {
        lower.contains("write in your response") || lower.contains("write in") -> "输入自定义回答..."
        lower.contains("tell the agent") || lower.contains("what to do instead") -> "告诉 Agent 应该怎么做..."
        else -> raw
    }
}
