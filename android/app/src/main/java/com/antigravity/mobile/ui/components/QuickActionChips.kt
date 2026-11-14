package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelsResponse
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.ui.theme.AntigravityTheme

@Composable
fun QuickActionChips(
    activeModel: String,
    onToggleModel: () -> Unit,
    onSelectModel: (String) -> Unit = {},
    modelCatalog: ModelsResponse? = null,
    modelSlidePickerState: ModelSlidePickerState? = null,
    onAddImage: () -> Unit,
    selectedSlashCommand: SlashCommandOption? = null,
    onClearSlashCommand: () -> Unit = {},
    onCommitAndPush: () -> Unit,
    commitLabel: String = "Commit",
    showCommit: Boolean = true,
    onShowChanges: () -> Unit,
    showChanges: Boolean = true,
    onShowWorkflowTemplates: (() -> Unit)? = null,
    showContinue: Boolean,
    onContinue: () -> Unit,
    showProceed: Boolean,
    onProceed: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val compactName = remember(activeModel, modelCatalog) {
        ModelDefaults.compactDisplayName(activeModel, modelCatalog?.models.orEmpty())
    }
    val providerKind = remember(activeModel) {
        ModelDefaults.providerKind(activeModel)
    }
    val chipTextColor = when (providerKind) {
        "claude" -> colors.accentOrange
        "orchestrator" -> colors.accentPurple
        "other" -> colors.accentIndigo
        else -> colors.accentBlue
    }
    val haptic = com.antigravity.mobile.ui.util.rememberHaptic()
    val hapticFeedback = LocalHapticFeedback.current
    val density = LocalDensity.current
    var chipBoundsInRoot by remember { mutableStateOf(Rect.Zero) }

    Row(
        modifier = modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(top = 6.dp, bottom = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        // 1. Add Image ➕
        Box(
            modifier = Modifier
                .height(32.dp)
                .width(34.dp)
                .clip(RoundedCornerShape(10.dp))
                .background(colors.surface)
                .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                .clickable {
                    haptic.light()
                    onAddImage()
                },
            contentAlignment = Alignment.Center
        ) {
            Icon(
                imageVector = Icons.Default.Add,
                contentDescription = "Add Media",
                tint = colors.accentIndigo,
                modifier = Modifier.size(16.dp)
            )
        }

        // 2. Gemini / Claude / Orchestrator Model Switcher (Tap to cycle, Long-press + slide up/right for iOS-style picker)
        Box(
            modifier = Modifier
                .height(32.dp)
                .onGloballyPositioned { coords ->
                    val rect = coords.boundsInRoot()
                    chipBoundsInRoot = rect
                    modelSlidePickerState?.anchorRect = rect
                }
                .clip(RoundedCornerShape(10.dp))
                .background(chipTextColor.copy(alpha = 0.12f))
                .border(
                    1.dp,
                    chipTextColor.copy(alpha = 0.35f),
                    RoundedCornerShape(10.dp)
                )
                .clickable {
                    haptic.light()
                    if (modelSlidePickerState?.isPresented == true) {
                        modelSlidePickerState.dismiss()
                    } else {
                        onToggleModel()
                    }
                }
                .pointerInput(providerKind, activeModel, modelCatalog, modelSlidePickerState) {
                    if (modelSlidePickerState == null) return@pointerInput
                    detectDragGesturesAfterLongPress(
                        onDragStart = {
                            haptic.medium()
                            modelSlidePickerState.open(
                                provider = providerKind,
                                activeModel = activeModel,
                                options = modelCatalog?.models.orEmpty()
                            )
                        },
                        onDrag = { change, _ ->
                            change.consume()
                            val rootPoint = Offset(
                                x = chipBoundsInRoot.left + change.position.x,
                                y = chipBoundsInRoot.top + change.position.y
                            )
                            modelSlidePickerState.updateDrag(rootPoint, density, hapticFeedback)
                        },
                        onDragEnd = {
                            modelSlidePickerState.endDrag(onSelect = onSelectModel)
                        },
                        onDragCancel = {
                            modelSlidePickerState.dismiss()
                        }
                    )
                }
                .padding(horizontal = 11.dp),
            contentAlignment = Alignment.Center
        ) {
            Text(
                text = compactName,
                color = chipTextColor,
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium
            )
        }

        // 2b. Workflow: 动态多智能体工作流预设面板（固定放到模型按钮右边）
        if (onShowWorkflowTemplates != null) {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.accentPurple.copy(alpha = 0.12f))
                    .border(
                        1.dp,
                        colors.accentPurple.copy(alpha = 0.35f),
                        RoundedCornerShape(10.dp)
                    )
                    .clickable {
                        haptic.light()
                        onShowWorkflowTemplates()
                    }
                    .padding(horizontal = 12.dp),
                contentAlignment = Alignment.Center
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.AccountTree,
                        contentDescription = "Workflow",
                        tint = colors.accentPurple,
                        modifier = Modifier.size(14.dp)
                    )
                    Text(
                        text = "Workflow",
                        color = colors.accentPurple,
                        fontSize = 13.sp,
                        fontWeight = FontWeight.Medium
                    )
                }
            }
        }

        // 2c. 已选中的斜杠命令按钮（紧跟右侧，样式同高对齐，右上角带小叉）
        selectedSlashCommand?.let { command ->
            SlashCommandChip(
                command = command,
                onClear = {
                    haptic.light()
                    onClearSlashCommand()
                }
            )
        }

        // 3. Commit：打开 Git 提交浮窗（直接调用网关，不再消耗一轮 Agent 对话）
        if (showCommit) {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                    .clickable {
                        haptic.light()
                        onCommitAndPush()
                    }
                    .padding(horizontal = 14.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = commitLabel,
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium
                )
            }
        }

        // 3b. Changes：本会话累计改动的 diff（当前 Antigravity 缺少所需接口时隐藏）
        if (showChanges) {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.surface)
                    .border(1.dp, colors.border, RoundedCornerShape(10.dp))
                    .clickable {
                        haptic.light()
                        onShowChanges()
                    }
                    .padding(horizontal = 14.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = "Changes",
                    color = colors.textPrimary,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.Medium
                )
            }
        }

        // 4. Continue Button (when error occurred)
        if (showContinue) {
            Row(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.accentBlue)
                    .clickable {
                        haptic.medium()
                        onContinue()
                    }
                    .padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.PlayArrow,
                    contentDescription = "Continue",
                    tint = Color.White,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "Continue",
                    color = Color.White,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold
                )
            }
        }

        // 5. Proceed Button (when plan ready)
        if (showProceed) {
            Row(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(colors.accentBlue)
                    .clickable {
                        haptic.heavy()
                        onProceed()
                    }
                    .padding(horizontal = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.PlayArrow,
                    contentDescription = "Proceed",
                    tint = Color.White,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "Proceed",
                    color = Color.White,
                    fontSize = 13.sp,
                    fontWeight = FontWeight.SemiBold
                )
            }
        }
    }
}
