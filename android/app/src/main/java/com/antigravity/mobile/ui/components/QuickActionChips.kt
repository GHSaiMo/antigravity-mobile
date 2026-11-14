package com.antigravity.mobile.ui.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.ModelDefaults
import com.antigravity.mobile.data.model.ModelFamily
import com.antigravity.mobile.data.model.ModelsResponse
import com.antigravity.mobile.data.model.SlashCommandOption
import com.antigravity.mobile.ui.theme.AntigravityTheme

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun QuickActionChips(
    activeModel: String,
    onToggleModel: () -> Unit,
    onSelectModel: (String) -> Unit = {},
    modelCatalog: ModelsResponse? = null,
    onAddImage: () -> Unit,
    selectedSlashCommand: SlashCommandOption? = null,
    onClearSlashCommand: () -> Unit = {},
    onCommitAndPush: () -> Unit,
    commitLabel: String = "Commit",
    showCommit: Boolean = true,
    onShowChanges: () -> Unit,
    showChanges: Boolean = true,
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
    var showModelMenu by remember { mutableStateOf(false) }
    var selectedFamily by remember { mutableStateOf<ModelFamily?>(null) }

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

        // 2. Gemini / Claude / Model Switcher (Tap to toggle, Long-press for hierarchical menu)
        Box {
            Box(
                modifier = Modifier
                    .height(32.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(chipTextColor.copy(alpha = 0.12f))
                    .border(
                        1.dp,
                        chipTextColor.copy(alpha = 0.35f),
                        RoundedCornerShape(10.dp)
                    )
                    .combinedClickable(
                        onClick = {
                            haptic.light()
                            onToggleModel()
                        },
                        onLongClick = {
                            haptic.medium()
                            selectedFamily = null
                            showModelMenu = true
                        }
                    )
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

            DropdownMenu(
                expanded = showModelMenu,
                onDismissRequest = {
                    showModelMenu = false
                    selectedFamily = null
                }
            ) {
                val families = remember(modelCatalog, providerKind) {
                    ModelDefaults.modelFamilies(modelCatalog?.models.orEmpty(), provider = providerKind)
                }
                val currentFamily = selectedFamily
                if (currentFamily == null) {
                    families.forEach { family ->
                        DropdownMenuItem(
                            text = {
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    Text(
                                        text = family.name,
                                        fontWeight = FontWeight.Medium
                                    )
                                    if (family.variants.size > 1) {
                                        Text(
                                            text = "›",
                                            color = colors.textSecondary,
                                            fontSize = 14.sp
                                        )
                                    }
                                }
                            },
                            onClick = {
                                if (family.variants.size == 1) {
                                    onSelectModel(family.variants.first().id)
                                    showModelMenu = false
                                } else {
                                    selectedFamily = family
                                }
                            }
                        )
                    }
                } else {
                    DropdownMenuItem(
                        text = {
                            Text(
                                text = "← ${currentFamily.name}",
                                color = colors.accentBlue,
                                fontWeight = FontWeight.SemiBold
                            )
                        },
                        onClick = { selectedFamily = null }
                    )
                    currentFamily.variants.forEach { variant ->
                        val isSelected = activeModel.equals(variant.id, ignoreCase = true)
                        DropdownMenuItem(
                            text = {
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    Text(
                                        text = variant.label,
                                        color = if (isSelected) colors.accentBlue else colors.textPrimary,
                                        fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                    )
                                    if (isSelected) {
                                        Text(
                                            text = "✓",
                                            color = colors.accentBlue,
                                            fontWeight = FontWeight.Bold
                                        )
                                    }
                                }
                            },
                            onClick = {
                                onSelectModel(variant.id)
                                showModelMenu = false
                                selectedFamily = null
                            }
                        )
                    }
                }
            }
        }

        // 2b. 已选中的斜杠命令按钮（紧跟 Gemini 右侧，样式同高对齐，右上角带小叉）
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
