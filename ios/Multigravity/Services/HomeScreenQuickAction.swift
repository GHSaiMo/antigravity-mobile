import Foundation
import UIKit

/// Static Home Screen Quick Actions declared in `Info.plist`, unified with `multigravity://` URL routing
/// so Home Screen long-press, Shortcuts, Action Button, and Back Tap share one execution path.
public enum HomeScreenQuickAction: String, CaseIterable, Sendable {
    case newChat = "com.ghsaimo.antigravity.shortcut.new-chat"
    case newWorkspace = "com.ghsaimo.antigravity.shortcut.new-workspace"
    case camera = "com.ghsaimo.antigravity.shortcut.camera"
    case scan = "com.ghsaimo.antigravity.shortcut.scan"

    /// Canonical deep-link host (`multigravity://<urlHost>`).
    public var urlHost: String {
        switch self {
        case .newChat: return "new-chat"
        case .newWorkspace: return "new-workspace"
        case .camera: return "camera"
        case .scan: return "scan"
        }
    }

    public var deepLinkURL: URL {
        URL(string: "multigravity://\(urlHost)")!
    }

    public init?(urlHost: String) {
        switch urlHost.lowercased() {
        case "new-chat", "newchat", "chat":
            self = .newChat
        case "new-workspace", "newworkspace", "workspace":
            self = .newWorkspace
        case "camera", "photo", "take-photo":
            self = .camera
        case "scan", "scan-document", "scanner":
            self = .scan
        default:
            return nil
        }
    }

    /// One-shot capture action triggered immediately after `ChatView` mounts.
    public enum ChatLaunchAction: Equatable, Sendable {
        case camera
        case documentScanner
    }

    @MainActor
    @discardableResult
    public static func handle(_ shortcutItem: UIApplicationShortcutItem) -> Bool {
        guard let action = HomeScreenQuickAction(rawValue: shortcutItem.type) else {
            return false
        }
        PushNotificationManager.shared.route(.deepLink(action.deepLinkURL))
        return true
    }
}

/// Lightweight scene delegate attached via `MultigravityAppDelegate` so SwiftUI's `App` lifecycle
/// receives both cold-launch (`connectionOptions.shortcutItem`) and warm-launch Quick Action callbacks
/// without interfering with SwiftUI's window management or `onOpenURL`.
public final class MultigravitySceneDelegate: NSObject, UIWindowSceneDelegate {
    public func scene(
        _ scene: UIScene,
        willConnectTo session: UISceneSession,
        options connectionOptions: UIScene.ConnectionOptions
    ) {
        if let shortcutItem = connectionOptions.shortcutItem {
            HomeScreenQuickAction.handle(shortcutItem)
        }
    }

    public func windowScene(
        _ windowScene: UIWindowScene,
        performActionFor shortcutItem: UIApplicationShortcutItem,
        completionHandler: @escaping (Bool) -> Void
    ) {
        let handled = HomeScreenQuickAction.handle(shortcutItem)
        completionHandler(handled)
    }
}
