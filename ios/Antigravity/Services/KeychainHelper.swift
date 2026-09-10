import Foundation
import Security

/// KeychainHelper provides secure storage for sensitive gateway credentials using iOS Keychain.
public final class KeychainHelper: Sendable {
    public static let shared = KeychainHelper()
    
    private let serviceName = "com.antigravity.mobile.auth"
    
    private init() {}
    
    public enum Key: String {
        case deviceToken = "antigravity.device_token"
        case deviceID = "antigravity.device_id"
    }
    
    /// Saves or updates a string value in the Keychain.
    @discardableResult
    public func save(key: Key, value: String) -> Bool {
        guard let data = value.data(using: .utf8) else { return false }
        return save(key: key.rawValue, data: data)
    }
    
    /// Reads a string value from the Keychain.
    public func read(key: Key) -> String? {
        guard let data = read(key: key.rawValue) else { return nil }
        return String(data: data, encoding: .utf8)
    }
    
    /// Deletes a key from the Keychain.
    @discardableResult
    public func delete(key: Key) -> Bool {
        delete(key: key.rawValue)
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
        
        let updateAttributes: [String: Any] = [
            kSecValueData as String: data
        ]
        
        let status = SecItemUpdate(query as CFDictionary, updateAttributes as CFDictionary)
        if status == errSecSuccess {
            return true
        } else if status == errSecItemNotFound {
            // Item does not exist, insert it
            var insertQuery = query
            insertQuery[kSecValueData as String] = data
            insertQuery[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            let insertStatus = SecItemAdd(insertQuery as CFDictionary, nil)
            return insertStatus == errSecSuccess
        }
        return false
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
