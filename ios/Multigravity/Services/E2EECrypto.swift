import CryptoKit
import Foundation
import os

public enum E2EEError: Error, LocalizedError, Sendable {
    case missingCredentials
    case invalidKey
    case invalidPublicKey
    case encryptionFailed
    case decryptionFailed
    case invalidEnvelope

    public var errorDescription: String? {
        switch self {
        case .missingCredentials: return "E2EE credentials missing"
        case .invalidKey: return "Invalid E2EE AES-256 key"
        case .invalidPublicKey: return "Invalid peer P-256 public key"
        case .encryptionFailed: return "E2EE encryption failed"
        case .decryptionFailed: return "E2EE decryption failed"
        case .invalidEnvelope: return "Invalid E2EE response envelope"
        }
    }
}

/// Zero-knowledge end-to-end encryption (mgy-e2ee-v1) matching Go `internal/e2ee`:
/// ECDH P-256 (65B uncompressed SEC1) + single-block RFC 5869 HKDF-SHA256 + AES-256-GCM.
public final class E2EECrypto: Sendable {
    public static let shared = E2EECrypto()

    public static let deviceHeader = "X-Mgy-E2EE-Device"
    public static let acceptEncHeader = "X-Mgy-E2EE-Enc"
    public static let supportedEncValue = "bin2,flate"
    private static let pairSalt = Data("mgy-e2ee-pair-v1".utf8)

    private let e2eeRequiredHostsLock = OSAllocatedUnfairLock(initialState: Set<String>())
    private let upgradeInFlightLock = OSAllocatedUnfairLock(initialState: false)
    private let lastUpgradeAttemptLock = OSAllocatedUnfairLock(initialState: Date.distantPast)

    private init() {}

    // MARK: - Keychain State

    public var currentDeviceID: String? {
        guard let id = KeychainHelper.shared.read(key: .deviceID)?.trimmingCharacters(in: .whitespacesAndNewlines),
              !id.isEmpty else {
            return nil
        }
        return id
    }

    public var currentDeviceKeyB64: String? {
        guard let b64 = KeychainHelper.shared.read(key: .e2eeKey)?.trimmingCharacters(in: .whitespacesAndNewlines),
              !b64.isEmpty else {
            return nil
        }
        return b64
    }

    public var currentDeviceKey: Data? {
        guard let b64 = currentDeviceKeyB64,
              let raw = Self.decodeBase64Flexible(b64),
              raw.count == 32 else {
            return nil
        }
        return raw
    }

    public var isReady: Bool {
        currentDeviceID != nil && currentDeviceKey != nil
    }

    public func saveDeviceKey(_ key: Data) {
        guard key.count == 32 else { return }
        KeychainHelper.shared.save(key: .e2eeKey, value: Self.base64URLEncode(key))
    }

    public func markHostRequiresE2EE(_ host: String) {
        let clean = NetworkTransport.extractHost(from: host)
        guard !clean.isEmpty else { return }
        e2eeRequiredHostsLock.withLock { _ = $0.insert(clean) }
    }

    public func isRelayHost(_ host: String) -> Bool {
        let clean = NetworkTransport.extractHost(from: host)
        guard !clean.isEmpty, !NetworkTransport.isLocalOrPrivateHost(clean) else {
            return false
        }
        if clean.hasPrefix("r-") {
            return true
        }
        return e2eeRequiredHostsLock.withLock { $0.contains(clean) }
    }

    /// Returns true if an outgoing request to `url` should be wrapped inside the E2EE transport channel.
    public func shouldEncrypt(url: URL?) -> Bool {
        guard let url,
              let scheme = url.scheme?.lowercased(),
              (scheme == "https" || scheme == "wss"),
              let host = url.host,
              !host.isEmpty,
              isReady else {
            return false
        }
        let path = url.path
        if path == "/healthz" || path.hasPrefix("/api/v1/e2ee/") {
            return false
        }
        return isRelayHost(host)
    }

    // MARK: - Cryptographic Primitives (Byte-for-Byte Identical to Go internal/e2ee)

    public static func base64URLEncode(_ data: Data) -> String {
        data.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    public static func decodeBase64Flexible(_ raw: String) -> Data? {
        var s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        guard !s.isEmpty else { return nil }
        let rem = s.count % 4
        if rem > 0 {
            s += String(repeating: "=", count: 4 - rem)
        }
        return Data(base64Encoded: s)
    }

    public static func generateEphemeralKeyPair() -> (privateKey: P256.KeyAgreement.PrivateKey, publicKeyB64: String) {
        let priv = P256.KeyAgreement.PrivateKey()
        let rawPub = priv.publicKey.x963Representation // 65-byte uncompressed SEC1 (0x04 || X || Y)
        return (priv, base64URLEncode(rawPub))
    }

    public static func sharedSecret(privateKey: P256.KeyAgreement.PrivateKey, peerPubB64: String) throws -> Data {
        guard let rawPeer = decodeBase64Flexible(peerPubB64), rawPeer.count == 65 else {
            throw E2EEError.invalidPublicKey
        }
        let peerPub = try P256.KeyAgreement.PublicKey(x963Representation: rawPeer)
        let secret = try privateKey.sharedSecretFromKeyAgreement(with: peerPub)
        return secret.withUnsafeBytes { Data($0) }
    }

    /// Single-block (32-byte) RFC 5869 HKDF-SHA256: HMAC(HMAC(salt, ikm), info || 0x01).
    public static func hkdfSHA256(ikm: Data, salt: Data, info: Data) -> Data {
        let effectiveSalt = salt.isEmpty ? Data(repeating: 0, count: 32) : salt
        let prk = HMAC<SHA256>.authenticationCode(for: ikm, using: SymmetricKey(data: effectiveSalt))
        var expandInput = info
        expandInput.append(0x01)
        let okm = HMAC<SHA256>.authenticationCode(for: expandInput, using: SymmetricKey(data: Data(prk)))
        return Data(okm)
    }

    public static func derivePairKey(sharedSecret: Data, clientPubB64: String, gatewayPubB64: String) -> Data {
        let info = Data("pair:\(clientPubB64.trimmingCharacters(in: .whitespacesAndNewlines)):\(gatewayPubB64.trimmingCharacters(in: .whitespacesAndNewlines))".utf8)
        return hkdfSHA256(ikm: sharedSecret, salt: pairSalt, info: info)
    }

    public static func deriveDeviceKey(sharedSecret: Data, salt: Data, deviceID: String) -> Data {
        let info = Data("mgy-e2ee-device-v1:\(deviceID.trimmingCharacters(in: .whitespacesAndNewlines))".utf8)
        return hkdfSHA256(ikm: sharedSecret, salt: salt, info: info)
    }

    public static func frameAAD(deviceID: String, direction: String) -> Data {
        Data("mgy-e2ee-v1:\(deviceID.trimmingCharacters(in: .whitespacesAndNewlines)):\(direction)".utf8)
    }

    public static func pairFrameAAD(clientPubB64: String, direction: String) -> Data {
        Data("mgy-e2ee-pair-v1:\(clientPubB64.trimmingCharacters(in: .whitespacesAndNewlines)):\(direction)".utf8)
    }

    public static func encrypt(key: Data, plaintext: Data, aad: Data) throws -> Data {
        guard key.count == 32 else { throw E2EEError.invalidKey }
        let symKey = SymmetricKey(data: key)
        let sealed = try AES.GCM.seal(plaintext, using: symKey, authenticating: aad)
        guard let combined = sealed.combined else {
            throw E2EEError.encryptionFailed
        }
        return combined // nonce(12B) || ciphertext || tag(16B)
    }

    public static func decrypt(key: Data, frame: Data, aad: Data) throws -> Data {
        guard key.count == 32 else { throw E2EEError.invalidKey }
        guard frame.count >= 12 + 16 else { throw E2EEError.decryptionFailed }
        let box = try AES.GCM.SealedBox(combined: frame)
        return try AES.GCM.open(box, using: SymmetricKey(data: key), authenticating: aad)
    }

    // MARK: - HTTP RPC Envelope

    private struct InnerHTTPRequest: Encodable {
        let m: String
        let p: String
        let h: [String: String]?
        let b: String?
    }

    private struct InnerHTTPResponse: Decodable {
        let s: Int
        let h: [String: String]?
        let b: String?
    }

    public func wrapHTTPRequest(_ request: URLRequest) throws -> (outerRequest: URLRequest, deviceID: String, key: Data) {
        guard let url = request.url,
              let scheme = url.scheme,
              let host = url.host,
              let deviceID = currentDeviceID,
              let key = currentDeviceKey else {
            throw E2EEError.missingCredentials
        }

        var pathWithQuery = url.path.isEmpty ? "/" : url.path
        if let query = url.query, !query.isEmpty {
            pathWithQuery += "?\(query)"
        }

        var headers = request.allHTTPHeaderFields ?? [:]
        headers.removeValue(forKey: "Accept-Encoding")
        headers.removeValue(forKey: "accept-encoding")
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty,
           headers["Authorization"] == nil && headers["authorization"] == nil {
            headers["Authorization"] = "Bearer \(token)"
        }

        let bodyB64: String? = request.httpBody.flatMap { $0.isEmpty ? nil : $0.base64EncodedString() }
        let inner = InnerHTTPRequest(
            m: (request.httpMethod ?? "GET").uppercased(),
            p: pathWithQuery,
            h: headers.isEmpty ? nil : headers,
            b: bodyB64
        )
        let plainBytes = try JSONEncoder().encode(inner)
        let cipherFrame = try Self.encrypt(key: key, plaintext: plainBytes, aad: Self.frameAAD(deviceID: deviceID, direction: "c2s"))

        var outerComps = URLComponents()
        outerComps.scheme = scheme
        outerComps.host = host
        outerComps.port = url.port
        outerComps.path = "/api/v1/e2ee/rpc"
        guard let outerURL = outerComps.url else {
            throw URLError(.badURL)
        }

        var outerReq = URLRequest(url: outerURL)
        outerReq.httpMethod = "POST"
        outerReq.timeoutInterval = request.timeoutInterval
        outerReq.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        outerReq.setValue(deviceID, forHTTPHeaderField: Self.deviceHeader)
        outerReq.setValue(Self.supportedEncValue, forHTTPHeaderField: Self.acceptEncHeader)
        outerReq.httpBody = cipherFrame
        return (outerReq, deviceID, key)
    }

    /// Decompresses RFC 1951 raw deflate data using Apple's native Compression framework (`.zlib`).
    public static func inflateRaw(_ data: Data) throws -> Data {
        guard !data.isEmpty else { return Data() }
        return try (data as NSData).decompressed(using: .zlib) as Data
    }

    private struct InnerHTTPMeta: Decodable {
        let s: Int
        let h: [String: String]?
    }

    public func unwrapHTTPResponse(
        encData: Data,
        originalURL: URL,
        deviceID: String,
        key: Data
    ) throws -> (Data, HTTPURLResponse) {
        let plain = try Self.decrypt(key: key, frame: encData, aad: Self.frameAAD(deviceID: deviceID, direction: "s2c"))
        guard let firstByte = plain.first else {
            throw E2EEError.invalidEnvelope
        }

        let statusCode: Int
        let headerFields: [String: String]?
        let bodyData: Data

        if firstByte == 0x01 || firstByte == 0x02 {
            // Binary v2 envelope: [0x01|0x02][optional raw-deflate([u32 BE metaLen][metaJSON][rawBody])]
            let unpacked: Data
            if firstByte == 0x02 {
                unpacked = try Self.inflateRaw(Data(plain.dropFirst()))
            } else {
                unpacked = Data(plain.dropFirst())
            }
            guard unpacked.count >= 4 else {
                throw E2EEError.invalidEnvelope
            }
            let b = [UInt8](unpacked.prefix(4))
            let metaLen = (Int(b[0]) << 24) | (Int(b[1]) << 16) | (Int(b[2]) << 8) | Int(b[3])
            guard metaLen >= 2, 4 + metaLen <= unpacked.count else {
                throw E2EEError.invalidEnvelope
            }
            let metaSlice = unpacked.subdata(in: 4..<(4 + metaLen))
            let meta = try JSONDecoder().decode(InnerHTTPMeta.self, from: metaSlice)
            statusCode = meta.s
            headerFields = meta.h
            bodyData = unpacked.subdata(in: (4 + metaLen)..<unpacked.count)
        } else {
            // Legacy JSON v1 envelope: {"s":200,"h":{...},"b":"<base64>"}
            let decoded = try JSONDecoder().decode(InnerHTTPResponse.self, from: plain)
            statusCode = decoded.s
            headerFields = decoded.h
            if let b64 = decoded.b, !b64.isEmpty {
                bodyData = Data(base64Encoded: b64) ?? Data()
            } else {
                bodyData = Data()
            }
        }

        guard let httpResp = HTTPURLResponse(
            url: originalURL,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: headerFields
        ) else {
            throw E2EEError.invalidEnvelope
        }
        return (bodyData, httpResp)
    }

    // MARK: - Encrypted Pairing Helpers

    public func encryptPairRequest(
        pairBody: Data,
        gatewayPubB64: String
    ) throws -> (privateKey: P256.KeyAgreement.PrivateKey, clientPubB64: String, envelopeBody: Data) {
        let (priv, clientPubB64) = Self.generateEphemeralKeyPair()
        let shared = try Self.sharedSecret(privateKey: priv, peerPubB64: gatewayPubB64)
        let pairKey = Self.derivePairKey(sharedSecret: shared, clientPubB64: clientPubB64, gatewayPubB64: gatewayPubB64)
        let cipher = try Self.encrypt(key: pairKey, plaintext: pairBody, aad: Self.pairFrameAAD(clientPubB64: clientPubB64, direction: "c2s"))
        let env: [String: String] = [
            "client_pub": clientPubB64,
            "ciphertext": cipher.base64EncodedString()
        ]
        let envData = try JSONSerialization.data(withJSONObject: env)
        return (priv, clientPubB64, envData)
    }

    public func decryptPairResponse(
        outerData: Data,
        privateKey: P256.KeyAgreement.PrivateKey,
        clientPubB64: String,
        gatewayPubB64: String
    ) throws -> (status: Int, body: Data) {
        guard let outerObj = try JSONSerialization.jsonObject(with: outerData) as? [String: Any],
              let cipherB64 = outerObj["ciphertext"] as? String,
              let cipherBytes = Self.decodeBase64Flexible(cipherB64) else {
            throw E2EEError.invalidEnvelope
        }
        let shared = try Self.sharedSecret(privateKey: privateKey, peerPubB64: gatewayPubB64)
        let pairKey = Self.derivePairKey(sharedSecret: shared, clientPubB64: clientPubB64, gatewayPubB64: gatewayPubB64)
        let plain = try Self.decrypt(key: pairKey, frame: cipherBytes, aad: Self.pairFrameAAD(clientPubB64: clientPubB64, direction: "s2c"))
        guard let wrapped = try JSONSerialization.jsonObject(with: plain) as? [String: Any],
              let status = wrapped["status"] as? Int,
              let bodyObj = wrapped["body"] else {
            throw E2EEError.invalidEnvelope
        }
        let bodyData = try JSONSerialization.data(withJSONObject: bodyObj)
        return (status, bodyData)
    }

    @discardableResult
    public func deriveAndSaveDeviceKey(
        privateKey: P256.KeyAgreement.PrivateKey,
        gatewayPubB64: String,
        saltB64: String,
        deviceID: String
    ) throws -> Data {
        guard let salt = Self.decodeBase64Flexible(saltB64), !salt.isEmpty else {
            throw E2EEError.invalidEnvelope
        }
        let shared = try Self.sharedSecret(privateKey: privateKey, peerPubB64: gatewayPubB64)
        let devKey = Self.deriveDeviceKey(sharedSecret: shared, salt: salt, deviceID: deviceID)
        saveDeviceKey(devKey)
        return devKey
    }

    // MARK: - Silent Legacy-to-E2EE Upgrade

    /// Silently negotiates an E2EE device key for previously paired devices upgrading to v1.2.5+
    /// over LAN or legacy Cloudflare Tunnel, without requiring a QR re-scan.
    public func silentUpgradeIfNeeded(reachableBaseURL: String) async {
        guard !isReady else { return }
        guard let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty else {
            return
        }
        let now = Date()
        let shouldAttempt = upgradeInFlightLock.withLock { inFlight -> Bool in
            if inFlight { return false }
            let last = lastUpgradeAttemptLock.withLock { $0 }
            if now.timeIntervalSince(last) < 60 { return false }
            inFlight = true
            lastUpgradeAttemptLock.withLock { $0 = now }
            return true
        }
        guard shouldAttempt else { return }
        defer { upgradeInFlightLock.withLock { $0 = false } }

        let cleanBase = reachableBaseURL.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard let url = URL(string: "\(cleanBase)/api/v1/auth/e2ee/upgrade") else { return }

        let (ephemPriv, clientPubB64) = Self.generateEphemeralKeyPair()
        guard let body = try? JSONSerialization.data(withJSONObject: ["client_pub": clientPubB64]) else { return }

        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.timeoutInterval = 6.0
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        req.httpBody = body

        do {
            let (data, resp) = try await NetworkTransport.shared.send(request: req)
            guard let http = resp as? HTTPURLResponse, http.statusCode == 200 else { return }
            struct UpgradeResp: Decodable {
                let device_id: String
                let gateway_pub: String
                let e2ee_salt: String
                let cloud_url: String?
            }
            let decoded = try JSONDecoder().decode(UpgradeResp.self, from: data)
            KeychainHelper.shared.save(key: .deviceID, value: decoded.device_id)
            _ = try deriveAndSaveDeviceKey(
                privateKey: ephemPriv,
                gatewayPubB64: decoded.gateway_pub,
                saltB64: decoded.e2ee_salt,
                deviceID: decoded.device_id
            )
            if let rawCloud = decoded.cloud_url?.trimmingCharacters(in: .whitespacesAndNewlines),
               !rawCloud.isEmpty {
                let cleanCloud = rawCloud.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                if let host = URL(string: cleanCloud)?.host, host.lowercased().hasPrefix("r-") {
                    markHostRequiresE2EE(host)
                }
                if NetworkTransport.isValidAdvertisedCloudURL(cleanCloud, trustedByE2EE: true) {
                    await MainActor.run {
                        AppSettings.shared.primaryCloudURL = cleanCloud
                        AppSettings.shared.syncWidgetConfiguration(reloadWidget: false)
                    }
                } else {
                    await MainActor.run {
                        AppSettings.shared.syncWidgetConfiguration(reloadWidget: false)
                    }
                }
            } else {
                await MainActor.run {
                    AppSettings.shared.syncWidgetConfiguration(reloadWidget: false)
                }
            }
        } catch {
            // Gateway may still be pre-1.2.5; quietly retry later when gateway updates.
        }
    }
}
