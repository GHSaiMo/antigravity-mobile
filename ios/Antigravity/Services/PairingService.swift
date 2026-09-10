import Foundation
import UIKit

/// PairingInfo represents the metadata parsed from an agy://pair QR code.
public struct PairingInfo: Equatable, Sendable {
    public let host: String
    public let port: Int
    public let code: String
    public let ssl: Bool
    
    public init(host: String, port: Int, code: String, ssl: Bool) {
        self.host = host
        self.port = port
        self.code = code
        self.ssl = ssl
    }
    
    public var serverBaseURL: String {
        let scheme = ssl ? "https://" : "http://"
        var formattedHost = host
        // Wrap raw IPv6 address in brackets if needed
        if !formattedHost.hasPrefix("[") && formattedHost.filter({ $0 == ":" }).count >= 2 {
            formattedHost = "[\(formattedHost)]"
        }
        return "\(scheme)\(formattedHost):\(port)"
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
            default:
                break
            }
        }
        
        guard let validHost = host, !validHost.isEmpty else {
            return .failure(.missingFields("未找到主机地址 (host)"))
        }
        guard let validPort = port else {
            return .failure(.missingFields("未找到有效端口号 (port)"))
        }
        guard let validCode = code, !validCode.isEmpty else {
            return .failure(.missingFields("未找到配对码 (code)"))
        }
        
        return .success(PairingInfo(host: validHost, port: validPort, code: validCode, ssl: ssl))
    }
    
    /// Sends pairing request to the gateway and saves credentials upon success.
    public func pair(with info: PairingInfo) async throws -> (deviceId: String, deviceToken: String) {
        let baseURL = info.serverBaseURL
        guard let endpointURL = URL(string: "\(baseURL)/api/v1/auth/pair") else {
            throw PairingError.invalidURI("无法构造配对 API 地址: \(baseURL)")
        }
        
        var request = URLRequest(url: endpointURL)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 10
        
        let deviceName = await MainActor.run { UIDevice.current.name }
        let payload: [String: Any] = [
            "pairing_code": info.code,
            "device_name": deviceName,
            "platform": "ios"
        ]
        
        do {
            request.httpBody = try JSONSerialization.data(withJSONObject: payload)
        } catch {
            throw PairingError.networkError("无法打包配对请求数据: \(error.localizedDescription)")
        }
        
        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await NetworkTransport.shared.send(request: request, preferCellular: AppSettings.shared.preferCellularNetwork)
        } catch {
            throw PairingError.networkError("连接网关失败: \(error.localizedDescription)")
        }
        
        guard let httpResponse = response as? HTTPURLResponse else {
            throw PairingError.networkError("无效的网络响应")
        }
        
        if httpResponse.statusCode != 200 {
            if let errObj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
               let errMsg = errObj["error"] as? String {
                throw PairingError.serverRejected(errMsg)
            }
            throw PairingError.serverRejected("HTTP \(httpResponse.statusCode)")
        }
        
        struct PairResp: Decodable {
            let device_id: String
            let device_token: String
        }
        
        let decoded: PairResp
        do {
            decoded = try JSONDecoder().decode(PairResp.self, from: data)
        } catch {
            throw PairingError.decodingError("解析凭证失败: \(error.localizedDescription)")
        }
        
        // Persist to Keychain
        KeychainHelper.shared.save(key: .deviceToken, value: decoded.device_token)
        KeychainHelper.shared.save(key: .deviceID, value: decoded.device_id)
        
        // Update AppSettings server URL on MainActor
        await MainActor.run {
            AppSettings.shared.rawServerURL = baseURL
        }
        
        return (decoded.device_id, decoded.device_token)
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
