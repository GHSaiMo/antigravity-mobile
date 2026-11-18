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
            ThemedRootContainer(settings: settings)
        }
    }
}

private struct ThemedRootContainer: View {
    @Environment(\.colorScheme) private var systemColorScheme
    var settings: AppSettings

    var body: some View {
        let preset = settings.activeThemePreset(for: systemColorScheme)
        ConversationListView()
            .tint(preset.accent)
            .preferredColorScheme(settings.colorScheme)
            .background(preset.isDefaultStyle ? Color.clear : preset.bg)
            .environment(\.appThemePreset, preset)
            .onAppear {
                settings.applyAppearance()
            }
    }
}
