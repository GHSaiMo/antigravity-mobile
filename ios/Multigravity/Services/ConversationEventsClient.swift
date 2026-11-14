import Foundation
import Network
import Observation
import os

/// Listens for conversation-list change hints and gateway heartbeats from `/gateway/events`.
/// Every `hello` (connect/reconnect) and `changed` frame calls `onChange`; the owner then
/// refetches the list through the normal list API. `isConnected` tells the owner when
/// it may relax its own polling and flips back to false as soon as the socket is lost.
///
/// Adaptive Transport Design:
/// 1. Default & Primary Transport: `URLSessionWebSocketTask`
///    - Automatically honors system Wi-Fi, HTTP, and SOCKS proxy configurations.
///    - Fully exempted by `NSAllowsLocalNetworking: true` for cleartext RFC1918 LAN hosts.
///    - Seamlessly supports Cloudflare Tunnel and WAN HTTPS connections.
/// 2. ATS Bypass Fallback: `NWConnection`
///    - Reserved for Tailscale private nodes (100.64.0.0/10, *.ts.net) using cleartext HTTP
///      where ATS blocks cleartext URLSession requests.
@Observable
@MainActor
public final class ConversationEventsClient {
    public private(set) var isConnected = false
    /// Local time of the last frame received from the gateway (any type).
    public private(set) var lastBeatAt: Date?
    /// Desktop language_server state from the last `hello`/`beat` (`ok` / `down` / `unknown`).
    public private(set) var desktopState: String?
    /// True once a `beat` (or `hello` carrying `desktop`) arrived, i.e. the gateway supports heartbeats.
    public private(set) var supportsBeat = false
    /// Bumped on every heartbeat so the indicator can pulse.
    public private(set) var beatCount = 0
    /// Round-trip hint: `now - server ts` is clock-skew-prone, so it is only shown as a rough latency when small.
    public private(set) var lastServerTS: Int64?
    public private(set) var hasEverConnected = false
    public private(set) var startedAt: Date?
    public private(set) var isStarted = false
    public var onChange: (() -> Void)?

    private var runTask: Task<Void, Never>?
    private var connectionWatchdogTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?
    private var attempt = 0
    private var generation = 0
    private var baseURLProvider: (() -> URL?)?
    /// Resolves the in-flight `connectOnce` continuation; lets `stop()` end it without polling.
    private var finishCurrent: (@MainActor () -> Void)?

    // URLSession Engine State
    private var currentSession: URLSession?
    private var currentTask: URLSessionWebSocketTask?

    // NWConnection Fallback Engine State
    private var connection: NWConnection?

    public init() {}

    /// Drops the current socket (if any) and reconnects immediately, e.g. from the indicator's "reconnect" button.
    public func reconnect() {
        guard let provider = baseURLProvider else { return }
        print("[EventsWS] Manual reconnect requested")
        stop()
        start(baseURL: provider)
    }

    /// Foreground check: reconnect right away when the heartbeat has gone stale instead of trusting an old green dot.
    public func reconnectIfStale(maxAge: TimeInterval = 12) {
        guard isStarted else { return }
        if !isConnected {
            print("[EventsWS] Reconnecting because link is currently disconnected")
            reconnect()
            return
        }
        if supportsBeat, let last = lastBeatAt, Date().timeIntervalSince(last) > maxAge {
            print("[EventsWS] Heartbeat stale (>\(maxAge)s), triggering reconnect")
            reconnect()
        }
    }

    /// The indicator state at `now` (see `LinkHealthState`).
    public func linkState(at now: Date = Date()) -> LinkHealthState {
        LinkHealthState.evaluate(
            isStarted: isStarted, isConnected: isConnected, hasEverConnected: hasEverConnected,
            supportsBeat: supportsBeat, lastBeatAt: lastBeatAt, startedAt: startedAt,
            desktopState: desktopState, now: now)
    }

    public func start(baseURL: @escaping () -> URL?) {
        guard runTask == nil else { return }
        isStarted = true
        startedAt = Date()
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
        isStarted = false
        attempt = 0
        tearDown()
        setConnected(false)
        pending?() // resumes the suspended connectOnce so its task can exit
    }

    private func setConnected(_ value: Bool) {
        if isConnected != value {
            isConnected = value
            print("[EventsWS] Connection state changed: isConnected=\(value)")
        }
    }

    private func tearDown() {
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        pingTask?.cancel()
        pingTask = nil

        if let task = currentTask {
            task.cancel(with: .normalClosure, reason: nil)
            currentTask = nil
        }
        if let session = currentSession {
            session.invalidateAndCancel()
            currentSession = nil
        }
        if let conn = connection {
            conn.stateUpdateHandler = nil
            conn.cancel()
            connection = nil
        }
    }

    /// Runs one socket until it ends. Returns true when the handshake succeeded.
    private func connectOnce(generation gen: Int) async -> Bool {
        guard let baseURL = baseURLProvider?(),
              var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true) else {
            print("[EventsWS] Invalid or missing baseURL")
            return false
        }

        var path = components.path
        if !path.hasSuffix("/") { path += "/" }
        components.path = path + "gateway/events"
        components.queryItems = nil
        guard let targetURL = components.url else { return false }

        let hostStr = (targetURL.host ?? "").lowercased()
        let scheme = targetURL.scheme?.lowercased() ?? "http"

        // Tailscale cleartext HTTP cannot pass URLSession ATS; use NWConnection fallback for it
        let isTailscaleCleartext = (scheme == "http" || scheme == "ws") && NetworkTransport.isTailscaleHost(hostStr)

        if isTailscaleCleartext {
            print("[EventsWS] Target is Tailscale cleartext (\(hostStr)), routing via NWConnection fallback")
            return await connectWithNWConnection(targetURL: targetURL, generation: gen)
        } else {
            return await connectWithURLSession(targetURL: targetURL, generation: gen)
        }
    }

    // MARK: - Primary Transport: URLSessionWebSocketTask (Supports System / Wi-Fi Proxies)

    private func connectWithURLSession(targetURL: URL, generation gen: Int) async -> Bool {
        var wsComponents = URLComponents(url: targetURL, resolvingAgainstBaseURL: true)
        let isSecure = wsComponents?.scheme?.lowercased() == "https" || wsComponents?.scheme?.lowercased() == "wss"
        wsComponents?.scheme = isSecure ? "wss" : "ws"
        guard let wsURL = wsComponents?.url ?? targetURL as URL? else { return false }

        var request = URLRequest(url: wsURL)
        request.timeoutInterval = 15

        let hostStr = (wsURL.host ?? "").lowercased()
        let isCloudflare = CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr)
        if isCloudflare {
            request.setValue(hostStr, forHTTPHeaderField: "Host")
        }
        request.setValue("Multigravity/1.1.6 (iPhone; iOS)", forHTTPHeaderField: "User-Agent")

        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let isLoopback = hostStr == "127.0.0.1" || hostStr == "::1" || hostStr == "localhost"

        print("[EventsWS] Connecting via URLSessionWebSocketTask to \(wsURL.absoluteString)")

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

            let delegate = EventsSessionDelegate(isLoopback: isLoopback) {
                Task { @MainActor [weak self] in
                    guard let self, self.generation == gen, !finished else { return }
                    if !opened {
                        print("[EventsWS] WebSocket handshake established (didOpenWithProtocol)")
                        opened = true
                        self.hasEverConnected = true
                        self.connectionWatchdogTask?.cancel()
                        self.connectionWatchdogTask = nil
                        self.setConnected(true)
                        self.startKeepAlivePing(generation: gen)
                    }
                }
            } onClose: { error in
                Task { @MainActor in
                    if let error {
                        print("[EventsWS] WebSocket disconnected with error: \(error.localizedDescription)")
                    } else {
                        print("[EventsWS] WebSocket closed normally")
                    }
                    finish()
                }
            }

            let config = URLSessionConfiguration.default
            config.requestCachePolicy = .reloadIgnoringLocalCacheData
            config.timeoutIntervalForRequest = 20
            config.timeoutIntervalForResource = 60
            let session = URLSession(configuration: config, delegate: delegate, delegateQueue: nil)
            let task = session.webSocketTask(with: request)
            self.currentSession = session
            self.currentTask = task

            task.resume()

            self.receiveURLSessionLoop(task: task, generation: gen, finish: finish, onOpened: { [weak self] in
                guard let self, self.generation == gen, !finished else { return }
                if !opened {
                    print("[EventsWS] First frame received, marking connection active")
                    opened = true
                    self.hasEverConnected = true
                    self.connectionWatchdogTask?.cancel()
                    self.connectionWatchdogTask = nil
                    self.setConnected(true)
                    self.startKeepAlivePing(generation: gen)
                }
            })

            connectionWatchdogTask = Task { [weak self] in
                try? await Task.sleep(nanoseconds: 10_000_000_000)
                guard let self, !Task.isCancelled, self.generation == gen, !opened else { return }
                print("[EventsWS] Handshake watchdog timed out (10s), finishing attempt")
                finish()
            }
        }
    }

    private func receiveURLSessionLoop(
        task: URLSessionWebSocketTask,
        generation gen: Int,
        finish: @escaping @MainActor () -> Void,
        onOpened: @escaping @MainActor () -> Void
    ) {
        task.receive { [weak self, weak task] result in
            Task { @MainActor in
                guard let self, let task, self.generation == gen, self.currentTask === task else { return }
                switch result {
                case .success(let message):
                    onOpened()
                    let data: Data?
                    switch message {
                    case .string(let text):
                        data = text.data(using: .utf8)
                    case .data(let d):
                        data = d
                    @unknown default:
                        data = nil
                    }
                    if let data, !data.isEmpty, let frame = ConversationEventFrame.parse(data) {
                        self.noteFrame(frame)
                        if frame.requestsRefresh {
                            self.onChange?()
                        }
                    }
                    self.receiveURLSessionLoop(task: task, generation: gen, finish: finish, onOpened: onOpened)
                case .failure(let error):
                    print("[EventsWS] Receive error: \(error.localizedDescription)")
                    finish()
                }
            }
        }
    }

    private func startKeepAlivePing(generation gen: Int) {
        pingTask?.cancel()
        pingTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 20_000_000_000) // 20s
                guard let self, !Task.isCancelled, self.generation == gen, let task = self.currentTask else { break }
                task.sendPing { error in
                    if let error {
                        print("[EventsWS] Ping failure: \(error.localizedDescription)")
                    }
                }
            }
        }
    }

    // MARK: - ATS Bypass Fallback: NWConnection (For Tailscale Cleartext Hosts)

    private func connectWithNWConnection(targetURL: URL, generation gen: Int) async -> Bool {
        var components = URLComponents(url: targetURL, resolvingAgainstBaseURL: true)
        components?.scheme = (targetURL.scheme?.lowercased() == "https") ? "wss" : "ws"
        guard let wsURL = components?.url else { return false }

        let hostStr = (wsURL.host ?? "").lowercased()
        let isCloudflare = CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr)
        let endpoint = NWEndpoint.url(wsURL)

        let wsOptions = NWProtocolWebSocket.Options()
        wsOptions.autoReplyPing = true
        wsOptions.maximumMessageSize = 16 * 1024 * 1024
        var extraHeaders: [(String, String)] = []
        if isCloudflare { extraHeaders.append(("Host", hostStr)) }
        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            extraHeaders.append(("Authorization", "Bearer \(token)"))
        }
        if !extraHeaders.isEmpty { wsOptions.setAdditionalHeaders(extraHeaders) }

        let parameters: NWParameters
        if wsURL.scheme?.lowercased() == "wss" {
            let tlsOptions = NWProtocolTLS.Options()
            let isLoopback = hostStr == "127.0.0.1" || hostStr == "::1" || hostStr == "localhost"
            if isLoopback {
                sec_protocol_options_set_verify_block(tlsOptions.securityProtocolOptions, { _, _, completion in
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
                        print("[EventsWS][NWConnection] Connection ready")
                        opened = true
                        self.hasEverConnected = true
                        self.connectionWatchdogTask?.cancel()
                        self.connectionWatchdogTask = nil
                        self.setConnected(true)
                        self.receiveNWLoop(on: conn, generation: gen, finish: finish)
                    case .waiting(let err):
                        print("[EventsWS][NWConnection] Connection waiting: \(err)")
                    case .failed(let err):
                        print("[EventsWS][NWConnection] Connection failed: \(err)")
                        finish()
                    case .cancelled:
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
                print("[EventsWS][NWConnection] Handshake watchdog timed out (10s)")
                finish()
            }
        }
    }

    private func receiveNWLoop(on conn: NWConnection, generation gen: Int, finish: @escaping @MainActor () -> Void) {
        conn.receiveMessage { [weak self] content, _, isComplete, error in
            Task { @MainActor in
                guard let self, self.generation == gen, self.connection === conn else { return }
                if let error {
                    print("[EventsWS][NWConnection] Receive error: \(error)")
                    finish()
                    return
                }
                if let data = content, !data.isEmpty, let frame = ConversationEventFrame.parse(data) {
                    self.noteFrame(frame)
                    if frame.requestsRefresh { self.onChange?() }
                } else if content == nil && isComplete {
                    finish()
                    return
                }
                self.receiveNWLoop(on: conn, generation: gen, finish: finish)
            }
        }
    }

    private func noteFrame(_ frame: ConversationEventFrame) {
        guard frame.isHeartbeat else { return }
        lastBeatAt = Date()
        beatCount &+= 1
        if let desktop = frame.desktop {
            supportsBeat = true
            desktopState = desktop
        }
        if let ts = frame.ts { lastServerTS = ts }
    }
}

// MARK: - EventsSessionDelegate

private final class EventsSessionDelegate: NSObject, URLSessionWebSocketDelegate, Sendable {
    private let onOpen: @Sendable () -> Void
    private let onClose: @Sendable (Error?) -> Void
    private let isLoopback: Bool

    init(isLoopback: Bool, onOpen: @escaping @Sendable () -> Void, onClose: @escaping @Sendable (Error?) -> Void) {
        self.isLoopback = isLoopback
        self.onOpen = onOpen
        self.onClose = onClose
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol proto: String?) {
        onOpen()
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        onClose(nil)
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        onClose(error)
    }

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge, completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        if challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
           let trust = challenge.protectionSpace.serverTrust {
            let host = challenge.protectionSpace.host.lowercased()
            if isLoopback || host == "127.0.0.1" || host == "localhost" || host == "::1" {
                completionHandler(.useCredential, URLCredential(trust: trust))
                return
            }
        }
        completionHandler(.performDefaultHandling, nil)
    }
}
