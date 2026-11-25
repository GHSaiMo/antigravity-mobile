import Foundation
import os
import Security

/// KeychainHelper provides secure storage for sensitive gateway credentials using iOS Keychain.
/// Supports per-gateway profile namespaces while remaining 100% backward-compatible with legacy single-gateway keys.
public final class KeychainHelper: Sendable {
    public static let shared = KeychainHelper()
    
    private let serviceName = "com.antigravity.mobile.auth"
    private let memoryCache = OSAllocatedUnfairLock(initialState: [String: String?]())
    private let activeProfileLock = OSAllocatedUnfairLock(initialState: Optional<String>.none)
    
    private init() {}
    
    public enum Key: String {
        case deviceToken = "antigravity.device_token"
        case deviceID = "antigravity.device_id"
        case e2eeKey = "antigravity.e2ee_key"
    }

    public func setActiveProfileId(_ profileId: String?) {
        activeProfileLock.withLock { $0 = profileId }
    }

    public var activeProfileId: String? {
        activeProfileLock.withLock { $0 }
    }

    private func storageAccount(for key: Key, profileId: String?) -> String {
        guard let pid = profileId?.trimmingCharacters(in: .whitespacesAndNewlines), !pid.isEmpty else {
            return key.rawValue
        }
        return "p.\(pid).\(key.rawValue)"
    }
    
    /// Saves or updates a string value in the Keychain for the active gateway profile (or legacy unscoped slot).
    @discardableResult
    public func save(key: Key, value: String) -> Bool {
        let pid = activeProfileId
        if let pid, !pid.isEmpty {
            let ok = save(key: key, value: value, profileId: pid)
            // Keep legacy key mirrored for legacy_primary profile so downgrade/migration never loses credentials.
            if pid == "legacy_primary" {
                _ = saveRaw(account: key.rawValue, value: value)
            }
            return ok
        }
        return saveRaw(account: key.rawValue, value: value)
    }

    /// Saves or updates a string value in the Keychain scoped explicitly to `profileId`.
    @discardableResult
    public func save(key: Key, value: String, profileId: String) -> Bool {
        let account = storageAccount(for: key, profileId: profileId)
        return saveRaw(account: account, value: value)
    }
    
    /// Reads a string value from the Keychain for the active gateway profile (with automatic legacy fallback).
    public func read(key: Key) -> String? {
        let pid = activeProfileId
        if let pid, !pid.isEmpty {
            if let scoped = read(key: key, profileId: pid) {
                return scoped
            }
            if pid == "legacy_primary" {
                return readRaw(account: key.rawValue)
            }
            return nil
        }
        return readRaw(account: key.rawValue)
    }

    /// Reads a string value scoped explicitly to `profileId`.
    public func read(key: Key, profileId: String) -> String? {
        let account = storageAccount(for: key, profileId: profileId)
        if let val = readRaw(account: account) {
            return val
        }
        if profileId == "legacy_primary" {
            return readRaw(account: key.rawValue)
        }
        return nil
    }

    /// Reads the legacy unscoped Keychain slot directly (used during first-time single->multi migration).
    public func readLegacy(key: Key) -> String? {
        readRaw(account: key.rawValue)
    }
    
    /// Deletes a key from the active profile slot (and legacy slot if active is `legacy_primary` or nil).
    @discardableResult
    public func delete(key: Key) -> Bool {
        let pid = activeProfileId
        if let pid, !pid.isEmpty {
            let ok = delete(key: key, profileId: pid)
            if pid == "legacy_primary" {
                _ = deleteRaw(account: key.rawValue)
            }
            return ok
        }
        return deleteRaw(account: key.rawValue)
    }

    @discardableResult
    public func delete(key: Key, profileId: String) -> Bool {
        deleteRaw(account: storageAccount(for: key, profileId: profileId))
    }

    /// Deletes all credentials scoped to a specific gateway profile.
    public func clear(profileId: String) {
        _ = delete(key: .deviceToken, profileId: profileId)
        _ = delete(key: .deviceID, profileId: profileId)
        _ = delete(key: .e2eeKey, profileId: profileId)
    }

    /// Deletes legacy unscoped credentials.
    public func clearLegacy() {
        _ = deleteRaw(account: Key.deviceToken.rawValue)
        _ = deleteRaw(account: Key.deviceID.rawValue)
        _ = deleteRaw(account: Key.e2eeKey.rawValue)
    }
    
    /// Deletes all credentials associated with the active profile.
    public func clearAll() {
        delete(key: .deviceToken)
        delete(key: .deviceID)
        delete(key: .e2eeKey)
    }

    private func saveRaw(account: String, value: String) -> Bool {
        guard let data = value.data(using: .utf8) else { return false }
        let ok = save(key: account, data: data)
        if ok {
            memoryCache.withLock { $0[account] = .some(value) }
        }
        return ok
    }

    private func readRaw(account: String) -> String? {
        if let cached = memoryCache.withLock({ $0[account] }) {
            return cached
        }
        let val: String?
        if let data = read(key: account) {
            val = String(data: data, encoding: .utf8)
        } else {
            val = nil
        }
        memoryCache.withLock { $0[account] = .some(val) }
        return val
    }

    private func deleteRaw(account: String) -> Bool {
        let ok = delete(key: account)
        memoryCache.withLock { $0[account] = .some(nil) }
        return ok
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
