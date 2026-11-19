package com.antigravity.mobile.ui.components

import com.antigravity.mobile.ui.util.loc
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.filled.StopCircle
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import android.graphics.BitmapFactory
import android.util.Base64
import androidx.compose.foundation.Image
import com.antigravity.mobile.data.model.BrowserSubagentResponse
import com.antigravity.mobile.data.model.SubagentItem
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.fetchBrowserSubagent
import com.antigravity.mobile.data.service.sendSubagentMessage
import com.antigravity.mobile.data.service.skipBrowserSubagent
import com.antigravity.mobile.data.service.whitelistBrowserUrl
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.rememberHaptic
import kotlinx.coroutines.launch

/**
 * 工作流子智能体原位详情半屏抽屉 (SubagentDetailBottomSheet)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SubagentDetailBottomSheet(
    subagent: SubagentItem,
    onDismissRequest: () -> Unit,
    onStopSubagent: (suspend (SubagentItem) -> Unit)? = null,
    onAskBtw: ((String) -> Unit)? = null
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = false)
    val colors = AntigravityTheme.colors
    val haptic = rememberHaptic()
    val scope = rememberCoroutineScope()
    var isStopping by remember { mutableStateOf(false) }
    val isRunning = subagent.isRunning
    val isBrowser = (subagent.typeName ?: "").contains("browser", ignoreCase = true)
    var messageInput by remember { mutableStateOf("") }
    var actionNotice by remember { mutableStateOf<String?>(null) }
    var browserState by remember { mutableStateOf<BrowserSubagentResponse?>(null) }

    LaunchedEffect(subagent.conversationId, isBrowser) {
        if (isBrowser && subagent.conversationId.isNotBlank()) {
            ApiClient.defaultInstance?.fetchBrowserSubagent(subagent.conversationId)?.onSuccess {
                browserState = it
            }
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismissRequest,
        sheetState = sheetState,
        containerColor = colors.surface
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .padding(bottom = 32.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            // Header
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Box(
                    modifier = Modifier
                        .size(44.dp)
                        .clip(CircleShape)
                        .background(colors.accentIndigo.copy(alpha = 0.18f)),
                    contentAlignment = Alignment.Center
                ) {
                    Icon(
                        imageVector = Icons.Default.Memory,
                        contentDescription = null,
                        tint = colors.accentIndigo,
                        modifier = Modifier.size(24.dp)
                    )
                }

                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = subagent.displayName,
                        style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                        color = colors.textPrimary
                    )

                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        subagent.modelTier?.takeIf { it.isNotBlank() }?.let { tier ->
                            Surface(
                                shape = RoundedCornerShape(4.dp),
                                color = if (tier.equals("pro", ignoreCase = true)) Color(0xFF9C27B0) else colors.accentBlue
                            ) {
                                Text(
                                    text = tier.uppercase(),
                                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                                    style = MaterialTheme.typography.labelSmall.copy(
                                        fontSize = 9.sp,
                                        fontWeight = FontWeight.Bold,
                                        fontFamily = FontFamily.Monospace
                                    ),
                                    color = Color.White
                                )
                            }
                        }

                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(6.dp)
                                    .clip(CircleShape)
                                    .background(if (isRunning) colors.accentGreen else colors.textMuted)
                            )
                            Text(
                                text = if (isRunning) "执行中".loc else "已完成".loc,
                                style = MaterialTheme.typography.bodySmall,
                                color = colors.textSecondary
                            )
                        }

                        if ((subagent.stepCount ?: 0) > 0) {
                            Text(
                                text = "• ${subagent.stepCount} 步".loc,
                                style = MaterialTheme.typography.bodySmall,
                                color = colors.textSecondary
                            )
                        }
                    }
                }
            }

            // Prompt Section
            subagent.prompt?.takeIf { it.isNotBlank() }?.let { prompt ->
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = "分配指令 (Initial Prompt)".loc,
                            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                            color = colors.textSecondary
                        )
                        if (onAskBtw != null) {
                            OutlinedButton(
                                onClick = {
                                    onDismissRequest()
                                    onAskBtw(prompt)
                                },
                                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                            ) {
                                Text("旁路快问".loc, fontSize = 11.sp, color = colors.accentIndigo)
                            }
                        }
                    }

                    Surface(
                        shape = RoundedCornerShape(10.dp),
                        color = colors.surfaceVariant.copy(alpha = 0.6f),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            text = prompt,
                            modifier = Modifier.padding(10.dp),
                            style = MaterialTheme.typography.bodyMedium,
                            color = colors.textPrimary
                        )
                    }
                }
            }

            // Metadata info
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = "运行时元数据".loc,
                    style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                    color = colors.textSecondary
                )

                Surface(
                    shape = RoundedCornerShape(10.dp),
                    color = colors.surfaceVariant.copy(alpha = 0.5f),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Column(
                        modifier = Modifier.padding(10.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text("智能体类型".loc, style = MaterialTheme.typography.bodySmall, color = colors.textSecondary)
                            Text(subagent.typeName ?: "self", style = MaterialTheme.typography.bodySmall, color = colors.textPrimary)
                        }
                        HorizontalDivider(color = colors.border.copy(alpha = 0.3f))
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Text("会话 ID".loc, style = MaterialTheme.typography.bodySmall, color = colors.textSecondary)
                            Text(
                                text = subagent.conversationId.ifBlank { "未生成".loc },
                                style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                                color = colors.textPrimary
                            )
                        }
                    }
                }
            }

            // Browser Subagent Live View / Skip / Whitelist
            if (isBrowser && browserState != null) {
                val b = browserState!!
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        text = "浏览器实时画布".loc,
                        style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                        color = colors.textSecondary
                    )
                    b.url?.takeIf { it.isNotBlank() }?.let { url ->
                        Text(
                            text = url,
                            style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                            color = colors.accentBlue
                        )
                    }
                    b.imageBase64?.takeIf { it.isNotBlank() }?.let { b64 ->
                        val bmp = remember(b64) {
                            try {
                                val pure = if (b64.contains(",")) b64.substringAfter(",") else b64
                                val bytes = Base64.decode(pure, Base64.DEFAULT)
                                BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
                            } catch (_: Exception) { null }
                        }
                        if (bmp != null) {
                            Image(
                                bitmap = bmp,
                                contentDescription = "浏览器截图".loc,
                                contentScale = ContentScale.FillWidth,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(10.dp))
                            )
                        }
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        b.url?.takeIf { it.isNotBlank() }?.let { url ->
                            OutlinedButton(onClick = {
                                scope.launch {
                                    ApiClient.defaultInstance?.whitelistBrowserUrl(url)?.onSuccess {
                                        actionNotice = "已添加到浏览器允许名单".loc
                                    }
                                }
                            }) {
                                Text("加白名单".loc, fontSize = 12.sp)
                            }
                        }
                        OutlinedButton(onClick = {
                            scope.launch {
                                ApiClient.defaultInstance?.skipBrowserSubagent(subagent.conversationId)?.onSuccess {
                                    actionNotice = "已请求跳过此步骤".loc
                                }
                            }
                        }) {
                            Text("跳过此步".loc, color = colors.accentOrange, fontSize = 12.sp)
                        }
                    }
                }
            }

            // Mid-run SendAgentMessage box
            if (subagent.conversationId.isNotBlank()) {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        text = "向子代理发送指示".loc,
                        style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                        color = colors.textSecondary
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        OutlinedTextField(
                            value = messageInput,
                            onValueChange = { messageInput = it },
                            placeholder = { Text("中途插话 / 补充要求…".loc, fontSize = 13.sp) },
                            singleLine = true,
                            modifier = Modifier.weight(1f)
                        )
                        IconButton(
                            onClick = {
                                val txt = messageInput.trim()
                                if (txt.isNotEmpty()) {
                                    messageInput = ""
                                    scope.launch {
                                        ApiClient.defaultInstance?.sendSubagentMessage(subagent.conversationId, txt)
                                            ?.onSuccess { actionNotice = "指示已送达子代理".loc }
                                            ?.onFailure { actionNotice = "发送失败: ${it.message}".loc }
                                    }
                                }
                            }
                        ) {
                            Icon(Icons.Default.Send, contentDescription = "发送".loc, tint = colors.accentBlue)
                        }
                    }
                    actionNotice?.let { note ->
                        Text(note, style = MaterialTheme.typography.bodySmall, color = colors.accentGreen)
                    }
                }
            }

            // Stop Subagent Button (if running)
            if (isRunning && onStopSubagent != null) {
                Button(
                    onClick = {
                        haptic.heavy()
                        isStopping = true
                        // Coroutine launch
                    },
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(10.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentRed),
                    enabled = !isStopping
                ) {
                    if (isStopping) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(16.dp),
                            color = Color.White,
                            strokeWidth = 2.dp
                        )
                        Spacer(modifier = Modifier.width(6.dp))
                    }
                    Icon(
                        imageVector = Icons.Default.StopCircle,
                        contentDescription = null,
                        modifier = Modifier.size(16.dp)
                    )
                    Spacer(modifier = Modifier.width(6.dp))
                    Text("终止该子代理".loc, style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.Bold))
                }
            }
        }
    }
}
