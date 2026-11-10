import Foundation
import os
import Security

/// KeychainHelper provides secure storage for sensitive gateway credentials using iOS Keychain.
public final class KeychainHelper: Sendable {
    public static let shared = KeychainHelper()
    
    private let serviceName = "com.antigravity.mobile.auth"
    private let memoryCache = OSAllocatedUnfairLock(initialState: [String: String?]())
    
    private init() {}
    
    public enum Key: String {
        case deviceToken = "antigravity.device_token"
        case deviceID = "antigravity.device_id"
    }
    
    /// Saves or updates a string value in the Keychain.
    @discardableResult
    public func save(key: Key, value: String) -> Bool {
        guard let data = value.data(using: .utf8) else { return false }
        let ok = save(key: key.rawValue, data: data)
        if ok {
            memoryCache.withLock { $0[key.rawValue] = .some(value) }
        }
        return ok
    }
    
    /// Reads a string value from the Keychain (backed by a write-through in-memory cache).
    public func read(key: Key) -> String? {
        if let cached = memoryCache.withLock({ $0[key.rawValue] }) {
            return cached
        }
        let val: String?
        if let data = read(key: key.rawValue) {
            val = String(data: data, encoding: .utf8)
        } else {
            val = nil
        }
        memoryCache.withLock { $0[key.rawValue] = .some(val) }
        return val
    }
    
    /// Deletes a key from the Keychain.
    @discardableResult
    public func delete(key: Key) -> Bool {
        let ok = delete(key: key.rawValue)
        memoryCache.withLock { $0[key.rawValue] = .some(nil) }
        return ok
    }
    
    /// Deletes all credentials associated with the service.
    public func clearAll() {
        delete(key: .deviceToken)
        delete(key: .deviceID)
    }
    
    // MARK: - Internal SecItem primitives
    
    private func save(key: String, data: Data) -> Bool {
        // Query to check if the item already exists
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: key
        ]
        
        _ = SecItemDelete(query as CFDictionary)
        var insertQuery = query
        insertQuery[kSecValueData as String] = data
        insertQuery[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        return SecItemAdd(insertQuery as CFDictionary, nil) == errSecSuccess
    }
    
    private func read(key: String) -> Data? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecSuccess, let data = result as? Data {
            return data
        }
        return nil
    }
    
    private func delete(key: String) -> Bool {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: serviceName,
            kSecAttrAccount as String: key
        ]
        let status = SecItemDelete(query as CFDictionary)
        return status == errSecSuccess || status == errSecItemNotFound
    }
}
