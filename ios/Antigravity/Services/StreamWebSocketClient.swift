import Foundation
import Network
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
    public let totalMessages: Int?
    public let hasMore: Bool?
    public let nextOffset: Int?
    public let workspaceUri: String?
    public let messages: [PaginatedMessagesResponse.GatewayMessageItem]?
    public let isFullSnapshot: Bool?
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]?
    public let runningTasks: [RunningTaskItem]?
    public let activeModel: String?
    public let modelDisplayName: String?
    public let hasError: Bool?
    public let errorMessage: String?
}

@Observable
@MainActor
public final class StreamWebSocketClient {
    public var status: StreamConnectionStatus = .disconnected
    
    public var onUpdate: ((StreamUpdatePayload) -> Void)?
    public var onStatusChange: ((StreamConnectionStatus) -> Void)?
    
    private var connection: NWConnection?
    private var reconnectTask: Task<Void, Never>?
    private var connectionWatchdogTask: Task<Void, Never>?
    
    private var activeURL: URL?
    private var activeCascadeId: String?
    private var isIntentionallyClosed: Bool = false
    
    public init() {}
    
    public func connect(baseURL: URL, cascadeId: String, force: Bool = false) {
        // If already connected or currently connecting to the same session and URL, do not abort/reconnect unless forced
        if !force && (status == .connected || status == .connecting) && activeCascadeId == cascadeId && activeURL == baseURL {
            return
        }
        
        disconnect(intentional: true)
        
        self.activeURL = baseURL
        self.activeCascadeId = cascadeId
        self.isIntentionallyClosed = false
        
        startConnection()
    }
    
    public func reconnect(force: Bool = true) {
        guard let baseURL = activeURL, let cascadeId = activeCascadeId else { return }
        connect(baseURL: baseURL, cascadeId: cascadeId, force: force)
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
        
        var queryItems = [
            URLQueryItem(name: "cascadeId", value: cascadeId),
            URLQueryItem(name: "client", value: "ios"),
            URLQueryItem(name: "format", value: "messages")
        ]
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            queryItems.append(URLQueryItem(name: "auth_token", value: token))
        }
        components.queryItems = queryItems
        
        guard let wsURL = components.url else {
            updateStatus(.failed)
            return
        }
        
        updateStatus(.connecting)
        
        let endpoint = NWEndpoint.url(wsURL)
        let wsOptions = NWProtocolWebSocket.Options()
        wsOptions.autoReplyPing = true
        
        let parameters: NWParameters
        if wsURL.scheme?.lowercased() == "wss" {
            let tlsOptions = NWProtocolTLS.Options()
            let hostStr = (wsURL.host ?? "").lowercased()
            let isLoopback = hostStr == "127.0.0.1" || hostStr == "::1" || hostStr == "localhost"
            sec_protocol_options_set_verify_block(tlsOptions.securityProtocolOptions, { (metadata, trust, completion) in
                if isLoopback {
                    // Trust self-signed certificates only for local gateway connections
                    completion(true)
                } else {
                    // Standard X.509 trust evaluation for all remote hosts
                    let secTrust = sec_trust_copy_ref(trust).takeRetainedValue()
                    SecTrustEvaluateAsyncWithError(secTrust, DispatchQueue.global()) { _, result, _ in
                        completion(result)
                    }
                }
            }, .global())
            parameters = NWParameters(tls: tlsOptions)
        } else {
            parameters = NWParameters.tcp
        }
        parameters.defaultProtocolStack.applicationProtocols.insert(wsOptions, at: 0)
        
        let conn = NWConnection(to: endpoint, using: parameters)
        self.connection = conn
        
        conn.stateUpdateHandler = { [weak self] state in
            guard let self = self else { return }
            Task { @MainActor in
                self.handleStateUpdate(state: state, wsURL: wsURL)
            }
        }
        
        conn.start(queue: .main)
        
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 10_000_000_000)
            guard let self, !Task.isCancelled else { return }
            if self.status != .connected && !self.isIntentionallyClosed {
                print("[StreamWS] Connection attempt timed out (10s), triggering retry...")
                self.handleConnectionLoss()
            }
        }
    }
    
    private func handleStateUpdate(state: NWConnection.State, wsURL: URL) {
        switch state {
        case .ready:
            connectionWatchdogTask?.cancel()
            connectionWatchdogTask = nil
            updateStatus(.connected)
            receiveNextMessage()
        case .waiting(let error):
            print("[StreamWS] Connection waiting: \(error)")
        case .failed(let error):
            print("[StreamWS] Connection failed: \(error)")
            handleConnectionLoss()
        case .cancelled:
            if !isIntentionallyClosed {
                updateStatus(.disconnected)
            }
        default:
            break
        }
    }
    
    private func receiveNextMessage() {
        guard let conn = connection, status == .connected, !isIntentionallyClosed else { return }
        
        conn.receiveMessage { [weak self] content, _, isComplete, error in
            guard let self = self else { return }
            Task { @MainActor in
                if let error = error {
                    print("[StreamWS] Receive error: \(error)")
                    self.handleConnectionLoss()
                    return
                }
                
                if let data = content, !data.isEmpty, let cascadeId = self.activeCascadeId {
                    self.handlePayloadData(data, expectedCascadeId: cascadeId)
                } else if content == nil && isComplete {
                    print("[StreamWS] Stream connection closed by peer (EOF)")
                    self.handleConnectionLoss()
                    return
                }
                
                if !self.isIntentionallyClosed {
                    self.receiveNextMessage()
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
    
    private func handleConnectionLoss() {
        updateStatus(.failed)
        cleanupCurrentSocket()
        
        guard !isIntentionallyClosed else { return }
        
        reconnectTask?.cancel()
        reconnectTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 2_000_000_000)
            guard let self, !self.isIntentionallyClosed else { return }
            print("[StreamWS] Reconnecting to stream...")
            self.startConnection()
        }
    }
    
    private func cleanupCurrentSocket() {
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        if let conn = connection {
            conn.stateUpdateHandler = nil
            conn.cancel()
            self.connection = nil
        }
    }
    
    public func disconnect(intentional: Bool = true) {
        self.isIntentionallyClosed = intentional
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
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
