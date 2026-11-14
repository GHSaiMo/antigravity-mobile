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

public enum ModelProviderKind: String, Sendable {
    case gemini
    case claude
    case orchestrator
    case other
}

public enum ModelEffortRank: Int, Comparable, Sendable {
    case low = 0
    case medium = 1
    case high = 2
    case extra = 3

    public static func < (lhs: ModelEffortRank, rhs: ModelEffortRank) -> Bool {
        lhs.rawValue < rhs.rawValue
    }

    public var displayLabel: String {
        switch self {
        case .low: return "Low"
        case .medium: return "Medium"
        case .high: return "High"
        case .extra: return "Extra"
        }
    }

    public static func parse(_ raw: String) -> ModelEffortRank? {
        let lower = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if lower == "low" || lower.hasSuffix("-low") { return .low }
        if lower == "medium" || lower == "med" || lower.hasSuffix("-medium") { return .medium }
        if lower == "high" || lower.hasSuffix("-high") { return .high }
        if lower == "xhigh" || lower == "extra" || lower == "extra high" || lower == "extra-high"
            || lower.hasSuffix("-xhigh") || lower.hasSuffix("-extra") {
            return .extra
        }
        return nil
    }
}

public struct ModelVariant: Identifiable, Hashable, Sendable {
    public var id: String
    public var label: String
    public var rawName: String
    public var tag: String?

    public init(id: String, label: String, rawName: String, tag: String? = nil) {
        self.id = id
        self.label = label
        self.rawName = rawName
        self.tag = tag
    }

    public var effortRank: ModelEffortRank? {
        ModelEffortRank.parse(label) ?? ModelEffortRank.parse(id)
    }

    public var effortDisplayLabel: String {
        effortRank?.displayLabel ?? label
    }
}

public struct ModelFamily: Identifiable, Hashable, Sendable {
    public var id: String
    public var name: String
    public var provider: String
    public var variants: [ModelVariant]

    public init(id: String, name: String, provider: String, variants: [ModelVariant]) {
        self.id = id
        self.name = name
        self.provider = provider
        self.variants = variants
    }

    /// True when this family has multiple thinking-effort tiers (Low / Medium / High / Extra).
    public var hasEffortVariants: Bool {
        variants.count > 1 && variants.allSatisfy { $0.effortRank != nil }
    }

    /// Variants ordered from top to bottom in the floating effort list:
    /// Bottom-to-top is Low -> Medium -> High -> Extra, so top-to-bottom is Extra -> High -> Medium -> Low.
    public var variantsTopToBottom: [ModelVariant] {
        guard hasEffortVariants else { return variants }
        return variants.sorted { lhs, rhs in
            let l = lhs.effortRank?.rawValue ?? -1
            let r = rhs.effortRank?.rawValue ?? -1
            return l > r
        }
    }

    /// Default variant when selecting the family: Medium first, then High, else first.
    public var defaultVariant: ModelVariant? {
        let ordered = variantsTopToBottom
        if let med = ordered.first(where: { $0.effortRank == .medium }) { return med }
        if let high = ordered.first(where: { $0.effortRank == .high }) { return high }
        return ordered.first
    }

    /// Index of the anchor variant (Medium when available) in `variantsTopToBottom` so it aligns horizontally with the family row.
    public var defaultVariantIndexTopToBottom: Int {
        let ordered = variantsTopToBottom
        guard let target = defaultVariant,
              let idx = ordered.firstIndex(where: { $0.id == target.id }) else {
            return 0
        }
        return idx
    }
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
    public static let fallbackOrchestrator = "gemini-pro-orchestrator"

    private static let claudeEnumTokens = ["m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405"]
    private static let orchestratorEnumTokens = ["m667", "m681"]

    public static let staticEnumByID: [String: String] = [
        "gemini-4-carbon-high": "MODEL_PLACEHOLDER_M427",
        "gemini-4-carbon-medium": "MODEL_PLACEHOLDER_M437",
        "gemini-4-carbon-low": "MODEL_PLACEHOLDER_M438",
        "gemini-4-carbon": "MODEL_PLACEHOLDER_M437",
        "gemini-4-argon-high": "MODEL_PLACEHOLDER_M417",
        "gemini-4-argon-medium": "MODEL_PLACEHOLDER_M418",
        "gemini-4-argon-low": "MODEL_PLACEHOLDER_M419",
        "gemini-4-hydrogen-xhigh": "MODEL_PLACEHOLDER_M528",
        "gemini-4-hydrogen-high": "MODEL_PLACEHOLDER_M525",
        "gemini-4-hydrogen-medium": "MODEL_PLACEHOLDER_M526",
        "gemini-4-hydrogen-low": "MODEL_PLACEHOLDER_M527",
        "gemini-pro-orchestrator": "MODEL_PLACEHOLDER_M667",
        "gemini-pro-orchestrator-argon": "MODEL_PLACEHOLDER_M681",
        "orchestrator-carbon": "MODEL_PLACEHOLDER_M667",
        "orchestrator-argon": "MODEL_PLACEHOLDER_M681",
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

    public static let staticIDByEnum: [String: String] = {
        var map = Dictionary(
            staticEnumByID.map { ($0.value, $0.key) },
            uniquingKeysWith: { current, _ in current }
        )
        // Legacy Carbon placeholders & canonical orchestrator precedence
        map["MODEL_PLACEHOLDER_M420"] = "gemini-4-carbon-high"
        map["MODEL_PLACEHOLDER_M421"] = "gemini-4-carbon-medium"
        map["MODEL_PLACEHOLDER_M422"] = "gemini-4-carbon-low"
        map["MODEL_PLACEHOLDER_M667"] = "gemini-pro-orchestrator"
        map["MODEL_PLACEHOLDER_M681"] = "gemini-pro-orchestrator-argon"
        return map
    }()

    /// 内置/预设模型 ID → 统一展示名映射，避免离线、未拉取目录或切号过渡期露出小写裸 ID。
    public static let staticDisplayNameByID: [String: String] = [
        "gemini-4-carbon-high": "Gemini 4 Carbon (High)",
        "gemini-4-carbon-medium": "Gemini 4 Carbon (Medium)",
        "gemini-4-carbon-low": "Gemini 4 Carbon (Low)",
        "gemini-4-carbon": "Gemini 4 Carbon",
        "gemini-4-argon-high": "Gemini 4 Argon (High)",
        "gemini-4-argon-medium": "Gemini 4 Argon (Medium)",
        "gemini-4-argon-low": "Gemini 4 Argon (Low)",
        "gemini-4-hydrogen-xhigh": "Gemini 4 Hydrogen (Extra)",
        "gemini-4-hydrogen-high": "Gemini 4 Hydrogen (High)",
        "gemini-4-hydrogen-medium": "Gemini 4 Hydrogen (Medium)",
        "gemini-4-hydrogen-low": "Gemini 4 Hydrogen (Low)",
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
        "gemini-pro-orchestrator": "Orchestrator (Carbon)",
        "gemini-pro-orchestrator-argon": "Orchestrator (Argon)",
        "orchestrator-carbon": "Orchestrator (Carbon)",
        "orchestrator-argon": "Orchestrator (Argon)",
        "gpt-oss-120b-medium": "GPT-OSS 120B (Medium)",
    ]

    /// 顶栏芯片紧凑展示名映射（如 `Gemini 4 Carbon (High)` → `Carbon · High`）。
    public static let staticCompactDisplayNameByID: [String: String] = [
        "gemini-4-carbon-high": "Carbon · High",
        "gemini-4-carbon-medium": "Carbon · Med",
        "gemini-4-carbon-low": "Carbon · Low",
        "gemini-4-carbon": "Carbon",
        "gemini-4-argon-high": "Argon · High",
        "gemini-4-argon-medium": "Argon · Med",
        "gemini-4-argon-low": "Argon · Low",
        "gemini-4-hydrogen-xhigh": "Hydrogen · Extra",
        "gemini-4-hydrogen-high": "Hydrogen · High",
        "gemini-4-hydrogen-medium": "Hydrogen · Med",
        "gemini-4-hydrogen-low": "Hydrogen · Low",
        "gemini-3.8-flash-high": "Flash 3.8 · High",
        "gemini-3.8-flash-medium": "Flash 3.8 · Med",
        "gemini-3.8-flash-low": "Flash 3.8 · Low",
        "gemini-3.1-pro-high": "Pro 3.1 · High",
        "gemini-3.1-pro-low": "Pro 3.1 · Low",
        "claude-opus-5-5-high": "Opus 5.5 · High",
        "claude-opus-5-5-medium": "Opus 5.5 · Med",
        "claude-opus-5-5-low": "Opus 5.5 · Low",
        "claude-sonnet-5-5-high": "Sonnet 5.5 · High",
        "claude-sonnet-5-5-medium": "Sonnet 5.5 · Med",
        "claude-sonnet-5-5-low": "Sonnet 5.5 · Low",
        "claude-opus-4-6-thinking": "Opus 4.6 · Think",
        "claude-sonnet-4-6": "Sonnet 4.6 · Think",
        "gemini-pro-orchestrator": "Orch · Carbon",
        "gemini-pro-orchestrator-argon": "Orch · Argon",
        "orchestrator-carbon": "Orch · Carbon",
        "orchestrator-argon": "Orch · Argon",
        "gpt-oss-120b-medium": "GPT-OSS · Med",
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
        let trailingQualifiers: Set<String> = ["high", "medium", "low", "xhigh", "extra", "thinking"]
        var qualifier: String? = nil
        var core = tokens
        if let last = core.last, trailingQualifiers.contains(last.lowercased()), core.count > 1 {
            if last.lowercased() == "xhigh" || last.lowercased() == "extra" {
                qualifier = "Extra"
            } else {
                qualifier = last.prefix(1).uppercased() + last.dropFirst().lowercased()
            }
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

    private static func normalizeDisplayName(_ name: String) -> String {
        if name.hasSuffix("(XHigh)") {
            return String(name.dropLast("(XHigh)".count)) + "(Extra)"
        }
        return name
    }

    public static func displayName(forID id: String, options: [ModelOption] = []) -> String {
        let key = id.trimmingCharacters(in: .whitespacesAndNewlines)
        if let match = options.first(where: { $0.id.caseInsensitiveCompare(key) == .orderedSame }),
           !match.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return normalizeDisplayName(match.name)
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
        let normalizedName = normalizeDisplayName(name)
        guard let tag = tag?.trimmingCharacters(in: .whitespacesAndNewlines),
              !tag.isEmpty,
              !ignoredPickerTags.contains(tag.lowercased()) else {
            return normalizedName
        }
        return "\(normalizedName) · \(tag)"
    }

    public static func providerKind(forID id: String) -> ModelProviderKind {
        let lower = id.lowercased()
        if isClaude(lower) { return .claude }
        if lower.contains("orch") || orchestratorEnumTokens.contains(where: { lower.contains($0) }) {
            return .orchestrator
        }
        if lower.contains("gpt") { return .other }
        return .gemini
    }

    /// 顶栏芯片紧凑展示名（优先查预设表，再对全名进行自然紧凑格式化）
    public static func compactDisplayName(forID id: String, options: [ModelOption] = []) -> String {
        let key = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !key.isEmpty else { return "" }
        if let known = staticCompactDisplayNameByID[key.lowercased()] {
            return known
        }
        let full = displayName(forID: key, options: options)
        return formatFullNameToCompact(full)
    }

    public static func formatFullNameToCompact(_ full: String) -> String {
        let trimmed = full.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "" }
        if let openParen = trimmed.lastIndex(of: "("),
           trimmed.hasSuffix(")"),
           openParen > trimmed.startIndex {
            var base = String(trimmed[..<openParen]).trimmingCharacters(in: .whitespaces)
            let insideStartIndex = trimmed.index(after: openParen)
            let insideEndIndex = trimmed.index(before: trimmed.endIndex)
            var variant = String(trimmed[insideStartIndex..<insideEndIndex]).trimmingCharacters(in: .whitespaces)
            
            if base.lowercased().hasPrefix("gemini 4 ") {
                base = String(base.dropFirst("gemini 4 ".count))
            } else if base.lowercased().hasPrefix("gemini ") {
                base = String(base.dropFirst("gemini ".count))
            } else if base.lowercased().hasPrefix("claude ") {
                base = String(base.dropFirst("claude ".count))
            } else if base.lowercased().hasPrefix("orchestrator") {
                base = "Orch"
            }
            
            let vLower = variant.lowercased()
            if vLower == "medium" {
                variant = "Med"
            } else if vLower == "thinking" {
                variant = "Think"
            } else if vLower == "xhigh" {
                variant = "Extra"
            }
            return "\(base) · \(variant)"
        }
        
        var base = trimmed
        if base.lowercased().hasPrefix("gemini 4 ") {
            base = String(base.dropFirst("gemini 4 ".count))
        } else if base.lowercased().hasPrefix("gemini ") {
            base = String(base.dropFirst("gemini ".count))
        } else if base.lowercased().hasPrefix("claude ") {
            base = String(base.dropFirst("claude ".count))
        } else if base.lowercased().hasPrefix("orchestrator") {
            base = "Orch"
        }
        return base
    }

    private static let fallbackFamilyOrder: [String] = [
        "gemini-4-carbon-medium", "gemini-4-carbon-high", "gemini-4-carbon-low",
        "gemini-4-argon-medium", "gemini-4-argon-high", "gemini-4-argon-low",
        "gemini-4-hydrogen-xhigh", "gemini-4-hydrogen-high", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-low",
        "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low",
        "claude-opus-5-5-high", "claude-opus-5-5-medium", "claude-opus-5-5-low",
        "claude-sonnet-5-5-high", "claude-sonnet-5-5-medium", "claude-sonnet-5-5-low",
        "claude-opus-4-6-thinking", "claude-sonnet-4-6",
        "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"
    ]

    /// 将扁平模型列表按大类/家族归类（如 Gemini 4 Carbon、Gemini 4 Argon、Claude Opus 5.5、Orchestrator 等），
    /// 用于长按呼出二级选择菜单（上滑选大类、右滑选强度）。
    public static func modelFamilies(from options: [ModelOption]) -> [ModelFamily] {
        let effectiveOptions: [ModelOption]
        if options.isEmpty {
            effectiveOptions = fallbackFamilyOrder.compactMap { key in
                guard let name = staticDisplayNameByID[key] else { return nil }
                return ModelOption(
                    id: key,
                    name: name,
                    provider: providerKind(forID: key).rawValue
                )
            }
        } else {
            effectiveOptions = options
        }
        
        var familyMap: [String: (name: String, provider: String, variants: [ModelVariant])] = [:]
        var familyOrder: [String] = []
        
        for opt in effectiveOptions {
            let fullName = opt.name.isEmpty ? displayName(forID: opt.id, options: options) : normalizeDisplayName(opt.name)
            var familyName = fullName
            var variantLabel = "默认"
            
            if let openParen = fullName.lastIndex(of: "("),
               fullName.hasSuffix(")"),
               openParen > fullName.startIndex {
                familyName = String(fullName[..<openParen]).trimmingCharacters(in: .whitespaces)
                let insideStartIndex = fullName.index(after: openParen)
                let insideEndIndex = fullName.index(before: fullName.endIndex)
                variantLabel = String(fullName[insideStartIndex..<insideEndIndex]).trimmingCharacters(in: .whitespaces)
            }
            
            let p = opt.provider.lowercased()
            let familyProvider: String
            if isClaude(opt.id) || p == "claude" {
                familyProvider = "claude"
            } else if opt.id.lowercased().contains("orch") || familyName.lowercased().contains("orch") {
                familyProvider = "orchestrator"
            } else if p == "gemini" || opt.id.lowercased().contains("gemini") {
                familyProvider = "gemini"
            } else {
                familyProvider = "other"
            }
            
            let variant = ModelVariant(
                id: opt.id,
                label: variantLabel,
                rawName: fullName,
                tag: opt.tag
            )
            
            if familyMap[familyName] == nil {
                familyOrder.append(familyName)
                familyMap[familyName] = (name: familyName, provider: familyProvider, variants: [variant])
            } else {
                familyMap[familyName]?.variants.append(variant)
            }
        }
        
        return familyOrder.compactMap { key in
            guard let entry = familyMap[key] else { return nil }
            return ModelFamily(
                id: key,
                name: entry.name,
                provider: entry.provider,
                variants: entry.variants
            )
        }
    }

    /// 输入框上方胶囊的文字：只有厂商名，保持简洁。
    public static func providerLabel(_ modelId: String) -> String {
        switch providerKind(forID: modelId) {
        case .claude: return "Claude"
        case .orchestrator: return "Orchestrator"
        case .other: return "GPT"
        case .gemini: return "Gemini"
        }
    }

    /// 胶囊点击后要切到的默认模型：Ultra 账号在 Gemini → Claude → Orchestrator 间循环；否则 Gemini ↔ Claude。
    public static func toggleTarget(
        current: String,
        geminiDefault: String,
        claudeDefault: String,
        orchestratorDefault: String = fallbackOrchestrator,
        hasOrchestrator: Bool = false
    ) -> String {
        switch providerKind(forID: current) {
        case .gemini:
            return claudeDefault
        case .claude:
            return hasOrchestrator ? orchestratorDefault : geminiDefault
        case .orchestrator, .other:
            return geminiDefault
        }
    }

    /// 网关推送的 activeModel 如何落到界面状态：
    /// 为空保持当前；已是可读模型 id 则原样保留（让后续消息继续用该会话实际的模型）；
    /// 只拿到未解析的裸枚举（MODEL_...）时优先用静态枚举反查，再退回该厂商的默认模型。
    public static func resolveActive(
        raw: String?,
        current: String,
        geminiDefault: String,
        claudeDefault: String,
        orchestratorDefault: String = fallbackOrchestrator
    ) -> String {
        let value = (raw ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if value.isEmpty { return current }
        if value.uppercased().hasPrefix("MODEL_") {
            if let known = staticIDByEnum[value] {
                return known
            }
            switch providerKind(forID: value) {
            case .claude: return claudeDefault
            case .orchestrator: return orchestratorDefault
            case .gemini, .other: return geminiDefault
            }
        }
        return value
    }

    /// 设置里保存的默认模型已不在实时列表时，换成网关建议的默认值。兜底（非实时）列表不触发重置。
    public static func validated(saved: String, provider: String, catalog: ModelsResponse) -> String {
        guard catalog.isLive else { return saved }
        let p = provider.lowercased()
        let matchesProvider: (ModelOption) -> Bool = { opt in
            let kind = providerKind(forID: opt.id).rawValue
            if kind == "orchestrator" { return p == "orchestrator" }
            return opt.provider.lowercased() == p || kind == p
        }
        if catalog.models.contains(where: { $0.id == saved && matchesProvider($0) }) { return saved }
        if let def = catalog.defaults[provider],
           catalog.models.contains(where: { $0.id == def && matchesProvider($0) }) {
            return def
        }
        return catalog.models.first(where: matchesProvider)?.id ?? saved
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
    private var lastRefreshedAt: Date = .distantPast

    private init() {
        if let data = UserDefaults.standard.data(forKey: Self.storageKey),
           let saved = try? JSONDecoder().decode(ModelsResponse.self, from: data) {
            catalog = saved
        }
    }

    public func options(for provider: String) -> [ModelOption] {
        let p = provider.lowercased()
        return (catalog?.models ?? []).filter { opt in
            let kind = ModelDefaultsLogic.providerKind(forID: opt.id).rawValue
            if kind == "orchestrator" { return p == "orchestrator" }
            return opt.provider.lowercased() == p || kind == p
        }
    }

    /// 当前账号实时模型目录中是否包含 Orchestrator（Ultra 账号专属）。
    public var hasOrchestratorModels: Bool {
        !options(for: "orchestrator").isEmpty
    }

    public func modelFamilies(for provider: String? = nil) -> [ModelFamily] {
        let all = ModelDefaultsLogic.modelFamilies(from: catalog?.models ?? [])
        guard let provider = provider?.lowercased(), !provider.isEmpty else { return all }
        let filtered = all.filter { $0.provider.lowercased() == provider }
        return filtered.isEmpty ? all : filtered
    }

    public var modelFamilies: [ModelFamily] {
        modelFamilies(for: nil)
    }

    public func compactDisplayName(forID id: String) -> String {
        ModelDefaultsLogic.compactDisplayName(forID: id, options: catalog?.models ?? [])
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

    /// 仅在目录缺失或超过 maxAge 时刷新，避免页面切换重复打网关。
    public func refreshIfNeeded(settings: AppSettings, baseURL: URL, maxAge: TimeInterval = 180) async {
        if isLoading { return }
        if let cat = catalog, cat.isLive, Date().timeIntervalSince(lastRefreshedAt) < maxAge {
            return
        }
        await refresh(settings: settings, baseURL: baseURL, force: false)
    }

    /// 从网关刷新目录；成功后同步校验设置里的默认模型与当前激活模型。
    public func refresh(apiClient: APIClient = .shared, settings: AppSettings, baseURL: URL, force: Bool = false) async {
        isLoading = true
        loadFailed = false
        defer { isLoading = false }
        do {
            let fresh = try await apiClient.fetchModels(baseURL: baseURL, refresh: force)
            catalog = fresh
            lastRefreshedAt = Date()
            if let data = try? JSONEncoder().encode(fresh) {
                UserDefaults.standard.set(data, forKey: Self.storageKey)
            }
            let validatedGemini = ModelDefaultsLogic.validated(saved: settings.defaultGeminiModel, provider: "gemini", catalog: fresh)
            let validatedClaude = ModelDefaultsLogic.validated(saved: settings.defaultClaudeModel, provider: "claude", catalog: fresh)
            let validatedOrch = ModelDefaultsLogic.validated(saved: settings.defaultOrchestratorModel, provider: "orchestrator", catalog: fresh)
            settings.defaultGeminiModel = validatedGemini
            settings.defaultClaudeModel = validatedClaude
            settings.defaultOrchestratorModel = validatedOrch
            if fresh.isLive && !fresh.models.contains(where: { $0.id == settings.activeModel }) {
                switch ModelDefaultsLogic.providerKind(forID: settings.activeModel) {
                case .claude: settings.activeModel = validatedClaude
                case .orchestrator: settings.activeModel = hasOrchestratorModels ? validatedOrch : validatedGemini
                case .gemini, .other: settings.activeModel = validatedGemini
                }
            }
        } catch {
            loadFailed = true
        }
    }
}
