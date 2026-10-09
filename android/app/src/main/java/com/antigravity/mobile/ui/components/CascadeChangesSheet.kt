package com.antigravity.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Difference
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.antigravity.mobile.data.model.RevertPreviewFile
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChangesSheetState

/** 「本会话改动」：Agent 在当前会话里累计改动了哪些文件，点开可看正向 diff。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CascadeChangesSheet(
    state: ChangesSheetState,
    onDismiss: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var viewingDiffFile by remember { mutableStateOf<RevertPreviewFile?>(null) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        sheetMaxWidth = Dp.Unspecified,
        containerColor = colors.surface,
        contentColor = colors.textPrimary,
        dragHandle = { BottomSheetDefaults.DragHandle() },
        contentWindowInsets = { WindowInsets(0, 0, 0, 0) },
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .navigationBarsPadding()
                .padding(horizontal = 20.dp)
                .padding(bottom = 24.dp)
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.padding(bottom = 4.dp)
            ) {
                Icon(Icons.Default.Difference, contentDescription = null, tint = colors.accentIndigo, modifier = Modifier.size(24.dp))
                Text("本会话改动", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = colors.textPrimary)
            }

            val data = state.data
            when {
                state.isLoading -> {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 28.dp),
                        horizontalArrangement = Arrangement.Center,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                        Spacer(Modifier.width(10.dp))
                        Text("正在汇总代码改动...", fontSize = 13.sp, color = colors.textMuted)
                    }
                }
                state.error != null -> {
                    ChangesNotice(state.error, isError = true)
                }
                data == null || !data.hasChanges -> {
                    ChangesNotice("本会话没有产生文件改动", isError = false)
                }
                else -> {
                    Row(
                        modifier = Modifier.padding(bottom = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Text("${data.files.size} 个文件", fontSize = 13.sp, color = colors.textSecondary)
                        Text("+${data.additions}", fontSize = 13.sp, fontWeight = FontWeight.Bold, fontFamily = FontFamily.Monospace, color = Color(0xFF4CAF50))
                        Text("-${data.deletions}", fontSize = 13.sp, fontWeight = FontWeight.Bold, fontFamily = FontFamily.Monospace, color = Color(0xFFF44336))
                    }
                    LazyColumn(
                        modifier = Modifier.fillMaxWidth().heightIn(max = 460.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        items(data.files, key = { it.fileUri.ifBlank { it.fileName } }) { file ->
                            FileChangeItem(file = file, onViewDiff = { viewingDiffFile = file })
                        }
                    }
                }
            }
        }
    }

    viewingDiffFile?.let { file ->
        DiffViewerDialog(file = file, onDismiss = { viewingDiffFile = null })
    }
}

@Composable
private fun ChangesNotice(text: String, isError: Boolean) {
    val colors = AntigravityTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 12.dp)
            .clip(RoundedCornerShape(10.dp))
            .background(colors.surfaceVariant)
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Icon(
            imageVector = if (isError) Icons.Default.ErrorOutline else Icons.Default.CheckCircle,
            contentDescription = null,
            tint = if (isError) colors.accentOrange else Color(0xFF4CAF50),
            modifier = Modifier.size(18.dp)
        )
        Text(text, fontSize = 13.sp, color = colors.textSecondary)
    }
}
