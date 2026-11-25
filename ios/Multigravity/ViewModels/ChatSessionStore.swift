import Foundation
import SwiftUI

/// Preserves live `ChatViewModel` and `ChatScrollHUDState` instances across dynamic container
/// layout switches (folding/unfolding iPhone Duo, rotating between single-column and split-column
/// layouts, or resizing in Split View multitasking).
///
/// Pure `peek` in `ChatView.init` avoids mutating state during SwiftUI view construction; entries
/// are registered on `onAppear` (`markMounted`) and evicted after a brief grace window on
/// `onDisappear` (`markUnmounted`) only when the same session does not immediately remount.
@MainActor
public final class ChatSessionStore {
    public static let shared = ChatSessionStore()

    struct RetainedSession {
        let viewModel: ChatViewModel
        let scrollHUD: ChatScrollHUDState
    }

    private struct Entry {
        let viewModel: ChatViewModel
        let scrollHUD: ChatScrollHUDState
        var mountCount: Int = 0
        var cleanupTask: Task<Void, Never>? = nil
    }

    private var entries: [String: Entry] = [:]

    public init() {}

    /// Read-only lookup safe to invoke from `ChatView.init`.
    func peek(sessionKey: String) -> RetainedSession? {
        guard !sessionKey.isEmpty, let entry = entries[sessionKey] else { return nil }
        return RetainedSession(viewModel: entry.viewModel, scrollHUD: entry.scrollHUD)
    }

    func markMounted(
        sessionKey: String,
        viewModel: ChatViewModel,
        scrollHUD: ChatScrollHUDState
    ) {
        guard !sessionKey.isEmpty else { return }
        if var existing = entries[sessionKey],
           ObjectIdentifier(existing.viewModel) == ObjectIdentifier(viewModel) {
            existing.cleanupTask?.cancel()
            existing.cleanupTask = nil
            existing.mountCount += 1
            entries[sessionKey] = existing
            return
        }
        entries[sessionKey]?.cleanupTask?.cancel()
        entries[sessionKey] = Entry(
            viewModel: viewModel,
            scrollHUD: scrollHUD,
            mountCount: 1,
            cleanupTask: nil
        )
    }

    /// Decrements mount count; if the session does not remount within the grace window,
    /// executes `onFinalCleanup` and evicts the entry.
    func markUnmounted(
        sessionKey: String,
        viewModel: ChatViewModel,
        graceNanoseconds: UInt64 = 250_000_000,
        onFinalCleanup: @escaping @MainActor () -> Void
    ) {
        guard !sessionKey.isEmpty,
              var entry = entries[sessionKey],
              ObjectIdentifier(entry.viewModel) == ObjectIdentifier(viewModel) else {
            onFinalCleanup()
            return
        }

        entry.mountCount = max(0, entry.mountCount - 1)
        entry.cleanupTask?.cancel()
        entry.cleanupTask = nil

        if entry.mountCount == 0 {
            let targetVMID = ObjectIdentifier(viewModel)
            entry.cleanupTask = Task { @MainActor [weak self] in
                try? await Task.sleep(nanoseconds: graceNanoseconds)
                guard !Task.isCancelled, let self else { return }
                guard let current = self.entries[sessionKey],
                      ObjectIdentifier(current.viewModel) == targetVMID,
                      current.mountCount == 0 else {
                    return
                }
                self.entries.removeValue(forKey: sessionKey)
                onFinalCleanup()
            }
        }

        entries[sessionKey] = entry
    }

    /// Evicts all retained chat sessions immediately (used when switching between decoupled gateways).
    func clearAll() {
        for entry in entries.values {
            entry.cleanupTask?.cancel()
            entry.viewModel.stopPolling()
        }
        entries.removeAll()
    }
}
