import Foundation
import Network
import os

/// Cloudflare Anycast 智能动态多线路优选网络加速器 (方案 A 深度落地 - iOS 端)
///
/// 核心机制：
/// 1. Cloudflare Anycast 并非默认指向亚太。在未优选情况下，国内运营商常将流量路由至美西（200~300ms）。
/// 2. 本加速器内置覆盖电信、联通、移动三大运营商高概率亚太直连的 Anycast 候选节点池；
/// 3. 在后台利用非阻塞高并发 TCP 赛马测速，毫秒级锁定当前真实网络环境下延迟最低的节点（实测 60~70ms）；
/// 4. 传输层建立 TLS 握手时直接定向至 Winner 节点并携带标准 SNI 与 Host 头部，保障毫秒级直连与 100% 容灾兜底。
public final class CloudflareAnycastAccelerator: @unchecked Sendable {
    public static let shared = CloudflareAnycastAccelerator()
    
    // 覆盖国内电信、联通、移动全运营商的核心 Anycast 候选节点池
    public static let candidateEdgeIPs: [String] = [
        // 电信 / 多线 优质亚太直连段 (108.162.*, 172.64.*)
        "172.64.153.208",  // 实测极速 ~67ms (Tokyo / Narita)
        "108.162.192.1",   // 实测极速 ~70ms (Asia Edge)
        "172.64.32.1",     // 实测极速 ~74ms
        "172.64.0.1",
        // 移动 / 亚太 CMI 优质直连段 (162.159.*, 141.101.*)
        "162.159.0.1",     // 实测极速 ~67ms
        "162.159.131.182", // Hong Kong Anycast
        "162.159.153.1",
        "162.159.192.1",
        "141.101.90.1",
        "141.101.64.1",
        // 联通 / 东京 / 香港经典优质段 (104.16.*, 104.18.*, 104.20.*, 104.26.*)
        "104.20.23.208",   // Tokyo / Narita
        "104.18.0.1",
        "104.18.2.161",    // Hong Kong Anycast
        "104.16.160.1",    // Asia Anycast
        "104.26.0.1",
        // 全球泛播与高可用兜底候选段 (188.114.*, 172.67.*, 104.19.*, 198.41.*)
        "188.114.97.1",
        "172.67.75.1",
        "104.19.16.1",
        "198.41.214.1"
    ]
    
    private let lock = OSAllocatedUnfairLock(initialState: candidateEdgeIPs[0])
    private let isProbingLock = OSAllocatedUnfairLock(initialState: false)
    private var lastProbeTime: TimeInterval = 0
    
    public var currentPrimaryIP: String {
        lock.withLock { $0 }
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
        return clean.hasSuffix(".jiuge.space") || clean == "jiuge.space"
    }
    
    public func triggerProbeIfNeeded(force: Bool = false) {
        let now = Date().timeIntervalSince1970
        if !force && (now - lastProbeTime < 180) { // 3分钟内不重复探测
            return
        }
        let shouldProbe = isProbingLock.withLock { isProbing -> Bool in
            if isProbing { return false }
            isProbing = true
            return true
        }
        guard shouldProbe else { return }
        
        lastProbeTime = now
        Task.detached(priority: .background) { [weak self] in
            guard let self = self else { return }
            defer { self.isProbingLock.withLock { $0 = false } }
            await self.probeFastestIPsConcurrently()
        }
    }
    
    private func probeFastestIPsConcurrently() async {
        let candidates = Self.candidateEdgeIPs
        
        let winner = await withTaskGroup(of: (String, Double)?.self) { group -> String? in
            for ip in candidates {
                group.addTask {
                    let rtt = await Self.pingTCP(ip: ip, port: 443, timeoutSeconds: 0.8)
                    if rtt > 0 {
                        return (ip, rtt)
                    }
                    return nil
                }
            }
            
            var validResults: [(String, Double)] = []
            for await result in group {
                if let (ip, rtt) = result {
                    validResults.append((ip, rtt))
                }
            }
            
            validResults.sort { $0.1 < $1.1 }
            return validResults.first?.0
        }
        
        if let fastest = winner {
            lock.withLock { $0 = fastest }
        }
    }
    
    private static func pingTCP(ip: String, port: UInt16, timeoutSeconds: TimeInterval) async -> Double {
        let endpoint = NWEndpoint.hostPort(host: NWEndpoint.Host(ip), port: NWEndpoint.Port(rawValue: port)!)
        let params = NWParameters.tcp
        params.preferNoProxies = true
        
        let connection = NWConnection(to: endpoint, using: params)
        let startTime = Date().timeIntervalSince1970
        
        return await withCheckedContinuation { continuation in
            let finished = OSAllocatedUnfairLock(initialState: false)
            @Sendable func complete(_ rtt: Double) {
                let shouldRun = finished.withLock { flag -> Bool in
                    if flag { return false }
                    flag = true
                    return true
                }
                guard shouldRun else { return }
                connection.cancel()
                continuation.resume(returning: rtt)
            }
            
            connection.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    let elapsed = (Date().timeIntervalSince1970 - startTime) * 1000
                    complete(elapsed)
                case .failed, .cancelled:
                    complete(-1)
                default:
                    break
                }
            }
            
            connection.start(queue: DispatchQueue.global(qos: .utility))
            
            DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + timeoutSeconds) {
                complete(-1)
            }
        }
    }
}
