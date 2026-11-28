import Foundation
import UIKit

/// PairingInfo represents the metadata parsed from an agy://pair QR code.
public struct PairingInfo: Equatable, Sendable {
    public let host: String
    public let port: Int
    public let code: String
    public let ssl: Bool
    public let lanHost: String?
    public let ddnsHost: String?
    public let relayHost: String?
    public let os: String?
    public let platform: String?
    public let e2eePubKey: String?
    
    public init(
        host: String,
        port: Int,
        code: String,
        ssl: Bool,
        lanHost: String? = nil,
        ddnsHost: String? = nil,
        relayHost: String? = nil,
        os: String? = nil,
        platform: String? = nil,
        e2eePubKey: String? = nil
    ) {
        self.host = host
        self.port = port
        self.code = code
        self.ssl = ssl
        self.lanHost = lanHost
        self.ddnsHost = ddnsHost
        self.relayHost = Self.normalizeHost(relayHost)
        self.os = os
        self.platform = platform
        let trimmedPK = e2eePubKey?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.e2eePubKey = (trimmedPK?.isEmpty == false) ? trimmedPK : nil
    }
    
    public var gatewayDisplayName: String {
        switch (platform ?? os)?.lowercased() {
        case "windows", "win":
            return String(localized: "Windows 网关")
        case "darwin", "macos", "mac":
            return String(localized: "Mac 网关")
        case "linux":
            return String(localized: "Linux 网关")
        default:
            return String(localized: "电脑网关")
        }
    }
    
    public static func formatURL(host: String, port: Int, ssl: Bool) -> String {
        let scheme = ssl ? "https://" : "http://"
        var formattedHost = host.trimmingCharacters(in: .whitespacesAndNewlines)
        if !formattedHost.hasPrefix("["), formattedHost.filter({ $0 == ":" }).count >= 2 {
            formattedHost = "[\(formattedHost)]"
        }
        if (ssl && port == 443) || (!ssl && port == 80) {
            return "\(scheme)\(formattedHost)"
        }
        return "\(scheme)\(formattedHost):\(port)"
    }

    /// Normalizes manual pairing form inputs (host, port, code, ssl) so pasting a full
    /// `https://...` URL, `host:port`, or an official managed relay host works seamlessly
    /// even if the user left the SSL toggle off or kept the default LAN port 58900.
    nonisolated public static func fromManualInput(
        rawHost: String,
        rawPort: Int,
        rawCode: String,
        ssl: Bool
    ) -> PairingInfo {
        var working = rawHost.trimmingCharacters(in: .whitespacesAndNewlines)
        var effectivePort = rawPort
        var effectiveSSL = ssl
        var hasExplicitSchemeHTTPS = false
        var hasExplicitURLPort = false

        let lower = working.lowercased()
        if lower.hasPrefix("https://") || lower.hasPrefix("http://") {
            hasExplicitSchemeHTTPS = lower.hasPrefix("https://")
            if hasExplicitSchemeHTTPS {
                effectiveSSL = true
            }
            if let range = working.range(of: "://") {
                working = String(working[range.upperBound...])
            }
        }
        if let cutIdx = working.firstIndex(where: { $0 == "/" || $0 == "?" || $0 == "#" }) {
            working = String(working[..<cutIdx])
        }
        working = working.trimmingCharacters(in: .whitespacesAndNewlines)

        if working.hasPrefix("["), let endBracket = working.firstIndex(of: "]") {
            let hostPart = String(working[working.index(after: working.startIndex)..<endBracket])
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let tail = String(working[working.index(after: endBracket)...])
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if tail.hasPrefix(":"), let parsed = Int(tail.dropFirst()), (1...65535).contains(parsed) {
                effectivePort = parsed
                hasExplicitURLPort = true
            }
            working = hostPart
        } else if working.filter({ $0 == ":" }).count == 1, let colonIdx = working.lastIndex(of: ":") {
            let portStr = working[working.index(after: colonIdx)...]
            if let parsed = Int(portStr), (1...65535).contains(parsed) {
                working = String(working[..<colonIdx]).trimmingCharacters(in: .whitespacesAndNewlines)
                effectivePort = parsed
                hasExplicitURLPort = true
            }
        }

        if !working.isEmpty && !working.contains(".") && !working.contains(":") && working.lowercased() != "localhost" {
            working = "\(working).jiuge.space"
        }

        let cleanLower = working.lowercased()
        let isKnownRelay = cleanLower.hasSuffix(".multigravity.app") ||
            cleanLower.hasSuffix(".mgy.jiuge.space") ||
            cleanLower.hasSuffix(".jiuge.space") ||
            cleanLower.hasSuffix(".trycloudflare.com")
        if isKnownRelay {
            effectiveSSL = true
        }
        if (hasExplicitSchemeHTTPS || isKnownRelay) && !hasExplicitURLPort && effectivePort == 58900 {
            effectivePort = 443
        }
        if effectivePort == 443 && !NetworkTransport.isLocalOrPrivateHost(working) {
            effectiveSSL = true
        }

        return PairingInfo(
            host: working,
            port: effectivePort,
            code: rawCode.trimmingCharacters(in: .whitespacesAndNewlines),
            ssl: effectiveSSL
        )
    }
    
    public var serverBaseURL: String {
        Self.formatURL(host: host, port: port, ssl: ssl)
    }
    
    public var lanBaseURL: String? {
        guard let lan = lanHost, !lan.isEmpty else { return nil }
        let lanPort = (ssl && port == 443) ? 58900 : port
        let lanSSL = (ssl && port == 443) ? false : ssl
        return Self.formatURL(host: lan, port: lanPort, ssl: lanSSL)
    }
    
    public var ddnsBaseURL: String? {
        guard let ddns = ddnsHost, !ddns.isEmpty else { return nil }
        return Self.formatURL(host: ddns, port: port, ssl: ssl)
    }
    
    public var relayBaseURL: String? {
        guard let relay = relayHost, !relay.isEmpty else { return nil }
        return Self.formatURL(host: relay, port: port, ssl: ssl)
    }
    
    public var candidateBaseURLs: [String] {
        var list: [String] = []
        if let lan = lanBaseURL, !list.contains(lan) { list.append(lan) }
        let prim = serverBaseURL
        if !list.contains(prim) { list.append(prim) }
        if let relay = relayBaseURL, !list.contains(relay) { list.append(relay) }
        if let ddns = ddnsBaseURL, !list.contains(ddns) { list.append(ddns) }
        return list
    }
    
    nonisolated static func normalizeHost(_ raw: String?) -> String? {
        guard var value = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !value.isEmpty else {
            return nil
        }
        if value.hasPrefix("http://") || value.hasPrefix("https://"), let url = URL(string: value), let host = url.host {
            value = host
        }
        return value.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
    }
}

public enum PairingError: LocalizedError, Sendable {
    case invalidURI(String)
    case missingFields(String)
    case networkError(String)
    case serverRejected(String)
    case gatewayOffline
    case decodingError(String)
    
    public var errorDescription: String? {
        switch self {
        case .invalidURI(let msg):
            return String(localized: "无效的二维码配对链接: \(msg)")
        case .missingFields(let msg):
            return String(localized: "二维码缺少必要参数: \(msg)")
        case .networkError(let msg):
            return String(localized: "配对网络请求失败: \(msg)")
        case .serverRejected(let msg):
            return String(localized: "配对失败: \(msg)")
        case .gatewayOffline:
            return String(localized: "电脑端网关暂未连接至中继节点（gateway_offline），请确认电脑上 mgy 已显示中继就绪并重新执行 mgy pair 扫码")
        case .decodingError(let msg):
            return String(localized: "配对响应解析失败: \(msg)")
        }
    }
}

private struct RelayLayerPairingError: Error {
    let isGatewayOffline: Bool
    let statusCode: Int
    let fallbackMessage: String
}

public final class PairingService: Sendable {
    public static let shared = PairingService()
    
    private init() {}
    
    private static let supportedPairingSchemes: Set<String> = ["agy", "multigravity", "antigravity"]

    private static func isPairingURIChar(_ scalar: UnicodeScalar) -> Bool {
        switch scalar.value {
        case 0x30...0x39, // 0-9
             0x41...0x5A, // A-Z
             0x61...0x7A: // a-z
            return true
        case 0x2D, 0x2E, 0x5F, 0x7E, // - . _ ~
             0x25, 0x2B, 0x26, 0x3D, // % + & =
             0x3A, 0x2F, 0x3F,       // : / ?
             0x5B, 0x5D:             // [ ] (IPv6 literals)
            return true
        default:
            return false
        }
    }

    /// Tolerantly extracts the first pairing URI (`agy://pair?...`, `multigravity://pair?...`,
    /// or `antigravity://pair?...`) from raw clipboard/terminal text, handling prefixes such as
    /// `🔗 配对 URI:` and hard line wraps emitted by narrow terminals without absorbing trailing prose.
    public func extractPairingURI(from rawText: String) -> String? {
        let normalized = rawText
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\r", with: "\n")
        let lines = normalized.components(separatedBy: "\n")
        let prefixes = ["agy://pair?", "multigravity://pair?", "antigravity://pair?"]

        var matchLineIndex: Int?
        var matchRange: Range<String.Index>?
        for (idx, line) in lines.enumerated() {
            let lower = line.lowercased()
            for prefix in prefixes {
                if let range = lower.range(of: prefix) {
                    if matchRange == nil || range.lowerBound < matchRange!.lowerBound {
                        matchLineIndex = idx
                        matchRange = range
                    }
                }
            }
            if matchLineIndex != nil {
                break
            }
        }

        guard let startIdx = matchLineIndex, let startRange = matchRange else {
            return nil
        }

        let firstLine = lines[startIdx]
        let lowerFirst = firstLine.lowercased()
        let offset = lowerFirst.distance(from: lowerFirst.startIndex, to: startRange.lowerBound)
        guard let originalStart = firstLine.index(firstLine.startIndex, offsetBy: offset, limitedBy: firstLine.endIndex) else {
            return nil
        }

        let tail = firstLine[originalStart...]
        var uriEnd = tail.startIndex
        while uriEnd < tail.endIndex {
            let ch = tail[uriEnd]
            if ch.unicodeScalars.allSatisfy(Self.isPairingURIChar) {
                uriEnd = tail.index(after: uriEnd)
            } else {
                break
            }
        }

        var result = String(tail[..<uriEnd])
        let remainderAfterURI = tail[uriEnd...].trimmingCharacters(in: .whitespacesAndNewlines)

        // Only stitch subsequent lines when the URI ran all the way to the end of the line
        // (typical terminal hard wrap).
        if remainderAfterURI.isEmpty {
            var prevLineLength = firstLine.trimmingCharacters(in: .whitespacesAndNewlines).count
            var nextIdx = startIdx + 1
            while nextIdx < lines.count {
                let rawNext = lines[nextIdx].trimmingCharacters(in: .whitespacesAndNewlines)
                if rawNext.isEmpty {
                    break
                }
                let isPureURILine = rawNext.unicodeScalars.allSatisfy(Self.isPairingURIChar)
                if !isPureURILine {
                    break
                }
                let lowerAcc = result.lowercased()
                let missingCoreParam = !lowerAcc.contains("host=") || !lowerAcc.contains("code=")
                let looksLikeQueryContinuation = rawNext.hasPrefix("&") || rawNext.contains("=")
                let likelyTerminalWrap = prevLineLength >= 32
                if missingCoreParam || looksLikeQueryContinuation || likelyTerminalWrap {
                    result += rawNext
                    prevLineLength = rawNext.count
                    nextIdx += 1
                } else {
                    break
                }
            }
        }

        return result.isEmpty ? nil : result
    }

    /// Parses an agy://pair?host=...&port=...&code=...&ssl=... URI string into PairingInfo.
    public func parsePairingURI(_ uriString: String) -> Result<PairingInfo, PairingError> {
        let candidate = extractPairingURI(from: uriString) ?? uriString.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = URL(string: candidate) else {
            return .failure(.invalidURI(String(localized: "无法识别的 URL 格式")))
        }
        
        guard let scheme = url.scheme?.lowercased(),
              Self.supportedPairingSchemes.contains(scheme),
              url.host?.lowercased() == "pair" else {
            return .failure(.invalidURI(String(localized: "无效的配对协议（需要 agy://pair）")))
        }
        
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let queryItems = components.queryItems else {
            return .failure(.invalidURI(String(localized: "缺少参数")))
        }
        
        var host: String?
        var port: Int?
        var code: String?
        var sslExplicit: Bool?
        var lanHost: String?
        var ddnsHost: String?
        var relayHost: String?
        var os: String?
        var platform: String?
        var e2eePubKey: String?
        
        for item in queryItems {
            switch item.name.lowercased() {
            case "host":
                host = item.value
            case "port":
                if let val = item.value, let p = Int(val), p > 0 && p <= 65535 {
                    port = p
                }
            case "code":
                code = item.value
            case "ssl":
                if let val = item.value {
                    sslExplicit = (val == "1" || val.lowercased() == "true")
                }
            case "lan":
                lanHost = item.value
            case "ddns":
                ddnsHost = item.value
            case "relay":
                relayHost = item.value
            case "os":
                os = item.value
            case "platform":
                platform = item.value
            case "pk":
                e2eePubKey = item.value
            default:
                break
            }
        }
        
        guard let rawHost = host?.trimmingCharacters(in: .whitespacesAndNewlines), !rawHost.isEmpty else {
            return .failure(.missingFields(String(localized: "未找到主机地址 (host)")))
        }
        guard let validCode = code?.trimmingCharacters(in: .whitespacesAndNewlines), !validCode.isEmpty else {
            return .failure(.missingFields(String(localized: "未找到配对码 (code)")))
        }
        let validPort = port ?? 443
        
        // Auto-expand compressed subdomain (e.g. 825a5a50 -> 825a5a50.jiuge.space)
        var validHost = rawHost
        if !validHost.contains(".") && !validHost.contains(":") && validHost.lowercased() != "localhost" {
            validHost = "\(validHost).jiuge.space"
        }
        
        let effectiveSSL = sslExplicit ?? (validPort == 443 || !NetworkTransport.isLocalOrPrivateHost(validHost))
        
        var validRelay = relayHost?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let r = validRelay, !r.isEmpty, !r.contains("."), !r.contains(":"), r.lowercased() != "localhost" {
            validRelay = "\(r).jiuge.space"
        }
        
        return .success(PairingInfo(
            host: validHost,
            port: validPort,
            code: validCode,
            ssl: effectiveSSL,
            lanHost: lanHost,
            ddnsHost: ddnsHost,
            relayHost: validRelay,
            os: os,
            platform: platform,
            e2eePubKey: e2eePubKey
        ))
    }
    
    /// Classifies outer HTTP status/body returned before a pairing request reaches the gateway
    /// (e.g. mgy-relay `503 gateway_offline`, `502 gateway_unreachable`, `429 tunnel_busy`).
    /// Never matches decrypted inner gateway responses (which arrive inside outer HTTP 200).
    nonisolated static func classifyRelayLayerPairingFailure(
        outerStatus: Int,
        rawData: Data
    ) -> (isRelayLayer: Bool, isGatewayOffline: Bool) {
        let snippet = String(data: rawData.prefix(320), encoding: .utf8)?.lowercased() ?? ""
        switch outerStatus {
        case 503:
            if snippet.contains("upstream_not_connected") ||
               snippet.contains("antigravity upstream not connected") ||
               snippet.contains("language_server") {
                return (false, false)
            }
            return (true, snippet.contains("gateway_offline"))
        case 502, 520, 521, 522, 523, 524:
            return (true, false)
        case 429:
            return (snippet.contains("tunnel_busy"), false)
        default:
            return (false, false)
        }
    }

    /// Sends pairing request to the gateway, trying candidate endpoints and saving credentials upon success.
    /// Automatically probes sibling VPS relay nodes via `/api/relay/pool` when an official relay
    /// returns `503 gateway_offline` / transient relay errors (e.g. stale QR cache or VPS failover).
    public func pair(with info: PairingInfo) async throws -> (deviceId: String, deviceToken: String) {
        var candidates = info.candidateBaseURLs
        if NetworkTransport.shared.isCellular {
            // Off-LAN: try cloud relay before RFC1918 addresses that will just time out.
            let privateLAN = candidates.filter { url in
                guard let host = URL(string: url)?.host else { return false }
                return NetworkTransport.isLocalOrPrivateHost(host)
            }
            let publicPaths = candidates.filter { cand in !privateLAN.contains(cand) }
            candidates = publicPaths + privateLAN
        }
        guard !candidates.isEmpty else {
            throw PairingError.invalidURI(String(localized: "无可用网关端点地址"))
        }
        
        let deviceName = await MainActor.run { UIDevice.current.name }
        let (ephemPriv, clientPubB64) = E2EECrypto.generateEphemeralKeyPair()
        let payload: [String: Any] = [
            "pairing_code": info.code,
            "device_name": deviceName,
            "platform": "ios",
            "e2ee_pub": clientPubB64
        ]
        
        let bodyData: Data
        do {
            bodyData = try JSONSerialization.data(withJSONObject: payload)
        } catch {
            throw PairingError.networkError(String(localized: "无法打包配对请求数据: \(error.localizedDescription)"))
        }
        
        struct EndpointResp: Decodable {
            let type: String
            let url: String
        }
        
        struct PairResp: Decodable {
            let device_id: String
            let device_token: String
            let endpoints: [EndpointResp]?
            let os: String?
            let platform: String?
            let gateway_id: String?
            let gateway_name: String?
            let e2ee_gateway_pub: String?
            let e2ee_salt: String?
            let relay_cloud_url: String?
        }

        func attemptPair(at baseURL: String) async throws -> (deviceId: String, deviceToken: String) {
            let candHost = URL(string: baseURL)?.host ?? ""
            let useEncryptedPair = (info.e2eePubKey != nil) && !NetworkTransport.isLocalOrPrivateHost(candHost)
            let pairPath = useEncryptedPair ? "/api/v1/e2ee/pair" : "/api/v1/auth/pair"
            guard let endpointURL = URL(string: "\(baseURL)\(pairPath)") else {
                throw PairingError.invalidURI(String(localized: "无可用网关端点地址"))
            }
            
            var request = URLRequest(url: endpointURL)
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.timeoutInterval = 8 // Fast timeout per candidate

            var activePairPriv = ephemPriv
            var activeClientPub = clientPubB64
            if useEncryptedPair, let gwPub = info.e2eePubKey {
                let enc = try E2EECrypto.shared.encryptPairRequest(pairBody: bodyData, gatewayPubB64: gwPub)
                activePairPriv = enc.privateKey
                activeClientPub = enc.clientPubB64
                request.httpBody = enc.envelopeBody
            } else {
                request.httpBody = bodyData
            }
            
            let (rawData, response) = try await NetworkTransport.shared.send(request: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                throw PairingError.networkError(String(localized: "无效网络响应"))
            }
            
            let outerStatus = httpResponse.statusCode
            let effectiveStatus: Int
            let data: Data
            if useEncryptedPair, outerStatus == 200, let gwPub = info.e2eePubKey {
                let dec = try E2EECrypto.shared.decryptPairResponse(
                    outerData: rawData,
                    privateKey: activePairPriv,
                    clientPubB64: activeClientPub,
                    gatewayPubB64: gwPub
                )
                effectiveStatus = dec.status
                data = dec.body
            } else {
                if outerStatus != 200 {
                    let relayCheck = Self.classifyRelayLayerPairingFailure(outerStatus: outerStatus, rawData: rawData)
                    if relayCheck.isRelayLayer {
                        let parsedErr = (try? JSONSerialization.jsonObject(with: rawData) as? [String: Any])?["error"] as? String
                        throw RelayLayerPairingError(
                            isGatewayOffline: relayCheck.isGatewayOffline,
                            statusCode: outerStatus,
                            fallbackMessage: parsedErr ?? "HTTP \(outerStatus)"
                        )
                    }
                }
                effectiveStatus = outerStatus
                data = rawData
            }

            if effectiveStatus != 200 {
                if let errObj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                   let errMsg = errObj["error"] as? String {
                    throw PairingError.serverRejected(errMsg)
                }
                throw PairingError.serverRejected("HTTP \(effectiveStatus)")
            }
            
            let decoded = try JSONDecoder().decode(PairResp.self, from: data)
            
            // Derive per-device E2EE key in memory first so we store it strictly under the target profile's namespace
            // without ever overwriting another gateway profile's credentials.
            var derivedE2EEKeyB64: String? = nil
            if let gwPub = decoded.e2ee_gateway_pub, !gwPub.isEmpty,
               let saltB64 = decoded.e2ee_salt, !saltB64.isEmpty,
               let shared = try? E2EECrypto.sharedSecret(privateKey: activePairPriv, peerPubB64: gwPub),
               let salt = E2EECrypto.decodeBase64Flexible(saltB64) {
                let devKey = E2EECrypto.deriveDeviceKey(sharedSecret: shared, salt: salt, deviceID: decoded.device_id)
                derivedE2EEKeyB64 = E2EECrypto.base64URLEncode(devKey)
            }

            let resolvedGatewayId = decoded.gateway_id?.trimmingCharacters(in: .whitespacesAndNewlines)
                .nilIfEmpty
                ?? GatewayProfileStore.deriveGatewayId(fromE2EEPubKeyB64: decoded.e2ee_gateway_pub ?? info.e2eePubKey)
            let resolvedGatewayName = decoded.gateway_name?.trimmingCharacters(in: .whitespacesAndNewlines).nilIfEmpty
            let resolvedPlatform = decoded.platform ?? decoded.os ?? info.platform ?? info.os
            let hasE2EEKey = (derivedE2EEKeyB64 != nil)
            
            // Classify endpoints and persist profile on MainActor
            await MainActor.run {
                var lanURL: String? = info.lanBaseURL
                var relayURL: String? = nil
                var cloudURL: String? = nil
                var e2eeHosts: [String] = []
                
                let candClean = baseURL.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                let cleanCandHost = NetworkTransport.extractHost(from: candClean)
                if !cleanCandHost.isEmpty && !NetworkTransport.isLocalOrPrivateHost(cleanCandHost) {
                    cloudURL = candClean
                    if cleanCandHost.hasPrefix("r-") {
                        relayURL = candClean
                        e2eeHosts.append(cleanCandHost)
                    }
                } else if info.ssl || !NetworkTransport.isLocalOrPrivateHost(info.host) {
                    cloudURL = info.serverBaseURL
                }
                
                if let eps = decoded.endpoints {
                    for ep in eps {
                        guard Self.isTrustedEndpoint(ep.url, pairing: info, usedBase: baseURL) || (useEncryptedPair && NetworkTransport.isValidAdvertisedCloudURL(ep.url, trustedByE2EE: true)) else {
                            continue
                        }
                        switch ep.type.lowercased() {
                        case "lan":
                            lanURL = ep.url
                        case "relay":
                            relayURL = ep.url
                            cloudURL = ep.url
                            if let rHost = URL(string: ep.url)?.host {
                                e2eeHosts.append(NetworkTransport.extractHost(from: rHost))
                            }
                        case "cloudflare":
                            if cloudURL == nil || relayURL == nil {
                                cloudURL = ep.url
                            }
                        case "primary":
                            let h = URL(string: ep.url)?.host ?? ""
                            if NetworkTransport.isLocalOrPrivateHost(h) {
                                if lanURL == nil { lanURL = ep.url }
                            } else {
                                if cloudURL == nil { cloudURL = ep.url }
                            }
                        default:
                            break
                        }
                    }
                }

                if let dualRelay = decoded.relay_cloud_url?.trimmingCharacters(in: .whitespacesAndNewlines),
                   !dualRelay.isEmpty,
                   hasE2EEKey,
                   NetworkTransport.isValidAdvertisedCloudURL(dualRelay, trustedByE2EE: true) {
                    relayURL = dualRelay
                    cloudURL = dualRelay
                    if let rHost = URL(string: dualRelay)?.host {
                        e2eeHosts.append(NetworkTransport.extractHost(from: rHost))
                    }
                }
                
                // Fallback classify successful candidate if slots are unassigned
                if let candHost = URL(string: candClean)?.host {
                    if NetworkTransport.isLocalOrPrivateHost(candHost) {
                        if lanURL == nil { lanURL = candClean }
                    } else {
                        if cloudURL == nil { cloudURL = candClean }
                    }
                }

                let store = GatewayProfileStore.shared
                let hadActiveBefore = (store.activeProfileId != nil && AppSettings.shared.isPaired && !AppSettings.shared.isDemoMode)
                let (profile, _) = store.upsertPairedProfile(
                    gatewayId: resolvedGatewayId,
                    gatewayName: resolvedGatewayName,
                    platform: resolvedPlatform,
                    deviceID: decoded.device_id,
                    deviceToken: decoded.device_token,
                    e2eeKeyB64: derivedE2EEKeyB64,
                    lanURL: lanURL,
                    relayURL: relayURL,
                    cloudURL: cloudURL,
                    activeURL: candClean,
                    e2eeRequiredHosts: e2eeHosts
                )

                if hadActiveBefore && store.activeProfileId != profile.id {
                    _ = GatewaySwitcher.shared.switchTo(profileId: profile.id, showToast: true)
                } else {
                    store.setActiveProfileId(profile.id)
                    E2EECrypto.shared.resetForGatewaySwitch(requiredHosts: profile.e2eeRequiredHosts)
                    CacheManager.shared.switchStorageNamespace(profile.storageNamespace)
                    ProjectCacheManager.shared.switchStorageNamespace(profile.storageNamespace)
                    DocumentCacheManager.shared.switchStorageNamespace(profile.storageNamespace)
                    WeChatArchiveCacheManager.shared.switchStorageNamespace(profile.storageNamespace)
                    DraftFileStore.shared.switchStorageNamespace(profile.storageNamespace)
                    AppSettings.shared.applyProfile(profile)
                    AppSettings.shared.syncWidgetConfiguration(reloadWidget: true)
                }

                ActivityManager.shared.startRemoteTokenObservers()
                PushNotificationManager.shared.syncAllTokensIfPaired(force: true)
                Task {
                    await PushNotificationManager.shared.refreshAuthorizationStatus()
                    if PushNotificationManager.shared.authorizationStatus == .notDetermined {
                        _ = await PushNotificationManager.shared.requestAuthorizationAndRegister()
                    }
                }
            }
            
            return (decoded.device_id, decoded.device_token)
        }
        
        var lastError: Error?
        var sawGatewayOffline = false
        var attemptedPeerDiscovery = false
        var triedURLs = Set<String>()
        
        for baseURL in candidates {
            let cleanBase = baseURL.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            guard triedURLs.insert(cleanBase).inserted else { continue }
            
            do {
                return try await attemptPair(at: baseURL)
            } catch let pairErr as PairingError {
                // If gateway explicitly rejected (e.g. invalid/expired code), stop looping immediately
                if case .serverRejected = pairErr {
                    throw pairErr
                }
                lastError = pairErr
            } catch let relayErr as RelayLayerPairingError {
                if relayErr.isGatewayOffline {
                    sawGatewayOffline = true
                }
                lastError = relayErr.isGatewayOffline
                    ? PairingError.gatewayOffline
                    : PairingError.serverRejected(relayErr.fallbackMessage)
                
                // Auto-heal stale QR relay host or mid-pairing VPS failover via official RELAY_POOL
                if !attemptedPeerDiscovery && RelayPoolResolver.shared.isOfficialOrPoolHost(baseURL) {
                    attemptedPeerDiscovery = true
                    if let peerURL = await RelayPoolResolver.shared.discoverPeerRelayForPairing(
                        failedURLString: baseURL,
                        e2eePubKey: info.e2eePubKey
                    ) {
                        let cleanPeer = peerURL.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
                        if triedURLs.insert(cleanPeer).inserted {
                            do {
                                return try await attemptPair(at: peerURL)
                            } catch let peerPairErr as PairingError {
                                if case .serverRejected = peerPairErr {
                                    throw peerPairErr
                                }
                                lastError = peerPairErr
                            } catch let peerRelayErr as RelayLayerPairingError {
                                if peerRelayErr.isGatewayOffline {
                                    sawGatewayOffline = true
                                }
                                lastError = peerRelayErr.isGatewayOffline
                                    ? PairingError.gatewayOffline
                                    : PairingError.serverRejected(peerRelayErr.fallbackMessage)
                            } catch {
                                lastError = error
                            }
                        }
                    } else if !Task.isCancelled {
                        // Gateway may still be finishing its initial 1-2s WSS handshake on this node.
                        try? await Task.sleep(nanoseconds: 1_200_000_000)
                        if !Task.isCancelled {
                            do {
                                return try await attemptPair(at: baseURL)
                            } catch let retryPairErr as PairingError {
                                if case .serverRejected = retryPairErr {
                                    throw retryPairErr
                                }
                                lastError = retryPairErr
                            } catch let retryRelayErr as RelayLayerPairingError {
                                if retryRelayErr.isGatewayOffline {
                                    sawGatewayOffline = true
                                }
                                lastError = retryRelayErr.isGatewayOffline
                                    ? PairingError.gatewayOffline
                                    : PairingError.serverRejected(retryRelayErr.fallbackMessage)
                            } catch {
                                lastError = error
                            }
                        }
                    }
                }
            } catch {
                lastError = error
                if !attemptedPeerDiscovery && RelayPoolResolver.shared.isOfficialOrPoolHost(baseURL) {
                    attemptedPeerDiscovery = true
                    if let peerURL = await RelayPoolResolver.shared.discoverPeerRelayForPairing(
                        failedURLString: baseURL,
                        e2eePubKey: info.e2eePubKey
                    ) {
                        let cleanPeer = peerURL.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
                        if triedURLs.insert(cleanPeer).inserted {
                            do {
                                return try await attemptPair(at: peerURL)
                            } catch let peerPairErr as PairingError {
                                if case .serverRejected = peerPairErr {
                                    throw peerPairErr
                                }
                                lastError = peerPairErr
                            } catch let peerRelayErr as RelayLayerPairingError {
                                if peerRelayErr.isGatewayOffline {
                                    sawGatewayOffline = true
                                }
                                lastError = peerRelayErr.isGatewayOffline
                                    ? PairingError.gatewayOffline
                                    : PairingError.serverRejected(peerRelayErr.fallbackMessage)
                            } catch {
                                lastError = error
                            }
                        }
                    }
                }
            }
        }
        
        if sawGatewayOffline {
            throw PairingError.gatewayOffline
        }
        if let err = lastError {
            if let pairingErr = err as? PairingError {
                throw pairingErr
            }
            throw PairingError.networkError(String(localized: "所有网关端点连接均失败: \(err.localizedDescription)"))
        }
        
        throw PairingError.networkError(String(localized: "连接网关失败"))
    }
    
    /// Accept endpoints that match the QR hosts, the URL we just paired with, private/loopback addresses,
    /// or recognized Cloudflare/Multigravity tunnel domains over HTTPS.
    nonisolated static func isTrustedEndpoint(_ urlString: String, pairing info: PairingInfo, usedBase: String) -> Bool {
        guard let url = URL(string: urlString),
              let scheme = url.scheme?.lowercased(),
              url.user == nil,
              url.password == nil,
              let host = url.host else { return false }
        let h = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        guard !h.isEmpty else { return false }

        let isPrivate = NetworkTransport.isLocalOrPrivateHost(h)
        if scheme == "http" {
            return isPrivate
        }
        guard scheme == "https" else { return false }
        if isPrivate {
            return true
        }

        var allowed: [String] = [NetworkTransport.extractHost(from: info.host)]
        if let lan = info.lanHost { allowed.append(NetworkTransport.extractHost(from: lan)) }
        if let ddns = info.ddnsHost { allowed.append(NetworkTransport.extractHost(from: ddns)) }
        if let relay = info.relayHost { allowed.append(NetworkTransport.extractHost(from: relay)) }
        let usedHost = NetworkTransport.extractHost(from: usedBase)
        if !usedHost.isEmpty {
            allowed.append(usedHost)
        }
        if allowed.contains(h) {
            return true
        }
        if h.hasSuffix(".trycloudflare.com") || h.hasSuffix(".multigravity.app") || h.hasSuffix(".mgy.jiuge.space") || h.hasSuffix(".jiuge.space") || h.hasSuffix(".antigravity.internal") {
            return true
        }
        // Allow sibling subdomains under the same registrable parent domain as the QR host
        let parts = h.split(separator: ".")
        if parts.count >= 3 {
            let parentSuffix = "." + parts.dropFirst().joined(separator: ".")
            if allowed.contains(where: { !$0.isEmpty && $0.hasSuffix(parentSuffix) }) {
                return true
            }
        }
        return false
    }
    
    /// Clears saved credentials.
    public func unpair() {
        KeychainHelper.shared.clearAll()
    }
    
    /// Checks if the device has an existing pairing token.
    public var isPaired: Bool {
        guard let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty else {
            return false
        }
        return true
    }
}

private extension String {
    var nilIfEmpty: String? {
        isEmpty ? nil : self
    }
}
