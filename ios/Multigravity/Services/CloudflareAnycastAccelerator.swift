import Foundation
import Network
import os

/// Cloudflare Anycast 智能动态多线路优选网络加速器 (iOS 三层防 1034 自愈版)
///
/// 核心防护：
/// 1. 仅保留经无代理直连实测 HTTP 200 的健康 Anycast 边缘节点（已彻底移除 108.162.192.1 / 172.64.32.1 /
///    162.159.0.1 / 198.41.214.1 等触发 Cloudflare Error 1034 的受限边缘 IP）。
/// 2. 后台并发赛马测速升级为 TLS + HTTPS 7 层验活（请求 Cloudflare 边缘原生 /cdn-cgi/trace，零 Worker 消耗），
///    仅返回 HTTP 200 的节点才允许成为 verifiedPrimaryIP；返回 403/1034 的节点自动拉黑 24 小时。
/// 3. 在首个节点通过 HTTPS 验活前（或节点全部受限时），verifiedPrimaryIP 返回 nil，让上层直接走系统原生
///    URLSession / DNS，杜绝冷启动误连受限边缘 IP；运行时命中 403/1034 亦可瞬时拉黑并回退系统栈。
public final class CloudflareAnycastAccelerator: @unchecked Sendable {
    public static let shared = CloudflareAnycastAccelerator()

    nonisolated private static let probeHost = "dispatcher.multigravity.app"
    nonisolated private static let probeRequestData = Data(
        "GET /cdn-cgi/trace HTTP/1.1\r\nHost: dispatcher.multigravity.app\r\nUser-Agent: Multigravity-Probe/1.0\r\nConnection: close\r\n\r\n".utf8
    )
    nonisolated private static let probeTimeoutSeconds: TimeInterval = 1.8
    nonisolated private static let banTTLSeconds: TimeInterval = 24 * 60 * 60

    // 经无代理直连实测 HTTP 200 的健康 Cloudflare Anycast 边缘节点池
    public static let candidateEdgeIPs: [String] = [
        // 电信 / 多线 优质亚太直连段
        "172.64.153.208",  // Tokyo / Narita
        "104.20.23.208",   // Tokyo / Narita
        // 移动 / 亚太 CMI 优质直连段
        "162.159.131.182", // Hong Kong Anycast
        "162.159.153.1",
        "162.159.192.1",
        // 联通 / 东京 / 香港经典优质段
        "104.18.0.1",
        "104.18.2.161",    // Hong Kong Anycast
        "104.16.160.1",    // Asia Anycast
        "104.26.0.1",
        // 全球泛播高可用兜底候选段
        "188.114.97.1",
        "172.67.75.1",
        "104.19.16.1"
    ]

    nonisolated private static let maxConcurrentProbeIPs = 6

    private struct ProbeState {
        var cursor: Int = 0
        var lastProbeTime: TimeInterval = 0
    }

    private let winnerLock = OSAllocatedUnfairLock<String?>(initialState: nil)
    private let bannedLock = OSAllocatedUnfairLock<[String: TimeInterval]>(initialState: [:])
    private let isProbingLock = OSAllocatedUnfairLock(initialState: false)
    private let probeStateLock = OSAllocatedUnfairLock(initialState: ProbeState())

    /// 仅在存在通过 HTTPS `/cdn-cgi/trace` 200 验活且未被拉黑的边缘 IP 时返回非空值；
    /// 否则返回 nil，提示调用方安全回退至系统原生 DNS / URLSession。
    public var verifiedPrimaryIP: String? {
        let now = Date().timeIntervalSince1970
        guard let candidate = winnerLock.withLock({ $0 }) else {
            return nil
        }
        if isBanned(candidate, now: now) {
            winnerLock.withLock { current in
                if current == candidate { current = nil }
            }
            return nil
        }
        return candidate
    }

    public static var verifiedPrimaryIP: String? {
        shared.verifiedPrimaryIP
    }

    public var currentPrimaryIP: String {
        verifiedPrimaryIP ?? Self.candidateEdgeIPs[0]
    }

    public static var primaryIP: String {
        shared.currentPrimaryIP
    }

    public init() {
        Task {
            await probeFastestIPsConcurrently()
        }
    }

    public static func isCloudflareTunnelHost(_ host: String) -> Bool {
        let clean = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        if clean == "multigravity.app" || clean == "jiuge.space" || clean.hasSuffix(".trycloudflare.com") {
            return true
        }
        // Only 1st-level *.multigravity.app / *.jiuge.space subdomains (3 DNS labels) are Cloudflare Orange-Cloud proxied;
        // 2nd-level regional VPS subdomains (e.g. r-xxxx.jp1.multigravity.app, >=4 labels) use direct DNS.
        if clean.hasSuffix(".multigravity.app") || clean.hasSuffix(".jiuge.space") {
            let labels = clean.split(separator: ".")
            return labels.count == 3
        }
        return false
    }

    public static func isError1034Body(_ data: Data) -> Bool {
        guard !data.isEmpty, data.count <= 4096 else { return false }
        guard let text = String(data: data, encoding: .utf8) ?? String(data: data, encoding: .ascii) else {
            return false
        }
        return isError1034Text(text)
    }

    nonisolated private static func isError1034Text(_ text: String) -> Bool {
        let lower = text.lowercased()
        return lower.contains("error code: 1034") ||
            (lower.contains("1034") && lower.contains("edge ip"))
    }

    /// 运行时命中 HTTP 403 / error code: 1034 时调用：立即拉黑该 IP 24 小时并清空当前优选结果。
    public func reportEdgeRestricted(ip: String) {
        banIP(ip)
        winnerLock.withLock { current in
            if current == ip {
                current = nil
            }
        }
        triggerProbeIfNeeded(force: true)
    }

    public func triggerProbeIfNeeded(force: Bool = false) {
        let now = Date().timeIntervalSince1970
        let hasWinner = verifiedPrimaryIP != nil
        let minInterval: TimeInterval = hasWinner ? 300 : 60
        let lastTime = probeStateLock.withLock { $0.lastProbeTime }
        if !force && (now - lastTime < minInterval) {
            return
        }
        let shouldProbe = isProbingLock.withLock { isProbing -> Bool in
            if isProbing { return false }
            isProbing = true
            return true
        }
        guard shouldProbe else { return }

        probeStateLock.withLock { $0.lastProbeTime = now }
        Task.detached(priority: .background) { [weak self] in
            guard let self = self else { return }
            defer { self.isProbingLock.withLock { $0 = false } }
            await self.probeFastestIPsConcurrently()
        }
    }

    private func isBanned(_ ip: String, now: TimeInterval) -> Bool {
        bannedLock.withLock { map in
            guard let expiresAt = map[ip] else { return false }
            if now >= expiresAt {
                map.removeValue(forKey: ip)
                return false
            }
            return true
        }
    }

    private func banIP(_ ip: String) {
        let expiresAt = Date().timeIntervalSince1970 + Self.banTTLSeconds
        bannedLock.withLock { map in
            map[ip] = expiresAt
        }
    }

    private func selectProbeBatch() -> [String] {
        let all = Self.candidateEdgeIPs
        guard !all.isEmpty else { return [] }
        let now = Date().timeIntervalSince1970
        let bannedSet: Set<String> = bannedLock.withLock { map in
            var active = Set<String>()
            for (ip, expiresAt) in map {
                if now < expiresAt {
                    active.insert(ip)
                }
            }
            return active
        }
        let initial: [String] = if let currentWinner = verifiedPrimaryIP, !bannedSet.contains(currentWinner) {
            [currentWinner]
        } else {
            []
        }
        let maxCount = Self.maxConcurrentProbeIPs
        return probeStateLock.withLock { state -> [String] in
            var result = initial
            var scanned = 0
            var idx = state.cursor % all.count
            while result.count < maxCount && scanned < all.count {
                let candidate = all[idx]
                if !result.contains(candidate) && !bannedSet.contains(candidate) {
                    result.append(candidate)
                }
                idx = (idx + 1) % all.count
                scanned += 1
            }
            state.cursor = idx
            return result
        }
    }

    private enum ProbeResult: Sendable {
        case ok(Double)
        case restricted
        case failed
    }

    private func probeFastestIPsConcurrently() async {
        let candidates = selectProbeBatch()
        guard !candidates.isEmpty else { return }

        let (validResults, restrictedIPs, failedIPs) = await withTaskGroup(of: (String, ProbeResult).self) { group -> ([(String, Double)], Set<String>, Set<String>) in
            for ip in candidates {
                group.addTask {
                    let result = await Self.probeHTTPTrace(ip: ip, timeoutSeconds: Self.probeTimeoutSeconds)
                    return (ip, result)
                }
            }

            var okList: [(String, Double)] = []
            var restricted: Set<String> = []
            var failed: Set<String> = []
            for await (ip, result) in group {
                switch result {
                case .ok(let rtt):
                    okList.append((ip, rtt))
                case .restricted:
                    restricted.insert(ip)
                case .failed:
                    failed.insert(ip)
                }
            }

            okList.sort { $0.1 < $1.1 }
            return (okList, restricted, failed)
        }

        for ip in restrictedIPs {
            banIP(ip)
        }

        if let fastest = validResults.first?.0 {
            winnerLock.withLock { $0 = fastest }
        } else {
            winnerLock.withLock { current in
                if let cur = current, restrictedIPs.contains(cur) || failedIPs.contains(cur) {
                    current = nil
                }
            }
        }
    }

    nonisolated private static func probeHTTPTrace(ip: String, timeoutSeconds: TimeInterval) async -> ProbeResult {
        let requestData = Self.probeRequestData
        let tlsOptions = NWProtocolTLS.Options()
        sec_protocol_options_set_tls_server_name(tlsOptions.securityProtocolOptions, Self.probeHost)
        let params = NWParameters(tls: tlsOptions)
        params.preferNoProxies = true

        let endpoint = NWEndpoint.hostPort(host: NWEndpoint.Host(ip), port: .https)
        let connection = NWConnection(to: endpoint, using: params)
        let startTime = Date().timeIntervalSince1970

        return await withCheckedContinuation { continuation in
            let stateLock = OSAllocatedUnfairLock(initialState: (finished: false, sent: false))

            @Sendable func complete(_ result: ProbeResult) {
                let shouldRun = stateLock.withLock { state -> Bool in
                    if state.finished { return false }
                    state.finished = true
                    return true
                }
                guard shouldRun else { return }
                connection.cancel()
                continuation.resume(returning: result)
            }

            connection.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    let shouldSend = stateLock.withLock { s -> Bool in
                        if s.finished || s.sent { return false }
                        s.sent = true
                        return true
                    }
                    guard shouldSend else { return }
                    connection.send(content: requestData, completion: .contentProcessed { sendErr in
                        if sendErr != nil {
                            complete(.failed)
                            return
                        }
                        connection.receive(minimumIncompleteLength: 1, maximumLength: 512) { data, _, _, recvErr in
                            if recvErr != nil {
                                complete(.failed)
                                return
                            }
                            guard let data, !data.isEmpty,
                                  let text = String(data: data, encoding: .utf8) ?? String(data: data, encoding: .ascii) else {
                                complete(.failed)
                                return
                            }
                            let firstLine = text.components(separatedBy: "\r\n").first ?? text
                            let statusCode = firstLine.split(separator: " ").dropFirst().first.flatMap { Int($0) } ?? -1
                            let elapsedMs = (Date().timeIntervalSince1970 - startTime) * 1000
                            if statusCode == 200 {
                                complete(.ok(elapsedMs))
                            } else if statusCode == 403 || Self.isError1034Text(text) {
                                complete(.restricted)
                            } else {
                                complete(.failed)
                            }
                        }
                    })
                case .failed, .cancelled:
                    complete(.failed)
                default:
                    break
                }
            }

            connection.start(queue: DispatchQueue.global(qos: .utility))

            DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + timeoutSeconds) {
                complete(.failed)
            }
        }
    }
}
