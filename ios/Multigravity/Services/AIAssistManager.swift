import Foundation
import Observation

public struct AIAssistPrefsPayload: Codable, Equatable, Sendable {
    public var titleTranslateEnabled: Bool
    public var titleTranslateLang: String?
    public var titleTranslateLangName: String?
    public var stepPlanTranslateEnabled: Bool
    public var focusTranslateEnabled: Bool
    public var nextPromptEnabled: Bool
    public var nextPromptTapAction: String

    public static let defaultValue = AIAssistPrefsPayload(
        titleTranslateEnabled: false,
        titleTranslateLang: nil,
        titleTranslateLangName: nil,
        stepPlanTranslateEnabled: false,
        focusTranslateEnabled: false,
        nextPromptEnabled: false,
        nextPromptTapAction: "fill"
    )

    public init(
        titleTranslateEnabled: Bool = false,
        titleTranslateLang: String? = nil,
        titleTranslateLangName: String? = nil,
        stepPlanTranslateEnabled: Bool = false,
        focusTranslateEnabled: Bool = false,
        nextPromptEnabled: Bool = false,
        nextPromptTapAction: String = "fill"
    ) {
        self.titleTranslateEnabled = titleTranslateEnabled
        self.titleTranslateLang = titleTranslateLang
        self.titleTranslateLangName = titleTranslateLangName
        self.stepPlanTranslateEnabled = stepPlanTranslateEnabled
        self.focusTranslateEnabled = focusTranslateEnabled
        self.nextPromptEnabled = nextPromptEnabled
        self.nextPromptTapAction = nextPromptTapAction
    }

    enum CodingKeys: String, CodingKey {
        case titleTranslateEnabled, titleTranslateLang, titleTranslateLangName
        case stepPlanTranslateEnabled, focusTranslateEnabled
        case nextPromptEnabled, nextPromptTapAction
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.titleTranslateEnabled = try c.decodeIfPresent(Bool.self, forKey: .titleTranslateEnabled) ?? false
        self.titleTranslateLang = try c.decodeIfPresent(String.self, forKey: .titleTranslateLang)
        self.titleTranslateLangName = try c.decodeIfPresent(String.self, forKey: .titleTranslateLangName)
        self.stepPlanTranslateEnabled = try c.decodeIfPresent(Bool.self, forKey: .stepPlanTranslateEnabled) ?? false
        self.focusTranslateEnabled = try c.decodeIfPresent(Bool.self, forKey: .focusTranslateEnabled) ?? false
        self.nextPromptEnabled = try c.decodeIfPresent(Bool.self, forKey: .nextPromptEnabled) ?? false
        self.nextPromptTapAction = try c.decodeIfPresent(String.self, forKey: .nextPromptTapAction) ?? "fill"
    }
}

public struct AIAssistBatchStatusPayload: Codable, Equatable, Sendable {
    public let running: Bool
    public let scanned: Int
    public let translated: Int
    public let skipped: Int
    public let failed: Int
    public let targetLang: String?
    public let targetLangName: String?
    public let finishedAt: String?
    public let error: String?
}

public struct AIAssistStatusPayload: Codable, Equatable, Sendable {
    public let prefs: AIAssistPrefsPayload
    public let modelId: String?
    public let modelName: String?
    public let resolvedLang: String?
    public let resolvedLangName: String?
    public let translatedCount: Int?
    public let batchStatus: AIAssistBatchStatusPayload?
    public let available: Bool?
}

public struct AIAssistTranslateResultPayload: Codable, Sendable {
    public let ok: Bool?
    public let alreadyRunning: Bool?
    public let scanned: Int?
    public let translated: Int?
    public let skipped: Int?
    public let failed: Int?
    public let targetLang: String?
    public let targetLangName: String?
    public let translatedCount: Int?
    public let error: String?
}

public struct AIAssistRestoreResultPayload: Codable, Sendable {
    public let ok: Bool?
    public let restored: Int?
    public let failed: Int?
    public let translatedCount: Int?
}

/// Resolves the device's true preferred OS/app language for adaptive Title Translation UX:
/// - English (`en*`): hidden unless gateway already has title translation enabled.
/// - Chinese (`zh-Hans` / `zh-Hant`) & Japanese (`ja`): native localized menu.
/// - All other non-English languages (e.g. `pt-BR`, `ko`, `es`, `fr`): English menu dynamically naming the user's language.
public struct DeviceTitleTargetLanguage: Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case english
        case simplifiedChinese
        case traditionalChinese
        case japanese
        case otherNonEnglish
    }

    public let kind: Kind
    public let bcp47Tag: String
    public let englishDisplayName: String

    public var isEnglish: Bool { kind == .english }

    public static func current() -> DeviceTitleTargetLanguage {
        let raw = (Locale.preferredLanguages.first ?? Locale.current.identifier)
            .replacingOccurrences(of: "_", with: "-")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let lower = raw.lowercased()

        if lower == "en" || lower.hasPrefix("en-") {
            return DeviceTitleTargetLanguage(kind: .english, bcp47Tag: "en", englishDisplayName: "English")
        }
        if lower.hasPrefix("zh-hant") || lower.hasPrefix("zh-tw") || lower.hasPrefix("zh-hk") || lower.hasPrefix("zh-mo") {
            return DeviceTitleTargetLanguage(kind: .traditionalChinese, bcp47Tag: "zh-Hant", englishDisplayName: "Traditional Chinese")
        }
        if lower == "zh" || lower.hasPrefix("zh-") {
            return DeviceTitleTargetLanguage(kind: .simplifiedChinese, bcp47Tag: "zh-Hans", englishDisplayName: "Simplified Chinese")
        }
        if lower == "ja" || lower.hasPrefix("ja-") {
            return DeviceTitleTargetLanguage(kind: .japanese, bcp47Tag: "ja", englishDisplayName: "Japanese")
        }

        let baseCode = lower.split(separator: "-").first.map(String.init) ?? lower
        let englishLocale = Locale(identifier: "en")
        let displayName = englishLocale.localizedString(forLanguageCode: baseCode)
            ?? englishLocale.localizedString(forIdentifier: raw)
            ?? raw
        let cleaned = displayName.isEmpty ? raw : (displayName.prefix(1).uppercased() + displayName.dropFirst())
        return DeviceTitleTargetLanguage(kind: .otherNonEnglish, bcp47Tag: raw.isEmpty ? baseCode : raw, englishDisplayName: cleaned)
    }

    public func shouldShowTitleTranslationSection(prefs: AIAssistPrefsPayload) -> Bool {
        !isEnglish || prefs.titleTranslateEnabled
    }

    public func shouldShowLiveTranslationSection(prefs: AIAssistPrefsPayload) -> Bool {
        !isEnglish || prefs.stepPlanTranslateEnabled || prefs.focusTranslateEnabled
    }
}

public extension Notification.Name {
    static let aiAssistTitlesUpdated = Notification.Name("MultigravityAIAssistTitlesUpdated")
}

@Observable
@MainActor
public final class AIAssistManager {
    public static let shared = AIAssistManager()

    public private(set) var prefs: AIAssistPrefsPayload = .defaultValue
    public private(set) var modelName: String = "Gemini 3.5 Flash Lite"
    public private(set) var resolvedLangName: String = ""
    public private(set) var translatedCount: Int = 0
    public private(set) var isAvailable: Bool = true
    public private(set) var isLoading: Bool = false
    public private(set) var isTranslatingBatch: Bool = false
    public private(set) var isRestoringTitles: Bool = false
    public private(set) var syncError: String? = nil
    public private(set) var batchFeedback: String? = nil
    public private(set) var batchFeedbackIsError: Bool = false

    private var lastFetchedAt: Date? = nil

    private init() {}

    public func ensureFetched() async {
        if let last = lastFetchedAt, Date().timeIntervalSince(last) < 300 {
            return
        }
        await fetchStatus()
    }

    public var deviceLanguage: DeviceTitleTargetLanguage {
        DeviceTitleTargetLanguage.current()
    }

    public var shouldShowTitleTranslation: Bool {
        deviceLanguage.shouldShowTitleTranslationSection(prefs: prefs)
    }

    public var shouldShowLiveTranslation: Bool {
        deviceLanguage.shouldShowLiveTranslationSection(prefs: prefs)
    }

    public var summaryText: String {
        let titleOn = shouldShowTitleTranslation && prefs.titleTranslateEnabled
        let stepOn = shouldShowLiveTranslation && prefs.stepPlanTranslateEnabled
        let focusOn = shouldShowLiveTranslation && prefs.focusTranslateEnabled
        let nextOn = prefs.nextPromptEnabled
        let count = [titleOn, stepOn, focusOn, nextOn].filter { $0 }.count
        if count >= 2 {
            return String(localized: "已开启 \(count) 项")
        }
        if titleOn {
            return String(localized: "标题翻译")
        }
        if stepOn {
            return String(localized: "步骤翻译")
        }
        if focusOn {
            return String(localized: "状态翻译")
        }
        if nextOn {
            return String(localized: "提示词建议")
        }
        return String(localized: "已关闭")
    }

    private func makeRequest(path: String, method: String, timeout: TimeInterval = 10.0) -> URLRequest? {
        let settings = AppSettings.shared
        guard settings.isPaired, !settings.isDemoMode, let baseURL = settings.serverURL else { return nil }
        var req = URLRequest(url: baseURL.appendingPathComponent(path))
        req.httpMethod = method
        req.timeoutInterval = timeout
        if let authTok = KeychainHelper.shared.read(key: .deviceToken) ?? settings.deviceToken, !authTok.isEmpty {
            req.setValue("Bearer \(authTok)", forHTTPHeaderField: "Authorization")
            req.setValue(authTok, forHTTPHeaderField: "x-device-token")
        }
        return req
    }

    private func applyStatus(_ status: AIAssistStatusPayload) {
        self.lastFetchedAt = Date()
        self.prefs = status.prefs
        if let name = status.modelName, !name.isEmpty {
            self.modelName = name
        }
        self.resolvedLangName = status.resolvedLangName ?? ""
        self.translatedCount = status.translatedCount ?? 0
        self.isAvailable = status.available ?? true
    }

    public func fetchStatus() async {
        guard let req = makeRequest(path: "gateway/ai-assist/prefs", method: "GET", timeout: 8.0) else { return }
        isLoading = true
        syncError = nil
        defer { isLoading = false }
        do {
            let (data, resp) = try await NetworkTransport.shared.send(request: req)
            if let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode),
               let decoded = try? JSONDecoder().decode(AIAssistStatusPayload.self, from: data) {
                applyStatus(decoded)
            }
        } catch {
            // Older gateways without /gateway/ai-assist/prefs keep defaults silently.
        }
    }

    public func setTitleTranslateEnabled(_ enabled: Bool) async {
        let prev = prefs
        let dev = deviceLanguage
        var body: [String: Any] = ["titleTranslateEnabled": enabled]
        var next = prefs
        next.titleTranslateEnabled = enabled
        if enabled && !dev.isEnglish {
            body["titleTranslateLang"] = dev.bcp47Tag
            body["titleTranslateLangName"] = dev.englishDisplayName
            next.titleTranslateLang = dev.bcp47Tag
            next.titleTranslateLangName = dev.englishDisplayName
        }
        await patchPrefs(optimistic: next, previous: prev, body: body)
    }

    public func setStepPlanTranslateEnabled(_ enabled: Bool) async {
        let prev = prefs
        let dev = deviceLanguage
        var body: [String: Any] = ["stepPlanTranslateEnabled": enabled]
        var next = prefs
        next.stepPlanTranslateEnabled = enabled
        if enabled && !dev.isEnglish {
            body["titleTranslateLang"] = dev.bcp47Tag
            body["titleTranslateLangName"] = dev.englishDisplayName
            next.titleTranslateLang = dev.bcp47Tag
            next.titleTranslateLangName = dev.englishDisplayName
        }
        await patchPrefs(optimistic: next, previous: prev, body: body)
    }

    public func setFocusTranslateEnabled(_ enabled: Bool) async {
        let prev = prefs
        let dev = deviceLanguage
        var body: [String: Any] = ["focusTranslateEnabled": enabled]
        var next = prefs
        next.focusTranslateEnabled = enabled
        if enabled && !dev.isEnglish {
            body["titleTranslateLang"] = dev.bcp47Tag
            body["titleTranslateLangName"] = dev.englishDisplayName
            next.titleTranslateLang = dev.bcp47Tag
            next.titleTranslateLangName = dev.englishDisplayName
        }
        await patchPrefs(optimistic: next, previous: prev, body: body)
    }

    public func setNextPromptEnabled(_ enabled: Bool) async {
        let prev = prefs
        var next = prefs
        next.nextPromptEnabled = enabled
        await patchPrefs(optimistic: next, previous: prev, body: ["nextPromptEnabled": enabled])
    }

    public func setNextPromptTapAction(_ action: String) async {
        let normalized = (action == "send") ? "send" : "fill"
        let prev = prefs
        var next = prefs
        next.nextPromptTapAction = normalized
        await patchPrefs(optimistic: next, previous: prev, body: ["nextPromptTapAction": normalized])
    }

    private func patchPrefs(optimistic: AIAssistPrefsPayload, previous: AIAssistPrefsPayload, body: [String: Any]) async {
        self.prefs = optimistic
        self.syncError = nil

        guard var req = makeRequest(path: "gateway/ai-assist/prefs", method: "POST", timeout: 8.0) else { return }
        guard let data = try? JSONSerialization.data(withJSONObject: body) else { return }
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = data

        do {
            let (respData, resp) = try await NetworkTransport.shared.send(request: req)
            if let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode),
               let decoded = try? JSONDecoder().decode(AIAssistStatusPayload.self, from: respData) {
                applyStatus(decoded)
            } else {
                self.prefs = previous
                self.syncError = String(localized: "同步失败，请稍后重试")
            }
        } catch {
            self.prefs = previous
            self.syncError = String(localized: "同步失败，请检查网关连接")
        }
    }

    public func translateExistingTitles() async {
        guard !isTranslatingBatch else { return }
        let dev = deviceLanguage
        let targetLang = dev.isEnglish ? (prefs.titleTranslateLang ?? "") : dev.bcp47Tag
        let targetName = dev.isEnglish ? (prefs.titleTranslateLangName ?? resolvedLangName) : dev.englishDisplayName
        guard !targetLang.isEmpty, !targetLang.lowercased().hasPrefix("en") else { return }

        guard var req = makeRequest(path: "gateway/ai-assist/translate-titles", method: "POST", timeout: 60.0) else { return }
        let payload: [String: Any] = [
            "targetLang": targetLang,
            "targetLangName": targetName,
            "includeArchived": false
        ]
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try? JSONSerialization.data(withJSONObject: payload)

        isTranslatingBatch = true
        batchFeedback = nil
        batchFeedbackIsError = false
        defer { isTranslatingBatch = false }

        do {
            let (data, resp) = try await NetworkTransport.shared.send(request: req)
            if let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode),
               let decoded = try? JSONDecoder().decode(AIAssistTranslateResultPayload.self, from: data) {
                if let cnt = decoded.translatedCount {
                    self.translatedCount = cnt
                }
                if decoded.alreadyRunning == true {
                    self.batchFeedback = localizedBatchRunningText(dev: dev)
                } else {
                    let tr = decoded.translated ?? 0
                    let sk = decoded.skipped ?? 0
                    let fl = decoded.failed ?? 0
                    self.batchFeedback = localizedTranslateResultText(dev: dev, translated: tr, skipped: sk, failed: fl)
                    self.batchFeedbackIsError = (tr == 0 && fl > 0)
                    if tr > 0 {
                        NotificationCenter.default.post(name: .aiAssistTitlesUpdated, object: nil)
                    }
                }
            } else {
                self.batchFeedback = String(localized: "翻译失败，请确认网关已连接 Antigravity")
                self.batchFeedbackIsError = true
            }
        } catch {
            self.batchFeedback = String(localized: "翻译请求超时或连接中断")
            self.batchFeedbackIsError = true
        }
    }

    public func restoreOriginalTitles() async {
        guard !isRestoringTitles else { return }
        guard let req = makeRequest(path: "gateway/ai-assist/restore-titles", method: "POST", timeout: 30.0) else { return }

        isRestoringTitles = true
        batchFeedback = nil
        batchFeedbackIsError = false
        defer { isRestoringTitles = false }

        do {
            let (data, resp) = try await NetworkTransport.shared.send(request: req)
            if let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode),
               let decoded = try? JSONDecoder().decode(AIAssistRestoreResultPayload.self, from: data) {
                let restored = decoded.restored ?? 0
                if let cnt = decoded.translatedCount {
                    self.translatedCount = cnt
                }
                self.batchFeedback = localizedRestoreResultText(dev: deviceLanguage, restored: restored)
                if restored > 0 {
                    NotificationCenter.default.post(name: .aiAssistTitlesUpdated, object: nil)
                }
            } else {
                self.batchFeedback = String(localized: "还原失败，请稍后重试")
                self.batchFeedbackIsError = true
            }
        } catch {
            self.batchFeedback = String(localized: "还原失败，请检查网关连接")
            self.batchFeedbackIsError = true
        }
    }

    private func localizedBatchRunningText(dev: DeviceTitleTargetLanguage) -> String {
        switch dev.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "后台正在翻译中，请稍候"
        case .japanese:
            return "翻訳処理が実行中です。しばらくお待ちください"
        default:
            return "Translation is already running in the background"
        }
    }

    private func localizedTranslateResultText(dev: DeviceTitleTargetLanguage, translated: Int, skipped: Int, failed: Int) -> String {
        switch dev.kind {
        case .simplifiedChinese, .traditionalChinese:
            if translated == 0 && failed == 0 {
                return "所有活跃会话标题已是目标语言（跳过 \(skipped) 条）"
            }
            return failed > 0
                ? "已翻译 \(translated) 条，跳过 \(skipped) 条，失败 \(failed) 条"
                : "已翻译 \(translated) 条标题，跳过 \(skipped) 条"
        case .japanese:
            if translated == 0 && failed == 0 {
                return "翻訳対象の英語タイトルはありませんでした（\(skipped) 件スキップ）"
            }
            return failed > 0
                ? "\(translated) 件翻訳、\(skipped) 件スキップ、\(failed) 件失敗"
                : "\(translated) 件のタイトルを翻訳しました（\(skipped) 件スキップ）"
        default:
            if translated == 0 && failed == 0 {
                return "All active titles are already up to date (\(skipped) skipped)"
            }
            return failed > 0
                ? "Translated \(translated), skipped \(skipped), failed \(failed)"
                : "Translated \(translated) titles (\(skipped) skipped)"
        }
    }

    private func localizedRestoreResultText(dev: DeviceTitleTargetLanguage, restored: Int) -> String {
        switch dev.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "已还原 \(restored) 条原始标题"
        case .japanese:
            return "\(restored) 件の元のタイトルを復元しました"
        default:
            return "Restored \(restored) original titles"
        }
    }
}
