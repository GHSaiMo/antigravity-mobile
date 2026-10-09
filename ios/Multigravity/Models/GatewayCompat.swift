import Foundation
import Observation

/// 网关用这些 ID 标记「依赖的 language_server 接口缺失」的功能；改名需与网关 capabilities.go 同步。
public enum GatewayFeature {
    public static let search = "search"
    public static let export = "export"
    public static let changes = "changes"
    public static let revert = "revert"
    public static let slash = "slash"
}

/// 升级自检结果（/gateway/status 的 compat 字段）。
///
/// 原则：拿不到、解析不了、网关版本旧没有这个字段、网关无法检测（checked=false）时一律「放行」，
/// 宁可保留入口、出错时再报错，也不误隐藏可用功能。
public struct GatewayCompat: Codable, Sendable, Equatable {
    public var version: String?
    public var checked: Bool
    public var coreOk: Bool
    public var unavailable: [String]
    public var missingRpcs: [String]

    public init(version: String? = nil, checked: Bool = false, coreOk: Bool = true, unavailable: [String] = [], missingRpcs: [String] = []) {
        self.version = version
        self.checked = checked
        self.coreOk = coreOk
        self.unavailable = unavailable
        self.missingRpcs = missingRpcs
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = try c.decodeIfPresent(String.self, forKey: .version)
        checked = try c.decodeIfPresent(Bool.self, forKey: .checked) ?? false
        coreOk = try c.decodeIfPresent(Bool.self, forKey: .coreOk) ?? true
        unavailable = try c.decodeIfPresent([String].self, forKey: .unavailable) ?? []
        missingRpcs = try c.decodeIfPresent([String].self, forKey: .missingRpcs) ?? []
    }

    public func isAvailable(_ featureId: String) -> Bool {
        !(checked && unavailable.contains(featureId))
    }

    /// 基础能力（会话列表/读取/发送/流式/审批）缺失：整个 App 基本不可用，需要升级网关。
    public var isIncompatible: Bool { checked && !coreOk }

    /// 首页顶部提示条文案。
    public var bannerText: String {
        let ver = (version?.isEmpty == false) ? "（\(version!)）" : ""
        return "当前 Antigravity 版本\(ver) 与网关不兼容，部分基础功能可能无法使用。请升级网关（mgy）。"
    }

    /// 从 /gateway/status 响应体取 compat；缺失或格式不对返回放行的默认值。
    public static func fromStatusData(_ data: Data) -> GatewayCompat {
        struct Envelope: Decodable { let compat: GatewayCompat? }
        return (try? JSONDecoder().decode(Envelope.self, from: data))?.compat ?? GatewayCompat()
    }
}

/// 全应用共享的最新自检结果；首页刷新会话列表时更新，其余界面只读。
@MainActor
@Observable
public final class GatewayCompatStore {
    public static let shared = GatewayCompatStore()
    public private(set) var compat = GatewayCompat()
    private init() {}

    public func update(_ value: GatewayCompat) {
        if compat != value { compat = value }
    }
    public func isAvailable(_ featureId: String) -> Bool { compat.isAvailable(featureId) }
}
