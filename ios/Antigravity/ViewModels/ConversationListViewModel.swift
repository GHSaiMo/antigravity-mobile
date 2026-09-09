import Foundation
import Observation

@Observable
@MainActor
public final class ConversationListViewModel {
    public var conversations: [ConversationItem] = []
    public var searchQuery: String = ""
    public var isLoading: Bool = false
    public var errorMessage: String? = nil
    
    private let apiClient: APIClient
    private let settings: AppSettings
    private let cacheManager: CacheManager
    
    public init(
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.cacheManager = cacheManager ?? .shared
        
        // Immediate local cache restore
        let cached = self.cacheManager.loadConversations()
        if !cached.isEmpty {
            self.conversations = cached
        }
    }
    
    public var filteredConversations: [ConversationItem] {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return conversations }
        return conversations.filter {
            $0.title.lowercased().contains(q) ||
            $0.workspaceName.lowercased().contains(q) ||
            $0.id.lowercased().contains(q)
        }
    }
    
    @MainActor
    public func fetchConversations() async {
        if conversations.isEmpty {
            let cached = cacheManager.loadConversations()
            if !cached.isEmpty {
                self.conversations = cached
            }
        }
        
        guard let url = settings.serverURL else {
            if conversations.isEmpty {
                self.errorMessage = "请在设置中配置有效的服务器地址"
            }
            return
        }
        
        if conversations.isEmpty {
            self.isLoading = true
        }
        self.errorMessage = nil
        
        do {
            let items = try await apiClient.fetchConversations(baseURL: url)
            self.conversations = items
            cacheManager.saveConversations(items)
            self.isLoading = false
        } catch {
            if conversations.isEmpty {
                self.errorMessage = error.localizedDescription
            }
            self.isLoading = false
        }
    }
}
