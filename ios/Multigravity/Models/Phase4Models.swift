import Foundation

// MARK: - 1. BrowserViewerService (Remote Live Frame & Touch Takeover)

public struct BrowserViewerTab: Codable, Hashable, Sendable, Identifiable {
    public var id: String { tabId }
    public let tabId: String
    public let title: String?
    public let url: String?
    public let active: Bool?
    public let agentOwned: Bool?
    public let loading: Bool?
    public let widthPx: Int?
    public let heightPx: Int?
    public let rootConversationId: String?

    public var displayTitle: String {
        if let t = title?.trimmingCharacters(in: .whitespacesAndNewlines), !t.isEmpty { return t }
        if let u = url?.trimmingCharacters(in: .whitespacesAndNewlines), !u.isEmpty { return u }
        return tabId
    }
}

public struct BrowserTabsResponse: Codable, Sendable {
    public let ok: Bool?
    public let hasBrowserHost: Bool?
    public let tabs: [BrowserViewerTab]?
    public let error: String?
}

public struct BrowserOpenViewResponse: Codable, Sendable {
    public let ok: Bool?
    public let sessionId: String?
    public let streamId: String?
    public let expiresUnixMs: Int64?
    public let tab: BrowserViewerTab?
    public let error: String?
}

public struct BrowserFrameResponse: Codable, Sendable {
    public let ok: Bool?
    public let imageData: String?
    public let mimeType: String?
    public let widthPx: Int?
    public let heightPx: Int?
    public let tab: BrowserViewerTab?
    public let error: String?
}

public struct BrowserTakeoverResponse: Codable, Sendable {
    public let ok: Bool?
    public let controlToken: String?
    public let controlExpiresUnixMs: Int64?
    public let tab: BrowserViewerTab?
    public let error: String?
}

public struct BrowserControlResponse: Codable, Sendable {
    public let ok: Bool?
    public let tab: BrowserViewerTab?
    public let error: String?
}

// MARK: - 2. RemotingService (SSH Remote Hosts & Interactive Prompts)

public struct RemoteHostHop: Codable, Hashable, Sendable {
    public let host: String?
    public let user: String?
    public let port: Int?
}

public struct RemoteMultiHop: Codable, Hashable, Sendable {
    public let hops: [RemoteHostHop]?
}

public struct RemoteHostDescriptor: Codable, Hashable, Sendable, Identifiable {
    public var id: String {
        if let rid = remoteId, !rid.isEmpty { return rid }
        if let n = name, !n.isEmpty { return n }
        if let h = remoteMultiHop?.hops?.first?.host, !h.isEmpty { return h }
        return UUID().uuidString
    }
    public let remoteId: String?
    public let name: String?
    public let description: String?
    public let type: String?
    public let remoteMultiHop: RemoteMultiHop?

    public var displayName: String {
        if let n = name?.trimmingCharacters(in: .whitespacesAndNewlines), !n.isEmpty { return n }
        if let hop = remoteMultiHop?.hops?.first, let h = hop.host, !h.isEmpty {
            if let u = hop.user, !u.isEmpty { return "\(u)@\(h)" }
            return h
        }
        return remoteId ?? "SSH Host"
    }

    public var hostSummary: String {
        if let hop = remoteMultiHop?.hops?.first, let h = hop.host, !h.isEmpty {
            let userPrefix = (hop.user?.isEmpty == false) ? "\(hop.user!)@" : ""
            let portSuffix = (hop.port != nil && hop.port != 22 && hop.port != 0) ? ":\(hop.port!)" : ""
            return "\(userPrefix)\(h)\(portSuffix)"
        }
        return description ?? ""
    }
}

public struct RemoteConnectPrompt: Codable, Hashable, Sendable {
    public let message: String
    public let isSecret: Bool?
}

public struct RemoteConnectJob: Codable, Hashable, Sendable, Identifiable {
    public var id: String { jobId }
    public let jobId: String
    public let name: String?
    public let remoteId: String?
    public let status: String // "connecting" | "prompt" | "connected" | "error"
    public let message: String?
    public let error: String?
    public let prompt: RemoteConnectPrompt?
    public let done: Bool?
    public let updatedAt: String?
}

public struct EnvironmentItem: Codable, Hashable, Sendable, Identifiable {
    public var id: String {
        if let eid = envId, !eid.isEmpty { return eid }
        if let n = name, !n.isEmpty { return n }
        return UUID().uuidString
    }
    public let envId: String?
    public let name: String?
    public let displayName: String?
    public let state: String?
    public let status: String?

    enum CodingKeys: String, CodingKey {
        case envId = "id"
        case name, displayName, state, status
    }
}

public struct RemotesListResponse: Codable, Sendable {
    public let ok: Bool?
    public let remotes: [RemoteHostDescriptor]?
    public let connected: [RemoteHostDescriptor]?
    public let environments: [EnvironmentItem]?
    public let jobs: [RemoteConnectJob]?
    public let error: String?
}

public struct RemoteConnectJobResponse: Codable, Sendable {
    public let ok: Bool?
    public let job: RemoteConnectJob?
    public let error: String?
}

// MARK: - 3. Marketplace, Plugin Uninstall & Plugin/MCP OAuth

public struct MarketplaceItem: Codable, Hashable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let description: String?
    public let author: String?
    public let version: String?
    public let kind: String?
    public let installCount: Int64?
    public let userCount: Int64?
    public let homepage: String?
    public let repository: String?
    public let logo: String?
    public let marketplace: String?
    public let installed: Bool?

    public var isInstalled: Bool { installed ?? false }
}

public struct MarketplaceSearchResponse: Codable, Sendable {
    public let ok: Bool?
    public let items: [MarketplaceItem]?
    public let nextPageToken: String?
    public let error: String?
}

public struct PluginAuthAccountInfo: Codable, Hashable, Sendable, Identifiable {
    public var id: String { accountId }
    public let accountId: String
    public let alias: String?
    public let identity: String?
    public let label: String?
    public let isDefault: Bool?
    public let needsAuth: Bool?
    public let needsAuthReason: String?

    public var displayLabel: String {
        let base = [label, identity, alias].compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }.first(where: { !$0.isEmpty }) ?? accountId
        return base
    }
}

public struct OAuthStartResponse: Codable, Sendable {
    public let ok: Bool?
    public let authUrl: String?
    public let accountId: String?
    public let mode: String?
    public let isLoopbackCallback: Bool?
    public let error: String?
}

public struct OAuthCallbackForwardResponse: Codable, Sendable {
    public let ok: Bool?
    public let statusCode: Int?
    public let message: String?
    public let error: String?
}

// MARK: - 4. Desktop Self-Update & Changelog

public struct DesktopUpdateStatus: Codable, Hashable, Sendable {
    public let ok: Bool?
    public let currentVersion: String?
    public let latestVersion: String?
    public let updateAvailable: Bool?
    public let runningCount: Int?
    public let error: String?

    public var hasUpdate: Bool { updateAvailable ?? false }
}

public struct DesktopChangelogResponse: Codable, Sendable {
    public let ok: Bool?
    public let version: String?
    public let path: String?
    public let markdown: String?
    public let error: String?
}

public struct DesktopApplyUpdateResponse: Codable, Sendable {
    public let ok: Bool?
    public let notifiedHost: Bool?
    public let message: String?
    public let error: String?
}

// MARK: - 5. Workspace Code & File Search

public struct CodeSearchHit: Codable, Hashable, Sendable, Identifiable {
    public var id: String { "\(path):\(lineNumber):\(matchStartCol ?? 0)" }
    public let path: String
    public let relativePath: String
    public let uri: String
    public let lineNumber: Int
    public let snippet: String
    public let matchStartCol: Int?
    public let matchEndCol: Int?
    public let contextBefore: [String]?
    public let contextAfter: [String]?
}

public struct CodeSearchResponse: Codable, Sendable {
    public let ok: Bool?
    public let workspaceUri: String?
    public let source: String?
    public let results: [CodeSearchHit]?
    public let error: String?
}

public struct FileSearchHit: Codable, Hashable, Sendable, Identifiable {
    public var id: String { path }
    public let path: String
    public let relativePath: String
    public let uri: String
    public let isDirectory: Bool?
}

public struct FileSearchResponse: Codable, Sendable {
    public let ok: Bool?
    public let workspaceUri: String?
    public let source: String?
    public let results: [FileSearchHit]?
    public let error: String?
}

// MARK: - 6. Artifact Version Snapshots

public struct ArtifactSnapshotItem: Codable, Hashable, Sendable, Identifiable {
    public var id: String { "\(artifactUri)#\(index)" }
    public let index: Int
    public let artifactName: String
    public let artifactUri: String
    public let content: String
    public let lastEdited: String?
    public let reviewState: String?
    public let summary: String?
}

public struct ArtifactSnapshotsResponse: Codable, Sendable {
    public let ok: Bool?
    public let cascadeId: String?
    public let snapshots: [ArtifactSnapshotItem]?
    public let error: String?
}

// MARK: - 7. Worktree PR & Conversation Starring

public struct WorktreePRResponse: Codable, Sendable {
    public let ok: Bool?
    public let prUrl: String?
    public let error: String?
}

public struct StarCascadeResponse: Codable, Sendable {
    public let ok: Bool?
    public let cascadeId: String?
    public let starred: Bool?
    public let error: String?
}
