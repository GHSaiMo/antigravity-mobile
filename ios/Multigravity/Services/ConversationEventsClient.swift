import Foundation
import Observation

/// Listens for conversation-list change hints and gateway heartbeats from `/gateway/events`.
/// Every `hello` (connect/reconnect) and `changed` frame calls `onChange`; the owner then
/// refetches the list through the normal list API. `isConnected` tells the owner when
/// it may relax its own polling and flips back to false as soon as the socket is lost.
///
/// Transport:
/// Routes through `GatewayWebSocketTask`, which transparently:
/// 1. Wraps public relay traffic inside E2EE (`/api/v1/e2ee/ws`, AES-256-GCM) so relay hosts
///    enforcing `HTTP 426 e2ee_required` stay permanently connected.
/// 2. Preserves system Wi-Fi / proxy support via `URLSessionWebSocketTask` for LAN & HTTPS.
/// 3. Switches to `NWConnection` for Tailscale cleartext `ws://` hosts blocked by iOS ATS.
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
    public var onConnectionStateChange: ((Bool) -> Void)?
    /// Invoked when the events socket hits relay 502/503 or fails to establish so the owner can
    /// trigger endpoint probing (e.g. fast-switching back to LAN or a healthy relay node).
    public var onNeedRouteProbe: (() -> Void)?

    private var runTask: Task<Void, Never>?
    private var connectionWatchdogTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?
    private var attempt = 0
    private var generation = 0
    private var currentOpenedAt: Date?
    private var currentReceivedValidFrame = false
    private var currentAuthRejected = false
    private var baseURLProvider: (() -> URL?)?
    private var lastRev: Int64 = 0
    private var lastRevHost: String?
    /// Resolves the in-flight `connectOnce` continuation; lets `stop()` end it without polling.
    private var finishCurrent: (@MainActor () -> Void)?
    private var currentTask: GatewayWebSocketTask?

    public init() {}

    /// Drops the current socket (if any) and reconnects immediately, e.g. from the indicator's "reconnect" button
    /// or after the active gateway route switches.
    public func reconnect(onlyIfStarted: Bool = false) {
        if onlyIfStarted && !isStarted { return }
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
                self.currentOpenedAt = nil
                self.currentReceivedValidFrame = false
                self.currentAuthRejected = false
                let opened = await self.connectOnce(generation: gen)
                guard !Task.isCancelled, self.generation == gen else { return }
                if self.currentAuthRejected {
                    self.stop()
                    return
                }
                let livedLongEnough: Bool = {
                    guard let t0 = self.currentOpenedAt else { return false }
                    return Date().timeIntervalSince(t0) >= 15.0
                }()
                let wasHealthy = opened && (self.currentReceivedValidFrame || livedLongEnough)
                self.attempt = wasHealthy ? 0 : min(self.attempt + 1, 6)
                if !wasHealthy && (self.attempt == 1 || self.attempt % 2 == 1) {
                    self.onNeedRouteProbe?()
                }
                let baseDelay: Double = wasHealthy ? 1.0 : min(2.5 * pow(2.0, Double(max(self.attempt - 1, 0))), 60.0)
                let delay = baseDelay * Double.random(in: 0.75...1.25)
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
        currentOpenedAt = nil
        currentReceivedValidFrame = false
        currentAuthRejected = false
        tearDown()
        setConnected(false)
        pending?() // resumes the suspended connectOnce so its task can exit
    }

    private func setConnected(_ value: Bool) {
        if isConnected != value {
            isConnected = value
            if value && currentOpenedAt == nil {
                currentOpenedAt = Date()
            }
            print("[EventsWS] Connection state changed: isConnected=\(value)")
            onConnectionStateChange?(value)
        }
    }

    private func tearDown() {
        connectionWatchdogTask?.cancel()
        connectionWatchdogTask = nil
        pingTask?.cancel()
        pingTask = nil

        if let task = currentTask {
            currentTask = nil
            task.cancel()
        }
    }

    /// Runs one socket until it ends. Returns true when the handshake succeeded.
    private func connectOnce(generation gen: Int) async -> Bool {
        if DemoGateway.isEnabled {
            return await runDemoHeartbeat(generation: gen)
        }

        guard let baseURL = baseURLProvider?(),
              var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: true) else {
            print("[EventsWS] Invalid or missing baseURL")
            return false
        }

        var path = components.path
        if !path.hasSuffix("/") { path += "/" }
        components.path = path + "gateway/events"

        let hostStr = (components.host ?? "").lowercased()
        if lastRevHost != nil && lastRevHost != hostStr {
            lastRev = 0
        }
        lastRevHost = hostStr

        if lastRev > 0 {
            components.queryItems = [URLQueryItem(name: "sinceRev", value: "\(lastRev)")]
        } else {
            components.queryItems = nil
        }

        let isSecure = components.scheme?.lowercased() == "https" || components.scheme?.lowercased() == "wss"
        components.scheme = isSecure ? "wss" : "ws"
        guard let wsURL = components.url else { return false }

        var request = URLRequest(url: wsURL)
        request.timeoutInterval = 15
        if CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr) {
            request.setValue(hostStr, forHTTPHeaderField: "Host")
        }
        request.setValue("Multigravity/1.2.5 (iPhone; iOS)", forHTTPHeaderField: "User-Agent")
        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        print("[EventsWS] Connecting via GatewayWebSocketTask to \(wsURL.absoluteString)")

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

            let markOpened: @MainActor () -> Void = { [weak self] in
                guard let self, self.generation == gen, !finished else { return }
                if !opened {
                    print("[EventsWS] WebSocket handshake established")
                    opened = true
                    self.hasEverConnected = true
                    self.connectionWatchdogTask?.cancel()
                    self.connectionWatchdogTask = nil
                    self.setConnected(true)
                    self.startKeepAlivePing(generation: gen)
                }
            }

            let wsTask = GatewayWebSocketTask(
                request: request,
                requestTimeout: 20,
                resourceTimeout: 86_400,
                waitsForConnectivity: false,
                onOpen: {
                    Task { @MainActor in
                        markOpened()
                    }
                },
                onClose: {
                    Task { @MainActor in
                        print("[EventsWS] WebSocket closed normally")
                        finish()
                    }
                },
                onFail: { error in
                    Task { @MainActor in
                        print("[EventsWS] WebSocket disconnected with error: \(error.localizedDescription)")
                        finish()
                    }
                },
                onAuthRejected: { [weak self] in
                    Task { @MainActor [weak self] in
                        self?.currentAuthRejected = true
                    }
                },
                onRelayUnavailable: { [weak self] in
                    Task { @MainActor [weak self] in
                        self?.onNeedRouteProbe?()
                    }
                }
            )
            self.currentTask = wsTask
            wsTask.resume()
            self.receiveLoop(task: wsTask, generation: gen, finish: finish, onOpened: markOpened)

            connectionWatchdogTask = Task { [weak self] in
                try? await Task.sleep(nanoseconds: 10_000_000_000)
                guard let self, !Task.isCancelled, self.generation == gen, !opened else { return }
                print("[EventsWS] Handshake watchdog timed out (10s), finishing attempt")
                finish()
            }
        }
    }

    /// Simulates an active `/gateway/events` heartbeat stream while running in Demo Mode so the
    /// home indicator dot and link diagnostics reflect live connected telemetry without network I/O.
    private func runDemoHeartbeat(generation gen: Int) async -> Bool {
        guard !Task.isCancelled, self.generation == gen else { return false }
        hasEverConnected = true
        supportsBeat = true
        desktopState = "ok"
        lastBeatAt = Date()
        lastServerTS = Int64(Date().timeIntervalSince1970 * 1000)
        beatCount &+= 1
        setConnected(true)
        onChange?()

        while !Task.isCancelled, self.generation == gen, DemoGateway.isEnabled {
            try? await Task.sleep(nanoseconds: 5_000_000_000)
            guard !Task.isCancelled, self.generation == gen, DemoGateway.isEnabled else { break }
            lastBeatAt = Date()
            lastServerTS = Int64(Date().timeIntervalSince1970 * 1000)
            desktopState = "ok"
            beatCount &+= 1
        }
        setConnected(false)
        return true
    }

    private func receiveLoop(
        task: GatewayWebSocketTask,
        generation gen: Int,
        finish: @escaping @MainActor () -> Void,
        onOpened: @escaping @MainActor () -> Void
    ) {
        task.receive { [weak self, weak task] result in
            Task { @MainActor [weak self, weak task] in
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
                    }
                    if let data, !data.isEmpty, let frame = ConversationEventFrame.parse(data) {
                        self.noteFrame(frame)
                        if frame.requestsRefresh {
                            self.onChange?()
                        }
                    }
                    self.receiveLoop(task: task, generation: gen, finish: finish, onOpened: onOpened)
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

    private func noteFrame(_ frame: ConversationEventFrame) {
        currentReceivedValidFrame = true
        if frame.rev > 0 { lastRev = frame.rev }
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
