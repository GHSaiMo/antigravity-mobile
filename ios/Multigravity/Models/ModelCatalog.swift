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

    public static func parsePure(_ raw: String) -> ModelEffortRank? {
        switch raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
        case "xhigh", "extra", "extra high", "extra-high": return .extra
        case "high": return .high
        case "medium", "med": return .medium
        case "low": return .low
        default: return nil
        }
    }

    public static func parse(_ raw: String) -> ModelEffortRank? {
        if let pure = parsePure(raw) { return pure }
        let lower = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if lower.hasSuffix("-xhigh") || lower.hasSuffix("-extra-high") || lower.hasSuffix("-extra") {
            return .extra
        }
        if lower.hasSuffix("-high") { return .high }
        if lower.hasSuffix("-medium") { return .medium }
        if lower.hasSuffix("-low") { return .low }
        return nil
    }

    /// Splits a trailing effort qualifier from a combined label such as `"Neon, High"`, `"Neon - Extra High"`, or `"Neon Medium"`.
    public static func extractTrailingEffort(from raw: String) -> (base: String, effort: ModelEffortRank)? {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        if let direct = parsePure(trimmed) {
            return ("", direct)
        }
        let lower = trimmed.lowercased()
        let candidates: [(suffix: String, rank: ModelEffortRank)] = [
            ("extra high", .extra),
            ("extra-high", .extra),
            ("xhigh", .extra),
            ("extra", .extra),
            ("medium", .medium),
            ("med", .medium),
            ("high", .high),
            ("low", .low),
        ]
        let separators: Set<Character> = [" ", "\t", ",", "-", "·", "/", "|", ":", "–", "—"]
        for (suffix, rank) in candidates {
            guard lower.hasSuffix(suffix) else { continue }
            let splitIdx = trimmed.index(trimmed.endIndex, offsetBy: -suffix.count)
            if splitIdx == trimmed.startIndex {
                return ("", rank)
            }
            let prevChar = trimmed[trimmed.index(before: splitIdx)]
            guard separators.contains(prevChar) else { continue }
            var baseEnd = splitIdx
            while baseEnd > trimmed.startIndex {
                let c = trimmed[trimmed.index(before: baseEnd)]
                if separators.contains(c) {
                    baseEnd = trimmed.index(before: baseEnd)
                } else {
                    break
                }
            }
            let base = String(trimmed[..<baseEnd]).trimmingCharacters(in: .whitespacesAndNewlines)
            return (base, rank)
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
    /// Matches desktop order: Low (top) -> Medium -> High -> Extra (bottom).
    public var variantsTopToBottom: [ModelVariant] {
        let shouldSortByEffort = hasEffortVariants || (variants.count > 1 && variants.contains(where: { $0.effortRank != nil }))
        guard shouldSortByEffort else { return variants }
        return variants.sorted { lhs, rhs in
            let l = lhs.effortRank?.rawValue ?? Int.max
            let r = rhs.effortRank?.rawValue ?? Int.max
            return l < r
        }
    }

    /// Default variant when selecting the family:
    /// Orchestrator prefers High first (then Medium, Extra, else first);
    /// other families prefer Medium first, then High, else first.
    public var defaultVariant: ModelVariant? {
        let ordered = variantsTopToBottom
        if provider.lowercased() == "orchestrator" {
            if let high = ordered.first(where: { $0.effortRank == .high }) { return high }
            if let med = ordered.first(where: { $0.effortRank == .medium }) { return med }
            if let extra = ordered.first(where: { $0.effortRank == .extra }) { return extra }
            return ordered.first
        }
        if let med = ordered.first(where: { $0.effortRank == .medium }) { return med }
        if let high = ordered.first(where: { $0.effortRank == .high }) { return high }
        return ordered.first
    }

    /// Index of the anchor variant (High for Orchestrator, Medium for others when available) in `variantsTopToBottom` so it aligns horizontally with the family row.
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
    /// 桌面端 Antigravity 客户端当前选中的模型 id 与展示名。
    public var desktopActiveModel: String?
    public var desktopActiveModelName: String?

    public var isLive: Bool { live ?? false }

    public init(
        models: [ModelOption],
        defaults: [String: String],
        live: Bool? = nil,
        desktopActiveModel: String? = nil,
        desktopActiveModelName: String? = nil
    ) {
        self.models = models
        self.defaults = defaults
        self.live = live
        self.desktopActiveModel = desktopActiveModel
        self.desktopActiveModelName = desktopActiveModelName
    }
}

// MARK: - 默认模型与当前会话模型的解析规则（纯函数，便于单测）

public enum ModelDefaultsLogic {
    public static let fallbackGemini = "gemini-4-neon-high"
    public static let fallbackClaude = "claude-opus-4-6-thinking"
    public static let fallbackOrchestrator = "gemini-pro-orchestrator-neon"

    private static let claudeEnumTokens = ["m26", "m35", "m400", "m401", "m402", "m403", "m404", "m405"]
    private static let orchestratorEnumTokens = ["m537", "m667", "m681"]

    public static let staticEnumByID: [String: String] = [
        "gemini-4-neon-xhigh": "MODEL_PLACEHOLDER_M535",
        "gemini-4-neon-high": "MODEL_PLACEHOLDER_M532",
        "gemini-4-neon-medium": "MODEL_PLACEHOLDER_M533",
        "gemini-4-neon-low": "MODEL_PLACEHOLDER_M534",
        "gemini-4-neon": "MODEL_PLACEHOLDER_M532",
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
        "gemini-pro-orchestrator-neon": "MODEL_PLACEHOLDER_M537",
        "gemini-pro-orchestrator": "MODEL_PLACEHOLDER_M667",
        "gemini-pro-orchestrator-argon": "MODEL_PLACEHOLDER_M681",
        "orchestrator-neon": "MODEL_PLACEHOLDER_M537",
        "orchestrator-carbon": "MODEL_PLACEHOLDER_M667",
        "orchestrator-argon": "MODEL_PLACEHOLDER_M681",
        "gemini-flash-worker-agy": "MODEL_PLACEHOLDER_M669",
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
        map["MODEL_PLACEHOLDER_M532"] = "gemini-4-neon-high"
        map["MODEL_PLACEHOLDER_M537"] = "gemini-pro-orchestrator-neon"
        map["MODEL_PLACEHOLDER_M667"] = "gemini-pro-orchestrator"
        map["MODEL_PLACEHOLDER_M681"] = "gemini-pro-orchestrator-argon"
        return map
    }()

    /// 内置/预设模型 ID → 统一展示名映射，避免离线、未拉取目录或切号过渡期露出小写裸 ID。
    public static let staticDisplayNameByID: [String: String] = [
        "gemini-4-neon-xhigh": "Gemini 4 Neon (Extra)",
        "gemini-4-neon-high": "Gemini 4 Neon (High)",
        "gemini-4-neon-medium": "Gemini 4 Neon (Medium)",
        "gemini-4-neon-low": "Gemini 4 Neon (Low)",
        "gemini-4-neon": "Gemini 4 Neon",
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
        "gemini-pro-orchestrator-neon": "Orchestrator (Neon)",
        "orchestrator-neon": "Orchestrator (Neon)",
        "gemini-pro-orchestrator": "Orchestrator (Carbon)",
        "gemini-pro-orchestrator-argon": "Orchestrator (Argon)",
        "orchestrator-carbon": "Orchestrator (Carbon)",
        "orchestrator-argon": "Orchestrator (Argon)",
        "gemini-flash-worker-agy": "Gemini 4 Hydrogen (Orchestrator)",
        "gpt-oss-120b-medium": "GPT-OSS 120B (Medium)",
    ]

    /// 顶栏芯片紧凑展示名映射（如 `Gemini 4 Carbon (High)` → `Carbon · High`）。
    public static let staticCompactDisplayNameByID: [String: String] = [
        "gemini-4-neon-xhigh": "Neon · Extra",
        "gemini-4-neon-high": "Neon · High",
        "gemini-4-neon-medium": "Neon · Med",
        "gemini-4-neon-low": "Neon · Low",
        "gemini-4-neon": "Neon",
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
        "gemini-pro-orchestrator-neon": "Orch Neon",
        "orchestrator-neon": "Orch Neon",
        "gemini-pro-orchestrator": "Orch Carbon",
        "gemini-pro-orchestrator-argon": "Orch Argon",
        "orchestrator-carbon": "Orch Carbon",
        "orchestrator-argon": "Orch Argon",
        "gpt-oss-120b-medium": "GPT-OSS · Med",
    ]

    /// 将未知模型 ID 转为统一规范的展示名（如 `claude-sonnet-5-5-high` → `Claude Sonnet 5.5 (High)`，
    /// `gemini-pro-orchestrator-neon-high` → `Orchestrator (Neon) (High)`）。
    public static func formatIDAsDisplayName(_ id: String) -> String {
        let trimmed = id.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "" }
        if let known = staticDisplayNameByID[trimmed.lowercased()] {
            return known
        }
        let tokens = trimmed.split(separator: "-").map(String.init)
        guard !tokens.isEmpty else { return trimmed }
        var qualifier: String? = nil
        var core = tokens
        if core.count >= 2,
           core[core.count - 2].lowercased() == "extra",
           core[core.count - 1].lowercased() == "high" {
            qualifier = "Extra"
            core.removeLast(2)
        } else {
            let trailingQualifiers: Set<String> = ["high", "medium", "low", "xhigh", "extra", "thinking"]
            if let last = core.last, trailingQualifiers.contains(last.lowercased()), core.count > 1 {
                if last.lowercased() == "xhigh" || last.lowercased() == "extra" {
                    qualifier = "Extra"
                } else {
                    qualifier = last.prefix(1).uppercased() + last.dropFirst().lowercased()
                }
                core.removeLast()
            }
        }
        let coreLower = core.map { $0.lowercased() }
        if coreLower.starts(with: ["gemini", "pro", "orchestrator"]) || coreLower.first == "orchestrator" {
            let isGeminiProOrch = coreLower.starts(with: ["gemini", "pro", "orchestrator"])
            let rest = isGeminiProOrch ? Array(core.dropFirst(3)) : Array(core.dropFirst(1))
            let modelName: String
            if rest.isEmpty {
                modelName = isGeminiProOrch ? "Carbon" : ""
            } else {
                modelName = rest.map { tok in
                    tok.first?.isLetter == true ? tok.prefix(1).uppercased() + tok.dropFirst() : tok
                }.joined(separator: " ")
            }
            let base = modelName.isEmpty ? "Orchestrator" : "Orchestrator (\(modelName))"
            if let q = qualifier {
                return "\(base) (\(q))"
            }
            return base
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
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return name }
        if let regex = try? NSRegularExpression(pattern: #"(\(|[\s,\-·/|:]\s*)(xhigh|extra[\s\-]+high)\)"#, options: [.caseInsensitive]) {
            let range = NSRange(trimmed.startIndex..., in: trimmed)
            return regex.stringByReplacingMatches(in: trimmed, range: range, withTemplate: "$1Extra)")
        }
        if trimmed.hasSuffix("(XHigh)") {
            return String(trimmed.dropLast("(XHigh)".count)) + "(Extra)"
        }
        return trimmed
    }

    private static func cleanOrchestratorModelPart(_ raw: String) -> String {
        var s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimChars = CharacterSet(charactersIn: " -·,/:|–—()")
        s = s.trimmingCharacters(in: trimChars)
        let lower = s.lowercased()
        if lower.hasPrefix("gemini 4 ") {
            s = String(s.dropFirst("gemini 4 ".count))
        } else if lower.hasPrefix("gemini ") {
            s = String(s.dropFirst("gemini ".count))
        } else if lower.hasPrefix("claude ") {
            s = String(s.dropFirst("claude ".count))
        }
        return s.trimmingCharacters(in: trimChars)
    }

    /// 解析任意格式的 Orchestrator 展示名与 ID，拆出底层模型名（如 `Neon` / `Carbon` / `Argon`）和可选思考强度。
    /// 支持：`Orchestrator (Neon) (High)`、`Orchestrator Neon (High)`、`Orchestrator (Neon, High)`、
    /// `Orchestrator (Neon - Extra High)`、`Orchestrator (Neon High)`、`Gemini 4 Neon Orchestrator (High)`、`Orchestrator (Neon)` 等。
    public static func parseOrchestratorName(_ fullName: String, id: String = "") -> (modelName: String, effort: ModelEffortRank?) {
        let normalized = normalizeDisplayName(fullName).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalized.isEmpty else {
            return ("", ModelEffortRank.parse(id))
        }
        let stripOrchPrefixOrSuffix: (String) -> String = { text in
            var s = text.trimmingCharacters(in: .whitespacesAndNewlines)
            if s.lowercased().hasPrefix("orchestrator") {
                s = String(s.dropFirst("orchestrator".count)).trimmingCharacters(in: .whitespacesAndNewlines)
            }
            if s.lowercased().hasSuffix("(orchestrator)") {
                s = String(s.dropLast("(orchestrator)".count)).trimmingCharacters(in: .whitespacesAndNewlines)
            } else if s.lowercased().hasSuffix("orchestrator") {
                s = String(s.dropLast("orchestrator".count)).trimmingCharacters(in: .whitespacesAndNewlines)
            }
            return cleanOrchestratorModelPart(s)
        }

        if let openParen = normalized.lastIndex(of: "("),
           normalized.hasSuffix(")"),
           openParen > normalized.startIndex {
            let prefix = String(normalized[..<openParen]).trimmingCharacters(in: .whitespacesAndNewlines)
            let inside = String(normalized[normalized.index(after: openParen)..<normalized.index(before: normalized.endIndex)])
                .trimmingCharacters(in: .whitespacesAndNewlines)

            if let pureEffort = ModelEffortRank.parsePure(inside) {
                // e.g. "Orchestrator (Neon) (High)", "Orchestrator Neon (High)", "Orchestrator (High)", "Gemini 4 Neon Orchestrator (High)"
                if let pOpen = prefix.lastIndex(of: "("), prefix.hasSuffix(")"), pOpen > prefix.startIndex {
                    let pBefore = String(prefix[..<pOpen]).trimmingCharacters(in: .whitespacesAndNewlines)
                    let pInner = String(prefix[prefix.index(after: pOpen)..<prefix.index(before: prefix.endIndex)])
                        .trimmingCharacters(in: .whitespacesAndNewlines)
                    if pBefore.lowercased().hasPrefix("orchestrator") {
                        return (cleanOrchestratorModelPart(pInner), pureEffort)
                    }
                    if pInner.lowercased() == "orchestrator" {
                        return (cleanOrchestratorModelPart(pBefore), pureEffort)
                    }
                }
                return (stripOrchPrefixOrSuffix(prefix), pureEffort)
            }

            if let split = ModelEffortRank.extractTrailingEffort(from: inside) {
                // e.g. "Orchestrator (Neon, High)", "Orchestrator (Neon - Extra High)", "Orchestrator (Neon High)", "Gemini 4 Neon (Orchestrator, High)"
                if prefix.lowercased().hasPrefix("orchestrator") {
                    return (cleanOrchestratorModelPart(split.base), split.effort)
                }
                if split.base.lowercased() == "orchestrator" {
                    return (cleanOrchestratorModelPart(prefix), split.effort)
                }
                return (cleanOrchestratorModelPart(split.base), split.effort)
            }

            // No effort inside parentheses, e.g. "Orchestrator (Neon)" or "Gemini 4 Hydrogen (Orchestrator)"
            let idEffort = ModelEffortRank.parse(id)
            if prefix.lowercased().hasPrefix("orchestrator") {
                return (cleanOrchestratorModelPart(inside), idEffort)
            }
            if inside.lowercased() == "orchestrator" {
                return (cleanOrchestratorModelPart(prefix), idEffort)
            }
            return (stripOrchPrefixOrSuffix(normalized), idEffort)
        }

        if let split = ModelEffortRank.extractTrailingEffort(from: normalized) {
            return (stripOrchPrefixOrSuffix(split.base), split.effort)
        }
        return (stripOrchPrefixOrSuffix(normalized), ModelEffortRank.parse(id))
    }

    /// Orchestrator 一级菜单展示名：`Orch <模型名>`（无模型名时退回 `Orch`）。
    public static func formatOrchestratorSlidePickerTitle(_ rawTitle: String) -> String {
        let parsed = parseOrchestratorName(rawTitle)
        return parsed.modelName.isEmpty ? "Orch" : "Orch \(parsed.modelName)"
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
        let trimmed = normalizeDisplayName(full).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "" }
        if trimmed.lowercased().hasPrefix("orchestrator") {
            let parsed = parseOrchestratorName(trimmed)
            let base = parsed.modelName.isEmpty ? "Orch" : "Orch \(parsed.modelName)"
            if let effort = parsed.effort {
                let shortEffort = effort == .medium ? "Med" : effort.displayLabel
                return "\(base) · \(shortEffort)"
            }
            return base
        }
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
            } else if vLower == "xhigh" || vLower == "extra high" || vLower == "extra-high" {
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
        "gemini-4-neon-high", "gemini-4-neon-xhigh", "gemini-4-neon-medium", "gemini-4-neon-low",
        "gemini-4-carbon-medium", "gemini-4-carbon-high", "gemini-4-carbon-low",
        "gemini-4-argon-medium", "gemini-4-argon-high", "gemini-4-argon-low",
        "gemini-4-hydrogen-xhigh", "gemini-4-hydrogen-high", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-low",
        "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low",
        "claude-opus-5-5-high", "claude-opus-5-5-medium", "claude-opus-5-5-low",
        "claude-sonnet-5-5-high", "claude-sonnet-5-5-medium", "claude-sonnet-5-5-low",
        "claude-opus-4-6-thinking", "claude-sonnet-4-6",
        "gemini-pro-orchestrator-neon", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"
    ]

    public static var fallbackOptions: [ModelOption] {
        fallbackFamilyOrder.compactMap { key in
            guard let name = staticDisplayNameByID[key] else { return nil }
            return ModelOption(
                id: key,
                name: name,
                provider: providerKind(forID: key).rawValue
            )
        }
    }

    private static func providerSortRank(_ provider: String) -> Int {
        switch provider.lowercased() {
        case "gemini": return 0
        case "claude": return 1
        case "orchestrator": return 2
        default: return 3
        }
    }

    public static func isExcludedPickerModelID(_ id: String) -> Bool {
        let lower = id.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return lower.contains("gpt-oss") || lower.hasSuffix("-disabled")
    }

    /// 将扁平模型列表按大类/家族归类（如 Gemini 4 Carbon、Gemini 4 Argon、Claude Opus 5.5、Orchestrator 等），
    /// 并按 Gemini (蓝) -> Claude (橙) -> Orchestrator (钛灰) -> Other 的厂商顺序排列，
    /// 用于长按呼出二级选择菜单（上下选模型、右滑选强度）。
    public static func modelFamilies(from options: [ModelOption]) -> [ModelFamily] {
        let filteredOptions = options.filter { !isExcludedPickerModelID($0.id) }
        let effectiveOptions: [ModelOption] = filteredOptions.isEmpty ? fallbackOptions : filteredOptions
        
        var familyMap: [String: (name: String, provider: String, variants: [ModelVariant])] = [:]
        var familyOrder: [String] = []
        
        for opt in effectiveOptions {
            if isExcludedPickerModelID(opt.id) { continue }
            let fullName = opt.name.isEmpty ? displayName(forID: opt.id, options: options) : normalizeDisplayName(opt.name)
            let p = opt.provider.lowercased()
            let familyProvider: String
            if isClaude(opt.id) || p == "claude" {
                familyProvider = "claude"
            } else if p == "orchestrator"
                || providerKind(forID: opt.id) == .orchestrator
                || fullName.lowercased().hasPrefix("orchestrator") {
                familyProvider = "orchestrator"
            } else if p == "gemini" || opt.id.lowercased().contains("gemini") {
                familyProvider = "gemini"
            } else {
                familyProvider = "other"
            }

            var familyName = fullName
            var variantLabel = String(localized: "默认")
            if familyProvider == "orchestrator" {
                let parsed = parseOrchestratorName(fullName, id: opt.id)
                familyName = parsed.modelName.isEmpty ? "Orchestrator" : "Orchestrator (\(parsed.modelName))"
                if let effort = parsed.effort {
                    variantLabel = effort.displayLabel
                }
            } else if let openParen = fullName.lastIndex(of: "("),
                      fullName.hasSuffix(")"),
                      openParen > fullName.startIndex {
                familyName = String(fullName[..<openParen]).trimmingCharacters(in: .whitespaces)
                let insideStartIndex = fullName.index(after: openParen)
                let insideEndIndex = fullName.index(before: fullName.endIndex)
                variantLabel = String(fullName[insideStartIndex..<insideEndIndex]).trimmingCharacters(in: .whitespaces)
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
        
        let ordered = familyOrder.compactMap { key -> ModelFamily? in
            guard let entry = familyMap[key] else { return nil }
            return ModelFamily(
                id: key,
                name: entry.name,
                provider: entry.provider,
                variants: entry.variants
            )
        }
        // 稳定分组排序：Gemini -> Claude -> Orchestrator -> Other，同厂商内保持目录原有顺序
        return ordered.enumerated().sorted { lhs, rhs in
            let lr = providerSortRank(lhs.element.provider)
            let rr = providerSortRank(rhs.element.provider)
            if lr != rr { return lr < rr }
            return lhs.offset < rhs.offset
        }.map { $0.element }
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

    /// 单击胶囊在两个常用模型之间快速切换：
    /// 当前是常用模型 1 -> 切到常用模型 2；
    /// 当前是常用模型 2 或通过长按选中的其他模型 -> 切回常用模型 1。
    public static func toggleFavoriteTarget(
        current: String,
        favorite1: String,
        favorite2: String
    ) -> String {
        let f1 = favorite1.trimmingCharacters(in: .whitespacesAndNewlines)
        let f2 = favorite2.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !f1.isEmpty else { return f2.isEmpty ? current : f2 }
        guard !f2.isEmpty else { return f1 }
        if current.caseInsensitiveCompare(f1) == .orderedSame {
            return f2
        }
        return f1
    }

    /// 校验并修正常用模型：若已保存的模型在实时目录中下线/不可用，则自动替换为可用模型（尽量避开 `avoid`）。
    public static func validatedFavorite(
        saved: String,
        fallback: String,
        avoid: String? = nil,
        catalog: ModelsResponse
    ) -> String {
        guard catalog.isLive, !catalog.models.isEmpty else { return saved }
        let selectable = catalog.models.filter { !isExcludedPickerModelID($0.id) }
        guard !selectable.isEmpty else { return saved }
        if selectable.contains(where: { $0.id == saved }) {
            return saved
        }
        let isDifferentFromAvoid: (String) -> Bool = { id in
            guard let avoid, !avoid.isEmpty else { return true }
            return id.caseInsensitiveCompare(avoid) != .orderedSame
        }
        let savedProvider = providerKind(forID: saved).rawValue
        if let def = catalog.defaults[savedProvider],
           selectable.contains(where: { $0.id == def }),
           isDifferentFromAvoid(def) {
            return def
        }
        if selectable.contains(where: { $0.id == fallback }), isDifferentFromAvoid(fallback) {
            return fallback
        }
        if let diff = selectable.first(where: { isDifferentFromAvoid($0.id) })?.id {
            return diff
        }
        return selectable.first?.id ?? saved
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
    private var lastRefreshedBaseURL: String? = nil

    private init() {
        if let data = UserDefaults.standard.data(forKey: Self.storageKey),
           let saved = try? JSONDecoder().decode(ModelsResponse.self, from: data) {
            catalog = saved
        }
    }

    /// 所有可用模型（实时/缓存目录优先，离线时退回内置常用列表），按 Gemini -> Claude -> Orchestrator -> Other 排序。
    public var allOptions: [ModelOption] {
        let raw = (catalog?.models ?? []).filter { !ModelDefaultsLogic.isExcludedPickerModelID($0.id) }
        let list = raw.isEmpty ? ModelDefaultsLogic.fallbackOptions : raw
        let rank: (String) -> Int = { p in
            switch p.lowercased() {
            case "gemini": return 0
            case "claude": return 1
            case "orchestrator": return 2
            default: return 3
            }
        }
        return list.enumerated().sorted { lhs, rhs in
            let lk = ModelDefaultsLogic.providerKind(forID: lhs.element.id).rawValue
            let rk = ModelDefaultsLogic.providerKind(forID: rhs.element.id).rawValue
            let lp = (lk == "orchestrator" || lhs.element.provider.isEmpty) ? lk : lhs.element.provider
            let rp = (rk == "orchestrator" || rhs.element.provider.isEmpty) ? rk : rhs.element.provider
            let lr = rank(lp)
            let rr = rank(rp)
            if lr != rr { return lr < rr }
            return lhs.offset < rhs.offset
        }.map { $0.element }
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

    /// 用来自会话接口/流推送的最新桌面端选中模型同步本地目录缓存，避免跨会话切换读到旧缓存。
    public func updateDesktopActiveModel(_ model: String?, name: String? = nil) {
        guard let model, !model.isEmpty else { return }
        let normalizedName = (name?.isEmpty == false) ? name : nil
        if var existing = catalog {
            if existing.desktopActiveModel == model &&
                (normalizedName == nil || existing.desktopActiveModelName == normalizedName) {
                return
            }
            existing.desktopActiveModel = model
            if let normalizedName {
                existing.desktopActiveModelName = normalizedName
            }
            catalog = existing
            if let data = try? JSONEncoder().encode(existing) {
                UserDefaults.standard.set(data, forKey: Self.storageKey)
            }
        } else {
            let seeded = ModelsResponse(
                models: ModelDefaultsLogic.fallbackOptions,
                defaults: [:],
                live: false,
                desktopActiveModel: model,
                desktopActiveModelName: normalizedName
            )
            catalog = seeded
            if let data = try? JSONEncoder().encode(seeded) {
                UserDefaults.standard.set(data, forKey: Self.storageKey)
            }
        }
    }

    /// 仅在目录缺失、宿主机地址变更或超过 maxAge 时刷新，避免页面切换重复打网关。
    public func refreshIfNeeded(settings: AppSettings, baseURL: URL, maxAge: TimeInterval = 180) async {
        if isLoading { return }
        let sameHost = lastRefreshedBaseURL == baseURL.absoluteString
        if sameHost, let cat = catalog, cat.isLive, Date().timeIntervalSince(lastRefreshedAt) < maxAge {
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
            lastRefreshedBaseURL = baseURL.absoluteString
            if let data = try? JSONEncoder().encode(fresh) {
                UserDefaults.standard.set(data, forKey: Self.storageKey)
            }
            let validatedFav1 = ModelDefaultsLogic.validatedFavorite(
                saved: settings.favoriteModel1,
                fallback: ModelDefaultsLogic.fallbackGemini,
                avoid: nil,
                catalog: fresh
            )
            let validatedFav2 = ModelDefaultsLogic.validatedFavorite(
                saved: settings.favoriteModel2,
                fallback: ModelDefaultsLogic.fallbackClaude,
                avoid: validatedFav1,
                catalog: fresh
            )
            settings.favoriteModel1 = validatedFav1
            settings.favoriteModel2 = validatedFav2
            if fresh.isLive && !fresh.models.contains(where: { $0.id == settings.activeModel }) {
                settings.activeModel = validatedFav1
            }
        } catch {
            loadFailed = true
        }
    }
}
