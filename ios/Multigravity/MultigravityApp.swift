import SwiftUI

@main
struct MultigravityApp: App {
    @UIApplicationDelegateAdaptor(MultigravityAppDelegate.self) private var appDelegate
    @Environment(\.scenePhase) private var scenePhase
    @State private var settings = AppSettings.shared

    init() {
        // Bootstrap multi-gateway profile store and align all isolated cache namespaces before views load.
        let store = GatewayProfileStore.shared
        if let active = store.activeProfile {
            CacheManager.shared.switchStorageNamespace(active.storageNamespace)
            ProjectCacheManager.shared.switchStorageNamespace(active.storageNamespace)
            DocumentCacheManager.shared.switchStorageNamespace(active.storageNamespace)
            WeChatArchiveCacheManager.shared.switchStorageNamespace(active.storageNamespace)
            DraftFileStore.shared.switchStorageNamespace(active.storageNamespace)
        }
        let memoryCapacity = 50 * 1024 * 1024 // 50 MB
        let diskCapacity = 200 * 1024 * 1024  // 200 MB
        URLCache.shared = URLCache(
            memoryCapacity: memoryCapacity,
            diskCapacity: diskCapacity,
            diskPath: "multigravity_url_cache"
        )
    }

    var body: some Scene {
        WindowGroup {
            ConversationListView()
                .tint(.indigo)
                .onAppear {
                    settings.applyAppearance()
                    ConnectionManager.shared.ensureStarted()
                    ActivityManager.shared.startRemoteTokenObservers()
                    AppReviewManager.shared.recordActiveDayIfEligible()
                }
                .onChange(of: scenePhase) { _, newPhase in
                    if newPhase == .active {
                        settings.applyAppearance()
                        AppReviewManager.shared.recordActiveDayIfEligible()
                    }
                }
        }
    }
}
