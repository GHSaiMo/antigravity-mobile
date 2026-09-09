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
    public let cascadeId: String
    public let title: String?
    public let status: String?
    public let duration: String?
    public let totalSteps: Int?
    public let totalTools: Int?
    public let workspaceUri: String?
    public let messages: [PaginatedMessagesResponse.GatewayMessageItem]?
    public let isFullSnapshot: Bool?
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
}

@Observable
@MainActor
public final class StreamWebSocketClient {
    public var status: StreamConnectionStatus = .disconnected
    
    public var onUpdate: ((StreamUpdatePayload) -> Void)?
    public var onStatusChange: ((StreamConnectionStatus) -> Void)?
    
    private var webSocketTask: URLSessionWebSocketTask?
    private var readTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?
    private var reconnectTask: Task<Void, Never>?
    
    private var activeURL: URL?
    private var activeCascadeId: String?
    private var isIntentionallyClosed: Bool = false
    private let session: URLSession
    
    public init() {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 20
        config.timeoutIntervalForResource = 60
        config.httpShouldSetCookies = true
        config.httpCookieAcceptPolicy = .always
        config.httpCookieStorage = HTTPCookieStorage.shared
        self.session = URLSession(configuration: config)
    }
    
    public func connect(baseURL: URL, cascadeId: String) {
        // If already connected to the same session, no need to reconnect
        if status == .connected && activeCascadeId == cascadeId {
            return
        }
        
        disconnect(intentional: true)
        
        self.activeURL = baseURL
        self.activeCascadeId = cascadeId
        self.isIntentionallyClosed = false
        
        startConnection()
    }
    
    private func startConnection() {
        guard let baseURL = activeURL, let cascadeId = activeCascadeId, !isIntentionallyClosed else { return }
        
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true) else {
            updateStatus(.failed)
            return
        }
        
        // Transform protocol to ws:// or wss://
        let scheme = components.scheme?.lowercased()
        if scheme == "https" {
            components.scheme = "wss"
        } else {
            components.scheme = "ws"
        }
        
        var path = components.path
        if !path.hasSuffix("/") {
            path += "/"
        }
        path += "gateway/cascade/stream"
        components.path = path
        components.queryItems = [URLQueryItem(name: "cascadeId", value: cascadeId)]
        
        guard let wsURL = components.url else {
            updateStatus(.failed)
            return
        }
        
        var request = URLRequest(url: wsURL)
        request.timeoutInterval = 15
        
        // Inject Cloudflare Access headers if configured
        let settings = AppSettings.shared
        if !settings.cfAccessClientId.isEmpty {
            request.setValue(settings.cfAccessClientId, forHTTPHeaderField: "CF-Access-Client-Id")
        }
        if !settings.cfAccessClientSecret.isEmpty {
            request.setValue(settings.cfAccessClientSecret, forHTTPHeaderField: "CF-Access-Client-Secret")
        }
        
        updateStatus(.connecting)
        
        let task = session.webSocketTask(with: request)
        self.webSocketTask = task
        task.resume()
        
        startPingLoop(for: task)
        startReadLoop(for: task, cascadeId: cascadeId)
    }
    
    private func startReadLoop(for task: URLSessionWebSocketTask, cascadeId: String) {
        readTask?.cancel()
        readTask = Task { [weak self] in
            while !Task.isCancelled {
                do {
                    let message = try await task.receive()
                    guard let self else { break }
                    
                    if self.status != .connected {
                        self.updateStatus(.connected)
                    }
                    
                    switch message {
                    case .string(let text):
                        if let data = text.data(using: .utf8) {
                            self.handlePayloadData(data, expectedCascadeId: cascadeId)
                        }
                    case .data(let data):
                        self.handlePayloadData(data, expectedCascadeId: cascadeId)
                    @unknown default:
                        break
                    }
                } catch {
                    guard let self else { break }
                    if !Task.isCancelled && !self.isIntentionallyClosed {
                        print("[StreamWS] Connection interrupted: \(error.localizedDescription)")
                        self.handleConnectionLoss()
                    }
                    break
                }
            }
        }
    }
    
    private func handlePayloadData(_ data: Data, expectedCascadeId: String) {
        do {
            let payload = try JSONDecoder().decode(StreamUpdatePayload.self, from: data)
            if payload.cascadeId == expectedCascadeId {
                self.onUpdate?(payload)
            }
        } catch {
            print("[StreamWS] Payload decoding failed: \(error)")
        }
    }
    
    private func startPingLoop(for task: URLSessionWebSocketTask) {
        pingTask?.cancel()
        pingTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 15_000_000_000) // 15 seconds
                guard !Task.isCancelled else { break }
                task.sendPing { error in
                    if let error = error {
                        print("[StreamWS] Ping failed: \(error.localizedDescription)")
                    }
                }
            }
        }
    }
    
    private func handleConnectionLoss() {
        updateStatus(.failed)
        cleanupCurrentSocket()
        
        guard !isIntentionallyClosed else { return }
        
        // Automatically schedule reconnect in 2.5s
        reconnectTask?.cancel()
        reconnectTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 2_500_000_000)
            guard let self, !self.isIntentionallyClosed else { return }
            print("[StreamWS] Reconnecting to stream...")
            self.startConnection()
        }
    }
    
    private func cleanupCurrentSocket() {
        pingTask?.cancel()
        pingTask = nil
        readTask?.cancel()
        readTask = nil
        
        if let task = webSocketTask {
            task.cancel(with: .goingAway, reason: nil)
            self.webSocketTask = nil
        }
    }
    
    public func disconnect(intentional: Bool = true) {
        self.isIntentionallyClosed = intentional
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
