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

class MainActivity : ComponentActivity() {

    private lateinit var prefs: PreferencesManager
    private lateinit var connectionManager: ConnectionManager
    private lateinit var apiClient: ApiClient
    private lateinit var wsClient: StreamWebSocketClient

    private lateinit var pairingViewModel: PairingViewModel
    private lateinit var conversationListViewModel: ConversationListViewModel
    private lateinit var chatViewModel: ChatViewModel

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

        // Initialize Coil SVG and Gateway Auth Header interceptor globally
        val coilOkHttpClient = okhttp3.OkHttpClient.Builder()
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
                .components {
                    add(coil.decode.SvgDecoder.Factory())
                }
                .build()
        )

        // Initialize ViewModels
        pairingViewModel = PairingViewModel(apiClient, prefs)
        conversationListViewModel = ConversationListViewModel(apiClient, prefs, cacheManager)
        pairingViewModel.setPreheatAction {
            conversationListViewModel.loadInitialData()
        }
        val liveActivityManager = com.antigravity.mobile.data.service.LiveActivityNotificationManager(applicationContext, prefs)
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
            }
        }
    }

    override fun onNewIntent(intent: Intent?) {
        super.onNewIntent(intent)
        intent?.let { handleDeepLink(it) }
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

    override fun onDestroy() {
        super.onDestroy()
        connectionManager.stopMonitoring()
    }
}
