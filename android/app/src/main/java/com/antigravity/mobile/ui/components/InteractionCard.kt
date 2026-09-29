package com.antigravity.mobile.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.HelpOutline
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.isSpecified
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
        "ask_question" -> Color(0xFF9333EA)
        "run_command" -> colors.accentOrange
        else -> colors.accentOrange
    }

    BoxWithConstraints(modifier = modifier.fillMaxWidth()) {
        val screenHeight = LocalConfiguration.current.screenHeightDp.dp
        val availableHeight = if (maxHeight.isSpecified && maxHeight < Dp.Infinity && maxHeight > 100.dp) maxHeight else screenHeight
        val fullHeight = availableHeight
        val halfHeight = availableHeight * 0.52f

        var isExpanded by remember { mutableStateOf(false) }
        var dragOffsetY by remember { mutableFloatStateOf(0f) }

        val targetHeight = if (isExpanded) fullHeight else halfHeight
        val animatedHeight by animateDpAsState(
            targetValue = targetHeight,
            animationSpec = spring(
                dampingRatio = Spring.DampingRatioLowBouncy,
                stiffness = Spring.StiffnessMediumLow
            ),
            label = "sheetHeight"
        )

        val currentHeight = if (dragOffsetY != 0f) {
            val base = if (isExpanded) fullHeight else halfHeight
            val density = LocalDensity.current
            val dragOffsetDp = with(density) { dragOffsetY.toDp() }
            (base - dragOffsetDp).coerceIn(halfHeight, fullHeight)
        } else {
            animatedHeight
        }

        val dragModifier = Modifier.pointerInput(isExpanded) {
            detectVerticalDragGestures(
                onDragStart = {
                    dragOffsetY = 0f
                },
                onDragEnd = {
                    val density = this
                    val dragOffsetDp = with(density) { dragOffsetY.toDp() }
                    if (isExpanded) {
                        if (dragOffsetDp > 40.dp) {
                            isExpanded = false
                            haptic.light()
                        }
                    } else {
                        if (dragOffsetDp < (-40).dp) {
                            isExpanded = true
                            haptic.light()
                        }
                    }
                    dragOffsetY = 0f
                },
                onDragCancel = {
                    dragOffsetY = 0f
                },
                onVerticalDrag = { change, dragAmount ->
                    change.consume()
                    dragOffsetY += dragAmount
                }
            )
        }

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .height(currentHeight)
                .shadow(
                    elevation = 10.dp,
                    shape = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp),
                    ambientColor = Color.Black.copy(alpha = 0.08f),
                    spotColor = Color.Black.copy(alpha = 0.16f)
                )
                .clip(RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp))
                .background(colors.surface)
                .border(
                    width = 1.dp,
                    color = headerColor.copy(alpha = 0.35f),
                    shape = RoundedCornerShape(topStart = 18.dp, topEnd = 18.dp)
                )
                .navigationBarsPadding()
                .imePadding()
                .padding(horizontal = 14.dp)
                .padding(top = 2.dp, bottom = 10.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            // Drag Handle on top (Tap to toggle, Drag to expand/collapse)
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable(
                        interactionSource = remember { MutableInteractionSource() },
                        indication = null
                    ) {
                        haptic.light()
                        isExpanded = !isExpanded
                    }
                    .then(dragModifier)
                    .padding(top = 8.dp, bottom = 4.dp),
                contentAlignment = Alignment.Center
            ) {
                Box(
                    modifier = Modifier
                        .width(38.dp)
                        .height(4.5.dp)
                        .clip(RoundedCornerShape(2.5.dp))
                        .background(colors.textSecondary.copy(alpha = 0.35f))
                )
            }

            // Header: Icon + Title + Expand/Collapse Button (also draggable)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier
                    .fillMaxWidth()
                    .then(dragModifier)
            ) {
                Icon(
                    imageVector = headerIcon,
                    contentDescription = null,
                    tint = headerColor,
                    modifier = Modifier.size(20.dp)
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
                    modifier = Modifier.weight(1f)
                )
                IconButton(
                    onClick = {
                        haptic.light()
                        isExpanded = !isExpanded
                    },
                    modifier = Modifier.size(28.dp)
                ) {
                    Icon(
                        imageVector = if (isExpanded) Icons.Default.KeyboardArrowDown else Icons.Default.KeyboardArrowUp,
                        contentDescription = if (isExpanded) "收起" else "全屏展开",
                        tint = colors.textSecondary,
                        modifier = Modifier.size(20.dp)
                    )
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
                        maxLines = 3
                    )
                }
            }

            // Question Options Area (Takes remaining vertical space and scrolls)
            Column(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState()),
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
                    .background(Color(0xFF9333EA))
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
