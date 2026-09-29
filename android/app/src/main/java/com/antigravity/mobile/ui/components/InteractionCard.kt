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
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.HelpOutline
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
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
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()

    val questions = interaction.questions
    val hasMultipleQuestions = questions != null && questions.size > 1

    // State for single question
    var selectedOptionId by remember(interaction) {
        mutableStateOf(interaction.defaultOptionId ?: interaction.options.firstOrNull()?.id ?: "1")
    }
    var singleWriteInText by remember(interaction) { mutableStateOf("") }

    // State for multiple questions: questionIndex -> optionId
    val multiSelections = remember(interaction) {
        val map = mutableStateMapOf<Int, String>()
        questions?.forEachIndexed { idx, q ->
            map[idx] = q.defaultOptionId ?: q.options.firstOrNull()?.id ?: "1"
        }
        map
    }
    val multiWriteIns = remember(interaction) {
        mutableStateMapOf<Int, String>()
    }

    val headerIcon = when (interaction.type) {
        "permission", "file_permission" -> Icons.Default.Lock
        "ask_question" -> Icons.AutoMirrored.Filled.HelpOutline
        "run_command" -> Icons.Default.Terminal
        else -> Icons.Default.Warning
    }

    val headerColor = when (interaction.type) {
        "permission", "file_permission" -> colors.accentBlue
        "ask_question" -> colors.accentOrange
        "run_command" -> colors.accentOrange
        else -> colors.accentOrange
    }

    // 默认自适应高度，只有超出半屏才截断至半屏，支持一键扩展到会话全屏
    var isExpanded by remember { mutableStateOf(false) }
    val screenHeight = LocalConfiguration.current.screenHeightDp.dp
    val halfHeight = (screenHeight * 0.46f).coerceIn(280.dp, 400.dp)
    val fullHeight = (screenHeight * 0.72f).coerceAtLeast(halfHeight)

    val scrollState = rememberScrollState()
    val canExpand = isExpanded || scrollState.maxValue > 0 || hasMultipleQuestions

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
            // Header: Icon + Full Multi-line Question Title + Expand/Collapse Button (if expandable)
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
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    modifier = Modifier.weight(1f),
                    verticalAlignment = Alignment.Top,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Icon(
                        imageVector = headerIcon,
                        contentDescription = null,
                        tint = headerColor,
                        modifier = Modifier
                            .padding(top = 2.dp)
                            .size(20.dp)
                    )
                    Text(
                        text = if (hasMultipleQuestions) {
                            "需要确认规格 (${questions?.size} 个问题)"
                        } else {
                            interaction.title.ifBlank { interaction.prompt ?: "需要用户审批操作" }
                        },
                        color = colors.textPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Bold,
                        lineHeight = 19.sp
                    )
                }

                if (canExpand) {
                    // 一键全屏 / 半屏切换按钮
                    Box(
                        modifier = Modifier
                            .size(28.dp)
                            .clip(CircleShape)
                            .background(colors.surfaceVariant.copy(alpha = 0.7f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.KeyboardArrowUp,
                            contentDescription = if (isExpanded) "收起为半屏" else "一键全屏",
                            tint = colors.textSecondary,
                            modifier = Modifier
                                .size(20.dp)
                                .graphicsLayer(rotationZ = arrowRotation)
                        )
                    }
                }
            }

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
                        maxLines = if (isExpanded) 8 else 3
                    )
                }
            }

            // Question Options Area (Adaptive height, scrollable when exceeding max height)
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f, fill = false)
                    .verticalScroll(scrollState),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
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
                    val options = interaction.options
                    if (options.isNotEmpty()) {
                        Column(
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            options.forEach { opt ->
                                val isSelected = selectedOptionId == opt.id
                                OptionRow(
                                    option = opt,
                                    isSelected = isSelected,
                                    onClick = {
                                        haptic.light()
                                        selectedOptionId = opt.id
                                    }
                                )
                            }
                        }

                        val selectedOpt = options.find { it.id == selectedOptionId }
                        val isDenyOrOther = selectedOpt?.isDeny == true || selectedOptionId == "5" || selectedOptionId == "__write_in__"
                        AnimatedVisibility(
                            visible = (interaction.hasWriteIn || isDenyOrOther) && isDenyOrOther,
                            enter = fadeIn() + expandVertically(),
                            exit = fadeOut() + shrinkVertically()
                        ) {
                            OutlinedTextField(
                                value = singleWriteInText,
                                onValueChange = { singleWriteInText = it },
                                placeholder = {
                                    Text(
                                        interaction.writeInPlaceholder ?: "(告诉 Agent 应该怎么做)",
                                        fontSize = 12.sp
                                    )
                                },
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(top = 4.dp),
                                shape = RoundedCornerShape(8.dp),
                                singleLine = false,
                                maxLines = 3
                            )
                        }
                    }
                }
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
                    Text("Skip (跳过)")
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
                    Text("Submit ↵ (提交)")
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
            question.options.forEach { opt ->
                OptionRow(
                    option = opt,
                    isSelected = selectedOptionId == opt.id,
                    onClick = { onSelectOption(opt.id) }
                )
            }
        }

        // Inline write-in if deny / other
        val isDenyOrOther = selectedOptionId == "5" || selectedOptionId == "__write_in__" || selectedOptionId.equals("other", ignoreCase = true)
        if (question.hasWriteIn && isDenyOrOther) {
            OutlinedTextField(
                value = writeInText,
                onValueChange = onWriteInChange,
                placeholder = {
                    Text(
                        question.writeInPlaceholder ?: "(输入自定义说明)",
                        fontSize = 11.5.sp
                    )
                },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 2.dp),
                shape = RoundedCornerShape(7.dp),
                singleLine = false,
                maxLines = 2
            )
        }
    }
}

@Composable
private fun OptionRow(
    option: InteractionOption,
    isSelected: Boolean,
    onClick: () -> Unit
) {
    val colors = AntigravityTheme.colors

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
                text = option.id,
                color = if (isSelected) Color.White else colors.textSecondary,
                fontSize = 11.sp,
                fontWeight = FontWeight.Bold,
                fontFamily = FontFamily.Monospace
            )
        }

        // Multi-line Option Text - Fully visible without horizontal scrolling
        Text(
            text = option.text,
            color = if (isSelected) colors.textPrimary else colors.textSecondary,
            fontSize = 12.5.sp,
            fontWeight = if (isSelected) FontWeight.Medium else FontWeight.Normal,
            lineHeight = 17.sp,
            modifier = Modifier
                .weight(1f)
                .padding(vertical = 1.dp)
        )

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
