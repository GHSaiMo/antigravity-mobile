import SwiftUI

// MARK: - Below-Header Clipped Accordion Reveal Container

/// Clips expandable accordion content strictly below its header using an animatable
/// height-reveal `Layout` (avoiding SwiftUI `VStack` conditional-removal drift over
/// or above the header row), while keeping collapsed content lazily unmounted.
private struct AccordionRevealLayout: Layout {
    var progress: CGFloat

    var animatableData: CGFloat {
        get { progress }
        set { progress = newValue }
    }

    func sizeThatFits(
        proposal: ProposedViewSize,
        subviews: Subviews,
        cache: inout ()
    ) -> CGSize {
        let clamped = min(max(progress, 0), 1)
        guard let subview = subviews.first else {
            return CGSize(width: proposal.width ?? 0, height: 0)
        }
        if clamped <= 0.0001, let width = proposal.width {
            return CGSize(width: width, height: 0)
        }
        let ideal = subview.sizeThatFits(
            ProposedViewSize(width: proposal.width, height: nil)
        )
        return CGSize(
            width: proposal.width ?? ideal.width,
            height: ideal.height * clamped
        )
    }

    func placeSubviews(
        in bounds: CGRect,
        proposal: ProposedViewSize,
        subviews: Subviews,
        cache: inout ()
    ) {
        guard let subview = subviews.first else { return }
        subview.place(
            at: CGPoint(x: bounds.minX, y: bounds.minY),
            anchor: .topLeading,
            proposal: ProposedViewSize(width: bounds.width, height: nil)
        )
    }
}

struct AccordionRevealContainer<Content: View>: View {
    let isExpanded: Bool
    let topSpacing: CGFloat
    let animation: Animation
    @ViewBuilder let content: () -> Content

    @State private var isMounted: Bool
    @State private var revealProgress: CGFloat
    @State private var unmountTask: Task<Void, Never>? = nil

    init(
        isExpanded: Bool,
        topSpacing: CGFloat = 0,
        animation: Animation = .spring(response: 0.28, dampingFraction: 1.0),
        @ViewBuilder content: @escaping () -> Content
    ) {
        self.isExpanded = isExpanded
        self.topSpacing = topSpacing
        self.animation = animation
        self.content = content
        self._isMounted = State(initialValue: isExpanded)
        self._revealProgress = State(initialValue: isExpanded ? 1 : 0)
    }

    var body: some View {
        Group {
            if isMounted {
                AccordionRevealLayout(progress: revealProgress) {
                    content()
                        .padding(.top, topSpacing)
                        .frame(maxWidth: .infinity, alignment: .topLeading)
                }
                .opacity(Double(min(max(revealProgress, 0), 1)))
                .clipped()
                .allowsHitTesting(isExpanded)
            }
        }
        .onAppear {
            var tx = Transaction()
            tx.disablesAnimations = true
            withTransaction(tx) {
                if isExpanded {
                    if !isMounted { isMounted = true }
                    if revealProgress != 1 { revealProgress = 1 }
                } else {
                    if revealProgress != 0 { revealProgress = 0 }
                    if isMounted { isMounted = false }
                }
            }
        }
        .onChange(of: isExpanded) { _, expanded in
            unmountTask?.cancel()
            unmountTask = nil
            if expanded {
                if !isMounted {
                    var tx = Transaction()
                    tx.disablesAnimations = true
                    withTransaction(tx) {
                        isMounted = true
                        revealProgress = 0
                    }
                    DispatchQueue.main.async {
                        guard isExpanded else { return }
                        withAnimation(animation) {
                            revealProgress = 1
                        }
                    }
                } else {
                    withAnimation(animation) {
                        revealProgress = 1
                    }
                }
            } else {
                withAnimation(animation) {
                    revealProgress = 0
                }
                unmountTask = Task { @MainActor in
                    try? await Task.sleep(nanoseconds: 420_000_000)
                    guard !Task.isCancelled, !isExpanded else { return }
                    var tx = Transaction()
                    tx.disablesAnimations = true
                    withTransaction(tx) {
                        isMounted = false
                    }
                }
            }
        }
        .onDisappear {
            unmountTask?.cancel()
            unmountTask = nil
        }
    }
}

// MARK: - 2×2 Pulsing 4-Dot Spinner (Matches Desktop Antigravity `$vc`)

public struct StepPlanDotSpinner: View {
    public var size: CGFloat = 15
    private let dotOffsets: [Double] = [0.0, 0.2, 0.4, 0.6]
    private let period: Double = 1.4

    public init(size: CGFloat = 15) {
        self.size = size
    }

    public var body: some View {
        let dotSize = max(3.0, (size / 4.0).rounded())
        let gap = max(2.0, (size / 7.5).rounded())
        TimelineView(.animation(minimumInterval: 1.0 / 30.0)) { context in
            let t = context.date.timeIntervalSinceReferenceDate
            VStack(spacing: gap) {
                HStack(spacing: gap) {
                    dotView(time: t, offset: dotOffsets[0], dotSize: dotSize)
                    dotView(time: t, offset: dotOffsets[1], dotSize: dotSize)
                }
                HStack(spacing: gap) {
                    dotView(time: t, offset: dotOffsets[2], dotSize: dotSize)
                    dotView(time: t, offset: dotOffsets[3], dotSize: dotSize)
                }
            }
            .frame(width: size, height: size)
        }
    }

    private func dotView(time: TimeInterval, offset: Double, dotSize: CGFloat) -> some View {
        let phase = ((time - offset).truncatingRemainder(dividingBy: period) + period)
            .truncatingRemainder(dividingBy: period) / period
        // Smooth ease-in-out pulse matching CSS agentStateDot
        let wave = 0.5 - 0.5 * cos(phase * 2.0 * .pi)
        let alpha = 0.26 + 0.62 * wave
        return Circle()
            .fill(Color.primary.opacity(alpha))
            .frame(width: dotSize, height: dotSize)
    }
}

// MARK: - Compact Circular Progress Ring (Matches Desktop Antigravity `lSc`)

public struct StepPlanProgressRing: View {
    public let done: Int
    public let total: Int
    public var size: CGFloat = 13

    public init(done: Int, total: Int, size: CGFloat = 13) {
        self.done = done
        self.total = total
        self.size = size
    }

    private var ratio: CGFloat {
        guard total > 0 else { return 0 }
        return min(1, max(0, CGFloat(done) / CGFloat(total)))
    }

    public var body: some View {
        ZStack {
            Circle()
                .stroke(Color.secondary.opacity(0.25), lineWidth: 1.8)
            Circle()
                .trim(from: 0, to: ratio)
                .stroke(
                    Color.secondary,
                    style: StrokeStyle(lineWidth: 1.8, lineCap: .round)
                )
                .rotationEffect(.degrees(-90))
        }
        .frame(width: size, height: size)
    }
}

// MARK: - Step Plan Card & Collapsed Progress Chip (Scheme A: 1:1 Desktop Parity)

public struct StepPlanCardView: View {
    public let plan: StepPlanItem
    public let isActivePlan: Bool
    @State private var isManuallyExpanded: Bool = false

    public init(plan: StepPlanItem, isActivePlan: Bool) {
        self.plan = plan
        self.isActivePlan = isActivePlan
    }

    private var chipLabel: String {
        var parts: [String] = []
        if plan.totalCount > 0 {
            parts.append(String(localized: "\(plan.doneCount)/\(plan.totalCount) 步"))
        }
        if plan.failedCount > 0 {
            parts.append(String(localized: "\(plan.failedCount) 失败"))
        }
        if plan.skippedCount > 0 {
            parts.append(String(localized: "\(plan.skippedCount) 跳过"))
        }
        return parts.joined(separator: " · ")
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if isActivePlan {
                checklistCard(isRunning: true)
            } else {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.spring(response: 0.28, dampingFraction: 1.0)) {
                        isManuallyExpanded.toggle()
                    }
                } label: {
                    HStack(spacing: 6) {
                        StepPlanProgressRing(done: plan.doneCount, total: plan.totalCount)
                        Text(chipLabel)
                            .font(.system(size: 12.5, weight: .medium).monospacedDigit())
                            .foregroundColor(isManuallyExpanded ? .primary : .secondary)
                        Image(systemName: "chevron.right")
                            .font(.system(size: 9.5, weight: .semibold))
                            .foregroundColor(.secondary.opacity(0.7))
                            .rotationEffect(.degrees(isManuallyExpanded ? 90 : 0))
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 5.5)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 9, style: .continuous)
                            .strokeBorder(Color.primary.opacity(0.08), lineWidth: 0.7)
                    )
                }
                .buttonStyle(.plain)
                .zIndex(2)

                AccordionRevealContainer(isExpanded: isManuallyExpanded, topSpacing: 6) {
                    checklistCard(isRunning: false)
                }
                .zIndex(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .onChange(of: isActivePlan) { wasActive, nowActive in
            if wasActive && !nowActive {
                withAnimation(.easeInOut(duration: 0.22)) {
                    isManuallyExpanded = false
                }
            }
        }
    }

    private func checklistCard(isRunning: Bool) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            ForEach(plan.items) { item in
                HStack(alignment: .top, spacing: 10) {
                    statusIcon(for: item.status, isRunning: isRunning)
                        .frame(width: 16, height: 18, alignment: .center)

                    Text(item.title)
                        .font(.system(size: 13.5, weight: item.status == "in_progress" ? .medium : .regular))
                        .foregroundColor(titleColor(for: item.status))
                        .strikethrough(item.status == "skipped", color: .secondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .background(Color(uiColor: .secondarySystemBackground).opacity(0.75))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .strokeBorder(Color.primary.opacity(0.10), lineWidth: 0.9)
        )
    }

    @ViewBuilder
    private func statusIcon(for status: String, isRunning: Bool) -> some View {
        switch status {
        case "done":
            Image(systemName: "checkmark.circle")
                .font(.system(size: 14.5, weight: .regular))
                .foregroundColor(.secondary)
        case "failed":
            Image(systemName: "exclamationmark.circle")
                .font(.system(size: 14.5, weight: .medium))
                .foregroundColor(.red)
        case "in_progress":
            if isRunning {
                StepPlanDotSpinner(size: 14)
            } else {
                Image(systemName: "pause.circle")
                    .font(.system(size: 14, weight: .regular))
                    .foregroundColor(.secondary)
            }
        case "skipped":
            Image(systemName: "circle")
                .font(.system(size: 14.5, weight: .regular))
                .foregroundColor(.secondary.opacity(0.45))
        default:
            Image(systemName: "circle")
                .font(.system(size: 14.5, weight: .regular))
                .foregroundColor(.secondary.opacity(0.75))
        }
    }

    private func titleColor(for status: String) -> Color {
        switch status {
        case "done", "skipped":
            return .secondary
        case "pending":
            return .primary.opacity(0.85)
        default:
            return .primary
        }
    }
}

// MARK: - Collapsible Thinking Process Block (Level-2 Disclosure inside Focus Row / Tool Batch)

public struct CollapsibleThinkingBlockView: View {
    public let thinking: String
    @State private var isExpanded: Bool

    public init(thinking: String, initiallyExpanded: Bool = false) {
        self.thinking = thinking
        self._isExpanded = State(initialValue: initiallyExpanded)
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Button {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.spring(response: 0.28, dampingFraction: 1.0)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 6) {
                    Label("思考过程", systemImage: "brain")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.indigo)

                    Spacer(minLength: 4)

                    Image(systemName: "chevron.right")
                        .font(.system(size: 9.5, weight: .semibold))
                        .foregroundColor(.indigo.opacity(0.7))
                        .rotationEffect(.degrees(isExpanded ? 90 : 0))
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            AccordionRevealContainer(isExpanded: isExpanded) {
                Text(thinking)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 12)
                    .padding(.top, 2)
                    .padding(.bottom, 8)
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background(Color.indigo.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .strokeBorder(Color.indigo.opacity(0.14), lineWidth: 0.5)
        )
    }
}

// MARK: - Live Focus Status Row + Elapsed Timer (Matches Desktop Antigravity `tSc` + `rSc`)

public enum ElapsedTimerFormatter {
    private static let isoFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let isoBasic: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    public static func parseDate(_ raw: String?) -> Date? {
        guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else {
            return nil
        }
        return isoFractional.date(from: trimmed) ?? isoBasic.date(from: trimmed)
    }

    public static func format(since start: Date, now: Date) -> String {
        let secs = max(0, Int(now.timeIntervalSince(start)))
        let h = secs / 3600
        let m = (secs % 3600) / 60
        let s = secs % 60
        if h > 0 {
            return "\(h)h \(m)m \(s)s"
        }
        if m > 0 {
            return "\(m)m \(s)s"
        }
        return "\(s)s"
    }
}

public struct ActiveFocusToolRowView: View {
    public let focusText: String
    public let startedAt: String?
    public let thinking: String?
    public let thinkingDuration: String?
    public let thinkingBlocks: [ThinkingBlockItem]?
    public let toolDetails: [ToolDetailItem]?
    public let fallbackTools: [String]
    public let isExpandable: Bool

    @State private var isExpanded: Bool = false
    @State private var shimmerOn: Bool = false

    public init(
        focusText: String,
        startedAt: String? = nil,
        thinking: String? = nil,
        thinkingDuration: String? = nil,
        thinkingBlocks: [ThinkingBlockItem]? = nil,
        toolDetails: [ToolDetailItem]? = nil,
        fallbackTools: [String] = [],
        isExpandable: Bool = true
    ) {
        self.focusText = focusText
        self.startedAt = startedAt
        self.thinking = thinking
        self.thinkingDuration = thinkingDuration
        self.thinkingBlocks = thinkingBlocks
        self.toolDetails = toolDetails
        self.fallbackTools = fallbackTools
        self.isExpandable = isExpandable
    }

    private var hasTools: Bool {
        !(toolDetails?.isEmpty ?? true) || !fallbackTools.isEmpty
    }

    private var hasDetails: Bool {
        let hasThinking = !(thinking?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
            || !(thinkingBlocks?.isEmpty ?? true)
        return isExpandable && (hasThinking || hasTools)
    }

    private var startDate: Date? {
        ElapsedTimerFormatter.parseDate(startedAt)
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Group {
                if hasDetails {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        withAnimation(.spring(response: 0.28, dampingFraction: 1.0)) {
                            isExpanded.toggle()
                        }
                    } label: {
                        rowContent
                    }
                    .buttonStyle(.plain)
                } else {
                    rowContent
                }
            }
            .zIndex(2)

            AccordionRevealContainer(isExpanded: isExpanded && hasDetails, topSpacing: 4) {
                DesktopToolTimelineListView(
                    thinking: thinking,
                    thinkingDuration: thinkingDuration,
                    thinkingBlocks: thinkingBlocks,
                    toolDetails: toolDetails,
                    fallbackTools: fallbackTools
                )
            }
            .zIndex(1)
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .onAppear {
            shimmerOn = true
        }
    }

    private var rowContent: some View {
        HStack(spacing: 6) {
            Text(focusText)
                .font(.system(size: 13, weight: .regular))
                .foregroundColor(.secondary)
                .opacity(shimmerOn ? 0.62 : 1.0)
                .animation(.easeInOut(duration: 1.2).repeatForever(autoreverses: true), value: shimmerOn)
                .lineLimit(1)
                .truncationMode(.tail)

            if let start = startDate {
                TimelineView(.periodic(from: .now, by: 1.0)) { context in
                    Text(ElapsedTimerFormatter.format(since: start, now: context.date))
                        .font(.system(size: 12.5, weight: .regular).monospacedDigit())
                        .foregroundColor(.secondary.opacity(0.85))
                        .fixedSize()
                }
            }

            if hasDetails {
                Image(systemName: "chevron.right")
                    .font(.system(size: 10, weight: .medium))
                    .foregroundColor(.secondary.opacity(0.65))
                    .rotationEffect(.degrees(isExpanded ? 90 : 0))
            }

            Spacer(minLength: 0)
        }
        .padding(.horizontal, 4)
        .padding(.vertical, 3)
        .contentShape(Rectangle())
    }
}
