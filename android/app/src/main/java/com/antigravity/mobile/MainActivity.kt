package com.antigravity.mobile

import android.content.Intent
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.ActivityResultLauncher
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.antigravity.mobile.data.service.ApiClient
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
    private lateinit var apiClient: ApiClient
    private lateinit var wsClient: StreamWebSocketClient

    private lateinit var pairingViewModel: PairingViewModel
    private lateinit var conversationListViewModel: ConversationListViewModel
    private lateinit var chatViewModel: ChatViewModel

    private lateinit var qrScanLauncher: ActivityResultLauncher<ScanOptions>

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // Initialize Services
        prefs = PreferencesManager(applicationContext)
        apiClient = ApiClient(prefs)
        wsClient = StreamWebSocketClient(prefs)

        // Initialize ViewModels
        pairingViewModel = PairingViewModel(apiClient, prefs)
        conversationListViewModel = ConversationListViewModel(apiClient, prefs)
        chatViewModel = ChatViewModel(apiClient, wsClient)

        // Register ZXing Scanner
        qrScanLauncher = registerForActivityResult(ScanContract()) { result ->
            if (result.contents != null) {
                val scannedContent = result.contents
                pairingViewModel.pairWithUri(scannedContent)
            } else {
                Toast.makeText(this, "已取消扫码", Toast.LENGTH_SHORT).show()
            }
        }

        // Handle DeepLink if opened from URL
        handleDeepLink(intent)

        setContent {
            val themeMode by prefs.themeModeFlow.collectAsState()
            AntigravityTheme(themeMode = themeMode) {
                val navController = rememberNavController()
                val startDestination = if (prefs.isPaired()) "conversations" else "pair"

                NavHost(
                    navController = navController,
                    startDestination = startDestination
                ) {
                    composable("pair") {
                        PairingScreen(
                            viewModel = pairingViewModel,
                            onLaunchScanner = { launchScanner() },
                            onPairedSuccess = {
                                conversationListViewModel.loadConversations()
                                navController.navigate("conversations") {
                                    popUpTo("pair") { inclusive = true }
                                }
                            }
                        )
                    }

                    composable("conversations") {
                        ConversationListScreen(
                            viewModel = conversationListViewModel,
                            onSelectConversation = { cascadeId, title ->
                                val encodedTitle = URLEncoder.encode(title, "UTF-8")
                                navController.navigate("chat/$cascadeId/$encodedTitle")
                            },
                            onNavigateToPair = {
                                conversationListViewModel.unpair()
                                navController.navigate("pair") {
                                    popUpTo("conversations") { inclusive = true }
                                }
                            }
                        )
                    }

                    composable(
                        route = "chat/{cascadeId}/{title}",
                        arguments = listOf(
                            navArgument("cascadeId") { type = NavType.StringType },
                            navArgument("title") { type = NavType.StringType }
                        )
                    ) { backStackEntry ->
                        val cascadeId = backStackEntry.arguments?.getString("cascadeId") ?: ""
                        val rawTitle = backStackEntry.arguments?.getString("title") ?: ""
                        val title = URLDecoder.decode(rawTitle, "UTF-8")

                        ChatScreen(
                            cascadeId = cascadeId,
                            initialTitle = title,
                            viewModel = chatViewModel,
                            onNavigateBack = {
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
            setPrompt("将镜头对准 Mac 终端的配对二维码")
            setCameraId(0)
            setBeepEnabled(true)
            setBarcodeImageEnabled(false)
            setOrientationLocked(true)
        }
        qrScanLauncher.launch(options)
    }

    private fun handleDeepLink(intent: Intent) {
        val uri = intent.data ?: return
        if (uri.scheme.equals("agy", ignoreCase = true) && uri.host.equals("pair", ignoreCase = true)) {
            pairingViewModel.pairWithUri(uri.toString())
        }
    }
}
