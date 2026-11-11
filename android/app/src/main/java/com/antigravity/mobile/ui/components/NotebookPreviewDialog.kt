package com.antigravity.mobile.ui.components

import android.graphics.BitmapFactory
import android.util.Base64
import android.widget.Toast
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.ui.theme.AntigravityTheme
import org.json.JSONObject

private data class NotebookCellData(
    val cellType: String,
    val source: String,
    val executionCount: Int?,
    val outputs: List<NotebookOutputData>
)

private data class NotebookOutputData(
    val outputType: String,
    val text: String?,
    val imageBase64: String?
)

@Composable
fun NotebookPreviewDialog(
    jsonContent: String,
    fileName: String,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    val bodyScale = LocalMarkdownBodyScale.current
    val codeScale = LocalMarkdownCodeScale.current

    val cells = remember(jsonContent) {
        val list = mutableListOf<NotebookCellData>()
        try {
            val root = JSONObject(jsonContent)
            val cellsArr = root.optJSONArray("cells")
            if (cellsArr != null) {
                for (i in 0 until cellsArr.length()) {
                    val cellObj = cellsArr.getJSONObject(i)
                    val type = cellObj.optString("cell_type", "raw")
                    val source = extractText(cellObj.opt("source"))
                    val execCount = if (cellObj.has("execution_count") && !cellObj.isNull("execution_count")) {
                        cellObj.getInt("execution_count")
                    } else null

                    val outs = mutableListOf<NotebookOutputData>()
                    val outsArr = cellObj.optJSONArray("outputs")
                    if (outsArr != null) {
                        for (j in 0 until outsArr.length()) {
                            val outObj = outsArr.getJSONObject(j)
                            val otype = outObj.optString("output_type", "")
                            var otext: String? = null
                            var ob64: String? = null

                            if (outObj.has("text")) {
                                otext = extractText(outObj.opt("text"))
                            }
                            val dataObj = outObj.optJSONObject("data")
                            if (dataObj != null) {
                                if (dataObj.has("text/plain")) {
                                    otext = (otext ?: "") + extractText(dataObj.opt("text/plain"))
                                }
                                if (dataObj.has("image/png")) {
                                    val rawPng = extractText(dataObj.opt("image/png")).trim()
                                    val cleaned = rawPng.substringAfter("base64,", rawPng).trim()
                                    if (cleaned.isNotEmpty()) {
                                        ob64 = cleaned
                                    }
                                }
                            }
                            val tbArr = outObj.optJSONArray("traceback")
                            if (tbArr != null) {
                                val sb = StringBuilder()
                                for (k in 0 until tbArr.length()) {
                                    sb.append(tbArr.getString(k)).append("\n")
                                }
                                otext = sb.toString()
                            }
                            outs.add(NotebookOutputData(otype, otext, ob64))
                        }
                    }
                    list.add(NotebookCellData(type, source, execCount, outs))
                }
            }
        } catch (e: Exception) {
            list.add(NotebookCellData("markdown", "解析 Notebook 失败: ${e.message}", null, emptyList()))
        }
        list
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = Modifier
                .fillMaxSize()
                .padding(12.dp),
            shape = RoundedCornerShape(16.dp),
            color = colors.background
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                // Header
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text(
                        text = fileName,
                        color = colors.textPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.weight(1f)
                    )
                    IconButton(onClick = onDismiss) {
                        Icon(imageVector = Icons.Default.Close, contentDescription = "关闭", tint = colors.textSecondary)
                    }
                }

                Divider(color = colors.border, thickness = 0.5.dp)

                // Cells
                LazyColumn(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .padding(horizontal = 14.dp, vertical = 10.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    itemsIndexed(cells) { index, cell ->
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(colors.surface)
                                .border(0.5.dp, colors.border, RoundedCornerShape(10.dp))
                                .padding(12.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            if (cell.cellType == "markdown") {
                                Text(
                                    text = cell.source,
                                    color = colors.textPrimary,
                                    fontSize = (13.5f * bodyScale).sp,
                                    lineHeight = (19f * bodyScale).sp
                                )
                            } else if (cell.cellType == "code") {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                    modifier = Modifier.fillMaxWidth()
                                ) {
                                    Text(
                                        text = "In [${cell.executionCount ?: " "}]:",
                                        color = colors.accentBlue,
                                        fontSize = 11.5.sp,
                                        fontFamily = FontFamily.Monospace,
                                        fontWeight = FontWeight.Bold
                                    )

                                    IconButton(
                                        onClick = {
                                            clipboard.setText(AnnotatedString(cell.source))
                                            Toast.makeText(context, "代码已复制", Toast.LENGTH_SHORT).show()
                                        },
                                        modifier = Modifier.size(24.dp)
                                    ) {
                                        Icon(
                                            imageVector = Icons.Default.ContentCopy,
                                            contentDescription = "复制",
                                            tint = colors.textSecondary,
                                            modifier = Modifier.size(14.dp)
                                        )
                                    }
                                }

                                Box(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(6.dp))
                                        .background(colors.surfaceVariant)
                                        .padding(8.dp)
                                        .horizontalScroll(rememberScrollState())
                                ) {
                                    Text(
                                        text = cell.source,
                                        color = colors.textPrimary,
                                        fontSize = (12f * codeScale).sp,
                                        fontFamily = FontFamily.Monospace
                                    )
                                }

                                if (cell.outputs.isNotEmpty()) {
                                    cell.outputs.forEach { out ->
                                        if (out.imageBase64 != null) {
                                            val bitmap = remember(out.imageBase64) {
                                                try {
                                                    val bytes = Base64.decode(out.imageBase64, Base64.DEFAULT)
                                                    BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
                                                } catch (_: Exception) { null }
                                            }
                                            if (bitmap != null) {
                                                Image(
                                                    bitmap = bitmap,
                                                    contentDescription = null,
                                                    modifier = Modifier
                                                        .fillMaxWidth()
                                                        .clip(RoundedCornerShape(6.dp))
                                                        .padding(vertical = 4.dp)
                                                )
                                            }
                                        }

                                        if (!out.text.isNullOrBlank()) {
                                            Box(
                                                modifier = Modifier
                                                    .fillMaxWidth()
                                                    .clip(RoundedCornerShape(6.dp))
                                                    .background(colors.background)
                                                    .padding(8.dp)
                                                    .horizontalScroll(rememberScrollState())
                                            ) {
                                                Text(
                                                    text = out.text,
                                                    color = if (out.outputType == "error") colors.accentOrange else colors.textSecondary,
                                                    fontSize = 11.5.sp,
                                                    fontFamily = FontFamily.Monospace
                                                )
                                            }
                                        }
                                    }
                                }
                            } else {
                                Text(
                                    text = cell.source,
                                    color = colors.textSecondary,
                                    fontSize = 12.sp,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun extractText(raw: Any?): String {
    return when (raw) {
        is String -> raw
        is org.json.JSONArray -> {
            val sb = StringBuilder()
            for (i in 0 until raw.length()) {
                sb.append(raw.optString(i))
            }
            sb.toString()
        }
        else -> ""
    }
}
