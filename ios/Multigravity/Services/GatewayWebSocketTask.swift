import Foundation
import Network
import os

/// Unified WebSocket task wrapper that uses `URLSessionWebSocketTask` by default (preserving system
/// proxy support on RFC1918 LAN and HTTPS) and transparently switches to `NWConnection` with
/// `NWProtocolWebSocket` when targeting Tailscale cleartext `ws://` hosts blocked by iOS ATS.
public final class GatewayWebSocketTask: @unchecked Sendable {
    public enum Message: Sendable {
        case string(String)
        case data(Data)
    }

    private enum Engine {
        case urlSession(session: URLSession, task: URLSessionWebSocketTask, delegate: SessionDelegate)
        case nwConnection(connection: NWConnection)
    }

    private struct E2EEConfig: Sendable {
        let deviceID: String
        let key: Data
        let openPath: String
        let openHeaders: [String: String]
    }

    private let engine: Engine
    private let e2eeConfig: E2EEConfig?
    private let stateLock = OSAllocatedUnfairLock(initialState: State())
    private let onOpen: (@Sendable () -> Void)?
    private let onClose: (@Sendable () -> Void)?
    private let onFail: (@Sendable (any Error) -> Void)?
    private let onAuthRejected: (@Sendable () -> Void)?
    private let originalHost: String?

    private struct State {
        var didOpen = false
        var didFinish = false
        var isCancelled = false
    }

    public init(
        request: URLRequest,
        requestTimeout: TimeInterval = 15,
        resourceTimeout: TimeInterval = 600,
        waitsForConnectivity: Bool = false,
        onOpen: (@Sendable () -> Void)? = nil,
        onClose: (@Sendable () -> Void)? = nil,
        onFail: (@Sendable (any Error) -> Void)? = nil,
        onAuthRejected: (@Sendable () -> Void)? = nil
    ) {
        self.onOpen = onOpen
        self.onClose = onClose
        self.onFail = onFail
        self.onAuthRejected = onAuthRejected
        self.originalHost = request.url?.host

        var effectiveRequest = request
        var resolvedE2EE: E2EEConfig? = nil

        if let rawURL = request.url,
           E2EECrypto.shared.shouldEncrypt(url: rawURL),
           let deviceID = E2EECrypto.shared.currentDeviceID,
           let key = E2EECrypto.shared.currentDeviceKey,
           let host = rawURL.host {
            var innerPath = rawURL.path.isEmpty ? "/" : rawURL.path
            if let q = rawURL.query, !q.isEmpty {
                innerPath += "?\(q)"
            }
            var innerHeaders = request.allHTTPHeaderFields ?? [:]
            if innerHeaders["Authorization"] == nil && innerHeaders["authorization"] == nil,
               let tok = KeychainHelper.shared.read(key: .deviceToken), !tok.isEmpty {
                innerHeaders["Authorization"] = "Bearer \(tok)"
            }
            resolvedE2EE = E2EEConfig(deviceID: deviceID, key: key, openPath: innerPath, openHeaders: innerHeaders)

            var outerComps = URLComponents()
            outerComps.scheme = "wss"
            outerComps.host = host
            outerComps.port = rawURL.port
            outerComps.path = "/api/v1/e2ee/ws"
            outerComps.queryItems = [URLQueryItem(name: "d", value: deviceID)]
            if let outerURL = outerComps.url {
                var outerReq = URLRequest(url: outerURL)
                outerReq.timeoutInterval = request.timeoutInterval
                if let hostHdr = request.value(forHTTPHeaderField: "Host") {
                    outerReq.setValue(hostHdr, forHTTPHeaderField: "Host")
                }
                effectiveRequest = outerReq
            }
        }
        self.e2eeConfig = resolvedE2EE

        if let rawURL = effectiveRequest.url, NetworkTransport.requiresCleartextWebSocketATSBypass(rawURL) {
            var comps = URLComponents(url: rawURL, resolvingAgainstBaseURL: true)
            if comps?.scheme?.lowercased() == "http" {
                comps?.scheme = "ws"
            }
            let wsURL = comps?.url ?? rawURL
            let wsOptions = NWProtocolWebSocket.Options()
            wsOptions.autoReplyPing = true
            wsOptions.maximumMessageSize = 16 * 1024 * 1024

            var extraHeaders: [(String, String)] = []
            var subprotocols: [String] = []
            if let fields = effectiveRequest.allHTTPHeaderFields {
                for (key, value) in fields {
                    let lower = key.trimmingCharacters(in: .whitespaces).lowercased()
                    if lower == "host" || lower == "connection" || lower == "upgrade" ||
                        lower == "sec-websocket-key" || lower == "sec-websocket-version" {
                        continue
                    }
                    if lower == "sec-websocket-protocol" {
                        let parts = value.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
                        subprotocols.append(contentsOf: parts)
                    } else {
                        extraHeaders.append((key, value))
                    }
                }
            }
            if !subprotocols.isEmpty {
                wsOptions.setSubprotocols(subprotocols)
            }
            if !extraHeaders.isEmpty {
                wsOptions.setAdditionalHeaders(extraHeaders)
            }

            let params = NWParameters.tcp
            params.defaultProtocolStack.applicationProtocols.insert(wsOptions, at: 0)
            let conn = NWConnection(to: .url(wsURL), using: params)
            self.engine = .nwConnection(connection: conn)

            conn.stateUpdateHandler = { [weak self] state in
                guard let self else { return }
                switch state {
                case .ready:
                    let fire = self.stateLock.withLock { st -> Bool in
                        guard !st.didOpen && !st.isCancelled else { return false }
                        st.didOpen = true
                        return true
                    }
                    if fire {
                        self.onOpen?()
                    }
                case .failed(let error):
                    let fire = self.stateLock.withLock { st -> Bool in
                        guard !st.didFinish && !st.isCancelled else { return false }
                        st.didFinish = true
                        return true
                    }
                    if fire {
                        self.onFail?(error)
                    }
                case .cancelled:
                    let fire = self.stateLock.withLock { st -> Bool in
                        guard !st.didFinish && !st.isCancelled else { return false }
                        st.didFinish = true
                        return true
                    }
                    if fire {
                        self.onClose?()
                    }
                default:
                    break
                }
            }
        } else {
            let config = URLSessionConfiguration.default
            config.timeoutIntervalForRequest = requestTimeout
            config.timeoutIntervalForResource = resourceTimeout
            config.waitsForConnectivity = waitsForConnectivity

            let delegate = SessionDelegate()
            let session = URLSession(configuration: config, delegate: delegate, delegateQueue: nil)
            let task = session.webSocketTask(with: effectiveRequest)
            self.engine = .urlSession(session: session, task: task, delegate: delegate)
            delegate.owner = self
        }
    }

    public func resume() {
        switch engine {
        case .urlSession(_, let task, _):
            task.resume()
        case .nwConnection(let connection):
            connection.start(queue: DispatchQueue.global(qos: .userInitiated))
        }
    }

    public func send(_ message: Message, completionHandler: @escaping @Sendable ((any Error)?) -> Void) {
        let outgoing: Message
        if let e2ee = e2eeConfig {
            var plain = Data()
            switch message {
            case .string(let text):
                plain.append(1) // websocket.TextMessage
                plain.append(Data(text.utf8))
            case .data(let data):
                plain.append(2) // websocket.BinaryMessage
                plain.append(data)
            }
            do {
                let enc = try E2EECrypto.encrypt(
                    key: e2ee.key,
                    plaintext: plain,
                    aad: E2EECrypto.frameAAD(deviceID: e2ee.deviceID, direction: "ws-c2s")
                )
                outgoing = .data(enc)
            } catch {
                completionHandler(error)
                return
            }
        } else {
            outgoing = message
        }

        switch engine {
        case .urlSession(_, let task, _):
            switch outgoing {
            case .string(let text):
                task.send(.string(text), completionHandler: completionHandler)
            case .data(let data):
                task.send(.data(data), completionHandler: completionHandler)
            }
        case .nwConnection(let connection):
            switch outgoing {
            case .string(let text):
                let meta = NWProtocolWebSocket.Metadata(opcode: .text)
                let ctx = NWConnection.ContentContext(identifier: "ws-text", metadata: [meta])
                connection.send(content: Data(text.utf8), contentContext: ctx, isComplete: true, completion: .contentProcessed { err in
                    completionHandler(err)
                })
            case .data(let data):
                let meta = NWProtocolWebSocket.Metadata(opcode: .binary)
                let ctx = NWConnection.ContentContext(identifier: "ws-binary", metadata: [meta])
                connection.send(content: data, contentContext: ctx, isComplete: true, completion: .contentProcessed { err in
                    completionHandler(err)
                })
            }
        }
    }

    public func receive(completionHandler: @escaping @Sendable (Result<Message, any Error>) -> Void) {
        let e2ee = self.e2eeConfig
        let deliver: @Sendable (Result<Message, any Error>) -> Void = { result in
            guard let e2ee else {
                completionHandler(result)
                return
            }
            switch result {
            case .failure(let err):
                completionHandler(.failure(err))
            case .success(let msg):
                guard case .data(let enc) = msg else {
                    completionHandler(.failure(E2EEError.decryptionFailed))
                    return
                }
                do {
                    let plain = try E2EECrypto.decrypt(
                        key: e2ee.key,
                        frame: enc,
                        aad: E2EECrypto.frameAAD(deviceID: e2ee.deviceID, direction: "ws-s2c")
                    )
                    guard let rawHeader = plain.first else {
                        completionHandler(.failure(E2EEError.decryptionFailed))
                        return
                    }
                    let isCompressed = (rawHeader & 0x80) != 0
                    let msgType = rawHeader & 0x7F
                    let rawPayload = Data(plain.dropFirst())
                    let payload = isCompressed ? try E2EECrypto.inflateRaw(rawPayload) : rawPayload
                    if msgType == 1 {
                        let text = String(data: payload, encoding: .utf8) ?? ""
                        completionHandler(.success(.string(text)))
                    } else {
                        completionHandler(.success(.data(payload)))
                    }
                } catch {
                    completionHandler(.failure(error))
                }
            }
        }

        switch engine {
        case .urlSession(_, let task, _):
            task.receive { result in
                switch result {
                case .success(let wsMsg):
                    switch wsMsg {
                    case .string(let str):
                        deliver(.success(.string(str)))
                    case .data(let data):
                        deliver(.success(.data(data)))
                    @unknown default:
                        deliver(.success(.data(Data())))
                    }
                case .failure(let err):
                    deliver(.failure(err))
                }
            }
        case .nwConnection(let connection):
            connection.receiveMessage { [weak self] content, context, isComplete, error in
                if let error {
                    deliver(.failure(error))
                    return
                }
                let meta = context?.protocolMetadata(definition: NWProtocolWebSocket.definition) as? NWProtocolWebSocket.Metadata
                if let opcode = meta?.opcode {
                    switch opcode {
                    case .text:
                        let text = String(data: content ?? Data(), encoding: .utf8) ?? ""
                        deliver(.success(.string(text)))
                        return
                    case .binary:
                        deliver(.success(.data(content ?? Data())))
                        return
                    case .close:
                        if let self {
                            let fire = self.stateLock.withLock { st -> Bool in
                                guard !st.didFinish && !st.isCancelled else { return false }
                                st.didFinish = true
                                return true
                            }
                            if fire {
                                self.onClose?()
                            }
                        }
                        deliver(.failure(URLError(.networkConnectionLost)))
                        return
                    case .ping, .pong, .cont:
                        self?.receive(completionHandler: completionHandler)
                        return
                    @unknown default:
                        break
                    }
                }
                if let data = content, !data.isEmpty {
                    deliver(.success(.data(data)))
                } else if isComplete {
                    deliver(.failure(URLError(.networkConnectionLost)))
                } else {
                    self?.receive(completionHandler: completionHandler)
                }
            }
        }
    }

    public func sendPing(completionHandler: @escaping @Sendable ((any Error)?) -> Void) {
        switch engine {
        case .urlSession(_, let task, _):
            task.sendPing(pongReceiveHandler: completionHandler)
        case .nwConnection(let connection):
            let meta = NWProtocolWebSocket.Metadata(opcode: .ping)
            let ctx = NWConnection.ContentContext(identifier: "ws-ping", metadata: [meta])
            connection.send(content: Data(), contentContext: ctx, isComplete: true, completion: .contentProcessed { err in
                completionHandler(err)
            })
        }
    }

    public func cancel() {
        stateLock.withLock { st in
            st.isCancelled = true
            st.didFinish = true
        }
        switch engine {
        case .urlSession(let session, let task, let delegate):
            delegate.owner = nil
            task.cancel(with: .normalClosure, reason: nil)
            session.invalidateAndCancel()
        case .nwConnection(let connection):
            let meta = NWProtocolWebSocket.Metadata(opcode: .close)
            meta.closeCode = .protocolCode(.normalClosure)
            let ctx = NWConnection.ContentContext(identifier: "ws-close", metadata: [meta])
            connection.send(content: nil, contentContext: ctx, isComplete: true, completion: .idempotent)
            connection.cancel()
        }
    }

    fileprivate func notifySessionOpened() {
        let fire = stateLock.withLock { st -> Bool in
            guard !st.didOpen && !st.isCancelled else { return false }
            st.didOpen = true
            return true
        }
        guard fire else { return }

        if let e2ee = e2eeConfig {
            struct OpenPayload: Encodable {
                let path: String
                let headers: [String: String]?
                let enc: String?
            }
            do {
                let openJSON = try JSONEncoder().encode(OpenPayload(
                    path: e2ee.openPath,
                    headers: e2ee.openHeaders.isEmpty ? nil : e2ee.openHeaders,
                    enc: "flate"
                ))
                let frame0 = try E2EECrypto.encrypt(
                    key: e2ee.key,
                    plaintext: openJSON,
                    aad: E2EECrypto.frameAAD(deviceID: e2ee.deviceID, direction: "ws-open")
                )
                switch engine {
                case .urlSession(_, let task, _):
                    task.send(.data(frame0)) { [weak self] err in
                        guard let self else { return }
                        if let err {
                            self.notifySessionFailed(err)
                        } else {
                            self.onOpen?()
                        }
                    }
                case .nwConnection(let connection):
                    let meta = NWProtocolWebSocket.Metadata(opcode: .binary)
                    let ctx = NWConnection.ContentContext(identifier: "ws-e2ee-open", metadata: [meta])
                    connection.send(content: frame0, contentContext: ctx, isComplete: true, completion: .contentProcessed { [weak self] err in
                        guard let self else { return }
                        if let err {
                            self.notifySessionFailed(err)
                        } else {
                            self.onOpen?()
                        }
                    })
                }
            } catch {
                notifySessionFailed(error)
            }
            return
        }

        onOpen?()
    }

    fileprivate func notifySessionClosed() {
        let fire = stateLock.withLock { st -> Bool in
            guard !st.didFinish && !st.isCancelled else { return false }
            st.didFinish = true
            return true
        }
        if fire {
            onClose?()
        }
    }

    fileprivate func notifySessionFailed(_ error: any Error) {
        let fire = stateLock.withLock { st -> Bool in
            guard !st.didFinish && !st.isCancelled else { return false }
            st.didFinish = true
            return true
        }
        if fire {
            onFail?(error)
        }
    }

    fileprivate func notifyAuthRejected() {
        onAuthRejected?()
    }

    fileprivate func handleHTTPUpgradeStatus(_ statusCode: Int) {
        if statusCode == 401 || statusCode == 403 {
            onAuthRejected?()
        } else if statusCode == 426, let host = originalHost, !host.isEmpty, E2EECrypto.shared.isReady {
            E2EECrypto.shared.markHostRequiresE2EE(host)
        }
    }

    private final class SessionDelegate: NSObject, URLSessionWebSocketDelegate, @unchecked Sendable {
        weak var owner: GatewayWebSocketTask?

        func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocol: String?) {
            owner?.notifySessionOpened()
        }

        func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
            if closeCode.rawValue == 4001 || closeCode.rawValue == 4003 {
                owner?.notifyAuthRejected()
            }
            owner?.notifySessionClosed()
        }

        func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: (any Error)?) {
            if let httpCode = (task.response as? HTTPURLResponse)?.statusCode {
                owner?.handleHTTPUpgradeStatus(httpCode)
            }
            if let error {
                owner?.notifySessionFailed(error)
            } else {
                owner?.notifySessionClosed()
            }
        }

        func urlSession(
            _ session: URLSession,
            didReceive challenge: URLAuthenticationChallenge,
            completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void
        ) {
            if challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
               let trust = challenge.protectionSpace.serverTrust {
                let host = challenge.protectionSpace.host.lowercased()
                if host == "127.0.0.1" || host == "localhost" || host == "::1" {
                    completionHandler(.useCredential, URLCredential(trust: trust))
                    return
                }
            }
            completionHandler(.performDefaultHandling, nil)
        }
    }
}
