import Foundation
import Observation

// MARK: - 模型目录（网关 /gateway/models）

/// 网关返回的可选模型。provider 为 "gemini" | "claude" | "other"。
public struct ModelOption: Codable, Sendable, Identifiable, Hashable {
    public var id: String
    public var name: String
    public var provider: String
    public var `enum`: String?
    public var supportsImages: Bool?
    public var thinking: Bool?
}

public struct ModelsResponse: Codable, Sendable {
    public var models: [ModelOption]
    /// 网关建议的各厂商默认模型（provider -> model id）。
    public var defaults: [String: String]
    /// false 表示网关暂时取不到实时列表，返回的是内置兜底。
    public var live: Bool?

    public var isLive: Bool { live ?? false }

    public init(models: [ModelOption], defaults: [String: String], live: Bool? = nil) {
        self.models = models
        self.defaults = defaults
        self.live = live
    }
}

// MARK: - 默认模型与当前会话模型的解析规则（纯函数，便于单测）

public enum ModelDefaultsLogic {
    public static let fallbackGemini = "gemini-3.8-flash-high"
    public static let fallbackClaude = "claude-opus-4-6-thinking"

    public static func isClaude(_ modelId: String) -> Bool {
        let lower = modelId.lowercased()
        return lower.contains("claude") || lower.contains("m26")
    }

    /// 输入框上方胶囊的文字：只有厂商名，保持简洁。
    public static func providerLabel(_ modelId: String) -> String {
        if isClaude(modelId) { return "Claude" }
        if modelId.lowercased().contains("gpt") { return "GPT" }
        return "Gemini"
    }

    /// 胶囊点击后要切到的默认模型：Claude → Gemini 默认；其它 → Claude 默认。
    public static func toggleTarget(current: String, geminiDefault: String, claudeDefault: String) -> String {
        isClaude(current) ? geminiDefault : claudeDefault
    }

    /// 网关推送的 activeModel 如何落到界面状态：
    /// 为空保持当前；已是可读模型 id 则原样保留（让后续消息继续用该会话实际的模型）；
    /// 只拿到未解析的裸枚举（MODEL_...）时退回该厂商的默认模型。
    public static func resolveActive(raw: String?, current: String, geminiDefault: String, claudeDefault: String) -> String {
        let value = (raw ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if value.isEmpty { return current }
        if value.uppercased().hasPrefix("MODEL_") {
            return isClaude(value) ? claudeDefault : geminiDefault
        }
        return value
    }

    /// 设置里保存的默认模型已不在实时列表时，换成网关建议的默认值。兜底（非实时）列表不触发重置。
    public static func validated(saved: String, provider: String, catalog: ModelsResponse) -> String {
        guard catalog.isLive else { return saved }
        if catalog.models.contains(where: { $0.id == saved && $0.provider == provider }) { return saved }
        return catalog.defaults[provider]
            ?? catalog.models.first(where: { $0.provider == provider })?.id
            ?? saved
    }

    /// 从缓存的 cascadeConfigRaw 里取出规划模型枚举（planModel 优先，否则第一个 MODEL_ 记号）。
    public static func extractModelEnum(fromConfig cfg: String) -> String? {
        let patterns = [#""planModel"\s*:\s*"(MODEL_[A-Za-z0-9_]+)""#, #"(MODEL_[A-Za-z0-9_]+)"#]
        for pattern in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern) else { continue }
            let range = NSRange(cfg.startIndex..., in: cfg)
            if let m = regex.firstMatch(in: cfg, range: range), m.numberOfRanges > 1,
               let r = Range(m.range(at: 1), in: cfg) {
                return String(cfg[r])
            }
        }
        return nil
    }
}

// MARK: - 目录缓存

/// 缓存最近一次取到的模型目录：设置页展示、id ↔ 枚举互查都读它。离线或网关未更新时回退到内置映射。
@MainActor
@Observable
public final class ModelCatalogStore {
    public static let shared = ModelCatalogStore()
    private static let storageKey = "antigravity.model_catalog_json"

    public private(set) var catalog: ModelsResponse?
    public private(set) var isLoading = false
    public private(set) var loadFailed = false

    private init() {
        if let data = UserDefaults.standard.data(forKey: Self.storageKey),
           let saved = try? JSONDecoder().decode(ModelsResponse.self, from: data) {
            catalog = saved
        }
    }

    public func options(for provider: String) -> [ModelOption] {
        (catalog?.models ?? []).filter { $0.provider == provider }
    }

    public func displayName(forID id: String) -> String? {
        catalog?.models.first(where: { $0.id == id })?.name
    }

    /// 枚举：目录里有就用目录的，没有就由调用方回退到内置映射。
    public func modelEnum(forID id: String) -> String? {
        catalog?.models.first(where: { $0.id == id })?.enum.flatMap { $0.isEmpty ? nil : $0 }
    }

    public func id(forEnum modelEnum: String) -> String? {
        catalog?.models.first(where: { $0.enum == modelEnum })?.id
    }

    /// 从网关刷新目录；成功后同步校验设置里的默认模型。
    public func refresh(apiClient: APIClient = .shared, settings: AppSettings, baseURL: URL) async {
        isLoading = true
        loadFailed = false
        defer { isLoading = false }
        do {
            let fresh = try await apiClient.fetchModels(baseURL: baseURL)
            catalog = fresh
            if let data = try? JSONEncoder().encode(fresh) {
                UserDefaults.standard.set(data, forKey: Self.storageKey)
            }
            settings.defaultGeminiModel = ModelDefaultsLogic.validated(saved: settings.defaultGeminiModel, provider: "gemini", catalog: fresh)
            settings.defaultClaudeModel = ModelDefaultsLogic.validated(saved: settings.defaultClaudeModel, provider: "claude", catalog: fresh)
        } catch {
            loadFailed = true
        }
    }
}
