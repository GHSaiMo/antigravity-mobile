package com.antigravity.mobile

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedContentTransitionScope
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.core.content.ContextCompat
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.CacheManager
import com.antigravity.mobile.data.service.ConnectionManager
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.StreamWebSocketClient
import com.antigravity.mobile.ui.screen.ChatScreen
import com.antigravity.mobile.ui.screen.ConversationListScreen
import com.antigravity.mobile.ui.screen.PairingScreen
import com.antigravity.mobile.ui.theme.AntigravityTheme
import com.antigravity.mobile.ui.viewmodel.ChatViewModel
import com.antigravity.mobile.ui.viewmodel.ConversationListViewModel
import com.antigravity.mobile.ui.viewmodel.PairingViewModel
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import java.net.URLDecoder
import java.net.URLEncoder
import com.antigravity.mobile.ui.viewmodel.prepareSession
import com.antigravity.mobile.ui.viewmodel.addAttachmentsFromUris
import com.antigravity.mobile.data.service.ShareInbox
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.ui.components.ShareTargetSheet
import androidx.core.content.IntentCompat
import androidx.lifecycle.lifecycleScope
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.collectAsState
import androidx.compose.ui.unit.dp
import android.net.Uri
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainActivity : ComponentActivity() {

    private lateinit var prefs: PreferencesManager
    private lateinit var connectionManager: ConnectionManager
    private lateinit var apiClient: ApiClient
    private lateinit var wsClient: StreamWebSocketClient

    private lateinit var pairingViewModel: PairingViewModel
    private lateinit var conversationListViewModel: ConversationListViewModel
    private lateinit var chatViewModel: ChatViewModel
    private lateinit var liveActivityManager: com.antigravity.mobile.data.service.LiveActivityNotificationManager

    private lateinit var qrScanLauncher: ActivityResultLauncher<ScanOptions>
    private lateinit var notificationPermissionLauncher: ActivityResultLauncher<String>
    private var activeNavController: NavHostController? = null
    private var pendingCascadeId: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)

        // Initialize Services
        prefs = PreferencesManager(applicationContext)
        connectionManager = ConnectionManager(applicationContext)
        connectionManager.startMonitoring(prefs)
        apiClient = ApiClient(applicationContext, prefs, connectionManager)
        wsClient = StreamWebSocketClient(prefs, connectionManager)
        val cacheManager = CacheManager(applicationContext)
        val documentCacheManager = com.antigravity.mobile.data.service.DocumentCacheManager(applicationContext)
        liveActivityManager = com.antigravity.mobile.data.service.LiveActivityNotificationManager(applicationContext, prefs)

        // Initialize Coil SVG and Gateway Auth Header interceptor globally (sharing core connection pool)
        val coilOkHttpClient = apiClient.okHttpClient.newBuilder()
            .addInterceptor { chain ->
                val request = chain.request()
                val token = prefs.deviceToken
                val path = request.url.encodedPath
                val isGatewayRequest = path.startsWith("/api/v1/files/raw") || path.startsWith("/static/")
                if (!token.isNullOrBlank() && isGatewayRequest && request.header("Authorization") == null) {
                    val newRequest = request.newBuilder()
                        .header("Authorization", "Bearer $token")
                        .header("x-device-token", token)
                        .build()
                    chain.proceed(newRequest)
                } else {
                    chain.proceed(request)
                }
            }
            .build()

        coil.Coil.setImageLoader(
            coil.ImageLoader.Builder(this)
                .okHttpClient(coilOkHttpClient)
                .respectCacheHeaders(false)
                .memoryCache {
                    coil.memory.MemoryCache.Builder(this)
                        .maxSizePercent(0.20)
                        .build()
                }
                .diskCache {
                    coil.disk.DiskCache.Builder()
                        .directory(cacheDir.resolve("image_cache"))
                        .maxSizeBytes(100L * 1024L * 1024L)
                        .build()
                }
                .components {
                    add(coil.decode.SvgDecoder.Factory())
                }
                .build()
        )

        // Initialize ViewModels
        pairingViewModel = PairingViewModel(apiClient, prefs)
        conversationListViewModel = ConversationListViewModel(apiClient, prefs, cacheManager, liveActivityManager)
        pairingViewModel.setPreheatAction {
            conversationListViewModel.loadInitialData()
        }
        chatViewModel = ChatViewModel(apiClient, wsClient, prefs, documentCacheManager, liveActivityManager, cacheManager).apply {
            onConversationUpdated = { item ->
                conversationListViewModel.upsertConversation(item)
            }
        }

        // Register ZXing Scanner
        qrScanLauncher = registerForActivityResult(ScanContract()) { result ->
            if (result.contents != null) {
                val scannedContent = result.contents
                pairingViewModel.pairWithUri(scannedContent)
            } else {
                Toast.makeText(this, "已取消扫码", Toast.LENGTH_SHORT).show()
            }
        }

        // Register Notification Permission Launcher
        notificationPermissionLauncher = registerForActivityResult(
            ActivityResultContracts.RequestPermission()
        ) { isGranted ->
            if (!isGranted) {
                Log.d("MainActivity", "POST_NOTIFICATIONS permission not granted by user")
            }
        }
        checkNotificationPermission()

        // Handle DeepLink if opened from URL
        handleDeepLink(intent)

        // Files shared from other apps; skipped on recreation so the same intent is not staged twice.
        if (savedInstanceState == null) {
            ShareInbox.cleanupStale(applicationContext)
            handleShareIntent(intent)
        }

        setContent {
            val themeMode by prefs.themeModeFlow.collectAsState()
            AntigravityTheme(themeMode = themeMode) {
                val navController = rememberNavController()
                activeNavController = navController

                LaunchedEffect(navController) {
                    pendingCascadeId?.let { cid ->
                        pendingCascadeId = null
                        navigateToCascade(cid)
                    }
                }

                val startDestination = if (prefs.isPaired()) "conversations" else "pair"

                androidx.compose.foundation.layout.Box(androidx.compose.ui.Modifier.fillMaxSize()) {
                NavHost(
                    navController = navController,
                    startDestination = startDestination,
                    enterTransition = {
                        slideIntoContainer(
                            AnimatedContentTransitionScope.SlideDirection.Left,
                            animationSpec = spring(
                                dampingRatio = 0.82f,
                                stiffness = Spring.StiffnessMediumLow
                            )
                        )
                    },
                    exitTransition = {
                        slideOutOfContainer(
                            AnimatedContentTransitionScope.SlideDirection.Left,
                            animationSpec = spring(
                                dampingRatio = 0.82f,
                                stiffness = Spring.StiffnessMediumLow
                            )
                        )
                    },
                    popEnterTransition = {
                        slideIntoContainer(
                            AnimatedContentTransitionScope.SlideDirection.Right,
                            animationSpec = spring(
                                dampingRatio = 0.82f,
                                stiffness = Spring.StiffnessMediumLow
                            )
                        )
                    },
                    popExitTransition = {
                        slideOutOfContainer(
                            AnimatedContentTransitionScope.SlideDirection.Right,
                            animationSpec = spring(
                                dampingRatio = 0.82f,
                                stiffness = Spring.StiffnessMediumLow
                            )
                        )
                    }
                ) {
                    composable("pair") {
                        PairingScreen(
                            viewModel = pairingViewModel,
                            onLaunchScanner = { launchScanner() },
                            onPairedSuccess = {
                                conversationListViewModel.startAutoRefresh()
                                navController.navigate("conversations") {
                                    popUpTo("pair") { inclusive = true }
                                }
                            }
                        )
                    }

                    composable("conversations") {
                        ConversationListScreen(
                            viewModel = conversationListViewModel,
                            onSelectConversation = { cascadeId, title, isNew, isUnread, status, lastModifiedTime ->
                                conversationListViewModel.notifySessionFocus(cascadeId)
                                val wsName = conversationListViewModel.getWorkspaceName(cascadeId)
                                val draftProject = conversationListViewModel.getDraftProject(cascadeId)
                                chatViewModel.prepareSession(
                                    cascadeId = cascadeId,
                                    initialTitle = title,
                                    isNewConversation = isNew,
                                    workspaceName = wsName,
                                    isUnread = isUnread,
                                    conversationStatus = status,
                                    draftProject = draftProject,
                                    lastModifiedTime = lastModifiedTime
                                )
                                conversationListViewModel.markConversationAsRead(cascadeId)
                                val encodedTitle = URLEncoder.encode(title, "UTF-8")
                                navController.navigate("chat/$cascadeId/$encodedTitle?isNew=$isNew&isUnread=$isUnread&status=${status.name}")
                            },
                            onNavigateToPair = {
                                wsClient.disconnect(intentional = true)
                                pairingViewModel.resetState()
                                conversationListViewModel.unpair {
                                    navController.navigate("pair") {
                                        popUpTo(0) { inclusive = true }
                                    }
                                }
                            }
                        )
                    }

                    composable(
                        route = "chat/{cascadeId}/{title}?isNew={isNew}&isUnread={isUnread}&status={status}",
                        arguments = listOf(
                            navArgument("cascadeId") { type = NavType.StringType },
                            navArgument("title") { type = NavType.StringType },
                            navArgument("isNew") {
                                type = NavType.BoolType
                                defaultValue = false
                            },
                            navArgument("isUnread") {
                                type = NavType.BoolType
                                defaultValue = false
                            },
                            navArgument("status") {
                                type = NavType.StringType
                                defaultValue = ""
                            }
                        )
                    ) { backStackEntry ->
                        val cascadeId = backStackEntry.arguments?.getString("cascadeId") ?: ""
                        val rawTitle = backStackEntry.arguments?.getString("title") ?: ""
                        val isNew = backStackEntry.arguments?.getBoolean("isNew") ?: false
                        val isUnread = backStackEntry.arguments?.getBoolean("isUnread") ?: false
                        val statusRaw = backStackEntry.arguments?.getString("status") ?: ""
                        val status = try {
                            if (statusRaw.isNotBlank()) com.antigravity.mobile.data.model.ConversationStatus.valueOf(statusRaw) else null
                        } catch (_: Exception) { null }
                        val title = URLDecoder.decode(rawTitle, "UTF-8")

                        ChatScreen(
                            cascadeId = cascadeId,
                            initialTitle = title,
                            isNewConversation = isNew,
                            isUnreadOnEntry = isUnread,
                            initialStatus = status,
                            viewModel = chatViewModel,
                            onNavigateBack = {
                                conversationListViewModel.reloadFromCache()
                                navController.popBackStack()
                            }
                        )
                    }
                }
                ShareOverlay(navController)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        intent?.let {
            handleDeepLink(it)
            handleShareIntent(it)
        }
    }

    /** Stages files received through SEND / SEND_MULTIPLE / VIEW and opens the destination sheet. */
    private fun handleShareIntent(intent: Intent?) {
        intent ?: return
        val uris = mutableListOf<Uri>()
        when (intent.action) {
            Intent.ACTION_SEND ->
                IntentCompat.getParcelableExtra(intent, Intent.EXTRA_STREAM, Uri::class.java)?.let { uris.add(it) }
            Intent.ACTION_SEND_MULTIPLE ->
                IntentCompat.getParcelableArrayListExtra(intent, Intent.EXTRA_STREAM, Uri::class.java)?.let { uris.addAll(it) }
            Intent.ACTION_VIEW -> intent.data?.let { uris.add(it) }
            else -> return
        }
        if (uris.isEmpty()) {
            intent.clipData?.let { clip -> for (i in 0 until clip.itemCount) clip.getItemAt(i).uri?.let { uris.add(it) } }
        }
        // Only content:// is accepted; file:// could point into this app's own private storage.
        val safe = uris.filter { it.scheme == "content" }
        // Handled: do not re-process this intent on configuration change.
        setIntent(Intent(this, MainActivity::class.java))
        if (safe.isEmpty()) {
            if (intent.action != Intent.ACTION_VIEW) {
                Toast.makeText(this, "暂只支持接收文件", Toast.LENGTH_SHORT).show()
            }
            return
        }
        if (!prefs.isPaired()) {
            Toast.makeText(this, "请先完成配对，再分享文件到 Multigravity", Toast.LENGTH_LONG).show()
            return
        }
        lifecycleScope.launch(Dispatchers.IO) {
            val staged = ShareInbox.stage(applicationContext, safe)
            if (staged == 0) withContext(Dispatchers.Main) {
                Toast.makeText(this@MainActivity, "无法读取分享的文件", Toast.LENGTH_SHORT).show()
            }
        }
    }

    /** Destination sheet while files are staged; a small pill on the list when the sheet was dismissed. */
    @androidx.compose.runtime.Composable
    private fun ShareOverlay(navController: NavHostController) {
        val files by ShareInbox.files.collectAsState()
        val sheetVisible by ShareInbox.sheetVisible.collectAsState()
        val projects by conversationListViewModel.projects.collectAsState()
        val chatState by chatViewModel.uiState.collectAsState()
        val backStack by navController.currentBackStackEntryAsState()
        val route = backStack?.destination?.route
        if (files.isEmpty() || !prefs.isPaired()) return
        val inChat = route?.startsWith("chat/") == true

        if (sheetVisible) {
            ShareTargetSheet(
                files = files,
                projects = projects,
                conversations = conversationListViewModel.allConversations(),
                currentConversationId = if (inChat) chatState.cascadeId.takeIf { it.isNotBlank() } else null,
                onRefreshProjects = { conversationListViewModel.loadProjects() },
                onRemoveFile = { ShareInbox.remove(applicationContext, it) },
                onSelectProject = { project ->
                    val draft = conversationListViewModel.createLocalDraftSession(project)
                    val title = if (project.isPureChat) "新对话" else project.displayName
                    openChatForShare(draft.id, title, true, ConversationStatus.IDLE, null)
                },
                onSelectConversation = { item ->
                    openChatForShare(item.id, item.displayTitle, false, item.status, item.lastModifiedTime)
                },
                onSelectCurrent = {
                    chatViewModel.addAttachmentsFromUris(this@MainActivity, ShareInbox.deliverableUris(this@MainActivity))
                    ShareInbox.release()
                },
                onDiscard = { ShareInbox.clear(applicationContext) },
                onDismiss = { ShareInbox.hideSheet() }
            )
        } else if (route == "conversations") {
            androidx.compose.foundation.layout.Box(
                modifier = androidx.compose.ui.Modifier
                    .fillMaxSize()
                    .navigationBarsPadding()
                    .padding(bottom = 20.dp),
                contentAlignment = androidx.compose.ui.Alignment.BottomCenter
            ) {
                androidx.compose.material3.Surface(
                    onClick = { ShareInbox.showSheet() },
                    shape = androidx.compose.foundation.shape.RoundedCornerShape(24.dp),
                    color = androidx.compose.material3.MaterialTheme.colorScheme.primary,
                    shadowElevation = 6.dp
                ) {
                    androidx.compose.material3.Text(
                        "${files.size} 个文件待投递 · 选择去向",
                        color = androidx.compose.material3.MaterialTheme.colorScheme.onPrimary,
                        modifier = androidx.compose.ui.Modifier.padding(horizontal = 18.dp, vertical = 10.dp)
                    )
                }
            }
        }
    }

    /** Opens a conversation (leaving any chat that is currently open) and drops the staged files into its input bar. */
    private fun openChatForShare(
        cascadeId: String,
        title: String,
        isNew: Boolean,
        status: ConversationStatus,
        lastModifiedTime: String?
    ) {
        val navController = activeNavController ?: return
        val uris = ShareInbox.deliverableUris(this)
        if (uris.isEmpty()) return
        if (navController.currentDestination?.route?.startsWith("chat/") == true) {
            navController.popBackStack("conversations", false)
        }
        conversationListViewModel.notifySessionFocus(cascadeId)
        chatViewModel.prepareSession(
            cascadeId = cascadeId,
            initialTitle = title,
            isNewConversation = isNew,
            workspaceName = conversationListViewModel.getWorkspaceName(cascadeId),
            isUnread = false,
            conversationStatus = status,
            draftProject = conversationListViewModel.getDraftProject(cascadeId),
            lastModifiedTime = lastModifiedTime
        )
        conversationListViewModel.markConversationAsRead(cascadeId)
        val encodedTitle = URLEncoder.encode(title, "UTF-8")
        try {
            navController.navigate("chat/$cascadeId/$encodedTitle?isNew=$isNew&isUnread=false&status=${status.name}")
        } catch (e: Exception) {
            Log.w("MainActivity", "Failed to open $cascadeId for shared files: ${e.message}")
            return
        }
        chatViewModel.addAttachmentsFromUris(this, uris)
        ShareInbox.release()
    }

    private fun launchScanner() {
        val options = ScanOptions().apply {
            setDesiredBarcodeFormats(ScanOptions.QR_CODE)
            setPrompt("将镜头对准电脑终端的配对二维码")
            setCameraId(0)
            setBeepEnabled(true)
            setBarcodeImageEnabled(false)
            setOrientationLocked(true)
        }
        qrScanLauncher.launch(options)
    }

    fun checkNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    private fun handleDeepLink(intent: Intent) {
        val uri = intent.data ?: return
        val scheme = uri.scheme?.lowercase() ?: return
        val host = uri.host?.lowercase() ?: return

        if ((scheme == "agy" || scheme == "multigravity" || scheme == "antigravity") && host == "pair") {
            pairingViewModel.pairWithUri(uri.toString())
            return
        }

        if (scheme == "antigravity" || scheme == "multigravity" || scheme == "agy") {
            val cascadeId = when (host) {
                "cascade", "session" -> {
                    uri.pathSegments.firstOrNull() ?: uri.path?.trim('/')
                }
                else -> {
                    if (uri.pathSegments.isNotEmpty()) {
                        uri.pathSegments.firstOrNull()
                    } else {
                        host
                    }
                }
            }?.trim('/')

            if (!cascadeId.isNullOrBlank()) {
                navigateToCascade(cascadeId)
            }
        }
    }

    private fun navigateToCascade(cascadeId: String) {
        val navController = activeNavController
        if (navController == null || !prefs.isPaired()) {
            pendingCascadeId = cascadeId
            return
        }

        val existing = conversationListViewModel.getConversation(cascadeId)
        val title = existing?.title ?: "会话"
        val isNew = false
        val isUnread = false
        val status = existing?.status ?: com.antigravity.mobile.data.model.ConversationStatus.RUNNING
        val lastModified = existing?.lastModifiedTime
        val wsName = conversationListViewModel.getWorkspaceName(cascadeId)
        val draftProject = conversationListViewModel.getDraftProject(cascadeId)

        conversationListViewModel.notifySessionFocus(cascadeId)
        chatViewModel.prepareSession(
            cascadeId = cascadeId,
            initialTitle = title,
            isNewConversation = isNew,
            workspaceName = wsName,
            isUnread = isUnread,
            conversationStatus = status,
            draftProject = draftProject,
            lastModifiedTime = lastModified
        )
        conversationListViewModel.markConversationAsRead(cascadeId)
        val encodedTitle = URLEncoder.encode(title, "UTF-8")
        try {
            navController.navigate("chat/$cascadeId/$encodedTitle?isNew=$isNew&isUnread=$isUnread&status=${status.name}")
        } catch (e: Exception) {
            Log.w("MainActivity", "Failed to navigate to cascade $cascadeId: ${e.message}")
        }
    }

    override fun onResume() {
        super.onResume()
        if (::liveActivityManager.isInitialized) {
            liveActivityManager.cleanUpOrphanedActivities()
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        connectionManager.stopMonitoring()
    }
}
