import Foundation
import Observation

public enum StreamConnectionStatus: String, Sendable {
    case disconnected
    case connecting
    case connected
    case failed
}

public struct StreamUpdatePayload: Decodable, Sendable {
    public let type: String
    public let rev: UInt64?
    public let cascadeId: String
    public let title: String?
    public let status: String?
    public let duration: String?
    public let totalSteps: Int?
    public let totalTools: Int?
    public let totalMessages: Int?
    public let hasMore: Bool?
    public let nextOffset: Int?
    public let workspaceUri: String?
    /// Complete window once `StreamWebSocketClient` has reassembled any `delta` frame (see StreamDeltaReassembler).
    public var messages: [PaginatedMessagesResponse.GatewayMessageItem]?
    public let turnOutline: [TurnOutlineItem]?
    public let isFullSnapshot: Bool?
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]?
    public let runningTasks: [RunningTaskItem]?
    public let subagents: [SubagentItem]?
    public let parentConversationId: String?
    public let subagentRole: String?
    public let activeModel: String?
    public let activeModelName: String?
    public let modelDisplayName: String?
    public let desktopActiveModel: String?
    public let desktopActiveModelName: String?
    public let startedAt: String?
    public let hasError: Bool?
    public let errorMessage: String?
    /// delta=1 frames: `messages` holds only new/changed items and `messageIds` the full ordered window.
    public let delta: Bool?
    public let messageIds: [String]?
    public let swarm: SwarmBrief?
    public let contextMeter: ContextMeterItem?
    public let suggestedResponses: [String]?
    public let suggestedResponseSource: String?
    public let activeFocus: String?
    public let activeFocusStartedAt: String?
    public let activePlan: StepPlanItem?
    public let branchOrigin: SideQuestionOrigin?
    public let sideQuestions: [SideQuestionCardItem]?
}

@Observable
@MainActor
public final class StreamWebSocketClient {
    public var status: StreamConnectionStatus = .disconnected

    public var onUpdate: ((StreamUpdatePayload) -> Void)?
    public var onStatusChange: ((StreamConnectionStatus) -> Void)?

    private var wsTask: GatewayWebSocketTask?
    private var connectionGeneration: UInt64 = 0
    private var reconnectTask: Task<Void, Never>?
    private var connectionWatchdogTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?

    private var activeURL: URL?
    private var activeCascadeId: String?
    private var isIntentionallyClosed: Bool = false
    private var reconnectAttempt: Int = 0
    private var openedAt: Date? = nil
    private var lastStreamRev: UInt64 = 0
    private var revCascadeId: String?
    private var deltaReassembler = StreamDeltaReassembler<PaginatedMessagesResponse.GatewayMessageItem>(idOf: { $0.id })

    public init() {
        NotificationCenter.default.addObserver(
            forName: .networkRoutingPreferenceChanged,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self = self else { return }
                if self.activeCascadeId != nil && !self.isIntentionallyClosed {
                    print("[StreamWS] Network route changed, reconnecting stream...")
                    self.disconnect(intentional: false)
                    self.startConnection()
                }
            }
        }
    }

    public func connect(baseURL: URL, cascadeId: String, force: Bool = false) {
        let effectiveURL = AppSettings.shared.serverURL ?? baseURL
        if !force && (status == .connected || status == .connecting) && activeCascadeId == cascadeId && activeURL == effectiveURL {
            return
        }

        if revCascadeId != cascadeId {
            lastStreamRev = 0
            revCascadeId = cascadeId
            deltaReassembler.reset()
        }

        disconnect(intentional: true)

        self.activeURL = effectiveURL
        self.activeCascadeId = cascadeId
        self.isIntentionallyClosed = false

        startConnection()
    }

    public func reconnect(force: Bool = true) {
        guard let cascadeId = activeCascadeId else { return }
        let effectiveURL = AppSettings.shared.serverURL ?? activeURL
        guard let baseURL = effectiveURL else { return }
        connect(baseURL: baseURL, cascadeId: cascadeId, force: force)
    }

    private func startConnection() {
        guard let cascadeId = activeCascadeId, !isIntentionallyClosed else { return }

        let effectiveURL = AppSettings.shared.serverURL ?? activeURL
        guard let baseURL = effectiveURL else {
            updateStatus(.failed)
            return
        }
        self.activeURL = effectiveURL

        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true) else {
            updateStatus(.failed)
            return
        }

        let scheme = components.scheme?.lowercased()
        components.scheme = (scheme == "https" || scheme == "wss") ? "wss" : "ws"

        var path = components.path
        if !path.hasSuffix("/") {
            path += "/"
        }
        path += "gateway/cascade/stream"
        components.path = path

        var queryItems = [
            URLQueryItem(name: "cascadeId", value: cascadeId),
            URLQueryItem(name: "client", value: "ios"),
            URLQueryItem(name: "format", value: "messages"),
            URLQueryItem(name: "delta", value: "1")
        ]
        if lastStreamRev > 0 && revCascadeId == cascadeId {
            queryItems.append(URLQueryItem(name: "sinceRev", value: "\(lastStreamRev)"))
        }
        components.queryItems = queryItems

        guard let wsURL = components.url else {
            updateStatus(.failed)
            return
        }

        let hostStr = (wsURL.host ?? "").lowercased()
        if wsURL.scheme?.lowercased() == "ws" && !NetworkTransport.isLocalOrPrivateHost(hostStr) {
            print("[StreamWS] Refusing cleartext ws:// connection to public host: \(hostStr)")
            updateStatus(.failed)
            return
        }

        cleanupCurrentSocket()
        connectionGeneration &+= 1
        let gen = connectionGeneration

        updateStatus(.connecting)

        var request = URLRequest(url: wsURL)
        request.timeoutInterval = 15
        request.cachePolicy = .reloadIgnoringLocalCacheData
        request.setValue("Multigravity/1.2.5 (iOS)", forHTTPHeaderField: "User-Agent")
        if CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr) {
            request.setValue(hostStr, forHTTPHeaderField: "Host")
        }
        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let task = GatewayWebSocketTask(
            request: request,
            requestTimeout: 15,
            resourceTimeout: 86_400,
            waitsForConnectivity: false,
            onOpen: { [weak self] in
                Task { @MainActor [weak self] in
                    self?.markSocketOpened(generation: gen)
                }
            },
            onClose: { [weak self] in
                Task { @MainActor [weak self] in
                    guard let self, self.connectionGeneration == gen, !self.isIntentionallyClosed else { return }
                    print("[StreamWS] Stream connection closed by peer")
                    self.handleConnectionLoss()
                }
            },
            onFail: { [weak self] error in
                Task { @MainActor [weak self] in
                    guard let self, self.connectionGeneration == gen, !self.isIntentionallyClosed else { return }
                    let errDesc = error.localizedDescription
                    print("[StreamWS] Connection failed: \(errDesc)")
                    if errDesc.contains(" 401") || errDesc.contains(" 403") || errDesc.contains("4001") || errDesc.contains("4003") {
                        self.isIntentionallyClosed = true
                        self.updateStatus(.failed)
                        self.cleanupCurrentSocket()
                        return
                    }
                    self.handleConnectionLoss()
                }
            }
        )
        self.wsTask = task
        task.resume()
        receiveNextMessage(task: task, generation: gen)

        connectionWatchdogTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 10_000_000_000)
            guard let self, !Task.isCancelled, self.connectionGeneration == gen else { return }
            if self.status != .connected && !self.isIntentionallyClosed {
                print("[StreamWS] Connection attempt timed out (10s), triggering retry...")
                self.handleConnectionLoss()
            }
        }
    }

    private func markSocketOpened(generation gen: UInt64) {
        guard self.connectionGeneration == gen, !isIntentionallyClosed else { return }
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        if openedAt == nil {
            openedAt = Date()
        }
        if lastStreamRev == 0 {
            deltaReassembler.reset()
        }
        if status != .connected {
            updateStatus(.connected)
            startKeepAlivePing(generation: gen)
        }
    }

    private func startKeepAlivePing(generation gen: UInt64) {
        pingTask?.cancel()
        pingTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 25_000_000_000) // 25s (< Cloudflare 100s WS idle limit)
                guard let self, !Task.isCancelled, self.connectionGeneration == gen, let task = self.wsTask else { break }
                task.sendPing { error in
                    if let error {
                        print("[StreamWS] Keep-alive ping error: \(error.localizedDescription)")
                    }
                }
            }
        }
    }

    private func receiveNextMessage(task: GatewayWebSocketTask, generation gen: UInt64) {
        guard connectionGeneration == gen, wsTask === task, !isIntentionallyClosed else { return }

        task.receive { [weak self, weak task] result in
            Task { @MainActor [weak self, weak task] in
                guard let self, let task, self.connectionGeneration == gen, self.wsTask === task, !self.isIntentionallyClosed else { return }
                switch result {
                case .success(let msg):
                    self.markSocketOpened(generation: gen)
                    let data: Data
                    switch msg {
                    case .string(let text):
                        data = Data(text.utf8)
                    case .data(let raw):
                        data = raw
                    }
                    if !data.isEmpty, let cascadeId = self.activeCascadeId {
                        self.handlePayloadData(data, expectedCascadeId: cascadeId)
                    }
                    if !self.isIntentionallyClosed && self.wsTask === task {
                        self.receiveNextMessage(task: task, generation: gen)
                    }
                case .failure(let error):
                    print("[StreamWS] Receive error: \(error.localizedDescription)")
                    self.handleConnectionLoss()
                }
            }
        }
    }

    private func handlePayloadData(_ data: Data, expectedCascadeId: String) {
        do {
            var payload = try JSONDecoder().decode(StreamUpdatePayload.self, from: data)
            if payload.cascadeId == expectedCascadeId {
                if payload.type == "noop" {
                    reconnectAttempt = 0
                    if let r = payload.rev, r > 0 {
                        lastStreamRev = r
                        revCascadeId = expectedCascadeId
                    }
                    return
                }
                switch deltaReassembler.apply(isDelta: payload.delta ?? false, messageIds: payload.messageIds, messages: payload.messages) {
                case .messages(let rebuilt):
                    payload.messages = rebuilt
                case .resync:
                    print("[StreamWS] Delta baseline mismatch, resyncing")
                    lastStreamRev = 0
                    deltaReassembler.reset()
                    handleConnectionLoss()
                    return
                }
                reconnectAttempt = 0
                if let r = payload.rev, r > 0 {
                    lastStreamRev = r
                    revCascadeId = expectedCascadeId
                }
                self.onUpdate?(payload)
            }
        } catch {
            print("[StreamWS] Payload decoding failed: \(error)")
        }
    }

    private func handleConnectionLoss() {
        if let opened = openedAt, Date().timeIntervalSince(opened) >= 15.0 {
            reconnectAttempt = 0
        }
        openedAt = nil
        updateStatus(.failed)
        cleanupCurrentSocket()

        guard !isIntentionallyClosed else { return }

        reconnectTask?.cancel()
        reconnectTask = Task { [weak self] in
            guard let self, !self.isIntentionallyClosed else { return }
            self.reconnectAttempt += 1
            let baseDelay = min(2.5 * pow(2.0, Double(min(self.reconnectAttempt - 1, 5))), 60.0)
            let delay = baseDelay * Double.random(in: 0.75...1.25)
            let delayNano = UInt64(delay * 1_000_000_000)
            try? await Task.sleep(nanoseconds: delayNano)
            guard !self.isIntentionallyClosed else { return }
            await ConnectionManager.shared.probeEndpoints()
            print("[StreamWS] Reconnecting to stream... (attempt \(self.reconnectAttempt))")
            self.startConnection()
        }
    }

    private func cleanupCurrentSocket() {
        openedAt = nil
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        pingTask?.cancel()
        pingTask = nil
        if let task = wsTask {
            self.wsTask = nil
            task.cancel()
        }
    }

    public func disconnect(intentional: Bool = true) {
        self.isIntentionallyClosed = intentional
        self.reconnectAttempt = 0
        connectionGeneration &+= 1
        reconnectTask?.cancel()
        reconnectTask = nil
        cleanupCurrentSocket()
        updateStatus(.disconnected)
    }

    private func updateStatus(_ newStatus: StreamConnectionStatus) {
        guard self.status != newStatus else { return }
        self.status = newStatus
        self.onStatusChange?(newStatus)
    }
}
