import Foundation
import Network

/// Listens for conversation-list change hints from the gateway so the list screen does not have to
/// poll every few seconds. Every `hello` (connect/reconnect) and `changed` frame calls `onChange`;
/// the owner then refetches the list through the normal list API. `isConnected` tells the owner when
/// it may relax its own polling and flips back to false as soon as the socket is lost.
///
/// Gateways without `/gateway/events` never complete the handshake (it backs off), so callers keep
/// their existing polling in that case.
///
/// The NWConnection setup mirrors `StreamWebSocketClient` (Cloudflare Anycast pinning, loopback TLS
/// trust, device-token headers) because App Transport Security blocks the URLSession WebSocket path
/// for cleartext LAN hosts. It is deliberately a copy rather than a refactor of the working stream
/// client; folding the two together is a possible follow-up.
@MainActor
public final class ConversationEventsClient {
    public private(set) var isConnected = false
    public var onChange: (() -> Void)?

    private var runTask: Task<Void, Never>?
    private var connection: NWConnection?
    private var connectionWatchdogTask: Task<Void, Never>?
    private var attempt = 0
    private var generation = 0
    private var baseURLProvider: (() -> URL?)?
    /// Resolves the in-flight `connectOnce` continuation; lets `stop()` end it without polling.
    private var finishCurrent: (@MainActor () -> Void)?

    public init() {}

    public func start(baseURL: @escaping () -> URL?) {
        guard runTask == nil else { return }
        baseURLProvider = baseURL
        generation += 1
        let gen = generation
        runTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self, self.generation == gen else { return }
                let opened = await self.connectOnce(generation: gen)
                guard !Task.isCancelled, self.generation == gen else { return }
                self.attempt = opened ? 0 : min(self.attempt + 1, 6)
                // Quick retry after a healthy session ended; back off while the gateway is unreachable or lacks the endpoint.
                let delay: Double = opened ? 1.0 : min(2.5 * pow(2.0, Double(max(self.attempt - 1, 0))), 60.0)
                try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
            }
        }
    }

    public func stop() {
        let pending = finishCurrent
        finishCurrent = nil
        generation += 1 // invalidates callbacks of the connection being torn down
        runTask?.cancel()
        runTask = nil
        attempt = 0
        tearDown()
        setConnected(false)
        pending?() // resumes the suspended connectOnce so its task can exit
    }

    private func setConnected(_ value: Bool) {
        isConnected = value
    }

    private func tearDown() {
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        if let conn = connection {
            conn.stateUpdateHandler = nil
            conn.cancel()
            connection = nil
        }
    }

    /// Runs one socket until it ends. Returns true when the handshake succeeded.
    private func connectOnce(generation gen: Int) async -> Bool {
        guard let baseURL = baseURLProvider?(),
              var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true) else { return false }

        components.scheme = (components.scheme?.lowercased() == "https") ? "wss" : "ws"
        var path = components.path
        if !path.hasSuffix("/") { path += "/" }
        components.path = path + "gateway/events"
        components.queryItems = nil
        guard let wsURL = components.url else { return false }

        let hostStr = (wsURL.host ?? "").lowercased()
        let isCloudflare = CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr)

        let endpoint: NWEndpoint
        if isCloudflare && attempt % 2 == 0 {
            let port = NWEndpoint.Port(rawValue: UInt16(wsURL.port ?? 443)) ?? .https
            endpoint = NWEndpoint.hostPort(host: NWEndpoint.Host(CloudflareAnycastAccelerator.primaryIP), port: port)
        } else {
            endpoint = NWEndpoint.url(wsURL)
        }

        let wsOptions = NWProtocolWebSocket.Options()
        wsOptions.autoReplyPing = true
        var extraHeaders: [(String, String)] = []
        if isCloudflare { extraHeaders.append(("Host", hostStr)) }
        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            extraHeaders.append(("Authorization", "Bearer \(token)"))
            extraHeaders.append(("x-device-token", token))
        }
        if !extraHeaders.isEmpty { wsOptions.setAdditionalHeaders(extraHeaders) }

        let parameters: NWParameters
        if wsURL.scheme?.lowercased() == "wss" {
            let tlsOptions = NWProtocolTLS.Options()
            let isLoopback = hostStr == "127.0.0.1" || hostStr == "::1" || hostStr == "localhost"
            if isLoopback {
                sec_protocol_options_set_verify_block(tlsOptions.securityProtocolOptions, { _, _, completion in
                    // Trust self-signed certificates only for local gateway loopback connections
                    completion(true)
                }, .global())
            }
            if isCloudflare {
                sec_protocol_options_set_tls_server_name(tlsOptions.securityProtocolOptions, hostStr)
            }
            parameters = NWParameters(tls: tlsOptions)
        } else {
            parameters = NWParameters.tcp
        }
        parameters.defaultProtocolStack.applicationProtocols.insert(wsOptions, at: 0)

        let conn = NWConnection(to: endpoint, using: parameters)
        tearDown()
        connection = conn

        return await withCheckedContinuation { (continuation: CheckedContinuation<Bool, Never>) in
            var opened = false
            var finished = false
            let finish: @MainActor () -> Void = { [weak self] in
                guard !finished else { return }
                finished = true
                if let self, self.generation == gen {
                    self.finishCurrent = nil
                    self.tearDown()
                    self.setConnected(false)
                }
                continuation.resume(returning: opened)
            }
            self.finishCurrent = finish

            conn.stateUpdateHandler = { [weak self] state in
                Task { @MainActor in
                    guard let self, self.generation == gen, !finished else { return }
                    switch state {
                    case .ready:
                        opened = true
                        self.connectionWatchdogTask?.cancel()
                        self.connectionWatchdogTask = nil
                        self.setConnected(true)
                        self.receive(on: conn, generation: gen, finish: finish)
                    case .failed, .cancelled:
                        finish()
                    default:
                        break
                    }
                }
            }
            conn.start(queue: .main)

            connectionWatchdogTask = Task { [weak self] in
                try? await Task.sleep(nanoseconds: 10_000_000_000)
                guard let self, !Task.isCancelled, self.generation == gen, !opened else { return }
                finish()
            }
        }
    }

    private func receive(on conn: NWConnection, generation gen: Int, finish: @escaping @MainActor () -> Void) {
        conn.receiveMessage { [weak self] content, _, isComplete, error in
            Task { @MainActor in
                guard let self, self.generation == gen, self.connection === conn else { return }
                if error != nil {
                    finish()
                    return
                }
                if let data = content, !data.isEmpty,
                   let frame = ConversationEventFrame.parse(data), frame.requestsRefresh {
                    self.onChange?()
                } else if content == nil && isComplete {
                    finish()
                    return
                }
                self.receive(on: conn, generation: gen, finish: finish)
            }
        }
    }
}
