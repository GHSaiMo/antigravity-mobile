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
    public var tag: String?
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

    private static let claudeEnumTokens = ["m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405"]

    public static let staticEnumByID: [String: String] = [
        "gemini-4-carbon-high": "MODEL_PLACEHOLDER_M420",
        "gemini-4-carbon-medium": "MODEL_PLACEHOLDER_M421",
        "gemini-4-carbon-low": "MODEL_PLACEHOLDER_M422",
        "gemini-4-carbon": "MODEL_PLACEHOLDER_M420",
        "gemini-4-argon-high": "MODEL_PLACEHOLDER_M417",
        "gemini-4-argon-medium": "MODEL_PLACEHOLDER_M418",
        "gemini-4-argon-low": "MODEL_PLACEHOLDER_M419",
        "gemini-3.8-flash-high": "MODEL_PLACEHOLDER_M318",
        "gemini-3.8-flash-medium": "MODEL_PLACEHOLDER_M319",
        "gemini-3.8-flash-low": "MODEL_PLACEHOLDER_M320",
        "gemini-3.1-pro-high": "MODEL_PLACEHOLDER_M37",
        "gemini-3.1-pro-low": "MODEL_PLACEHOLDER_M36",
        "claude-opus-5-5-high": "MODEL_PLACEHOLDER_M402",
        "claude-opus-5-5-medium": "MODEL_PLACEHOLDER_M401",
        "claude-opus-5-5-low": "MODEL_PLACEHOLDER_M400",
        "claude-sonnet-5-5-high": "MODEL_PLACEHOLDER_M405",
        "claude-sonnet-5-5-medium": "MODEL_PLACEHOLDER_M404",
        "claude-sonnet-5-5-low": "MODEL_PLACEHOLDER_M403",
        "claude-opus-4-6-thinking": "MODEL_PLACEHOLDER_M26",
        "claude-sonnet-4-6": "MODEL_PLACEHOLDER_M35",
        "gpt-oss-120b-medium": "MODEL_OPENAI_GPT_OSS_120B_MEDIUM",
    ]

    public static let staticIDByEnum: [String: String] = Dictionary(
        staticEnumByID.map { ($0.value, $0.key) },
        uniquingKeysWith: { current, _ in current }
    )

    /// 内置/预设模型 ID → 统一展示名映射，避免离线、未拉取目录或切号过渡期露出小写裸 ID。
    public static let staticDisplayNameByID: [String: String] = [
        "gemini-4-carbon-high": "Gemini 4 Carbon (High)",
        "gemini-4-carbon-medium": "Gemini 4 Carbon (Medium)",
        "gemini-4-carbon-low": "Gemini 4 Carbon (Low)",
        "gemini-4-carbon": "Gemini 4 Carbon",
        "gemini-4-argon-high": "Gemini 4 Argon (High)",
        "gemini-4-argon-medium": "Gemini 4 Argon (Medium)",
        "gemini-4-argon-low": "Gemini 4 Argon (Low)",
        "gemini-3.8-flash-high": "Gemini 3.8 Flash (High)",
        "gemini-3.8-flash-medium": "Gemini 3.8 Flash (Medium)",
        "gemini-3.8-flash-low": "Gemini 3.8 Flash (Low)",
        "gemini-3.1-pro-high": "Gemini 3.1 Pro (High)",
        "gemini-3.1-pro-low": "Gemini 3.1 Pro (Low)",
        "claude-opus-5-5-high": "Claude Opus 5.5 (High)",
        "claude-opus-5-5-medium": "Claude Opus 5.5 (Medium)",
        "claude-opus-5-5-low": "Claude Opus 5.5 (Low)",
        "claude-sonnet-5-5-high": "Claude Sonnet 5.5 (High)",
        "claude-sonnet-5-5-medium": "Claude Sonnet 5.5 (Medium)",
        "claude-sonnet-5-5-low": "Claude Sonnet 5.5 (Low)",
        "claude-opus-4-6-thinking": "Claude Opus 4.6 (Thinking)",
        "claude-sonnet-4-6": "Claude Sonnet 4.6 (Thinking)",
        "gpt-oss-120b-medium": "GPT-OSS 120B (Medium)",
    ]

    /// 将未知模型 ID 转为统一规范的展示名（如 `claude-sonnet-5-5-high` → `Claude Sonnet 5.5 (High)`）。
    public static func formatIDAsDisplayName(_ id: String) -> String {
        let trimmed = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "" }
        if let known = staticDisplayNameByID[trimmed.lowercased()] {
            return known
        }
        let tokens = trimmed.split(separator: "-").map(String.init)
        guard !tokens.isEmpty else { return trimmed }
        let trailingQualifiers: Set<String> = ["high", "medium", "low", "thinking"]
        var qualifier: String? = nil
        var core = tokens
        if let last = core.last, trailingQualifiers.contains(last.lowercased()), core.count > 1 {
            qualifier = last.prefix(1).uppercased() + last.dropFirst().lowercased()
            core.removeLast()
        }
        var parts: [String] = []
        for token in core {
            let isDigits = !token.isEmpty && token.allSatisfy(\.isNumber)
            if isDigits, let prev = parts.last, prev.allSatisfy({ $0.isNumber || $0 == "." }) {
                parts[parts.count - 1] = prev + "." + token
            } else if token.lowercased() == "gpt" || token.lowercased() == "oss" {
                parts.append(token.uppercased())
            } else if token.first?.isLetter == true {
                parts.append(token.prefix(1).uppercased() + token.dropFirst())
            } else {
                parts.append(token)
            }
        }
        let base = parts.joined(separator: " ")
        if let q = qualifier {
            return "\(base) (\(q))"
        }
        return base
    }

    public static func displayName(forID id: String, options: [ModelOption] = []) -> String {
        let key = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if let match = options.first(where: { $0.id.caseInsensitiveCompare(key) == .orderedSame }),
           !match.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return match.name
        }
        return formatIDAsDisplayName(key)
    }

    public static func isClaude(_ modelId: String) -> Bool {
        let lower = modelId.lowercased()
        if lower.contains("claude") { return true }
        return claudeEnumTokens.contains(where: { lower.contains($0) })
    }

    private static let ignoredPickerTags: Set<String> = ["new", "fast", "notice", "提示"]

    /// 格式化模型选择器选项展示名：过滤掉 "New" / "Fast" / "Notice" 等冗余特性标签及前导的点符号，保持手机端菜单简洁。
    public static func formatPickerLabel(name: String, tag: String?) -> String {
        guard let tag = tag?.trimmingCharacters(in: .whitespacesAndNewlines),
              !tag.isEmpty,
              !ignoredPickerTags.contains(tag.lowercased()) else {
            return name
        }
        return "\(name) · \(tag)"
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
        catalog?.models.first(where: { $0.id.caseInsensitiveCompare(id) == .orderedSame })?.name
    }

    /// 始终返回人类可读的统一展示名（实时目录 → 静态映射 → 规范格式化），不暴露小写裸 ID。
    public func resolvedDisplayName(forID id: String) -> String {
        ModelDefaultsLogic.displayName(forID: id, options: catalog?.models ?? [])
    }

    /// 枚举：目录里有就用目录的，没有就回退到内置静态映射表。
    public func modelEnum(forID id: String) -> String? {
        if let found = catalog?.models.first(where: { $0.id == id })?.enum, !found.isEmpty {
            return found
        }
        return ModelDefaultsLogic.staticEnumByID[id.lowercased()]
    }

    public func id(forEnum modelEnum: String) -> String? {
        if let found = catalog?.models.first(where: { $0.enum == modelEnum })?.id, !found.isEmpty {
            return found
        }
        return ModelDefaultsLogic.staticIDByEnum[modelEnum]
    }

    /// 从网关刷新目录；成功后同步校验设置里的默认模型与当前激活模型。
    public func refresh(apiClient: APIClient = .shared, settings: AppSettings, baseURL: URL, force: Bool = false) async {
        isLoading = true
        loadFailed = false
        defer { isLoading = false }
        do {
            let fresh = try await apiClient.fetchModels(baseURL: baseURL, refresh: force)
            catalog = fresh
            if let data = try? JSONEncoder().encode(fresh) {
                UserDefaults.standard.set(data, forKey: Self.storageKey)
            }
            let validatedGemini = ModelDefaultsLogic.validated(saved: settings.defaultGeminiModel, provider: "gemini", catalog: fresh)
            let validatedClaude = ModelDefaultsLogic.validated(saved: settings.defaultClaudeModel, provider: "claude", catalog: fresh)
            settings.defaultGeminiModel = validatedGemini
            settings.defaultClaudeModel = validatedClaude
            if fresh.isLive && !fresh.models.contains(where: { $0.id == settings.activeModel }) {
                settings.activeModel = ModelDefaultsLogic.isClaude(settings.activeModel) ? validatedClaude : validatedGemini
            }
        } catch {
            loadFailed = true
        }
    }
}
