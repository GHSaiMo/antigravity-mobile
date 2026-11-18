import SwiftUI

@main
struct MultigravityApp: App {
    @State private var settings = AppSettings.shared

    init() {
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
                .tint(settings.isNordTheme ? NordTheme.accent : .indigo)
                .preferredColorScheme(settings.colorScheme)
                .background(settings.isNordTheme ? NordTheme.background : Color.clear)
                .onAppear {
                    settings.applyAppearance()
                }
        }
    }
}
