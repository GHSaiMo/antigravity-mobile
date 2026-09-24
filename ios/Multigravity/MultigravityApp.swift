import SwiftUI

@main
struct MultigravityApp: App {
    var body: some Scene {
        WindowGroup {
            ConversationListView()
                .tint(.indigo)
        }
    }
}
