package com.antigravity.mobile.ui.components

import android.graphics.BitmapFactory
import android.util.Base64
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.*
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

// region 1. BrowserViewerDialog (Remote Live Browser Frame Stream & Touch Takeover)

@Composable
fun BrowserViewerDialog(
    initialUrl: String? = null,
    initialTabId: String? = null,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    val api = ApiClient.defaultInstance

    var tabs by remember { mutableStateOf<List<BrowserViewerTab>>(emptyList()) }
    var activeTab by remember { mutableStateOf<BrowserViewerTab?>(null) }
    var sessionId by remember { mutableStateOf<String?>(null) }
    var controlToken by remember { mutableStateOf<String?>(null) }
    var frameBitmap by remember { mutableStateOf<ImageBitmap?>(null) }
    var frameWidthPx by remember { mutableIntStateOf(1280) }
    var frameHeightPx by remember { mutableIntStateOf(800) }
    var imageViewSize by remember { mutableStateOf(IntSize.Zero) }
    var urlInput by remember { mutableStateOf(initialUrl.orEmpty()) }
    var textInput by remember { mutableStateOf("") }
    var isLoading by remember { mutableStateOf(true) }
    var statusNotice by remember { mutableStateOf<String?>(null) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    var showTabMenu by remember { mutableStateOf(false) }

    suspend fun pullSingleFrame(sid: String, tid: String) {
        api?.fetchBrowserFrame(sid, tid)?.onSuccess { resp ->
            resp.tab?.let { t ->
                activeTab = t
                if (!t.url.isNullOrBlank() && urlInput.isBlank()) urlInput = t.url
            }
            if ((resp.widthPx ?: 0) > 0) frameWidthPx = resp.widthPx!!
            if ((resp.heightPx ?: 0) > 0) frameHeightPx = resp.heightPx!!
            val b64 = resp.imageData.orEmpty()
            if (b64.isNotBlank()) {
                val bmp = runCatching {
                    val pure = if (b64.contains(",")) b64.substringAfter(",") else b64
                    val bytes = Base64.decode(pure, Base64.DEFAULT)
                    BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
                }.getOrNull()
                if (bmp != null) {
                    frameBitmap = bmp
                    if (resp.widthPx == null || resp.widthPx <= 0) frameWidthPx = bmp.width
                    if (resp.heightPx == null || resp.heightPx <= 0) frameHeightPx = bmp.height
                }
            }
            errorMessage = resp.error
        }
    }

    suspend fun ensureControlToken(): String? {
        controlToken?.let { return it }
        val sid = sessionId ?: return null
        val tid = activeTab?.tabId ?: return null
        val res = api?.takeoverBrowserTab(sid, tid, leaseSeconds = 90)?.getOrNull()
        val tok = res?.controlToken?.takeIf { it.isNotBlank() }
        if (tok != null) {
            controlToken = tok
            statusNotice = "已接管宿主浏览器控制权".loc
        } else {
            errorMessage = res?.error ?: "接管浏览器失败".loc
        }
        return tok
    }

    suspend fun sendAction(
        action: String,
        url: String? = null,
        inputKind: String? = null,
        xPx: Int? = null,
        yPx: Int? = null,
        deltaX: Double? = null,
        deltaY: Double? = null,
        key: String? = null,
        text: String? = null
    ) {
        val sid = sessionId ?: return
        val tid = activeTab?.tabId ?: return
        val tok = ensureControlToken() ?: return
        api?.controlBrowserTab(
            sessionId = sid,
            tabId = tid,
            controlToken = tok,
            action = action,
            url = url,
            inputKind = inputKind,
            xPx = xPx,
            yPx = yPx,
            deltaX = deltaX,
            deltaY = deltaY,
            key = key,
            text = text
        )?.onFailure { err ->
            errorMessage = err.message
        }
        delay(120)
        pullSingleFrame(sid, tid)
    }

    suspend fun openTab(targetTabId: String?, navigateToUrl: String? = null) {
        if (api == null) {
            errorMessage = "未配置网关地址".loc
            isLoading = false
            return
        }
        isLoading = true
        errorMessage = null
        val prevSid = sessionId
        val prevTid = activeTab?.tabId
        if (!prevSid.isNullOrBlank() && !prevTid.isNullOrBlank()) {
            controlToken?.let { api.releaseBrowserControl(prevSid, prevTid, it) }
            api.closeBrowserView(prevSid, prevTid)
        }
        controlToken = null
        sessionId = null

        val tabsRes = api.fetchBrowserTabs().getOrNull()
        if (tabsRes != null) {
            tabs = tabsRes.tabs
        }
        val chosenId = targetTabId?.takeIf { it.isNotBlank() }
            ?: tabs.firstOrNull { it.active == true }?.tabId
            ?: tabs.firstOrNull()?.tabId
        val openRes = api.openBrowserView(tabId = chosenId, leaseSeconds = 60).getOrElse { err ->
            errorMessage = err.message ?: "无法打开远程浏览器视图".loc
            isLoading = false
            return
        }
        if (!openRes.ok || openRes.sessionId.isNullOrBlank()) {
            errorMessage = openRes.error ?: "宿主浏览器未启动或无可用标签页".loc
            isLoading = false
            return
        }
        sessionId = openRes.sessionId
        val tab = openRes.tab ?: tabs.firstOrNull { it.tabId == chosenId }
        activeTab = tab
        urlInput = navigateToUrl?.takeIf { it.isNotBlank() } ?: tab?.url.orEmpty()
        isLoading = false

        if (!navigateToUrl.isNullOrBlank() && tab?.tabId != null) {
            sendAction(action = "BROWSER_VIEWER_CONTROL_ACTION_NAVIGATE", url = navigateToUrl)
        } else if (tab?.tabId != null) {
            pullSingleFrame(openRes.sessionId, tab.tabId)
        }
    }

    LaunchedEffect(Unit) {
        openTab(initialTabId, initialUrl)
    }

    // Periodic frame polling (~1.25fps) + lease renewal
    LaunchedEffect(sessionId, activeTab?.tabId) {
        val sid = sessionId ?: return@LaunchedEffect
        val tid = activeTab?.tabId ?: return@LaunchedEffect
        var tick = 0
        while (isActive) {
            delay(800)
            pullSingleFrame(sid, tid)
            tick++
            if (tick % 18 == 0) {
                api?.renewBrowserView(sid, tid, leaseSeconds = 60)
            }
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            val sid = sessionId
            val tid = activeTab?.tabId
            val tok = controlToken
            if (!sid.isNullOrBlank() && !tid.isNullOrBlank()) {
                kotlinx.coroutines.GlobalScope.launch {
                    if (!tok.isNullOrBlank()) api?.releaseBrowserControl(sid, tid, tok)
                    api?.closeBrowserView(sid, tid)
                }
            }
        }
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = colors.background
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .statusBarsPadding()
                    .navigationBarsPadding()
                    .imePadding()
            ) {
                // Top toolbar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(colors.surface)
                        .padding(horizontal = 10.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    IconButton(onClick = onDismiss, modifier = Modifier.size(34.dp)) {
                        Icon(Icons.Default.Close, contentDescription = "关闭".loc, tint = colors.textPrimary)
                    }
                    IconButton(
                        onClick = { scope.launch { sendAction("BROWSER_VIEWER_CONTROL_ACTION_BACK") } },
                        modifier = Modifier.size(32.dp)
                    ) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = null, tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                    }
                    IconButton(
                        onClick = { scope.launch { sendAction("BROWSER_VIEWER_CONTROL_ACTION_FORWARD") } },
                        modifier = Modifier.size(32.dp)
                    ) {
                        Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = null, tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                    }
                    IconButton(
                        onClick = { scope.launch { sendAction("BROWSER_VIEWER_CONTROL_ACTION_RELOAD") } },
                        modifier = Modifier.size(32.dp)
                    ) {
                        Icon(Icons.Default.Refresh, contentDescription = null, tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                    }
                    OutlinedTextField(
                        value = urlInput,
                        onValueChange = { urlInput = it },
                        singleLine = true,
                        placeholder = { Text("https://...", fontSize = 12.sp) },
                        textStyle = LocalTextStyle.current.copy(fontSize = 12.sp, fontFamily = FontFamily.Monospace),
                        modifier = Modifier.weight(1f).height(46.dp)
                    )
                    IconButton(
                        onClick = {
                            val target = urlInput.trim()
                            if (target.isNotEmpty()) {
                                val normalized = if (target.contains("://")) target else "https://$target"
                                urlInput = normalized
                                scope.launch { sendAction("BROWSER_VIEWER_CONTROL_ACTION_NAVIGATE", url = normalized) }
                            }
                        },
                        modifier = Modifier.size(34.dp)
                    ) {
                        Icon(Icons.Default.PlayArrow, contentDescription = "跳转".loc, tint = colors.accentIndigo)
                    }
                    if (tabs.size > 1) {
                        Box {
                            IconButton(onClick = { showTabMenu = true }, modifier = Modifier.size(34.dp)) {
                                Icon(Icons.Default.Tab, contentDescription = "切换标签页".loc, tint = colors.textSecondary, modifier = Modifier.size(18.dp))
                            }
                            DropdownMenu(expanded = showTabMenu, onDismissRequest = { showTabMenu = false }, containerColor = colors.surface) {
                                tabs.forEach { t ->
                                    DropdownMenuItem(
                                        text = {
                                            Text(
                                                t.displayTitle,
                                                maxLines = 1,
                                                overflow = TextOverflow.Ellipsis,
                                                fontWeight = if (t.tabId == activeTab?.tabId) FontWeight.Bold else FontWeight.Normal
                                            )
                                        },
                                        onClick = {
                                            showTabMenu = false
                                            scope.launch { openTab(t.tabId) }
                                        }
                                    )
                                }
                            }
                        }
                    }
                }

                // Status / error strip
                if (errorMessage != null || statusNotice != null) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .background(if (errorMessage != null) colors.accentRed.copy(alpha = 0.12f) else colors.accentIndigo.copy(alpha = 0.1f))
                            .padding(horizontal = 12.dp, vertical = 5.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            text = errorMessage ?: statusNotice.orEmpty(),
                            fontSize = 11.5.sp,
                            color = if (errorMessage != null) colors.accentRed else colors.accentIndigo,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }

                // Live Frame Viewport
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f)
                        .background(Color.Black),
                    contentAlignment = Alignment.Center
                ) {
                    val bmp = frameBitmap
                    if (isLoading && bmp == null) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            CircularProgressIndicator(color = colors.accentIndigo, modifier = Modifier.size(28.dp))
                            Text("正在连接宿主浏览器画面...".loc, color = Color.White.copy(alpha = 0.8f), fontSize = 13.sp)
                        }
                    } else if (bmp != null) {
                        Image(
                            bitmap = bmp,
                            contentDescription = "宿主浏览器实时画面".loc,
                            contentScale = ContentScale.Fit,
                            modifier = Modifier
                                .fillMaxSize()
                                .onSizeChanged { imageViewSize = it }
                                .pointerInput(frameWidthPx, frameHeightPx, imageViewSize, sessionId, activeTab?.tabId) {
                                    detectTapGestures { offset ->
                                        val vw = imageViewSize.width.toFloat()
                                        val vh = imageViewSize.height.toFloat()
                                        if (vw <= 0f || vh <= 0f || frameWidthPx <= 0 || frameHeightPx <= 0) return@detectTapGestures
                                        val imgAspect = frameWidthPx.toFloat() / frameHeightPx.toFloat()
                                        val viewAspect = vw / vh
                                        val (drawW, drawH, padX, padY) = if (imgAspect > viewAspect) {
                                            val dw = vw
                                            val dh = vw / imgAspect
                                            Quad(dw, dh, 0f, (vh - dh) / 2f)
                                        } else {
                                            val dh = vh
                                            val dw = vh * imgAspect
                                            Quad(dw, dh, (vw - dw) / 2f, 0f)
                                        }
                                        val rx = ((offset.x - padX) / drawW).coerceIn(0f, 1f)
                                        val ry = ((offset.y - padY) / drawH).coerceIn(0f, 1f)
                                        val px = (rx * frameWidthPx).toInt()
                                        val py = (ry * frameHeightPx).toInt()
                                        scope.launch {
                                            sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_POINTER_DOWN", xPx = px, yPx = py)
                                            sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_POINTER_UP", xPx = px, yPx = py)
                                        }
                                    }
                                }
                        )
                    } else {
                        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(errorMessage ?: "暂无浏览器画面".loc, color = Color.White.copy(alpha = 0.8f), fontSize = 13.sp)
                            OutlinedButton(onClick = { scope.launch { openTab(activeTab?.tabId) } }) {
                                Text("重新连接".loc, color = Color.White)
                            }
                        }
                    }
                }

                // Bottom Controls
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .background(colors.surface)
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        val controlling = !controlToken.isNullOrBlank()
                        FilledTonalButton(
                            onClick = {
                                scope.launch {
                                    if (controlling) {
                                        val sid = sessionId
                                        val tid = activeTab?.tabId
                                        val tok = controlToken
                                        if (sid != null && tid != null && tok != null) {
                                            api?.releaseBrowserControl(sid, tid, tok)
                                        }
                                        controlToken = null
                                        statusNotice = "已释放控制权".loc
                                    } else {
                                        ensureControlToken()
                                    }
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(32.dp)
                        ) {
                            Text(if (controlling) "释放接管".loc else "点击接管".loc, fontSize = 12.sp)
                        }
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_WHEEL", xPx = frameWidthPx / 2, yPx = frameHeightPx / 2, deltaY = -420.0)
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(32.dp)
                        ) { Text("↑ 上滚".loc, fontSize = 11.5.sp) }
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_WHEEL", xPx = frameWidthPx / 2, yPx = frameHeightPx / 2, deltaY = 420.0)
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(32.dp)
                        ) { Text("↓ 下滚".loc, fontSize = 11.5.sp) }
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_KEY_DOWN", key = "Enter")
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_KEY_UP", key = "Enter")
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(32.dp)
                        ) { Text("Enter", fontSize = 11.5.sp) }
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_KEY_DOWN", key = "Backspace")
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_KEY_UP", key = "Backspace")
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(32.dp)
                        ) { Text("⌫", fontSize = 12.sp) }
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_KEY_DOWN", key = "Escape")
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_KEY_UP", key = "Escape")
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                            modifier = Modifier.height(32.dp)
                        ) { Text("Esc", fontSize = 11.5.sp) }
                    }

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        OutlinedTextField(
                            value = textInput,
                            onValueChange = { textInput = it },
                            placeholder = { Text("输入并粘贴文本到页面焦点...".loc, fontSize = 12.5.sp) },
                            singleLine = true,
                            modifier = Modifier.weight(1f)
                        )
                        Button(
                            enabled = textInput.isNotEmpty(),
                            onClick = {
                                val txt = textInput
                                textInput = ""
                                scope.launch {
                                    sendAction("BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT", inputKind = "KIND_PASTE", text = txt)
                                }
                            }
                        ) {
                            Text("发送".loc, fontSize = 12.5.sp)
                        }
                    }
                }
            }
        }
    }
}

private data class Quad(val a: Float, val b: Float, val c: Float, val d: Float)

// endregion

// region 2. OAuthHybridDialog (Plugin & MCP OAuth Hybrid Closure)

@Composable
fun OAuthHybridDialog(
    title: String,
    startResponse: OAuthStartResponse,
    mcpServerName: String? = null,
    onDismiss: () -> Unit,
    onCompleted: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val uriHandler = LocalUriHandler.current
    val scope = rememberCoroutineScope()
    val api = ApiClient.defaultInstance

    var callbackUrlInput by remember { mutableStateOf("") }
    var mcpCodeInput by remember { mutableStateOf("") }
    var isSubmitting by remember { mutableStateOf(false) }
    var notice by remember { mutableStateOf<String?>(null) }
    var errorMsg by remember { mutableStateOf<String?>(null) }
    var showHostBrowser by remember { mutableStateOf(false) }

    val authUrl = startResponse.authUrl.orEmpty()

    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = colors.surface,
        title = {
            Text(title, fontSize = 16.5.sp, fontWeight = FontWeight.Bold, color = colors.textPrimary)
        },
        text = {
            Column(
                modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (startResponse.isLoopbackCallback) {
                    Text(
                        text = "该认证使用电脑本机回调地址 (127.0.0.1 / localhost)。推荐直接在宿主浏览器接管完成登录，或在手机浏览器登录后将回调链接粘贴回此处转发给电脑端。".loc,
                        fontSize = 12.5.sp,
                        color = colors.textSecondary,
                        lineHeight = 17.sp
                    )
                } else {
                    Text(
                        text = "请打开下方授权链接完成登录，完成后点击「完成并刷新」。".loc,
                        fontSize = 12.5.sp,
                        color = colors.textSecondary
                    )
                }

                if (authUrl.isNotBlank()) {
                    if (startResponse.isLoopbackCallback) {
                        Button(
                            onClick = { showHostBrowser = true },
                            modifier = Modifier.fillMaxWidth(),
                            shape = RoundedCornerShape(10.dp)
                        ) {
                            Text("在宿主浏览器直接接管登录".loc, fontSize = 13.sp)
                        }
                    }
                    OutlinedButton(
                        onClick = { runCatching { uriHandler.openUri(authUrl) } },
                        modifier = Modifier.fillMaxWidth(),
                        shape = RoundedCornerShape(10.dp)
                    ) {
                        Text("在手机系统浏览器打开".loc, fontSize = 13.sp)
                    }
                }

                if (startResponse.isLoopbackCallback) {
                    HorizontalDivider(color = colors.border.copy(alpha = 0.5f))
                    Text("备用：粘贴回调 URL 转发至电脑".loc, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, color = colors.textSecondary)
                    OutlinedTextField(
                        value = callbackUrlInput,
                        onValueChange = { callbackUrlInput = it },
                        placeholder = { Text("http://127.0.0.1:.../callback?code=...", fontSize = 11.5.sp) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                    FilledTonalButton(
                        enabled = !isSubmitting && callbackUrlInput.trim().startsWith("http"),
                        onClick = {
                            isSubmitting = true
                            errorMsg = null
                            scope.launch {
                                api?.forwardLoopbackOAuthCallback(callbackUrlInput.trim())
                                    ?.onSuccess { resp ->
                                        if (resp.ok) {
                                            notice = resp.message ?: "回调已成功转发至电脑端".loc
                                            onCompleted()
                                        } else {
                                            errorMsg = resp.error ?: "转发失败".loc
                                        }
                                    }
                                    ?.onFailure { errorMsg = it.message }
                                isSubmitting = false
                            }
                        },
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(if (isSubmitting) "正在转发...".loc else "转发回调完成授权".loc, fontSize = 12.5.sp)
                    }
                }

                if (!mcpServerName.isNullOrBlank()) {
                    HorizontalDivider(color = colors.border.copy(alpha = 0.5f))
                    Text("或手动提交 MCP 授权码 (Code)".loc, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, color = colors.textSecondary)
                    OutlinedTextField(
                        value = mcpCodeInput,
                        onValueChange = { mcpCodeInput = it },
                        placeholder = { Text("授权码 code".loc, fontSize = 12.sp) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                    OutlinedButton(
                        enabled = !isSubmitting && mcpCodeInput.isNotBlank(),
                        onClick = {
                            isSubmitting = true
                            errorMsg = null
                            scope.launch {
                                api?.completeMcpOAuth(mcpServerName, mcpCodeInput.trim())
                                    ?.onSuccess {
                                        notice = "MCP 授权已提交".loc
                                        onCompleted()
                                    }
                                    ?.onFailure { errorMsg = it.message }
                                isSubmitting = false
                            }
                        },
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("提交 MCP 授权码".loc, fontSize = 12.5.sp)
                    }
                }

                notice?.let { Text(it, fontSize = 12.sp, color = colors.accentGreen) }
                errorMsg?.let { Text(it, fontSize = 12.sp, color = colors.accentRed) }
            }
        },
        confirmButton = {
            TextButton(onClick = { onCompleted(); onDismiss() }) {
                Text("完成并刷新".loc, color = colors.accentIndigo, fontWeight = FontWeight.SemiBold)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("关闭".loc, color = colors.textSecondary)
            }
        }
    )

    if (showHostBrowser && authUrl.isNotBlank()) {
        BrowserViewerDialog(
            initialUrl = authUrl,
            onDismiss = {
                showHostBrowser = false
                onCompleted()
            }
        )
    }
}

// endregion

// region 3. DesktopUpdateDialog (Antigravity Desktop Self-Update & Changelog)

@Composable
fun DesktopUpdateDialog(
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    val api = ApiClient.defaultInstance

    var status by remember { mutableStateOf<DesktopUpdateStatus?>(null) }
    var changelog by remember { mutableStateOf<String?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var isApplying by remember { mutableStateOf(false) }
    var showConfirmApply by remember { mutableStateOf(false) }
    var resultNotice by remember { mutableStateOf<String?>(null) }
    var errorMsg by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) {
        isLoading = true
        api?.fetchDesktopUpdateStatus()?.onSuccess { status = it }?.onFailure { errorMsg = it.message }
        api?.fetchDesktopChangelog()?.onSuccess { changelog = it.markdown }
        isLoading = false
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = colors.surface,
        title = {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text("Antigravity 桌面端更新".loc, fontSize = 16.5.sp, fontWeight = FontWeight.Bold, color = colors.textPrimary)
                val cur = status?.currentVersion?.takeIf { it.isNotBlank() } ?: "-"
                val latest = status?.latestVersion?.takeIf { it.isNotBlank() } ?: cur
                Text(
                    text = if (status?.updateAvailable == true) "v$cur → v$latest" else "${"当前版本".loc}: v$cur",
                    fontSize = 12.sp,
                    fontFamily = FontFamily.Monospace,
                    color = colors.textSecondary
                )
            }
        },
        text = {
            Column(
                modifier = Modifier.fillMaxWidth().heightIn(max = 320.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                if (isLoading) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                        Text("正在检查桌面端版本与更新日志...".loc, fontSize = 13.sp, color = colors.textSecondary)
                    }
                } else {
                    if ((status?.runningCount ?: 0) > 0 && status?.updateAvailable == true) {
                        Text(
                            text = "注意：电脑端当前有 %d 个活跃会话运行中，应用更新会重启 Antigravity 桌面客户端。".loc(status?.runningCount ?: 0),
                            fontSize = 12.sp,
                            color = colors.accentOrange
                        )
                    }
                    resultNotice?.let { Text(it, fontSize = 12.5.sp, color = colors.accentGreen) }
                    errorMsg?.let { Text(it, fontSize = 12.5.sp, color = colors.accentRed) }
                    val md = changelog?.trim().orEmpty()
                    if (md.isNotEmpty()) {
                        MarkdownContentView(content = md)
                    } else if (status?.updateAvailable != true) {
                        Text("电脑端 Antigravity 已是最新版本。".loc, fontSize = 13.sp, color = colors.textSecondary)
                    }
                }
            }
        },
        confirmButton = {
            if (status?.updateAvailable == true && resultNotice == null) {
                Button(
                    enabled = !isApplying,
                    onClick = { showConfirmApply = true },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentIndigo)
                ) {
                    Text(if (isApplying) "正在触发更新...".loc else "立即更新桌面端".loc)
                }
            } else {
                TextButton(onClick = onDismiss) {
                    Text("关闭".loc, color = colors.accentIndigo)
                }
            }
        },
        dismissButton = {
            if (status?.updateAvailable == true && resultNotice == null) {
                TextButton(onClick = onDismiss) {
                    Text("稍后".loc, color = colors.textSecondary)
                }
            }
        }
    )

    if (showConfirmApply) {
        AlertDialog(
            onDismissRequest = { showConfirmApply = false },
            containerColor = colors.surface,
            title = { Text("确认重启并更新桌面端？".loc, fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = {
                Text(
                    "更新期间电脑端 Antigravity 将短暂重启，移动端网关会在重启完成后自动重连。".loc,
                    fontSize = 13.sp,
                    color = colors.textSecondary
                )
            },
            confirmButton = {
                Button(
                    onClick = {
                        showConfirmApply = false
                        isApplying = true
                        errorMsg = null
                        scope.launch {
                            api?.applyDesktopUpdate()
                                ?.onSuccess { resp ->
                                    if (resp.ok) resultNotice = resp.message ?: "已下发桌面端更新指令".loc
                                    else errorMsg = resp.error ?: "触发更新失败".loc
                                }
                                ?.onFailure { errorMsg = it.message }
                            isApplying = false
                        }
                    },
                    colors = ButtonDefaults.buttonColors(containerColor = colors.accentOrange)
                ) {
                    Text("确认更新".loc)
                }
            },
            dismissButton = {
                TextButton(onClick = { showConfirmApply = false }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            }
        )
    }
}

// endregion

// region 4. WorkspaceSearchTab (Embedded inside CascadeChangesSheet)

enum class WorkspaceSearchMode { CODE, FILES }

@Composable
fun WorkspaceSearchTab(
    cascadeId: String,
    mode: WorkspaceSearchMode,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    val api = ApiClient.defaultInstance

    var query by remember { mutableStateOf("") }
    var isRegex by remember { mutableStateOf(false) }
    var caseSensitive by remember { mutableStateOf(false) }
    var isSearching by remember { mutableStateOf(false) }
    var codeHits by remember { mutableStateOf<List<CodeSearchHit>>(emptyList()) }
    var fileHits by remember { mutableStateOf<List<FileSearchHit>>(emptyList()) }
    var errorMsg by remember { mutableStateOf<String?>(null) }

    var previewTitle by remember { mutableStateOf<String?>(null) }
    var previewContent by remember { mutableStateOf<String?>(null) }
    var previewHighlightLine by remember { mutableStateOf<Int?>(null) }
    var previewLoading by remember { mutableStateOf(false) }

    fun runSearch() {
        val q = query.trim()
        if (q.isEmpty() || cascadeId.isBlank()) return
        scope.launch {
            isSearching = true
            errorMsg = null
            if (mode == WorkspaceSearchMode.CODE) {
                api?.searchWorkspaceCode(cascadeId, q, isRegex = isRegex, caseSensitive = caseSensitive)
                    ?.onSuccess { codeHits = it }
                    ?.onFailure { errorMsg = it.message }
            } else {
                api?.searchWorkspaceFiles(cascadeId, q)
                    ?.onSuccess { fileHits = it }
                    ?.onFailure { errorMsg = it.message }
            }
            isSearching = false
        }
    }

    fun openPreview(title: String, uri: String, line: Int?) {
        previewTitle = title
        previewHighlightLine = line
        previewContent = null
        previewLoading = true
        scope.launch {
            api?.fetchFileContent(uri, cascadeId)
                ?.onSuccess { previewContent = it.content }
                ?.onFailure { previewContent = "读取文件失败: ${it.message.orEmpty()}" }
            previewLoading = false
        }
    }

    Column(modifier = modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            OutlinedTextField(
                value = query,
                onValueChange = { query = it },
                placeholder = {
                    Text(
                        if (mode == WorkspaceSearchMode.CODE) "输入代码关键字或正则...".loc else "输入文件名关键字...".loc,
                        fontSize = 13.sp
                    )
                },
                singleLine = true,
                modifier = Modifier.weight(1f)
            )
            Button(
                enabled = query.isNotBlank() && !isSearching,
                onClick = { runSearch() },
                shape = RoundedCornerShape(10.dp),
                contentPadding = PaddingValues(horizontal = 14.dp, vertical = 10.dp)
            ) {
                Text("搜索".loc, fontSize = 13.sp)
            }
        }

        if (mode == WorkspaceSearchMode.CODE) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = isRegex,
                    onClick = { isRegex = !isRegex },
                    label = { Text("正则 .*".loc, fontSize = 11.5.sp) }
                )
                FilterChip(
                    selected = caseSensitive,
                    onClick = { caseSensitive = !caseSensitive },
                    label = { Text("区分大小写 Aa".loc, fontSize = 11.5.sp) }
                )
            }
        }

        if (isSearching) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(vertical = 20.dp),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                Spacer(Modifier.width(8.dp))
                Text("正在检索工作区...".loc, fontSize = 13.sp, color = colors.textSecondary)
            }
        } else if (errorMsg != null) {
            Text(errorMsg.orEmpty(), fontSize = 12.5.sp, color = colors.accentRed)
        } else if (mode == WorkspaceSearchMode.CODE) {
            if (codeHits.isEmpty()) {
                Text(
                    text = if (query.isBlank()) "输入关键词检索工作区代码".loc else "未找到匹配的代码片段".loc,
                    fontSize = 13.sp,
                    color = colors.textMuted,
                    modifier = Modifier.padding(top = 12.dp)
                )
            } else {
                LazyColumn(
                    modifier = Modifier.fillMaxWidth().weight(1f),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    items(codeHits, key = { it.stableId }) { hit ->
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(colors.surfaceVariant)
                                .clickable { openPreview(hit.relativePath.ifBlank { hit.path }, hit.uri, hit.lineNumber) }
                                .padding(10.dp),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Text(
                                    text = hit.relativePath.ifBlank { hit.path },
                                    fontSize = 12.sp,
                                    fontWeight = FontWeight.SemiBold,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.accentIndigo,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f)
                                )
                                Text(
                                    text = "L${hit.lineNumber}",
                                    fontSize = 11.sp,
                                    fontFamily = FontFamily.Monospace,
                                    color = colors.textMuted
                                )
                            }
                            Text(
                                text = hit.snippet.trim(),
                                fontSize = 11.5.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textPrimary,
                                maxLines = 3,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                }
            }
        } else {
            if (fileHits.isEmpty()) {
                Text(
                    text = if (query.isBlank()) "输入文件名快速定位工作区文件".loc else "未找到匹配的文件".loc,
                    fontSize = 13.sp,
                    color = colors.textMuted,
                    modifier = Modifier.padding(top = 12.dp)
                )
            } else {
                LazyColumn(
                    modifier = Modifier.fillMaxWidth().weight(1f),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    items(fileHits, key = { it.path.ifBlank { it.uri } }) { hit ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(10.dp))
                                .background(colors.surfaceVariant)
                                .clickable(enabled = !hit.isDirectory) {
                                    openPreview(hit.relativePath.ifBlank { hit.path }, hit.uri, null)
                                }
                                .padding(horizontal = 12.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Icon(
                                imageVector = if (hit.isDirectory) Icons.Default.Folder else Icons.Default.Description,
                                contentDescription = null,
                                tint = if (hit.isDirectory) colors.accentOrange else colors.accentIndigo,
                                modifier = Modifier.size(16.dp)
                            )
                            Text(
                                text = hit.relativePath.ifBlank { hit.path },
                                fontSize = 12.5.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textPrimary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier.weight(1f)
                            )
                        }
                    }
                }
            }
        }
    }

    previewTitle?.let { title ->
        AlertDialog(
            onDismissRequest = { previewTitle = null },
            containerColor = colors.surface,
            title = {
                Text(title, fontSize = 14.5.sp, fontWeight = FontWeight.Bold, fontFamily = FontFamily.Monospace, maxLines = 2, overflow = TextOverflow.Ellipsis)
            },
            text = {
                if (previewLoading) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                        Text("正在加载文件...".loc, fontSize = 13.sp)
                    }
                } else {
                    val lines = (previewContent ?: "").lines()
                    val targetLine = previewHighlightLine
                    val startIdx = if (targetLine != null && lines.size > 160) (targetLine - 40).coerceAtLeast(0) else 0
                    val endIdx = (startIdx + 240).coerceAtMost(lines.size)
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(max = 380.dp)
                            .verticalScroll(rememberScrollState())
                            .horizontalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(2.dp)
                    ) {
                        for (i in startIdx until endIdx) {
                            val lineNo = i + 1
                            val isHit = targetLine == lineNo
                            Text(
                                text = "${lineNo.toString().padStart(4)}  ${lines[i]}",
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = if (isHit) colors.accentOrange else colors.textPrimary,
                                fontWeight = if (isHit) FontWeight.Bold else FontWeight.Normal,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .background(if (isHit) colors.accentOrange.copy(alpha = 0.16f) else Color.Transparent)
                            )
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = { previewTitle = null }) {
                    Text("关闭".loc, color = colors.accentIndigo)
                }
            }
        )
    }
}

// endregion

// region 5. ArtifactSnapshotsDialog

@Composable
fun ArtifactSnapshotsDialog(
    snapshots: List<ArtifactSnapshotItem>,
    onSelectSnapshot: (ArtifactSnapshotItem) -> Unit,
    onDismiss: () -> Unit
) {
    val colors = AntigravityTheme.colors
    var selectedIdx by remember(snapshots) { mutableIntStateOf((snapshots.size - 1).coerceAtLeast(0)) }
    val current = snapshots.getOrNull(selectedIdx)

    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = colors.surface,
        title = {
            Text("历史版本快照 (%d)".loc(snapshots.size), fontSize = 16.sp, fontWeight = FontWeight.Bold)
        },
        text = {
            Column(
                modifier = Modifier.fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    snapshots.forEachIndexed { idx, snap ->
                        val selected = idx == selectedIdx
                        FilterChip(
                            selected = selected,
                            onClick = { selectedIdx = idx },
                            label = { Text("v${idx + 1}", fontSize = 12.sp) }
                        )
                    }
                }
                if (current != null) {
                    current.lastEdited?.takeIf { it.isNotBlank() }?.let {
                        Text(it, fontSize = 11.sp, fontFamily = FontFamily.Monospace, color = colors.textMuted)
                    }
                    current.summary?.takeIf { it.isNotBlank() }?.let {
                        Text(it, fontSize = 12.sp, color = colors.textSecondary)
                    }
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(max = 300.dp)
                            .clip(RoundedCornerShape(10.dp))
                            .background(colors.background)
                            .padding(10.dp)
                            .verticalScroll(rememberScrollState())
                    ) {
                        MarkdownContentView(content = current.content)
                    }
                }
            }
        },
        confirmButton = {
            if (current != null) {
                TextButton(onClick = { onSelectSnapshot(current); onDismiss() }) {
                    Text("在当前视图查看".loc, color = colors.accentIndigo, fontWeight = FontWeight.SemiBold)
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("关闭".loc, color = colors.textSecondary)
            }
        }
    )
}

// endregion

// region 6. RemoteSshSection (Embedded inside NewConversationSheet)

@Composable
fun RemoteSshSection(
    onConnected: () -> Unit,
    modifier: Modifier = Modifier
) {
    val colors = AntigravityTheme.colors
    val scope = rememberCoroutineScope()
    val api = ApiClient.defaultInstance

    var remotes by remember { mutableStateOf<List<RemoteHostDescriptor>>(emptyList()) }
    var connectedIds by remember { mutableStateOf<Set<String>>(emptySet()) }
    var isLoading by remember { mutableStateOf(false) }
    var activeJob by remember { mutableStateOf<RemoteConnectJob?>(null) }
    var promptInput by remember { mutableStateOf("") }
    var showAddHostDialog by remember { mutableStateOf(false) }
    var newName by remember { mutableStateOf("") }
    var newHost by remember { mutableStateOf("") }
    var newUser by remember { mutableStateOf("") }
    var newPort by remember { mutableStateOf("22") }
    var statusNotice by remember { mutableStateOf<String?>(null) }

    fun refreshRemotes() {
        scope.launch {
            isLoading = true
            api?.fetchRemotes()?.onSuccess { resp ->
                remotes = resp.remotes
                connectedIds = resp.connected.mapTo(HashSet()) { it.stableId }
            }
            isLoading = false
        }
    }

    LaunchedEffect(Unit) {
        refreshRemotes()
    }

    // Poll active SSH connection job while connecting/prompt
    LaunchedEffect(activeJob?.jobId) {
        val jid = activeJob?.jobId ?: return@LaunchedEffect
        while (isActive) {
            val cur = activeJob ?: break
            if (cur.done || cur.status == "connected" || cur.status == "error") break
            delay(900)
            api?.fetchRemoteConnectStatus(jid)?.onSuccess { resp ->
                resp.job?.let { updated ->
                    activeJob = updated
                    if (updated.status == "connected") {
                        statusNotice = "已连接远程主机：%s".loc(updated.name ?: updated.remoteId.orEmpty())
                        refreshRemotes()
                        onConnected()
                    }
                }
            }
        }
    }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(top = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                text = "SSH 远端主机".loc,
                fontSize = 13.sp,
                fontWeight = FontWeight.SemiBold,
                color = colors.textSecondary
            )
            TextButton(
                onClick = { showAddHostDialog = true },
                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
            ) {
                Text("+ 添加主机".loc, fontSize = 12.sp, color = colors.accentIndigo)
            }
        }

        statusNotice?.let {
            Text(it, fontSize = 12.sp, color = colors.accentGreen, modifier = Modifier.padding(horizontal = 4.dp))
        }

        if (isLoading && remotes.isEmpty()) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
                horizontalArrangement = Arrangement.Center
            ) {
                CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
            }
        } else if (remotes.isEmpty()) {
            Text(
                text = "暂未配置 SSH 远端主机，可点击「+ 添加主机」连接远端工作区。".loc,
                fontSize = 12.sp,
                color = colors.textMuted,
                modifier = Modifier.padding(horizontal = 4.dp)
            )
        } else {
            remotes.forEach { remote ->
                val isConnected = connectedIds.contains(remote.stableId)
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(12.dp))
                        .background(colors.surfaceVariant)
                        .clickable {
                            val hop = remote.remoteMultiHop?.hops?.firstOrNull()
                            scope.launch {
                                api?.connectRemoteHost(
                                    name = remote.displayName,
                                    host = hop?.host,
                                    user = hop?.user,
                                    port = hop?.port,
                                    remoteId = remote.remoteId
                                )?.onSuccess { resp ->
                                    activeJob = resp.job
                                }?.onFailure { err ->
                                    statusNotice = "连接失败: ${err.message.orEmpty()}"
                                }
                            }
                        }
                        .padding(horizontal = 12.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Box(
                        modifier = Modifier
                            .size(36.dp)
                            .clip(CircleShape)
                            .background((if (isConnected) colors.accentGreen else colors.accentBlue).copy(alpha = 0.14f)),
                        contentAlignment = Alignment.Center
                    ) {
                        Icon(
                            imageVector = Icons.Default.Terminal,
                            contentDescription = null,
                            tint = if (isConnected) colors.accentGreen else colors.accentBlue,
                            modifier = Modifier.size(18.dp)
                        )
                    }
                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(
                            text = remote.displayName,
                            fontSize = 14.sp,
                            fontWeight = FontWeight.SemiBold,
                            color = colors.textPrimary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        if (remote.hostSummary.isNotBlank()) {
                            Text(
                                text = remote.hostSummary,
                                fontSize = 11.sp,
                                fontFamily = FontFamily.Monospace,
                                color = colors.textSecondary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                    if (isConnected) {
                        TextButton(
                            onClick = {
                                scope.launch {
                                    api?.disconnectRemoteHost(remote.remoteId ?: remote.stableId, remote.name)
                                    refreshRemotes()
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)
                        ) {
                            Text("断开".loc, fontSize = 11.5.sp, color = colors.accentOrange)
                        }
                    } else {
                        Text("连接".loc, fontSize = 12.sp, fontWeight = FontWeight.Medium, color = colors.accentIndigo)
                    }
                }
            }
        }
    }

    // Active SSH Connect / Interactive Prompt Dialog
    activeJob?.let { job ->
        AlertDialog(
            onDismissRequest = { if (job.done || job.status == "error" || job.status == "connected") activeJob = null },
            containerColor = colors.surface,
            title = {
                Text(
                    text = "SSH 连接 · %s".loc(job.name ?: job.remoteId.orEmpty()),
                    fontSize = 16.sp,
                    fontWeight = FontWeight.Bold
                )
            },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    when (job.status) {
                        "prompt" -> {
                            Text(
                                text = job.prompt?.message?.ifBlank { "请输入认证信息".loc } ?: "请输入认证信息".loc,
                                fontSize = 13.sp,
                                color = colors.textPrimary
                            )
                            OutlinedTextField(
                                value = promptInput,
                                onValueChange = { promptInput = it },
                                singleLine = true,
                                visualTransformation = if (job.prompt?.isSecret == true) PasswordVisualTransformation() else VisualTransformation.None,
                                placeholder = { Text(if (job.prompt?.isSecret == true) "密码 / 密钥口令".loc else "输入回复...".loc) },
                                modifier = Modifier.fillMaxWidth()
                            )
                        }
                        "connected" -> {
                            Text("连接成功，工作区已同步。".loc, fontSize = 13.sp, color = colors.accentGreen)
                        }
                        "error" -> {
                            Text(job.error ?: "SSH 连接失败".loc, fontSize = 13.sp, color = colors.accentRed)
                        }
                        else -> {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = colors.accentIndigo)
                                Text(job.message?.ifBlank { "正在建立 SSH 隧道...".loc } ?: "正在建立 SSH 隧道...".loc, fontSize = 13.sp)
                            }
                        }
                    }
                }
            },
            confirmButton = {
                if (job.status == "prompt") {
                    Button(
                        onClick = {
                            val ans = promptInput
                            promptInput = ""
                            scope.launch {
                                api?.respondRemotePrompt(job.jobId, ans)
                                activeJob = job.copy(status = "connecting", message = "正在校验...".loc)
                            }
                        }
                    ) { Text("提交".loc) }
                } else {
                    TextButton(onClick = { activeJob = null }) {
                        Text("关闭".loc, color = colors.accentIndigo)
                    }
                }
            },
            dismissButton = {
                if (job.status == "prompt") {
                    TextButton(onClick = { activeJob = null }) {
                        Text("取消".loc, color = colors.textSecondary)
                    }
                }
            }
        )
    }

    // Add SSH Host Dialog
    if (showAddHostDialog) {
        AlertDialog(
            onDismissRequest = { showAddHostDialog = false },
            containerColor = colors.surface,
            title = { Text("添加 SSH 远端主机".loc, fontSize = 16.sp, fontWeight = FontWeight.Bold) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(value = newName, onValueChange = { newName = it }, label = { Text("主机名称".loc) }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(value = newHost, onValueChange = { newHost = it }, label = { Text("主机地址 (IP / 域名)".loc) }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedTextField(value = newUser, onValueChange = { newUser = it }, label = { Text("用户名".loc) }, singleLine = true, modifier = Modifier.weight(1f))
                        OutlinedTextField(value = newPort, onValueChange = { newPort = it }, label = { Text("端口".loc) }, singleLine = true, modifier = Modifier.width(90.dp))
                    }
                }
            },
            confirmButton = {
                Button(
                    enabled = newHost.isNotBlank(),
                    onClick = {
                        val h = newHost.trim()
                        val n = newName.trim().ifBlank { h }
                        val u = newUser.trim().ifBlank { null }
                        val p = newPort.trim().toIntOrNull() ?: 22
                        showAddHostDialog = false
                        newName = ""; newHost = ""; newUser = ""; newPort = "22"
                        scope.launch {
                            api?.saveRemoteHost(n, h, u, p)?.onSuccess { refreshRemotes() }
                        }
                    }
                ) { Text("保存".loc) }
            },
            dismissButton = {
                TextButton(onClick = { showAddHostDialog = false }) {
                    Text("取消".loc, color = colors.textSecondary)
                }
            }
        )
    }
}

// endregion
