package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.TouchApp
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.ui.theme.AntigravityTheme

/**
 * Fine-grained text selection bottom sheet aligned 1:1 with iOS TextSelectionSheet.
 * Provides a clean selectable plain-text surface with a "全部复制" button (no search or /btw actions).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TextSelectionSheet(
    content: String,
    onDismiss: () -> Unit,
    title: String = "选择文本".loc,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val clipboard = LocalClipboardManager.current
    val haptic = LocalHapticFeedback.current
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var copiedHintVisible by remember { mutableStateOf(false) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = Color.Transparent,
        shape = RectangleShape,
        tonalElevation = 0.dp,
        dragHandle = null,
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
            .fillMaxWidth()
            .fillMaxHeight()
    ) {
        TallSheetBody(containerColor = colors.background, onDismiss = onDismiss) {
            SheetGrabHandle(colors.textMuted.copy(alpha = 0.35f))

            // Top Navigation Bar: centered title + trailing "全部复制"
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 6.dp)
            ) {
                Text(
                    text = title,
                    color = colors.textPrimary,
                    fontSize = 17.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.align(Alignment.Center)
                )

                Row(
                    modifier = Modifier
                        .align(Alignment.CenterEnd)
                        .clip(RoundedCornerShape(8.dp))
                        .clickable {
                            clipboard.setText(AnnotatedString(content))
                            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
                            copiedHintVisible = true
                        }
                        .padding(horizontal = 8.dp, vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.ContentCopy,
                        contentDescription = "全部复制".loc,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(15.dp)
                    )
                    Text(
                        text = "全部复制".loc,
                        color = colors.accentIndigo,
                        fontSize = 15.sp,
                        fontWeight = FontWeight.Medium
                    )
                }
            }

            // Hint Banner (matching iOS TextSelectionSheet)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(colors.surfaceVariant.copy(alpha = 0.6f))
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = Icons.Default.TouchApp,
                    contentDescription = null,
                    tint = colors.textSecondary,
                    modifier = Modifier.size(14.dp)
                )
                Text(
                    text = "长按拖动手柄可精细选择局部文本".loc,
                    color = colors.textSecondary,
                    fontSize = 12.sp
                )
            }

            HorizontalDivider(
                color = colors.border.copy(alpha = 0.5f),
                thickness = 0.5.dp
            )

            // Scrollable Selectable Plain Text Body
            Box(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
                    .background(colors.background)
                    .verticalScroll(rememberScrollState())
                    .padding(start = 16.dp, end = 16.dp, top = 14.dp, bottom = 24.dp)
                    .navigationBarsPadding()
            ) {
                SelectionContainer {
                    Text(
                        text = content,
                        color = colors.textPrimary,
                        fontSize = 15.5.sp,
                        lineHeight = 23.sp,
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }

            CopiedHint(
                visible = copiedHintVisible,
                onHidden = { copiedHintVisible = false }
            )
        }
    }
}
