import Foundation
import StoreKit
import UIKit

/// 原生 App Store 评分触发管理器（StoreKit `AppStore.requestReview(in:)`）。
///
/// 设计原则：
/// 1. 零 UI 侵入：不弹自定义引导框、不做“喜欢/不喜欢”分流（严格遵守 App Store 审核规范）。
/// 2. 只在用户刚经历明确正向“爽点”后的安静空窗触发：
///    - 手机前台见证 Agent 无报错跑完一轮有效回复（含工具调用或远程审批/Proceed 后交付权重更高）
///    - 手机远程一键 Git Commit / Push 成功并关闭提交弹窗
///    - 导出/保存或分享精美对话长图卡片并关闭预览弹窗
/// 3. 强情绪保护与防打扰：
///    - 演示模式 / 未配对时彻底静默（连计分都不做，避免干扰 App Review 或抓图）
///    - 新用户保护：距首次真实使用满 24h、累计跨 ≥ 2 个活跃自然日、累计正向爽点达标后才会触发
///    - 报错、用户主动取消任务、撤销回滚、推送失败等负面摩擦事件后进入 15 分钟情绪冷却期
///    - 键盘弹出、任意弹窗/Sheet 遮挡、通话/录音中、有未发送草稿、任务运行中均不打扰
///    - 珍惜苹果 365 天 3 次配额：每个 App 版本最多请求 1 次，且两次请求至少间隔 90 天
@MainActor
public final class AppReviewManager {
    public static let shared = AppReviewManager()

    public enum HighValueMoment: String {
        case gitCommitSucceeded
        case shareCardExported

        var score: Int { 3 }
    }

    // MARK: - Tuning Thresholds

    private enum Config {
        /// 首次真实已配对使用后至少等待 24 小时，避免刚安装就讨好评。
        static let minAgeSinceFirstUse: TimeInterval = 24 * 3600
        /// 至少在 2 个不同自然日活跃使用过。
        static let minActiveDays: Int = 2
        /// 累计正向爽点积分阈值。
        static let minDelightScore: Int = 5
        /// 至少前台见证过 2 轮成功的 Agent 回复闭环。
        static let minCompletedTurns: Int = 2
        /// 两次实际调用 `AppStore.requestReview` 之间的最小冷却期（对齐苹果 365 天 3 次配额）。
        static let promptCooldown: TimeInterval = 90 * 24 * 3600
        /// 遭遇错误、取消任务、撤销改动等摩擦后的情绪冷却期。
        static let frictionCooldown: TimeInterval = 15 * 60
        /// 挂起等待关闭弹窗/切回列表的爽点有效期（过期自动作废，避免莫名事后弹窗）。
        static let pendingMomentTTL: TimeInterval = 30
    }

    private enum Keys {
        static let firstPairedUseAt = "antigravity.review.first_paired_use_at"
        static let activeDays = "antigravity.review.active_days"
        static let delightScore = "antigravity.review.delight_score"
        static let completedTurnsCount = "antigravity.review.completed_turns_count"
        static let lastPromptedAt = "antigravity.review.last_prompted_at"
        static let lastPromptedVersion = "antigravity.review.last_prompted_version"
    }

    private struct PendingMoment {
        let reason: String
        let createdAt: Date
    }

    private let defaults = UserDefaults.standard
    private var isKeyboardVisible: Bool = false
    private var lastFrictionAt: Date = .distantPast
    private var hasRequestedInCurrentLaunch: Bool = false
    private var pendingMoment: PendingMoment?
    private var scheduledPromptTask: Task<Void, Never>?
    private var lastTurnCompletionKey: String = ""

    private static let dayFormatter: DateFormatter = {
        let f = DateFormatter()
        f.calendar = Calendar(identifier: .gregorian)
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f
    }()

    private init() {
        NotificationCenter.default.addObserver(
            forName: UIResponder.keyboardWillShowNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.isKeyboardVisible = true
                // 用户刚拉起键盘准备打字：立刻取消当前排队的延时弹窗，绝不打断输入
                self?.cancelScheduledPrompt()
            }
        }

        NotificationCenter.default.addObserver(
            forName: UIResponder.keyboardWillHideNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.isKeyboardVisible = false
            }
        }

        NotificationCenter.default.addObserver(
            forName: UIApplication.willResignActiveNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.cancelScheduledPrompt()
            }
        }
    }

    // MARK: - Eligibility Helpers

    private var isRealPairedSession: Bool {
        let settings = AppSettings.shared
        return settings.isPaired && !settings.isDemoMode && !DemoGateway.isEnabled
    }

    private var currentAppVersion: String {
        (Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? "0"
    }

    private var firstPairedUseAt: Date? {
        let ts = defaults.double(forKey: Keys.firstPairedUseAt)
        return ts > 0 ? Date(timeIntervalSince1970: ts) : nil
    }

    private var activeDays: [String] {
        defaults.stringArray(forKey: Keys.activeDays) ?? []
    }

    private var delightScore: Int {
        defaults.integer(forKey: Keys.delightScore)
    }

    private var completedTurnsCount: Int {
        defaults.integer(forKey: Keys.completedTurnsCount)
    }

    private var lastPromptedAt: Date? {
        let ts = defaults.double(forKey: Keys.lastPromptedAt)
        return ts > 0 ? Date(timeIntervalSince1970: ts) : nil
    }

    private var lastPromptedVersion: String? {
        defaults.string(forKey: Keys.lastPromptedVersion)
    }

    // MARK: - Public Recording API

    /// 在 App 启动/回前台/配对成功后记录真实活跃日。
    public func recordActiveDayIfEligible() {
        guard isRealPairedSession else { return }
        let now = Date()
        if firstPairedUseAt == nil {
            defaults.set(now.timeIntervalSince1970, forKey: Keys.firstPairedUseAt)
        }
        let today = Self.dayFormatter.string(from: now)
        var days = activeDays
        if !days.contains(today) {
            days.append(today)
            if days.count > 30 {
                days = Array(days.suffix(30))
            }
            defaults.set(days, forKey: Keys.activeDays)
        }
    }

    /// 负面体验（报错、手动取消任务、撤销回滚、推送失败、语音异常断开）时调用：
    /// 清除所有排队中的弹窗，并进入情绪冷却期。
    public func recordFriction() {
        lastFrictionAt = Date()
        pendingMoment = nil
        cancelScheduledPrompt()
    }

    /// 取消当前正在倒计时的延迟触发（例如用户开始输入或切走，不扣除已累计的爽点分）。
    public func cancelScheduledPrompt() {
        scheduledPromptTask?.cancel()
        scheduledPromptTask = nil
    }

    /// 记录用户在前台见证 Agent 无报错完成一轮有效回复。
    /// - Parameters:
    ///   - cascadeId: 会话 ID（用于去重）。
    ///   - anchorMessageId: 本轮最新 Agent 消息 ID（用于防止 Stream + Poll 重复计分）。
    ///   - usedTools: 本轮是否实际执行过工具调用（代码修改/命令执行/搜索等，成就感更强）。
    ///   - afterRemoteApproval: 本轮是否经过手机端远程批准操作或一键 Proceed 方案确认。
    ///   - isQuietCheck: 延迟到达时二次校验聊天页是否依然处于安静无打扰态。
    public func recordTurnCompleted(
        cascadeId: String,
        anchorMessageId: String,
        usedTools: Bool,
        afterRemoteApproval: Bool,
        isQuietCheck: @escaping @MainActor () -> Bool
    ) {
        guard isRealPairedSession else { return }
        guard !cascadeId.isEmpty, !anchorMessageId.isEmpty else { return }

        let dedupKey = "\(cascadeId)|\(anchorMessageId)"
        guard dedupKey != lastTurnCompletionKey else { return }
        lastTurnCompletionKey = dedupKey

        recordActiveDayIfEligible()

        let bonus = (usedTools || afterRemoteApproval) ? 2 : 1
        let nextScore = delightScore + bonus
        let nextTurns = completedTurnsCount + 1
        defaults.set(nextScore, forKey: Keys.delightScore)
        defaults.set(nextTurns, forKey: Keys.completedTurnsCount)

        guard meetsAllEligibilityCriteria() else { return }

        // 暂存一份短期窗口：若用户看完后顺手点返回回到会话列表，也能在列表空窗顺滑接住
        pendingMoment = PendingMoment(reason: "turn_completed", createdAt: Date())

        // 留出 2.8 秒让用户先扫完回复核心结果，期间若开始打字/弹窗/报错会自动取消
        schedulePrompt(after: 2.8, reason: "turn_completed", extraQuietCheck: isQuietCheck)
    }

    /// 记录高价值闭环爽点（远程 Git Commit/Push 成功、长图分享/保存成功）。
    /// - Parameters:
    ///   - moment: 爽点类型。
    ///   - deferUntilDismiss: 当前是否处于 Sheet 内（如 GitCommitSheet / MessageShareCardSheet）；
    ///     为 `true` 时先记分并挂起，待用户关掉 Sheet 回到主画面再顺势弹出。
    public func recordHighValueDelight(_ moment: HighValueMoment, deferUntilDismiss: Bool) {
        guard isRealPairedSession else { return }
        recordActiveDayIfEligible()

        let nextScore = delightScore + moment.score
        defaults.set(nextScore, forKey: Keys.delightScore)

        guard meetsAllEligibilityCriteria() else { return }

        if deferUntilDismiss {
            pendingMoment = PendingMoment(reason: moment.rawValue, createdAt: Date())
        } else {
            pendingMoment = PendingMoment(reason: moment.rawValue, createdAt: Date())
            schedulePrompt(after: 1.2, reason: moment.rawValue, extraQuietCheck: nil)
        }
    }

    /// 当承载高爽点操作的 Sheet 关闭、或用户从刚跑完的会话返回主列表时调用，
    /// 在画面稳定无遮挡后释放挂起的原生评分弹窗。
    public func flushPendingPromptIfReady(
        delay: TimeInterval = 0.9,
        isQuietCheck: (@MainActor () -> Bool)? = nil
    ) {
        guard let pending = pendingMoment else { return }
        let age = Date().timeIntervalSince(pending.createdAt)
        guard age >= 0, age <= Config.pendingMomentTTL else {
            pendingMoment = nil
            return
        }
        guard meetsAllEligibilityCriteria() else {
            pendingMoment = nil
            return
        }
        schedulePrompt(after: delay, reason: pending.reason, extraQuietCheck: isQuietCheck)
    }

    // MARK: - Core Gate & Invocation

    private func meetsAllEligibilityCriteria() -> Bool {
        guard isRealPairedSession else { return false }
        guard !hasRequestedInCurrentLaunch else { return false }

        let now = Date()
        if now.timeIntervalSince(lastFrictionAt) < Config.frictionCooldown {
            return false
        }

        // 版本防重：同一个 App 版本只请求一次
        let version = currentAppVersion
        if let lastVersion = lastPromptedVersion, !lastVersion.isEmpty, lastVersion == version {
            return false
        }

        // 时间冷却：两次请求至少间隔 90 天
        if let lastPrompted = lastPromptedAt, now.timeIntervalSince(lastPrompted) < Config.promptCooldown {
            return false
        }

        // 新用户信任养成期：首次真实使用满 24h 且跨 ≥ 2 个活跃日
        guard let firstUse = firstPairedUseAt,
              now.timeIntervalSince(firstUse) >= Config.minAgeSinceFirstUse else {
            return false
        }
        guard activeDays.count >= Config.minActiveDays else { return false }
        guard completedTurnsCount >= Config.minCompletedTurns else { return false }
        guard delightScore >= Config.minDelightScore else { return false }

        return true
    }

    private func schedulePrompt(
        after delay: TimeInterval,
        reason: String,
        extraQuietCheck: (@MainActor () -> Bool)?
    ) {
        cancelScheduledPrompt()
        let clampedDelay = max(0.3, delay)
        scheduledPromptTask = Task { @MainActor [weak self] in
            let nanos = UInt64(clampedDelay * 1_000_000_000)
            try? await Task.sleep(nanoseconds: nanos)
            guard !Task.isCancelled, let self else { return }
            self.scheduledPromptTask = nil
            self.attemptPresentReviewIfQuiet(reason: reason, extraQuietCheck: extraQuietCheck)
        }
    }

    private func attemptPresentReviewIfQuiet(
        reason: String,
        extraQuietCheck: (@MainActor () -> Bool)?
    ) {
        guard meetsAllEligibilityCriteria() else { return }
        guard UIApplication.shared.applicationState == .active else { return }
        guard !isKeyboardVisible else { return }
        guard !VoiceCallManager.shared.isInCall else { return }
        guard !AudioRecorderManager.shared.isRecording else { return }
        guard !PushNotificationManager.shared.hasBlockingModalPresented else { return }
        if let extraQuietCheck, !extraQuietCheck() { return }

        guard let scene = activeWindowScene(), !hasPresentedModal(in: scene) else {
            return
        }

        // 真正触发系统原生评分弹窗并记录频控状态
        pendingMoment = nil
        hasRequestedInCurrentLaunch = true
        defaults.set(Date().timeIntervalSince1970, forKey: Keys.lastPromptedAt)
        defaults.set(currentAppVersion, forKey: Keys.lastPromptedVersion)

        AppStore.requestReview(in: scene)
        #if DEBUG
        print("[AppReviewManager] Requested native StoreKit review (reason: \(reason), score: \(delightScore), turns: \(completedTurnsCount))")
        #endif
    }

    private func activeWindowScene() -> UIWindowScene? {
        UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .first(where: { $0.activationState == .foregroundActive })
    }

    /// 检查当前活跃窗口是否存在任何盖在顶层的 Sheet / FullScreenCover / Alert / ShareSheet。
    private func hasPresentedModal(in scene: UIWindowScene) -> Bool {
        for window in scene.windows where !window.isHidden && window.alpha > 0.01 {
            if let root = window.rootViewController, root.presentedViewController != nil {
                return true
            }
        }
        return false
    }
}
