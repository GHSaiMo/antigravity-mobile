import Foundation
import UIKit

/// PairingInfo represents the metadata parsed from an agy://pair QR code.
public struct PairingInfo: Equatable, Sendable {
    public let host: String
    public let port: Int
    public let code: String
    public let ssl: Bool
    public let lanHost: String?
    public let ipv6Host: String?
    public let ddnsHost: String?
    public let relayHost: String?
    public let os: String?
    public let platform: String?
    
    public init(
        host: String,
        port: Int,
        code: String,
        ssl: Bool,
        lanHost: String? = nil,
        ipv6Host: String? = nil,
        ddnsHost: String? = nil,
        relayHost: String? = nil,
        os: String? = nil,
        platform: String? = nil
    ) {
        self.host = host
        self.port = port
        self.code = code
        self.ssl = ssl
        self.lanHost = lanHost
        self.ipv6Host = ipv6Host
        self.ddnsHost = ddnsHost
        self.relayHost = Self.normalizeHost(relayHost)
        self.os = os
        self.platform = platform
    }
    
    public var gatewayDisplayName: String {
        switch (platform ?? os)?.lowercased() {
        case "windows", "win":
            return "Windows 网关"
        case "darwin", "macos", "mac":
            return "Mac 网关"
        case "linux":
            return "Linux 网关"
        default:
            return "电脑网关"
        }
    }
    
    public static func formatURL(host: String, port: Int, ssl: Bool) -> String {
        let scheme = ssl ? "https://" : "http://"
        var formattedHost = host
        // Wrap raw IPv6 address in brackets if needed
        if !formattedHost.hasPrefix("[") && formattedHost.filter({ $0 == ":" }).count >= 2 {
            formattedHost = "[\(formattedHost)]"
        }
        if (ssl && port == 443) || (!ssl && port == 80) {
            return "\(scheme)\(formattedHost)"
        }
        return "\(scheme)\(formattedHost):\(port)"
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
    
    public var ipv6BaseURL: String? {
        guard let v6 = ipv6Host, !v6.isEmpty else { return nil }
        return Self.formatURL(host: v6, port: port, ssl: ssl)
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
        if let v6 = ipv6BaseURL, !list.contains(v6) { list.append(v6) }
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
    case decodingError(String)
    
    public var errorDescription: String? {
        switch self {
        case .invalidURI(let msg):
            return "无效的二维码配对链接: \(msg)"
        case .missingFields(let msg):
            return "二维码缺少必要参数: \(msg)"
        case .networkError(let msg):
            return "配对网络请求失败: \(msg)"
        case .serverRejected(let msg):
            return "配对失败: \(msg)"
        case .decodingError(let msg):
            return "配对响应解析失败: \(msg)"
        }
    }
}

public final class PairingService: Sendable {
    public static let shared = PairingService()
    
    private init() {}
    
    /// Parses an agy://pair?host=...&port=...&code=...&ssl=... URI string into PairingInfo.
    public func parsePairingURI(_ uriString: String) -> Result<PairingInfo, PairingError> {
        let trimmed = uriString.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = URL(string: trimmed) else {
            return .failure(.invalidURI("无法识别的 URL 格式"))
        }
        
        guard url.scheme?.lowercased() == "agy" && url.host?.lowercased() == "pair" else {
            return .failure(.invalidURI("非 Antigravity 配对协议 (需要 agy://pair)"))
        }
        
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let queryItems = components.queryItems else {
            return .failure(.invalidURI("缺少参数"))
        }
        
        var host: String?
        var port: Int?
        var code: String?
        var ssl: Bool = false
        var lanHost: String?
        var ipv6Host: String?
        var ddnsHost: String?
        var relayHost: String?
        var os: String?
        var platform: String?
        
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
                ssl = (item.value == "1" || item.value?.lowercased() == "true")
            case "lan":
                lanHost = item.value
            case "ipv6":
                ipv6Host = item.value
            case "ddns":
                ddnsHost = item.value
            case "relay":
                relayHost = item.value
            case "os":
                os = item.value
            case "platform":
                platform = item.value
            default:
                break
            }
        }
        
        guard let rawHost = host?.trimmingCharacters(in: .whitespacesAndNewlines), !rawHost.isEmpty else {
            return .failure(.missingFields("未找到主机地址 (host)"))
        }
        guard let validPort = port else {
            return .failure(.missingFields("未找到有效端口号 (port)"))
        }
        guard let validCode = code?.trimmingCharacters(in: .whitespacesAndNewlines), !validCode.isEmpty else {
            return .failure(.missingFields("未找到配对码 (code)"))
        }
        
        // Auto-expand compressed subdomain (e.g. 9d6f460f -> 9d6f460f.jiuge.space)
        var validHost = rawHost
        if !validHost.contains(".") && !validHost.contains(":") && validHost.lowercased() != "localhost" {
            validHost = "\(validHost).jiuge.space"
        }
        
        var validRelay = relayHost?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let r = validRelay, !r.isEmpty, !r.contains("."), !r.contains(":"), r.lowercased() != "localhost" {
            validRelay = "\(r).jiuge.space"
        }
        
        return .success(PairingInfo(
            host: validHost,
            port: validPort,
            code: validCode,
            ssl: ssl,
            lanHost: lanHost,
            ipv6Host: ipv6Host,
            ddnsHost: ddnsHost,
            relayHost: validRelay,
            os: os,
            platform: platform
        ))
    }
    
    /// Sends pairing request to the gateway, trying candidate endpoints and saving credentials upon success.
    public func pair(with info: PairingInfo) async throws -> (deviceId: String, deviceToken: String) {
        var candidates = info.candidateBaseURLs
        if NetworkTransport.shared.isCellular {
            // Off-LAN: try cloud relay / IPv6 before RFC1918 addresses that will just time out.
            let privateLAN = candidates.filter { url in
                guard let host = URL(string: url)?.host else { return false }
                return NetworkTransport.isLocalOrPrivateHost(host)
            }
            let publicPaths = candidates.filter { cand in !privateLAN.contains(cand) }
            candidates = publicPaths + privateLAN
        }
        guard !candidates.isEmpty else {
            throw PairingError.invalidURI("无可用网关端点地址")
        }
        
        let deviceName = await MainActor.run { UIDevice.current.name }
        let payload: [String: Any] = [
            "pairing_code": info.code,
            "device_name": deviceName,
            "platform": "ios"
        ]
        
        let bodyData: Data
        do {
            bodyData = try JSONSerialization.data(withJSONObject: payload)
        } catch {
            throw PairingError.networkError("无法打包配对请求数据: \(error.localizedDescription)")
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
        }
        
        var lastError: Error?
        
        for baseURL in candidates {
            guard let endpointURL = URL(string: "\(baseURL)/api/v1/auth/pair") else {
                continue
            }
            
            var request = URLRequest(url: endpointURL)
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.timeoutInterval = 6 // Fast timeout per candidate
            request.httpBody = bodyData
            
            do {
                let (data, response) = try await NetworkTransport.shared.send(request: request)
                
                guard let httpResponse = response as? HTTPURLResponse else {
                    continue
                }
                
                if httpResponse.statusCode != 200 {
                    if let errObj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                       let errMsg = errObj["error"] as? String {
                        throw PairingError.serverRejected(errMsg)
                    }
                    throw PairingError.serverRejected("HTTP \(httpResponse.statusCode)")
                }
                
                let decoded = try JSONDecoder().decode(PairResp.self, from: data)
                
                // Persist to Keychain
                KeychainHelper.shared.save(key: .deviceToken, value: decoded.device_token)
                KeychainHelper.shared.save(key: .deviceID, value: decoded.device_id)
                
                // Update AppSettings endpoints on MainActor
                await MainActor.run {
                    if let plat = decoded.platform ?? decoded.os ?? info.platform ?? info.os {
                        AppSettings.shared.gatewayPlatform = plat
                    }
                    var lanURL: String? = info.lanBaseURL
                    var ipv6URL: String? = info.ipv6BaseURL
                    var relayURL: String? = nil
                    var cloudURL: String? = nil
                    
                    if info.ssl || !NetworkTransport.isLocalOrPrivateHost(info.host) {
                        cloudURL = info.serverBaseURL
                    }
                    
                    if let eps = decoded.endpoints {
                        for ep in eps {
                            guard Self.isTrustedEndpoint(ep.url, pairing: info, usedBase: baseURL) else {
                                continue
                            }
                            switch ep.type.lowercased() {
                            case "lan":
                                lanURL = ep.url
                            case "ipv6":
                                ipv6URL = ep.url
                            case "relay":
                                relayURL = ep.url
                                if cloudURL == nil { cloudURL = ep.url }
                            case "cloudflare":
                                cloudURL = ep.url
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
                    
                    // Fallback classify successful candidate if slots are unassigned
                    let candClean = baseURL.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                    if let candHost = URL(string: candClean)?.host {
                        if NetworkTransport.isLocalOrPrivateHost(candHost) {
                            if lanURL == nil { lanURL = candClean }
                        } else {
                            if cloudURL == nil { cloudURL = candClean }
                        }
                    }
                    
                    AppSettings.shared.updateEndpoints(
                        lan: lanURL,
                        ipv6: ipv6URL,
                        relay: relayURL,
                        custom: nil,
                        active: baseURL,
                        primaryCloud: cloudURL
                    )
                }
                
                return (decoded.device_id, decoded.device_token)
            } catch let pairErr as PairingError {
                // If server rejected (e.g. invalid code), don't keep looping with same code
                if case .serverRejected = pairErr {
                    throw pairErr
                }
                lastError = pairErr
            } catch {
                lastError = error
            }
        }
        
        if let err = lastError {
            if let pairingErr = err as? PairingError {
                throw pairingErr
            }
            throw PairingError.networkError("所有网关端点连接均失败: \(err.localizedDescription)")
        }
        
        throw PairingError.networkError("连接网关失败")
    }
    
    /// Accept endpoints that match the QR hosts, the URL we just paired with, or private/loopback addresses.
    nonisolated static func isTrustedEndpoint(_ urlString: String, pairing info: PairingInfo, usedBase: String) -> Bool {
        guard let url = URL(string: urlString), let host = url.host else { return false }
        let h = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        var allowed: [String] = [info.host.lowercased()]
        if let lan = info.lanHost { allowed.append(lan.lowercased()) }
        if let v6 = info.ipv6Host { allowed.append(v6.lowercased()) }
        if let ddns = info.ddnsHost { allowed.append(ddns.lowercased()) }
        if let relay = info.relayHost { allowed.append(relay.lowercased()) }
        if let used = URL(string: usedBase)?.host {
            allowed.append(used.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased())
        }
        if allowed.contains(h) {
            return true
        }
        if NetworkTransport.isLocalOrPrivateHost(h) || h.hasSuffix(".jiuge.space") || h.hasSuffix(".antigravity.internal") {
            return true
        }
        return url.scheme?.lowercased() == "https"
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
