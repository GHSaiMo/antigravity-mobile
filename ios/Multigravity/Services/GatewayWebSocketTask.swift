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

    private let engine: Engine
    private let stateLock = OSAllocatedUnfairLock(initialState: State())
    private let onOpen: (@Sendable () -> Void)?
    private let onClose: (@Sendable () -> Void)?
    private let onFail: (@Sendable (any Error) -> Void)?

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
        onFail: (@Sendable (any Error) -> Void)? = nil
    ) {
        self.onOpen = onOpen
        self.onClose = onClose
        self.onFail = onFail

        if let rawURL = request.url, NetworkTransport.requiresCleartextWebSocketATSBypass(rawURL) {
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
            if let fields = request.allHTTPHeaderFields {
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
            let task = session.webSocketTask(with: request)
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
        switch engine {
        case .urlSession(_, let task, _):
            switch message {
            case .string(let text):
                task.send(.string(text), completionHandler: completionHandler)
            case .data(let data):
                task.send(.data(data), completionHandler: completionHandler)
            }
        case .nwConnection(let connection):
            switch message {
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
        switch engine {
        case .urlSession(_, let task, _):
            task.receive { result in
                switch result {
                case .success(let wsMsg):
                    switch wsMsg {
                    case .string(let str):
                        completionHandler(.success(.string(str)))
                    case .data(let data):
                        completionHandler(.success(.data(data)))
                    @unknown default:
                        completionHandler(.success(.data(Data())))
                    }
                case .failure(let err):
                    completionHandler(.failure(err))
                }
            }
        case .nwConnection(let connection):
            connection.receiveMessage { [weak self] content, context, isComplete, error in
                if let error {
                    completionHandler(.failure(error))
                    return
                }
                let meta = context?.protocolMetadata(definition: NWProtocolWebSocket.definition) as? NWProtocolWebSocket.Metadata
                if let opcode = meta?.opcode {
                    switch opcode {
                    case .text:
                        let text = String(data: content ?? Data(), encoding: .utf8) ?? ""
                        completionHandler(.success(.string(text)))
                        return
                    case .binary:
                        completionHandler(.success(.data(content ?? Data())))
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
                        completionHandler(.failure(URLError(.networkConnectionLost)))
                        return
                    case .ping, .pong, .cont:
                        self?.receive(completionHandler: completionHandler)
                        return
                    @unknown default:
                        break
                    }
                }
                if let data = content, !data.isEmpty {
                    completionHandler(.success(.data(data)))
                } else if isComplete {
                    completionHandler(.failure(URLError(.networkConnectionLost)))
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
        if fire {
            onOpen?()
        }
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

    private final class SessionDelegate: NSObject, URLSessionWebSocketDelegate, @unchecked Sendable {
        weak var owner: GatewayWebSocketTask?

        func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didOpenWithProtocol protocol: String?) {
            owner?.notifySessionOpened()
        }

        func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask, didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
            owner?.notifySessionClosed()
        }

        func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: (any Error)?) {
            if let error {
                owner?.notifySessionFailed(error)
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
