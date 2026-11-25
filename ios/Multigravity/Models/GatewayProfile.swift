import CryptoKit
import Foundation
import Observation
import os

/// Thread-safe monotonic epoch & active-profile state readable from any isolation context
/// (including `nonisolated` `NetworkTransport` and `E2EECrypto`).
public final class GatewayRuntimeState: Sendable {
    public static let shared = GatewayRuntimeState()

    private struct State {
        var epoch: UInt64 = 1
        var activeProfileId: String? = nil
        var activeStorageNamespace: String = ""
        var profilesById: [String: GatewayProfile] = [:]
    }

    private let lock = OSAllocatedUnfairLock(initialState: State())

    private init() {}

    public var epoch: UInt64 {
        lock.withLock { $0.epoch }
    }

    @discardableResult
    public func bumpEpoch() -> UInt64 {
        lock.withLock {
            $0.epoch &+= 1
            if $0.epoch == 0 { $0.epoch = 1 }
            return $0.epoch
        }
    }

    public var activeProfileId: String? {
        lock.withLock { $0.activeProfileId }
    }

    public var activeStorageNamespace: String {
        lock.withLock { $0.activeStorageNamespace }
    }

    public func sync(activeProfileId: String?, profiles: [GatewayProfile]) {
        lock.withLock {
            $0.activeProfileId = activeProfileId
            var map: [String: GatewayProfile] = [:]
            for p in profiles {
                map[p.id] = p
            }
            $0.profilesById = map
            if let aid = activeProfileId, let active = map[aid] {
                $0.activeStorageNamespace = active.storageNamespace
            } else {
                $0.activeStorageNamespace = ""
            }
        }
    }

    public func profile(for id: String) -> GatewayProfile? {
        lock.withLock { $0.profilesById[id] }
    }

    public func allProfilesSnapshot() -> [GatewayProfile] {
        lock.withLock { Array($0.profilesById.values) }
    }
}

public struct GatewayProfile: Codable, Identifiable, Equatable, Sendable {
    public let id: String
    public var gatewayId: String?
    /// Empty string `""` denotes the legacy default storage namespace (zero-copy upgrade for existing users).
    /// Additional gateways use `id` as their isolated cache/UserDefaults namespace.
    public var storageNamespace: String
    public var displayName: String
    public var isCustomName: Bool
    public var platform: String?
    public var rawServerURL: String
    public var primaryCloudURL: String?
    public var lanServerURL: String?
    public var relayServerURL: String?
    public var customServerURL: String?
    public var activeServerURL: String?
    public var e2eeRequiredHosts: [String]
    public var cachedPermissionPreset: String
    public var workspaceNotes: [String: String]
    public var createdAt: Date
    public var lastUsedAt: Date

    public init(
        id: String,
        gatewayId: String? = nil,
        storageNamespace: String,
        displayName: String,
        isCustomName: Bool = false,
        platform: String? = nil,
        rawServerURL: String = "",
        primaryCloudURL: String? = nil,
        lanServerURL: String? = nil,
        relayServerURL: String? = nil,
        customServerURL: String? = nil,
        activeServerURL: String? = nil,
        e2eeRequiredHosts: [String] = [],
        cachedPermissionPreset: String = "strict",
        workspaceNotes: [String: String] = [:],
        createdAt: Date = Date(),
        lastUsedAt: Date = Date()
    ) {
        self.id = id
        self.gatewayId = gatewayId
        self.storageNamespace = storageNamespace
        self.displayName = displayName
        self.isCustomName = isCustomName
        self.platform = platform
        self.rawServerURL = rawServerURL
        self.primaryCloudURL = primaryCloudURL
        self.lanServerURL = lanServerURL
        self.relayServerURL = relayServerURL
        self.customServerURL = customServerURL
        self.activeServerURL = activeServerURL
        self.e2eeRequiredHosts = e2eeRequiredHosts
        self.cachedPermissionPreset = cachedPermissionPreset
        self.workspaceNotes = workspaceNotes
        self.createdAt = createdAt
        self.lastUsedAt = lastUsedAt
    }

    public var platformIconName: String {
        switch platform?.lowercased() {
        case "windows", "win":
            return "pc"
        case "linux":
            return "server.rack"
        case "darwin", "macos", "mac":
            return "desktopcomputer"
        default:
            return "laptopcomputer"
        }
    }

    public var defaultPlatformLabel: String {
        switch platform?.lowercased() {
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

    public var resolvedDisplayName: String {
        let trimmed = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmed.isEmpty {
            return trimmed
        }
        return defaultPlatformLabel
    }

    public func candidateURLs(isCellular: Bool) -> [URL] {
        var rawList: [String?] = []
        if isCellular {
            rawList = [activeServerURL, customServerURL, primaryCloudURL, relayServerURL, rawServerURL]
        } else {
            rawList = [lanServerURL, activeServerURL, customServerURL, primaryCloudURL, relayServerURL, rawServerURL]
        }
        var out: [URL] = []
        var seen = Set<String>()
        for raw in rawList {
            guard let r = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !r.isEmpty,
                  let url = AppSettings.normalize(raw: r) else {
                continue
            }
            if isCellular && NetworkTransport.isLanOnlyHost(url.host ?? "") {
                continue
            }
            let key = url.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            if seen.insert(key).inserted {
                out.append(url)
            }
        }
        return out
    }
}

/// Self-contained, side-effect-free credentials and endpoints for sending requests to any paired gateway
/// (active or inactive) without reading or mutating global `AppSettings` / `KeychainHelper` state.
public struct GatewayRequestContext: Sendable {
    public let profileId: String
    public let gatewayId: String?
    public let displayName: String
    public let deviceID: String?
    public let deviceToken: String?
    public let e2eeKey: Data?
    public let e2eeRequiredHosts: Set<String>
    public let lanURL: URL?
    public let customURL: URL?
    public let cloudURL: URL?
    public let activeURL: URL?

    public var isE2EEReady: Bool {
        guard let did = deviceID, !did.isEmpty, let key = e2eeKey, key.count == 32 else {
            return false
        }
        return true
    }

    public func shouldEncrypt(url: URL?) -> Bool {
        guard let url,
              let scheme = url.scheme?.lowercased(),
              (scheme == "https" || scheme == "wss"),
              let host = url.host,
              !host.isEmpty,
              isE2EEReady else {
            return false
        }
        let path = url.path
        if path == "/healthz" || path.hasPrefix("/api/v1/e2ee/") {
            return false
        }
        let clean = NetworkTransport.extractHost(from: host)
        if clean.isEmpty || NetworkTransport.isLocalOrPrivateHost(clean) {
            return false
        }
        return clean.hasPrefix("r-") || e2eeRequiredHosts.contains(clean)
    }

    public func candidateURLs(isCellular: Bool) -> [URL] {
        let ordered: [URL?] = isCellular
            ? [activeURL, customURL, cloudURL]
            : [lanURL, activeURL, customURL, cloudURL]
        var out: [URL] = []
        var seen = Set<String>()
        for u in ordered {
            guard let url = u else { continue }
            if isCellular && NetworkTransport.isLanOnlyHost(url.host ?? "") {
                continue
            }
            let key = url.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
            if seen.insert(key).inserted {
                out.append(url)
            }
        }
        return out
    }
}

public struct GatewaySummarySnapshot: Equatable, Sendable {
    public let profileId: String
    public var runningCount: Int
    public var actionCount: Int
    public var unreadCount: Int
    public var totalActive: Int
    public var isReachable: Bool
    public var isRefreshing: Bool
    public var gatewayName: String?
    public var updatedAt: Date

    public init(
        profileId: String,
        runningCount: Int = 0,
        actionCount: Int = 0,
        unreadCount: Int = 0,
        totalActive: Int = 0,
        isReachable: Bool = true,
        isRefreshing: Bool = false,
        gatewayName: String? = nil,
        updatedAt: Date = Date()
    ) {
        self.profileId = profileId
        self.runningCount = runningCount
        self.actionCount = actionCount
        self.unreadCount = unreadCount
        self.totalActive = totalActive
        self.isReachable = isReachable
        self.isRefreshing = isRefreshing
        self.gatewayName = gatewayName
        self.updatedAt = updatedAt
    }
}

@Observable
@MainActor
public final class GatewayProfileStore {
    public static let shared = GatewayProfileStore()

    private static let profilesKey = "antigravity.gateway_profiles_v1"
    private static let activeProfileIdKey = "antigravity.active_gateway_profile_id_v1"

    public private(set) var profiles: [GatewayProfile] = []
    public private(set) var activeProfileId: String? = nil
    public private(set) var summaries: [String: GatewaySummarySnapshot] = [:]

    private var lastInactiveRefreshDate: Date = .distantPast
    private var inFlightSummaryTask: Task<Void, Never>? = nil

    public var activeProfile: GatewayProfile? {
        guard let id = activeProfileId else { return profiles.first }
        return profiles.first(where: { $0.id == id }) ?? profiles.first
    }

    public var hasMultipleGateways: Bool {
        profiles.count > 1
    }

    private init() {
        loadFromDisk()
    }

    /// Derives the exact same `"gw_" + hex(sha256(pubBytes)[0..<8])` identifier as Go `e2ee.GatewayIDFromPubKeyB64`.
    public nonisolated static func deriveGatewayId(fromE2EEPubKeyB64 pubB64: String?) -> String? {
        guard let pubB64, !pubB64.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              let raw = E2EECrypto.decodeBase64Flexible(pubB64),
              raw.count == 65 else {
            return nil
        }
        let digest = SHA256.hash(data: raw)
        let prefixHex = digest.prefix(8).map { String(format: "%02x", $0) }.joined()
        return "gw_\(prefixHex)"
    }

    private func loadFromDisk() {
        let defaults = UserDefaults.standard
        if let data = defaults.data(forKey: Self.profilesKey),
           let decoded = try? JSONDecoder().decode([GatewayProfile].self, from: data),
           !decoded.isEmpty {
            self.profiles = decoded
            let savedActive = defaults.string(forKey: Self.activeProfileIdKey)
            if let savedActive, decoded.contains(where: { $0.id == savedActive }) {
                self.activeProfileId = savedActive
            } else {
                self.activeProfileId = decoded.first?.id
            }
            syncRuntimeAndKeychainScope()
            return
        }

        // Seamless zero-copy upgrade for existing single-gateway installs.
        if let legacyToken = KeychainHelper.shared.readLegacy(key: .deviceToken),
           !legacyToken.isEmpty {
            let legacyId = "legacy_primary"
            let rawURL = defaults.string(forKey: "antigravity.server_url") ?? ""
            let primaryCloud = defaults.string(forKey: "antigravity.primary_cloud_url")
            let lanURL = defaults.string(forKey: "antigravity.lan_server_url")
            let relayURL = defaults.string(forKey: "antigravity.relay_server_url")
            let customURL = defaults.string(forKey: "antigravity.custom_server_url")
            let activeURL = defaults.string(forKey: "antigravity.active_server_url")
            let platform = defaults.string(forKey: "antigravity.gateway_platform")
            let preset = defaults.string(forKey: "antigravity.cached_permission_preset") ?? "strict"
            let wsNotes = (defaults.dictionary(forKey: "antigravity.workspace_notes") as? [String: String]) ?? [:]

            let defaultName: String
            switch platform?.lowercased() {
            case "windows", "win": defaultName = String(localized: "Windows 网关")
            case "darwin", "macos", "mac": defaultName = String(localized: "Mac 网关")
            case "linux": defaultName = String(localized: "Linux 网关")
            default: defaultName = String(localized: "电脑网关")
            }

            let migrated = GatewayProfile(
                id: legacyId,
                gatewayId: nil,
                storageNamespace: "",
                displayName: defaultName,
                isCustomName: false,
                platform: platform,
                rawServerURL: rawURL,
                primaryCloudURL: primaryCloud,
                lanServerURL: lanURL,
                relayServerURL: relayURL,
                customServerURL: customURL,
                activeServerURL: activeURL,
                e2eeRequiredHosts: [],
                cachedPermissionPreset: preset,
                workspaceNotes: wsNotes
            )

            // Mirror legacy credentials into namespaced Keychain entries so isolated context reads always succeed.
            KeychainHelper.shared.save(key: .deviceToken, value: legacyToken, profileId: legacyId)
            if let did = KeychainHelper.shared.readLegacy(key: .deviceID), !did.isEmpty {
                KeychainHelper.shared.save(key: .deviceID, value: did, profileId: legacyId)
            }
            if let ek = KeychainHelper.shared.readLegacy(key: .e2eeKey), !ek.isEmpty {
                KeychainHelper.shared.save(key: .e2eeKey, value: ek, profileId: legacyId)
            }

            self.profiles = [migrated]
            self.activeProfileId = legacyId
            persistToDisk()
            syncRuntimeAndKeychainScope()
        } else {
            self.profiles = []
            self.activeProfileId = nil
            syncRuntimeAndKeychainScope()
        }
    }

    private func persistToDisk() {
        let defaults = UserDefaults.standard
        if let data = try? JSONEncoder().encode(profiles) {
            defaults.set(data, forKey: Self.profilesKey)
        }
        if let aid = activeProfileId {
            defaults.set(aid, forKey: Self.activeProfileIdKey)
        } else {
            defaults.removeObject(forKey: Self.activeProfileIdKey)
        }
        GatewayRuntimeState.shared.sync(activeProfileId: activeProfileId, profiles: profiles)
    }

    private func syncRuntimeAndKeychainScope() {
        GatewayRuntimeState.shared.sync(activeProfileId: activeProfileId, profiles: profiles)
        KeychainHelper.shared.setActiveProfileId(activeProfileId)
    }

    // MARK: - Profile Queries

    public func profile(for id: String) -> GatewayProfile? {
        profiles.first(where: { $0.id == id })
    }

    public func profile(matchingGatewayId rawGatewayId: String?) -> GatewayProfile? {
        guard let gw = rawGatewayId?.trimmingCharacters(in: .whitespacesAndNewlines), !gw.isEmpty else {
            return nil
        }
        return profiles.first(where: { $0.gatewayId == gw || $0.id == gw })
    }

    /// Fallback lookup when a notification arrives from an older gateway that omitted `gatewayId`.
    public func profileId(containingCachedCascade cascadeId: String) -> String? {
        guard !cascadeId.isEmpty else { return nil }
        for profile in profiles {
            if CacheManager.shared.containsCachedCascade(cascadeId, storageNamespace: profile.storageNamespace) {
                return profile.id
            }
        }
        return nil
    }

    public func requestContext(for profileId: String) -> GatewayRequestContext? {
        guard let p = profile(for: profileId) else { return nil }
        let token = KeychainHelper.shared.read(key: .deviceToken, profileId: p.id)
        let devId = KeychainHelper.shared.read(key: .deviceID, profileId: p.id)
        let e2eeKeyB64 = KeychainHelper.shared.read(key: .e2eeKey, profileId: p.id)
        let e2eeKey: Data?
        if let b64 = e2eeKeyB64, let raw = E2EECrypto.decodeBase64Flexible(b64), raw.count == 32 {
            e2eeKey = raw
        } else {
            e2eeKey = nil
        }
        var hosts = Set(p.e2eeRequiredHosts.map { NetworkTransport.extractHost(from: $0) }.filter { !$0.isEmpty })
        if let relay = p.relayServerURL, let h = URL(string: relay)?.host {
            let clean = NetworkTransport.extractHost(from: h)
            if !clean.isEmpty { hosts.insert(clean) }
        }
        return GatewayRequestContext(
            profileId: p.id,
            gatewayId: p.gatewayId,
            displayName: p.resolvedDisplayName,
            deviceID: devId,
            deviceToken: token,
            e2eeKey: e2eeKey,
            e2eeRequiredHosts: hosts,
            lanURL: p.lanServerURL.flatMap { AppSettings.normalize(raw: $0) },
            customURL: p.customServerURL.flatMap { AppSettings.normalize(raw: $0) },
            cloudURL: (p.primaryCloudURL ?? p.relayServerURL).flatMap { AppSettings.normalize(raw: $0) },
            activeURL: (p.activeServerURL ?? (p.rawServerURL.isEmpty ? nil : p.rawServerURL)).flatMap { AppSettings.normalize(raw: $0) }
        )
    }

    public func allRequestContexts() -> [GatewayRequestContext] {
        profiles.compactMap { requestContext(for: $0.id) }
    }

    // MARK: - Mutations

    /// Inserts a newly paired gateway or updates an existing profile if the same gateway was re-paired.
    @discardableResult
    public func upsertPairedProfile(
        gatewayId: String?,
        gatewayName: String?,
        platform: String?,
        deviceID: String,
        deviceToken: String,
        e2eeKeyB64: String?,
        lanURL: String?,
        relayURL: String?,
        cloudURL: String?,
        activeURL: String,
        e2eeRequiredHosts: [String]
    ) -> (profile: GatewayProfile, isNew: Bool) {
        let cleanGwId = gatewayId?.trimmingCharacters(in: .whitespacesAndNewlines)
        let validGwId = (cleanGwId?.isEmpty == false) ? cleanGwId : nil
        let cleanGwName = gatewayName?.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanCloud = cloudURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
        let cleanLan = lanURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()

        // Match existing profile by stable gatewayId or identical endpoint origin to avoid duplicates.
        let existingIdx: Int? = profiles.firstIndex { existing in
            if let gw = validGwId, (existing.gatewayId == gw || existing.id == gw) {
                return true
            }
            if let c = cleanCloud, !c.isEmpty,
               existing.primaryCloudURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased() == c {
                return true
            }
            if let l = cleanLan, !l.isEmpty,
               existing.lanServerURL?.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased() == l,
               (validGwId == nil || existing.gatewayId == nil) {
                return true
            }
            return false
        }

        let now = Date()
        if let idx = existingIdx {
            var updated = profiles[idx]
            if let gw = validGwId {
                updated.gatewayId = gw
            }
            if let plat = platform, !plat.isEmpty {
                updated.platform = plat
            }
            if !updated.isCustomName, let name = cleanGwName, !name.isEmpty {
                updated.displayName = name
            } else if updated.displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                updated.displayName = cleanGwName ?? updated.defaultPlatformLabel
            }
            if let lan = lanURL, !lan.isEmpty { updated.lanServerURL = lan }
            if let relay = relayURL, !relay.isEmpty { updated.relayServerURL = relay }
            if let cloud = cloudURL, !cloud.isEmpty, !NetworkTransport.isLocalOrPrivateHost(cloud) {
                updated.primaryCloudURL = cloud
            }
            updated.activeServerURL = activeURL
            updated.rawServerURL = activeURL
            var hostSet = Set(updated.e2eeRequiredHosts)
            for h in e2eeRequiredHosts where !h.isEmpty { hostSet.insert(h) }
            updated.e2eeRequiredHosts = Array(hostSet).sorted()
            updated.lastUsedAt = now
            profiles[idx] = updated

            KeychainHelper.shared.save(key: .deviceToken, value: deviceToken, profileId: updated.id)
            KeychainHelper.shared.save(key: .deviceID, value: deviceID, profileId: updated.id)
            if let ek = e2eeKeyB64, !ek.isEmpty {
                KeychainHelper.shared.save(key: .e2eeKey, value: ek, profileId: updated.id)
            }
            persistToDisk()
            return (updated, false)
        }

        let newId = validGwId ?? "gw_\(UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16).lowercased())"
        let useLegacyNamespace = profiles.isEmpty
        let resolvedName: String = {
            if let n = cleanGwName, !n.isEmpty { return n }
            switch platform?.lowercased() {
            case "windows", "win": return String(localized: "Windows 网关")
            case "darwin", "macos", "mac": return String(localized: "Mac 网关")
            case "linux": return String(localized: "Linux 网关")
            default: return String(localized: "电脑网关")
            }
        }()

        let created = GatewayProfile(
            id: newId,
            gatewayId: validGwId,
            storageNamespace: useLegacyNamespace ? "" : newId,
            displayName: resolvedName,
            isCustomName: false,
            platform: platform,
            rawServerURL: activeURL,
            primaryCloudURL: (cloudURL != nil && !NetworkTransport.isLocalOrPrivateHost(cloudURL!)) ? cloudURL : nil,
            lanServerURL: lanURL,
            relayServerURL: relayURL,
            customServerURL: nil,
            activeServerURL: activeURL,
            e2eeRequiredHosts: Array(Set(e2eeRequiredHosts)).sorted(),
            cachedPermissionPreset: "strict",
            workspaceNotes: [:],
            createdAt: now,
            lastUsedAt: now
        )
        KeychainHelper.shared.save(key: .deviceToken, value: deviceToken, profileId: created.id)
        KeychainHelper.shared.save(key: .deviceID, value: deviceID, profileId: created.id)
        if let ek = e2eeKeyB64, !ek.isEmpty {
            KeychainHelper.shared.save(key: .e2eeKey, value: ek, profileId: created.id)
        }
        profiles.append(created)
        persistToDisk()
        return (created, true)
    }

    public func setActiveProfileId(_ id: String?) {
        guard activeProfileId != id else { return }
        activeProfileId = id
        if let id, let idx = profiles.firstIndex(where: { $0.id == id }) {
            profiles[idx].lastUsedAt = Date()
        }
        persistToDisk()
        syncRuntimeAndKeychainScope()
    }

    public func updateActiveProfileFromSettings(_ settings: AppSettings) {
        guard let aid = activeProfileId,
              let idx = profiles.firstIndex(where: { $0.id == aid }) else {
            return
        }
        var p = profiles[idx]
        p.rawServerURL = settings.rawServerURL
        p.primaryCloudURL = settings.primaryCloudURL
        p.lanServerURL = settings.lanServerURL
        p.relayServerURL = settings.relayServerURL
        p.customServerURL = settings.customServerURL
        p.activeServerURL = settings.activeServerURL
        p.platform = settings.gatewayPlatform
        p.workspaceNotes = settings.workspaceNotes
        p.cachedPermissionPreset = settings.cachedPermissionPreset
        if !p.isCustomName, p.displayName.isEmpty {
            p.displayName = p.defaultPlatformLabel
        }
        profiles[idx] = p
        persistToDisk()
    }

    public func backfillGatewayMetadata(profileId: String, gatewayId: String?, gatewayName: String?, platform: String?) {
        guard let idx = profiles.firstIndex(where: { $0.id == profileId }) else { return }
        var p = profiles[idx]
        var changed = false
        if let gw = gatewayId?.trimmingCharacters(in: .whitespacesAndNewlines), !gw.isEmpty, p.gatewayId != gw {
            p.gatewayId = gw
            changed = true
        }
        if let plat = platform?.trimmingCharacters(in: .whitespacesAndNewlines), !plat.isEmpty, p.platform != plat {
            p.platform = plat
            changed = true
        }
        if !p.isCustomName,
           let name = gatewayName?.trimmingCharacters(in: .whitespacesAndNewlines),
           !name.isEmpty,
           p.displayName != name {
            p.displayName = name
            changed = true
        }
        if changed {
            profiles[idx] = p
            persistToDisk()
        }
    }

    public func updateCloudURL(profileId: String, cloudURL: String) {
        guard let idx = profiles.firstIndex(where: { $0.id == profileId }) else { return }
        let clean = cloudURL.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard !clean.isEmpty, !NetworkTransport.isLocalOrPrivateHost(clean) else { return }
        if profiles[idx].primaryCloudURL != clean {
            profiles[idx].primaryCloudURL = clean
            persistToDisk()
        }
    }

    public func markE2EEHost(profileId: String, host: String) {
        let clean = NetworkTransport.extractHost(from: host)
        guard !clean.isEmpty, let idx = profiles.firstIndex(where: { $0.id == profileId }) else { return }
        if !profiles[idx].e2eeRequiredHosts.contains(clean) {
            profiles[idx].e2eeRequiredHosts.append(clean)
            persistToDisk()
        }
    }

    public func renameProfile(id: String, newName: String) {
        let clean = String(newName.trimmingCharacters(in: .whitespacesAndNewlines).prefix(24))
        guard !clean.isEmpty, let idx = profiles.firstIndex(where: { $0.id == id }) else { return }
        profiles[idx].displayName = clean
        profiles[idx].isCustomName = true
        persistToDisk()
        if let ctx = requestContext(for: id) {
            let showLabel = profiles.count > 1
            Task {
                await PushNotificationManager.shared.syncGatewayLabel(for: ctx, showLabel: showLabel, label: clean)
            }
        }
    }

    /// Removes a gateway profile, clears its credentials and isolated caches, and returns the next active profile if any.
    @discardableResult
    public func removeProfile(id: String) -> (removed: GatewayProfile?, nextActive: GatewayProfile?) {
        guard let idx = profiles.firstIndex(where: { $0.id == id }) else {
            return (nil, activeProfile)
        }
        // Snapshot request context BEFORE removing the profile or clearing its Keychain entries.
        let ctx = requestContext(for: id)
        let removed = profiles.remove(at: idx)
        KeychainHelper.shared.clear(profileId: removed.id)
        if removed.storageNamespace.isEmpty {
            KeychainHelper.shared.clearLegacy()
        }
        CacheManager.shared.clearStorage(forNamespace: removed.storageNamespace)
        ProjectCacheManager.shared.clearStorage(forNamespace: removed.storageNamespace)
        DocumentCacheManager.shared.clearStorage(forNamespace: removed.storageNamespace)
        WeChatArchiveCacheManager.shared.clearStorage(forNamespace: removed.storageNamespace)
        DraftFileStore.shared.clearStorage(forNamespace: removed.storageNamespace)
        summaries.removeValue(forKey: removed.id)

        if let ctx {
            let candidates = ctx.candidateURLs(isCellular: NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi)
            let primaryURL = candidates.first
            let token = ctx.deviceToken
            let devId = ctx.deviceID
            Task {
                await PushNotificationManager.shared.unregisterFromGateway(context: ctx)
                await APIClient.shared.unpair(
                    baseURL: primaryURL,
                    candidates: candidates,
                    token: token,
                    deviceID: devId
                )
            }
        }

        if activeProfileId == removed.id {
            activeProfileId = profiles.first?.id
        }
        persistToDisk()
        syncRuntimeAndKeychainScope()

        // If we dropped down to 1 gateway, hide gateway subtitles on remaining gateway's pushes.
        if profiles.count == 1, let remaining = profiles.first, let remCtx = requestContext(for: remaining.id) {
            Task {
                await PushNotificationManager.shared.syncGatewayLabel(for: remCtx, showLabel: false, label: remaining.resolvedDisplayName)
            }
        }

        return (removed, activeProfile)
    }

    public func clearAllProfiles() {
        for p in profiles {
            KeychainHelper.shared.clear(profileId: p.id)
            CacheManager.shared.clearStorage(forNamespace: p.storageNamespace)
            ProjectCacheManager.shared.clearStorage(forNamespace: p.storageNamespace)
            DocumentCacheManager.shared.clearStorage(forNamespace: p.storageNamespace)
            WeChatArchiveCacheManager.shared.clearStorage(forNamespace: p.storageNamespace)
            DraftFileStore.shared.clearStorage(forNamespace: p.storageNamespace)
        }
        KeychainHelper.shared.clearLegacy()
        profiles.removeAll()
        summaries.removeAll()
        activeProfileId = nil
        persistToDisk()
        syncRuntimeAndKeychainScope()
    }

    // MARK: - Live & Inactive Host Summaries

    public func updateActiveSummary(from conversations: [ConversationItem]) {
        guard let aid = activeProfileId else { return }
        var actionCount = 0
        var runningCount = 0
        var unreadCount = 0
        var totalActive = 0
        for item in conversations where !item.isSubagent && !item.isArchived && !item.isDraft {
            totalActive += 1
            if item.status.needsAction {
                actionCount += 1
            } else if item.isWorking {
                runningCount += 1
            } else if item.isUnread {
                unreadCount += 1
            }
        }
        summaries[aid] = GatewaySummarySnapshot(
            profileId: aid,
            runningCount: runningCount,
            actionCount: actionCount,
            unreadCount: unreadCount,
            totalActive: totalActive,
            isReachable: true,
            isRefreshing: false,
            gatewayName: activeProfile?.resolvedDisplayName,
            updatedAt: Date()
        )
    }

    public func refreshInactiveSummaries(force: Bool = false, minInterval: TimeInterval = 45.0) {
        guard profiles.count > 1 else { return }
        if !force && Date().timeIntervalSince(lastInactiveRefreshDate) < minInterval {
            return
        }
        if inFlightSummaryTask != nil && !force {
            return
        }
        inFlightSummaryTask?.cancel()
        lastInactiveRefreshDate = Date()

        let activeId = activeProfileId
        let targets = profiles.filter { $0.id != activeId }
        let contexts: [GatewayRequestContext] = targets.compactMap { requestContext(for: $0.id) }
        guard !contexts.isEmpty else { return }

        for ctx in contexts {
            if var existing = summaries[ctx.profileId] {
                existing.isRefreshing = true
                summaries[ctx.profileId] = existing
            } else {
                summaries[ctx.profileId] = GatewaySummarySnapshot(
                    profileId: ctx.profileId,
                    isReachable: true,
                    isRefreshing: true,
                    gatewayName: ctx.displayName,
                    updatedAt: .distantPast
                )
            }
        }

        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        inFlightSummaryTask = Task { [weak self] in
            await withTaskGroup(of: (String, GatewaySummarySnapshot, String?, String?, String?).self) { group in
                for ctx in contexts {
                    group.addTask {
                        await Self.fetchSingleSummary(context: ctx, isCellular: isCell)
                    }
                }
                for await (pid, snap, gwId, gwName, platform) in group {
                    guard !Task.isCancelled, let self else { break }
                    await MainActor.run {
                        self.summaries[pid] = snap
                        if gwId != nil || gwName != nil || platform != nil {
                            self.backfillGatewayMetadata(profileId: pid, gatewayId: gwId, gatewayName: gwName, platform: platform)
                        }
                    }
                }
            }
            await MainActor.run { [weak self] in
                self?.inFlightSummaryTask = nil
            }
        }
    }

    private struct StatusSummaryResp: Decodable {
        struct Summary: Decodable {
            let running_count: Int?
            let action_count: Int?
            let unread_count: Int?
            let total_active: Int?
        }
        let status: String?
        let platform: String?
        let os: String?
        let gateway_id: String?
        let gateway_name: String?
        let summary: Summary?
    }

    private nonisolated static func fetchSingleSummary(
        context: GatewayRequestContext,
        isCellular: Bool
    ) async -> (String, GatewaySummarySnapshot, String?, String?, String?) {
        let candidates = context.candidateURLs(isCellular: isCellular)
        guard !candidates.isEmpty else {
            return (context.profileId, GatewaySummarySnapshot(profileId: context.profileId, isReachable: false, isRefreshing: false, gatewayName: context.displayName), nil, nil, nil)
        }

        for baseURL in candidates {
            let isLan = NetworkTransport.isLanOnlyHost(baseURL.host ?? "")
            var comps = URLComponents(url: baseURL.appendingPathComponent("gateway/status"), resolvingAgainstBaseURL: false)
            comps?.queryItems = [URLQueryItem(name: "summary", value: "1")]
            guard let statusURL = comps?.url else { continue }

            var req = URLRequest(url: statusURL)
            req.httpMethod = "GET"
            req.cachePolicy = .reloadIgnoringLocalCacheData
            req.timeoutInterval = isLan ? 0.9 : 2.8

            do {
                let (data, response) = try await NetworkTransport.shared.send(request: req, context: context)
                guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
                    continue
                }
                if let parsed = try? JSONDecoder().decode(StatusSummaryResp.self, from: data) {
                    let plat = parsed.platform ?? parsed.os
                    if let s = parsed.summary {
                        let snap = GatewaySummarySnapshot(
                            profileId: context.profileId,
                            runningCount: max(0, s.running_count ?? 0),
                            actionCount: max(0, s.action_count ?? 0),
                            unreadCount: max(0, s.unread_count ?? 0),
                            totalActive: max(0, s.total_active ?? 0),
                            isReachable: true,
                            isRefreshing: false,
                            gatewayName: parsed.gateway_name ?? context.displayName,
                            updatedAt: Date()
                        )
                        return (context.profileId, snap, parsed.gateway_id, parsed.gateway_name, plat)
                    }

                    // Older gateway without `summary` field: degrade gracefully via GetAllCascadeTrajectories.
                    let fallback = await fetchLegacySummaryCounts(baseURL: baseURL, context: context)
                    let snap = GatewaySummarySnapshot(
                        profileId: context.profileId,
                        runningCount: fallback.running,
                        actionCount: fallback.action,
                        unreadCount: fallback.unread,
                        totalActive: fallback.total,
                        isReachable: true,
                        isRefreshing: false,
                        gatewayName: parsed.gateway_name ?? context.displayName,
                        updatedAt: Date()
                    )
                    return (context.profileId, snap, parsed.gateway_id, parsed.gateway_name, plat)
                }
            } catch {
                continue
            }
        }

        // Unreachable: preserve any previously cached counters if available while marking offline.
        return (
            context.profileId,
            GatewaySummarySnapshot(
                profileId: context.profileId,
                isReachable: false,
                isRefreshing: false,
                gatewayName: context.displayName,
                updatedAt: Date()
            ),
            nil,
            nil,
            nil
        )
    }

    private nonisolated static func fetchLegacySummaryCounts(
        baseURL: URL,
        context: GatewayRequestContext
    ) async -> (running: Int, action: Int, unread: Int, total: Int) {
        let endpoint = baseURL.appendingPathComponent("api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories")
        var req = URLRequest(url: endpoint)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue("1", forHTTPHeaderField: "Connect-Protocol-Version")
        req.httpBody = Data("{}".utf8)
        req.timeoutInterval = 3.5

        guard let (data, resp) = try? await NetworkTransport.shared.send(request: req, context: context),
              let http = resp as? HTTPURLResponse,
              (200...299).contains(http.statusCode),
              let decoded = try? JSONDecoder().decode(GetAllCascadeTrajectoriesResponse.self, from: data),
              let summaries = decoded.trajectorySummaries else {
            return (0, 0, 0, 0)
        }

        var running = 0
        var action = 0
        var unread = 0
        var total = 0
        for (_, s) in summaries {
            if s.isSubagent || s.annotations?.archived == true { continue }
            total += 1
            if s.needsInput == true {
                action += 1
            } else if s.status == "CASCADE_RUN_STATUS_RUNNING" || s.hasRunningSubagents == true || (s.swarmWorkingCount ?? 0) > 0 {
                running += 1
            } else if s.annotations?.markedAsUnread == true {
                unread += 1
            }
        }
        return (running, action, unread, total)
    }
}
