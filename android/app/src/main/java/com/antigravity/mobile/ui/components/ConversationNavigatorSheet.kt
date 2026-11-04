package com.antigravity.mobile.ui.components

import androidx.compose.animation.*
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
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
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ConversationChapter
import com.antigravity.mobile.data.model.ConversationTurn
import com.antigravity.mobile.ui.theme.AntigravityTheme

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConversationNavigatorSheet(
    turns: List<ConversationTurn>,
    chapters: List<ConversationChapter>,
    currentTurnIndex: Int?,
    onSelectTurn: (ConversationTurn) -> Unit,
    onJumpToTop: () -> Unit,
    onJumpToBottom: () -> Unit,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val colors = AntigravityTheme.colors
    var collapsedChapterIds by remember { mutableStateOf(setOf<String>()) }
    val allCollapsed = chapters.isNotEmpty() && chapters.all { it.id in collapsedChapterIds }

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
                .padding(horizontal = 16.dp)
        ) {
            // Header bar
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(bottom = 12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = "对话大纲与步骤导轨",
                    fontSize = 17.sp,
                    fontWeight = FontWeight.Bold,
                    color = colors.textPrimary,
                    modifier = Modifier.weight(1f)
                )
            }

            // Summary banner
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(colors.surface)
                    .padding(horizontal = 14.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Box(
                    modifier = Modifier
                        .size(32.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo.copy(alpha = 0.12f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.LinearScale,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(18.dp)
                    )
                }

                Spacer(modifier = Modifier.width(12.dp))

                Column(modifier = Modifier.weight(1f)) {
                    val summaryText = if (chapters.size > 1) {
                        "共 ${chapters.size} 个章节 · ${turns.size} 轮问答"
                    } else {
                        "共 ${turns.size} 轮问答步骤"
                    }
                    Text(
                        text = summaryText,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.textPrimary
                    )
                    Text(
                        text = "点击任意步骤即可平滑瞬达对应提问与上下文",
                        fontSize = 12.sp,
                        color = colors.textSecondary
                    )
                }

                if (chapters.size > 1) {
                    Surface(
                        onClick = {
                            collapsedChapterIds = if (allCollapsed) emptySet() else chapters.map { it.id }.toSet()
                        },
                        shape = RoundedCornerShape(16.dp),
                        color = colors.accentIndigo.copy(alpha = 0.12f)
                    ) {
                        Row(
                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Icon(
                                imageVector = if (allCollapsed) Icons.Default.UnfoldMore else Icons.Default.UnfoldLess,
                                contentDescription = null,
                                tint = colors.accentIndigo,
                                modifier = Modifier.size(12.dp)
                            )
                            Spacer(modifier = Modifier.width(4.dp))
                            Text(
                                text = if (allCollapsed) "展开全部" else "折叠全部",
                                fontSize = 11.sp,
                                fontWeight = FontWeight.Medium,
                                color = colors.accentIndigo
                            )
                        }
                    }
                }
            }

            Spacer(modifier = Modifier.height(14.dp))

            // Scrollable Timeline
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(bottom = 32.dp)
            ) {
                if (chapters.size > 1) {
                    chapters.forEach { chapter ->
                        val isCollapsed = chapter.id in collapsedChapterIds
                        val containsCurrent = chapter.turns.any { it.turnIndex == currentTurnIndex }
                        val containsError = chapter.turns.any { it.hasError }
                        val containsActive = chapter.turns.any { it.isActive }
                        val chevronRotation by animateFloatAsState(targetValue = if (isCollapsed) -90f else 0f)

                        // 可折叠章节卡片头部
                        Surface(
                            onClick = {
                                collapsedChapterIds = if (isCollapsed) {
                                    collapsedChapterIds - chapter.id
                                } else {
                                    collapsedChapterIds + chapter.id
                                }
                            },
                            shape = RoundedCornerShape(10.dp),
                            color = if (isCollapsed) colors.surfaceVariant.copy(alpha = 0.7f) else colors.surfaceVariant.copy(alpha = 0.4f),
                            border = androidx.compose.foundation.BorderStroke(0.6.dp, colors.border.copy(alpha = 0.4f)),
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(top = 10.dp, bottom = 6.dp)
                        ) {
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 10.dp, vertical = 8.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Icon(
                                    imageVector = if (chapter.dateText != null) Icons.Default.CalendarToday else Icons.Default.Bookmark,
                                    contentDescription = null,
                                    tint = if (containsCurrent) colors.accentIndigo else colors.textSecondary,
                                    modifier = Modifier.size(14.dp)
                                )

                                Spacer(modifier = Modifier.width(8.dp))

                                Text(
                                    text = chapter.title,
                                    fontSize = 13.sp,
                                    fontWeight = FontWeight.Bold,
                                    color = if (containsCurrent) colors.textPrimary else colors.textSecondary,
                                    modifier = Modifier.weight(1f),
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )

                                if (containsCurrent) {
                                    Text(
                                        text = "当前",
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = Color.White,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(4.dp))
                                            .background(colors.accentIndigo)
                                            .padding(horizontal = 5.dp, vertical = 2.dp)
                                    )
                                    Spacer(modifier = Modifier.width(4.dp))
                                }

                                if (containsActive) {
                                    Text(
                                        text = "进行中",
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentBlue,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(4.dp))
                                            .background(colors.accentBlue.copy(alpha = 0.12f))
                                            .padding(horizontal = 5.dp, vertical = 2.dp)
                                    )
                                    Spacer(modifier = Modifier.width(4.dp))
                                }

                                if (containsError) {
                                    Text(
                                        text = "异常",
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        color = colors.accentOrange,
                                        modifier = Modifier
                                            .clip(RoundedCornerShape(4.dp))
                                            .background(colors.accentOrange.copy(alpha = 0.12f))
                                            .padding(horizontal = 5.dp, vertical = 2.dp)
                                    )
                                    Spacer(modifier = Modifier.width(4.dp))
                                }

                                Text(
                                    text = "${chapter.turns.size} 轮",
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.Medium,
                                    color = colors.textSecondary,
                                    modifier = Modifier
                                        .clip(RoundedCornerShape(12.dp))
                                        .background(colors.surface)
                                        .padding(horizontal = 6.dp, vertical = 2.dp)
                                )

                                Spacer(modifier = Modifier.width(4.dp))

                                Icon(
                                    imageVector = Icons.Default.ExpandMore,
                                    contentDescription = if (isCollapsed) "展开" else "折叠",
                                    tint = colors.textSecondary,
                                    modifier = Modifier
                                        .size(18.dp)
                                        .rotate(chevronRotation)
                                )
                            }
                        }

                        AnimatedVisibility(
                            visible = !isCollapsed,
                            enter = expandVertically() + fadeIn(),
                            exit = shrinkVertically() + fadeOut()
                        ) {
                            TurnsTimelineList(
                                turns = chapter.turns,
                                currentTurnIndex = currentTurnIndex,
                                onSelectTurn = {
                                    onDismiss()
                                    onSelectTurn(it)
                                }
                            )
                        }
                    }
                } else {
                    TurnsTimelineList(
                        turns = turns,
                        currentTurnIndex = currentTurnIndex,
                        onSelectTurn = {
                            onDismiss()
                            onSelectTurn(it)
                        }
                    )
                }
            }
        }
    }
}

@Composable
private fun TurnsTimelineList(
    turns: List<ConversationTurn>,
    currentTurnIndex: Int?,
    onSelectTurn: (ConversationTurn) -> Unit
) {
    turns.forEachIndexed { index, turn ->
        val isCurrent = (turn.turnIndex == currentTurnIndex)
        val isFirst = (index == 0)
        val isLast = (index == turns.lastIndex)

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 2.dp),
            verticalAlignment = Alignment.Top
        ) {
            // Timeline Node and Track
            TimelineTrackNode(
                turn = turn,
                isCurrent = isCurrent,
                isFirst = isFirst,
                isLast = isLast
            )

            Spacer(modifier = Modifier.width(10.dp))

            // Turn Card
            TurnCardItem(
                turn = turn,
                isCurrent = isCurrent,
                onClick = { onSelectTurn(turn) }
            )
        }
    }
}

@Composable
private fun TimelineTrackNode(
    turn: ConversationTurn,
    isCurrent: Boolean,
    isFirst: Boolean,
    isLast: Boolean
) {
    val colors = AntigravityTheme.colors
    val nodeColor = when {
        turn.hasError -> colors.accentOrange
        turn.isActive -> colors.accentBlue
        isCurrent -> colors.accentIndigo
        else -> colors.textSecondary.copy(alpha = 0.5f)
    }

    Column(
        modifier = Modifier.width(22.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        // Top line
        Box(
            modifier = Modifier
                .width(2.dp)
                .height(14.dp)
                .background(if (isFirst) Color.Transparent else colors.border)
        )

        // Circle Node
        Box(
            modifier = Modifier
                .size(if (isCurrent) 18.dp else 12.dp)
                .clip(CircleShape)
                .background(if (isCurrent) nodeColor.copy(alpha = 0.2f) else Color.Transparent)
                .padding(if (isCurrent) 3.dp else 0.dp),
            contentAlignment = Alignment.Center
        ) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .clip(CircleShape)
                    .background(nodeColor)
            )
        }

        // Bottom line
        Box(
            modifier = Modifier
                .width(2.dp)
                .height(48.dp)
                .background(if (isLast) Color.Transparent else colors.border)
        )
    }
}

@Composable
private fun TurnCardItem(
    turn: ConversationTurn,
    isCurrent: Boolean,
    onClick: () -> Unit
) {
    val colors = AntigravityTheme.colors

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.surface)
            .border(
                width = if (isCurrent) 1.5.dp else 0.5.dp,
                color = if (isCurrent) colors.accentIndigo else colors.border,
                shape = RoundedCornerShape(12.dp)
            )
            .clickable(onClick = onClick)
            .padding(12.dp)
    ) {
        Column(modifier = Modifier.fillMaxWidth()) {
            // Header row
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Box(
                    modifier = Modifier
                        .clip(RoundedCornerShape(6.dp))
                        .background(if (isCurrent) colors.accentIndigo else colors.accentIndigo.copy(alpha = 0.12f))
                        .padding(horizontal = 6.dp, vertical = 2.dp)
                ) {
                    Text(
                        text = "第 ${turn.turnIndex} 轮",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = if (isCurrent) Color.White else colors.accentIndigo
                    )
                }

                if (!turn.modelName.isNullOrBlank()) {
                    Spacer(modifier = Modifier.width(8.dp))
                    Text(
                        text = turn.modelName,
                        fontSize = 11.sp,
                        color = colors.textSecondary,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }

                Spacer(modifier = Modifier.weight(1f))

                if (turn.isActive) {
                    Text(
                        text = "处理中",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                        color = colors.accentBlue
                    )
                } else if (turn.hasError) {
                    Text(
                        text = "异常",
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                        color = colors.accentOrange
                    )
                }

                Spacer(modifier = Modifier.width(4.dp))
                Icon(
                    imageVector = Icons.Default.ChevronRight,
                    contentDescription = null,
                    tint = colors.textSecondary.copy(alpha = 0.5f),
                    modifier = Modifier.size(16.dp)
                )
            }

            Spacer(modifier = Modifier.height(6.dp))

            // Prompt summary
            Text(
                text = turn.promptSummary,
                fontSize = 14.sp,
                fontWeight = if (isCurrent) FontWeight.SemiBold else FontWeight.Medium,
                color = colors.textPrimary,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )

            // Chips
            if (turn.artifacts.isNotEmpty() || turn.totalTools > 0) {
                Spacer(modifier = Modifier.height(8.dp))
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    if (turn.artifacts.isNotEmpty()) {
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .background(colors.accentGreen.copy(alpha = 0.1f))
                                .padding(horizontal = 6.dp, vertical = 2.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Icon(
                                imageVector = Icons.Default.Description,
                                contentDescription = null,
                                tint = colors.accentGreen,
                                modifier = Modifier.size(12.dp)
                            )
                            Spacer(modifier = Modifier.width(3.dp))
                            Text(
                                text = "${turn.artifacts.size} 个产物",
                                fontSize = 10.sp,
                                color = colors.accentGreen,
                                fontWeight = FontWeight.Medium
                            )
                        }
                    }

                    if (turn.totalTools > 0) {
                        Row(
                            modifier = Modifier
                                .clip(RoundedCornerShape(4.dp))
                                .background(colors.accentGreen.copy(alpha = 0.12f))
                                .padding(horizontal = 6.dp, vertical = 2.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Icon(
                                imageVector = Icons.Default.Build,
                                contentDescription = null,
                                tint = colors.accentGreen,
                                modifier = Modifier.size(12.dp)
                            )
                            Spacer(modifier = Modifier.width(3.dp))
                            Text(
                                text = "${turn.totalTools} 次工具调用",
                                fontSize = 10.sp,
                                color = colors.accentGreen,
                                fontWeight = FontWeight.Medium
                            )
                        }
                    }
                }
            }
        }
    }
}
