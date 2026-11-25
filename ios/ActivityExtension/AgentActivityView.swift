import SwiftUI
import Foundation
import WidgetKit
import ActivityKit
import AppIntents
#if canImport(UIKit)
import UIKit
#endif

private func redactLiveActivityCommand(_ raw: String) -> String {
    var out = raw
    let patterns: [(String, String)] = [
        (#"(?i)(Bearer\s+)[A-Za-z0-9._\-~+/=]+"#, "$1***"),
        (#"(?i)((?:token|api[_-]?key|password|passwd|secret|auth_token)=)[^\s&"']+"#, "$1***"),
        (#"(-w\s+)[^\s"']+"#, "$1***")
    ]
    for (pat, repl) in patterns {
        if let regex = try? NSRegularExpression(pattern: pat) {
            let range = NSRange(out.startIndex..<out.endIndex, in: out)
            out = regex.stringByReplacingMatches(in: out, options: [], range: range, withTemplate: repl)
        }
    }
    return out
}

public struct AgentActivityWidget: Widget {
    public init() {}
    
    private func activityDeepLinkURL(for attributes: AgentActivityAttributes) -> URL? {
        if let gw = attributes.gatewayId?.trimmingCharacters(in: .whitespacesAndNewlines), !gw.isEmpty {
            return URL(string: "antigravity://cascade/\(attributes.cascadeId)?gw=\(gw)")
        }
        return URL(string: "antigravity://cascade/\(attributes.cascadeId)")
    }

    public var body: some WidgetConfiguration {
        ActivityConfiguration(for: AgentActivityAttributes.self) { context in
            // Routes .small (CarPlay Dashboard / Apple Watch Smart Stack) vs .medium (iPhone Lock Screen)
            AgentActivityPresentationView(context: context)
                .widgetURL(activityDeepLinkURL(for: context.attributes))
        } dynamicIsland: { context in
            DynamicIsland {
                // Expanded UI
                DynamicIslandExpandedRegion(.leading) {
                    expandedLeadingView(context: context)
                }
                
                DynamicIslandExpandedRegion(.trailing) {
                    expandedTrailingView(context: context)
                }
                
                DynamicIslandExpandedRegion(.bottom) {
                    expandedBottomView(context: context)
                }
            } compactLeading: {
                compactLeadingView(context: context)
            } compactTrailing: {
                compactTrailingView(context: context)
            } minimal: {
                minimalView(context: context)
            }
            .widgetURL(activityDeepLinkURL(for: context.attributes))
        }
        .supplementalActivityFamilies([.small])
    }
    
    // MARK: - Dynamic Island Subviews
    
    @ViewBuilder
    private func compactLeadingView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        if context.state.hasPendingAction {
            Image(systemName: "exclamationmark.circle.fill")
                .font(.system(size: 18))
                .foregroundColor(.orange)
        } else if context.state.runningTaskCount > 0 {
            Image(systemName: "terminal.fill")
                .font(.system(size: 14))
                .foregroundColor(Color(red: 0.0, green: 0.65, blue: 0.9))
        } else if context.state.status == "COMPLETED" {
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 18))
                .foregroundColor(.green)
        } else {
            Image("AppLogoTransparent")
                .renderingMode(.original)
                .resizable()
                .aspectRatio(contentMode: .fit)
                .frame(width: 22, height: 14)
        }
    }
    
    @ViewBuilder
    private func compactTrailingView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        if context.state.hasPendingAction {
            Text("待确认")
                .font(.system(size: 13, weight: .bold))
                .foregroundColor(.orange)
        } else if context.state.status == "COMPLETED" {
            Text("已完成")
                .font(.system(size: 13, weight: .bold))
                .foregroundColor(.green)
        } else if context.state.runningTaskCount > 0 {
            HStack(spacing: 3) {
                Image(systemName: "bolt.fill")
                    .font(.system(size: 10))
                Text("\(context.state.runningTaskCount) 任务")
                    .font(.system(size: 12.5, weight: .bold))
            }
            .foregroundColor(Color(red: 0.35, green: 0.88, blue: 1.0))
        } else {
            Text("第 \(context.state.stepCount) 步")
                .font(.system(size: 12.5, weight: .semibold, design: .rounded))
                .foregroundColor(.white)
        }
    }
    
    @ViewBuilder
    private func minimalView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        compactLeadingView(context: context)
    }
    
    @ViewBuilder
    private func expandedLeadingView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        HStack(spacing: 8) {
            if context.state.hasPendingAction {
                Image(systemName: "exclamationmark.circle.fill")
                    .font(.system(size: 19))
                    .foregroundColor(.orange)
            } else if context.state.runningTaskCount > 0 {
                Image(systemName: "terminal.fill")
                    .font(.system(size: 15))
                    .foregroundColor(Color(red: 0.0, green: 0.65, blue: 0.9))
            } else if context.state.status == "COMPLETED" {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 19))
                    .foregroundColor(.green)
            } else {
                Image("AppLogoTransparent")
                    .renderingMode(.original)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 24, height: 15)
            }
            
            Text(resolvedTitle(context: context))
                .font(.system(size: 15, weight: .bold))
                .foregroundColor(.white)
                .lineLimit(1)
        }
        .padding(.leading, 4)
    }
    
    @ViewBuilder
    private func expandedTrailingView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        if context.state.hasPendingAction {
            Text("待审批")
                .font(.system(size: 11.5, weight: .bold))
                .foregroundColor(.orange)
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(Color.orange.opacity(0.25))
                .cornerRadius(5)
        } else if context.state.runningTaskCount > 0 {
            Text("\(context.state.runningTaskCount) 任务运行中")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(.cyan)
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(Color.cyan.opacity(0.25))
                .cornerRadius(5)
        } else {
            let isRunning = context.state.status == "RUNNING"
            Text(isRunning ? String(localized: "Agent 执行中") : String(localized: "已就绪"))
                .font(.system(size: 11.5, weight: .bold))
                .foregroundColor(isRunning ? Color(red: 0.6, green: 0.72, blue: 1.0) : .green)
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background((isRunning ? Color.indigo : Color.green).opacity(0.25))
                .cornerRadius(5)
        }
    }
    
    @ViewBuilder
    private func expandedBottomView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("当前第 \(context.state.stepCount) 步")
                    .font(.system(size: 11.5, weight: .bold, design: .monospaced))
                    .foregroundColor(Color(red: 0.45, green: 0.75, blue: 1.0))
                Spacer()
                Text(context.state.lastUpdated, style: .relative)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
            }
            
            if context.state.runningTaskCount > 0 {
                let islandTasks: [AgentTaskSnapshot] = !context.state.runningTasks.isEmpty
                    ? Array(context.state.runningTasks.prefix(2))
                    : [AgentTaskSnapshot(id: "fallback", title: context.state.activeTaskTitle ?? String(localized: "后台任务"), command: context.state.activeTaskCommand)]
                
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(islandTasks) { task in
                        HStack(spacing: 6) {
                            Image(systemName: task.isWaiting ? "hourglass" : "terminal.fill")
                                .font(.system(size: 10))
                                .foregroundColor(task.isWaiting ? .orange : .cyan)
                            
                            if let cmd = task.command, !cmd.isEmpty {
                                Text(redactLiveActivityCommand(cmd))
                                    .font(.system(size: 11, design: .monospaced))
                                    .lineLimit(1)
                                    .foregroundColor(.cyan.opacity(0.95))
                                    .privacySensitive()
                            } else {
                                Text(task.title)
                                    .font(.system(size: 11))
                                    .lineLimit(1)
                                    .foregroundColor(.white)
                            }
                            
                            Spacer()
                        }
                        .padding(.horizontal, 7)
                        .padding(.vertical, 3.5)
                        .background(Color.white.opacity(0.1))
                        .cornerRadius(5)
                    }
                    
                    let islandRemaining = max(0, context.state.runningTaskCount - islandTasks.count)
                    if islandRemaining > 0 {
                        HStack {
                            Spacer()
                            Text("+\(islandRemaining) 个任务排队中")
                                .font(.system(size: 9.5, weight: .medium))
                                .foregroundColor(.secondary)
                        }
                    }
                }
            } else {
                HStack(alignment: .top, spacing: 6) {
                    let iconInfo = dynamicActionIcon(for: context.state.latestAction, hasPendingAction: context.state.hasPendingAction, hasRunningTasks: false)
                    Image(systemName: iconInfo.name)
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(iconInfo.color)
                        .padding(.top, 1)
                    
                    Text(renderMarkdownText(context.state.latestAction, defaultColor: Color.white.opacity(0.92), isDark: true))
                        .font(.system(size: 13, weight: .medium))
                        .lineLimit(2)
                        .lineSpacing(2)
                }
                .padding(.vertical, 2)
            }
            
            if context.state.hasPendingAction {
                if context.state.requiresInAppReview {
                    HStack(spacing: 5) {
                        Image(systemName: "arrow.up.forward.app")
                            .font(.system(size: 10.5, weight: .semibold))
                        Text("轻点进入 App 查看详情并确认")
                            .font(.system(size: 11.5, weight: .semibold))
                    }
                    .foregroundColor(.orange)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 5)
                    .background(Color.orange.opacity(0.18))
                    .clipShape(Capsule())
                    .padding(.top, 2)
                } else {
                    let stepIdx = context.state.pendingStepIndex ?? -1
                    HStack(spacing: 8) {
                        Button(intent: ApproveAgentActionIntent(cascadeId: context.attributes.cascadeId, expectedStepIndex: stepIdx, gatewayId: context.attributes.gatewayId ?? "")) {
                            HStack(spacing: 4) {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 10, weight: .bold))
                                Text("允许")
                                    .font(.system(size: 11.5, weight: .bold))
                            }
                            .foregroundColor(.white)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 5)
                            .background(Color.green.opacity(0.82))
                            .clipShape(Capsule())
                        }
                        .buttonStyle(.plain)
                        
                        Button(intent: RejectAgentActionIntent(cascadeId: context.attributes.cascadeId, expectedStepIndex: stepIdx, gatewayId: context.attributes.gatewayId ?? "")) {
                            HStack(spacing: 4) {
                                Image(systemName: "xmark")
                                    .font(.system(size: 10, weight: .bold))
                                Text("拒绝")
                                    .font(.system(size: 11.5, weight: .bold))
                            }
                            .foregroundColor(.white)
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 5)
                            .background(Color.red.opacity(0.75))
                            .clipShape(Capsule())
                        }
                        .buttonStyle(.plain)
                    }
                    .padding(.top, 2)
                }
            } else if context.state.status == "RUNNING" {
                HStack {
                    Spacer()
                    Button(intent: StopAgentRunIntent(cascadeId: context.attributes.cascadeId, gatewayId: context.attributes.gatewayId ?? "")) {
                        HStack(spacing: 4) {
                            Image(systemName: "stop.fill")
                                .font(.system(size: 9))
                            Text("停止")
                                .font(.system(size: 11, weight: .semibold))
                        }
                        .foregroundColor(Color(red: 1.0, green: 0.55, blue: 0.55))
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                        .background(Color.red.opacity(0.22))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(.horizontal, 4)
        .padding(.top, 4)
    }
}

// MARK: - Activity Family Router (iPhone Lock Screen vs CarPlay / Apple Watch .small)

private struct AgentActivityPresentationView: View {
    let context: ActivityViewContext<AgentActivityAttributes>
    @Environment(\.activityFamily) private var activityFamily

    var body: some View {
        switch activityFamily {
        case .small:
            AgentSmallActivityView(context: context)
        default:
            AgentLockScreenView(context: context)
        }
    }
}

/// Glancable 2-row high-contrast Live Activity view shared by CarPlay Dashboard (iOS 26+)
/// and Apple Watch Smart Stack. Pure informational (no interactive buttons for driving safety).
private struct AgentSmallActivityView: View {
    let context: ActivityViewContext<AgentActivityAttributes>
    @Environment(\.colorScheme) private var colorScheme

    private var isDark: Bool { colorScheme == .dark }

    private var primaryColor: Color {
        isDark ? Color(red: 0.98, green: 0.98, blue: 0.99) : Color(red: 0.08, green: 0.09, blue: 0.12)
    }

    private var secondaryColor: Color {
        isDark ? Color(red: 0.86, green: 0.88, blue: 0.92) : Color(red: 0.22, green: 0.24, blue: 0.29)
    }

    private var accentColor: Color {
        isDark ? Color(red: 0.45, green: 0.72, blue: 1.0) : Color(red: 0.12, green: 0.36, blue: 0.88)
    }

    private var statusBadgeLabel: String {
        if context.state.hasPendingAction {
            return String(localized: "待审批")
        }
        switch context.state.status {
        case "COMPLETED":
            return String(localized: "已完成")
        case "FAILED":
            return String(localized: "失败")
        case "CANCELLED":
            return String(localized: "已停止")
        default:
            return String(localized: "第 \(context.state.stepCount) 步")
        }
    }

    private var statusBadgeColor: Color {
        if context.state.hasPendingAction {
            return .orange
        }
        switch context.state.status {
        case "COMPLETED":
            return Color(red: 0.20, green: 0.78, blue: 0.40)
        case "FAILED":
            return .red
        case "CANCELLED":
            return .secondary
        default:
            return accentColor
        }
    }

    private var detailText: String {
        let trimmedAction = context.state.latestAction.trimmingCharacters(in: .whitespacesAndNewlines)
        if context.state.hasPendingAction {
            if !trimmedAction.isEmpty {
                return trimmedAction
            }
            return String(localized: "请在手机上确认审批")
        }
        if context.state.runningTaskCount > 0 {
            let taskTitle = (context.state.activeTaskTitle ?? context.state.runningTasks.first?.title ?? "")
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if !taskTitle.isEmpty {
                if context.state.runningTaskCount > 1 {
                    return "\(taskTitle) (+\(context.state.runningTaskCount - 1))"
                }
                return taskTitle
            }
        }
        if !trimmedAction.isEmpty {
            return trimmedAction
        }
        if context.state.status == "COMPLETED" {
            return String(localized: "任务已完成 · 共 \(context.state.stepCount) 步")
        }
        return String(localized: "正在执行第 \(context.state.stepCount) 步")
    }

    @ViewBuilder
    private var leadingIcon: some View {
        if context.state.hasPendingAction {
            Image(systemName: "exclamationmark.circle.fill")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(.orange)
        } else if context.state.status == "COMPLETED" {
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(Color(red: 0.20, green: 0.78, blue: 0.40))
        } else if context.state.status == "FAILED" {
            Image(systemName: "xmark.circle.fill")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(.red)
        } else {
            Image("AppLogoTransparent")
                .renderingMode(.original)
                .resizable()
                .aspectRatio(contentMode: .fit)
                .frame(width: 20, height: 14)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            // Row 1: Brand/status icon + Conversation Title + Step/Status pill
            HStack(alignment: .center, spacing: 6) {
                leadingIcon

                Text(resolvedTitle(context: context))
                    .font(.system(size: 14.5, weight: .bold))
                    .foregroundColor(primaryColor)
                    .lineLimit(1)

                Spacer(minLength: 4)

                Text(statusBadgeLabel)
                    .font(.system(size: 11.5, weight: .bold, design: .rounded))
                    .foregroundColor(statusBadgeColor)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2.5)
                    .background(statusBadgeColor.opacity(isDark ? 0.22 : 0.14))
                    .clipShape(Capsule())
                    .lineLimit(1)
            }

            // Row 2: Concrete current step / active background task (up to 2 lines, high legibility)
            HStack(alignment: .top, spacing: 6) {
                let iconInfo = dynamicActionIcon(
                    for: detailText,
                    hasPendingAction: context.state.hasPendingAction,
                    hasRunningTasks: context.state.runningTaskCount > 0
                )
                Image(systemName: iconInfo.name)
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(iconInfo.color)
                    .padding(.top, 2)

                Text(renderMarkdownText(detailText, defaultColor: secondaryColor, isDark: isDark))
                    .font(.system(size: 13.5, weight: .medium))
                    .lineLimit(2)
                    .lineSpacing(1.5)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
    }
}

// MARK: - Lock Screen Live Activity View (JD-Style Layout & Adaptive Contrast)

struct AgentLockScreenView: View {
    let context: ActivityViewContext<AgentActivityAttributes>
    @Environment(\.colorScheme) var colorScheme
    
    private var isDark: Bool {
        #if canImport(UIKit)
        if UITraitCollection.current.userInterfaceStyle == .dark {
            return true
        } else if UITraitCollection.current.userInterfaceStyle == .light {
            return false
        }
        #endif
        return colorScheme == .dark
    }
    
    // MARK: - Semantic Colors (Adaptive for Light and Dark)
    
    // Primary title text: High contrast dark charcoal in light mode, crisp white in dark mode
    private var titleColor: Color {
        isDark
            ? Color(red: 0.98, green: 0.98, blue: 0.99)
            : Color(red: 0.08, green: 0.09, blue: 0.12)
    }
    
    // Main action text (e.g. "已思考并执行 53 项操作"): Deep readable slate in light mode
    private var actionTextColor: Color {
        isDark
            ? Color(red: 0.90, green: 0.91, blue: 0.93)
            : Color(red: 0.22, green: 0.24, blue: 0.28)
    }
    
    // Accent / Brand color (sparkles, step counter): Deep vibrant tech blue in light mode
    private var accentColor: Color {
        isDark
            ? Color(red: 0.52, green: 0.68, blue: 1.0)
            : Color(red: 0.14, green: 0.35, blue: 0.88)
    }
    
    // Secondary metadata text (elapsed time, separators, sub-labels)
    private var secondaryTextColor: Color {
        isDark
            ? Color(red: 0.62, green: 0.65, blue: 0.70)
            : Color(red: 0.42, green: 0.45, blue: 0.50)
    }
    
    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            // 1. Left Avatar / Brand Badge (JD Delivery Style 44x44)
            avatarBadge
            
            // 2. Center-Right Main Info Column
            VStack(alignment: .leading, spacing: 5) {
                // Headline row: Title + Status Capsule
                HStack(alignment: .center, spacing: 8) {
                    Text(resolvedTitle(context: context))
                        .font(.system(size: 16, weight: .bold))
                        .foregroundColor(titleColor)
                        .lineLimit(1)
                    
                    Spacer(minLength: 6)
                    
                    statusBadge
                }
                
                // Middle row: Current Action / Subtask
                if context.state.runningTaskCount > 0 {
                    compositeRunningTasksBlock
                } else {
                    HStack(alignment: .top, spacing: 6) {
                        let iconInfo = dynamicActionIcon(for: context.state.latestAction, hasPendingAction: context.state.hasPendingAction, hasRunningTasks: false)
                        Image(systemName: iconInfo.name)
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(iconInfo.color)
                            .padding(.top, 1.5)
                        
                        Text(renderMarkdownText(context.state.latestAction, defaultColor: actionTextColor, isDark: isDark))
                            .font(.system(size: 13.5, weight: .medium))
                            .lineLimit(2)
                            .lineSpacing(2)
                    }
                    .padding(.vertical, 1)
                }
                
                // Footer row: Step Count + Elapsed Time + Task count
                HStack(spacing: 6) {
                    Text("当前第 \(context.state.stepCount) 步")
                        .font(.system(size: 11.5, weight: .bold, design: .monospaced))
                        .foregroundColor(accentColor)
                    
                    Text("·")
                        .font(.system(size: 11.5))
                        .foregroundColor(secondaryTextColor)
                    
                    Text(context.state.lastUpdated, style: .relative)
                        .font(.system(size: 11.5))
                        .foregroundColor(secondaryTextColor)
                    
                    if context.state.runningTaskCount > 0 {
                        Text("·")
                            .font(.system(size: 11.5))
                            .foregroundColor(secondaryTextColor)
                        Text("\(context.state.runningTaskCount) 个任务")
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(secondaryTextColor)
                    }
                    
                    Spacer(minLength: 4)
                    
                    if context.state.hasPendingAction {
                        if context.state.requiresInAppReview {
                            HStack(spacing: 3) {
                                Image(systemName: "arrow.up.forward.app")
                                    .font(.system(size: 9.5, weight: .semibold))
                                Text("进 App 确认")
                                    .font(.system(size: 10.5, weight: .bold))
                            }
                            .foregroundColor(isDark ? Color.orange : Color(red: 0.78, green: 0.42, blue: 0.0))
                            .padding(.horizontal, 9)
                            .padding(.vertical, 3.5)
                            .background(Color.orange.opacity(isDark ? 0.22 : 0.14))
                            .clipShape(Capsule())
                        } else {
                            let stepIdx = context.state.pendingStepIndex ?? -1
                            HStack(spacing: 6) {
                                Button(intent: ApproveAgentActionIntent(cascadeId: context.attributes.cascadeId, expectedStepIndex: stepIdx, gatewayId: context.attributes.gatewayId ?? "")) {
                                    Text("允许")
                                        .font(.system(size: 11, weight: .bold))
                                        .foregroundColor(.white)
                                        .padding(.horizontal, 10)
                                        .padding(.vertical, 3.5)
                                        .background(Color(red: 0.12, green: 0.68, blue: 0.34))
                                        .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)
                                
                                Button(intent: RejectAgentActionIntent(cascadeId: context.attributes.cascadeId, expectedStepIndex: stepIdx, gatewayId: context.attributes.gatewayId ?? "")) {
                                    Text("拒绝")
                                        .font(.system(size: 11, weight: .bold))
                                        .foregroundColor(isDark ? Color(red: 1.0, green: 0.55, blue: 0.55) : Color(red: 0.78, green: 0.14, blue: 0.14))
                                        .padding(.horizontal, 9)
                                        .padding(.vertical, 3.5)
                                        .background(isDark ? Color.red.opacity(0.24) : Color(red: 1.0, green: 0.90, blue: 0.90))
                                        .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    } else if context.state.status == "RUNNING" {
                        Button(intent: StopAgentRunIntent(cascadeId: context.attributes.cascadeId, gatewayId: context.attributes.gatewayId ?? "")) {
                            HStack(spacing: 3) {
                                Image(systemName: "stop.fill")
                                    .font(.system(size: 8.5))
                                Text("停止")
                                    .font(.system(size: 10.5, weight: .bold))
                            }
                            .foregroundColor(isDark ? Color(red: 1.0, green: 0.55, blue: 0.55) : Color(red: 0.78, green: 0.14, blue: 0.14))
                            .padding(.horizontal, 8)
                            .padding(.vertical, 3)
                            .background(isDark ? Color.red.opacity(0.20) : Color(red: 1.0, green: 0.92, blue: 0.92))
                            .clipShape(Capsule())
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .activityBackgroundTint(
            isDark
                ? Color(red: 0.110, green: 0.110, blue: 0.118)
                : Color.white
        )
        .activitySystemActionForegroundColor(
            isDark ? Color.white : Color(red: 0.10, green: 0.10, blue: 0.12)
        )
    }
    
    // MARK: - Avatar Badge (Frameless Floating Logo)
    @ViewBuilder
    private var avatarBadge: some View {
        ZStack {
            if context.state.hasPendingAction {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.system(size: 26))
                    .foregroundColor(.orange)
            } else if context.state.status == "COMPLETED" {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 26))
                    .foregroundColor(
                        isDark ? Color(red: 0.45, green: 0.90, blue: 0.55) : Color(red: 0.08, green: 0.55, blue: 0.25)
                    )
            } else {
                Image("AppLogoTransparent")
                    .renderingMode(.original)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 40, height: 26)
            }
        }
        .frame(width: 44, height: 44)
    }
    
    // MARK: - Status Badge
    @ViewBuilder
    private var statusBadge: some View {
        if context.state.hasPendingAction {
            Text("待审批")
                .font(.system(size: 11.5, weight: .bold))
                .foregroundColor(
                    isDark ? Color(red: 1.0, green: 0.70, blue: 0.40) : Color(red: 0.78, green: 0.28, blue: 0.0)
                )
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(
                    isDark ? Color.orange.opacity(0.22) : Color(red: 1.0, green: 0.93, blue: 0.86)
                )
                .cornerRadius(5)
        } else if context.state.runningTaskCount > 0 {
            Text("\(context.state.runningTaskCount) 任务运行中")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(
                    isDark ? Color(red: 0.20, green: 0.78, blue: 1.0) : Color(red: 0.0, green: 0.44, blue: 0.75)
                )
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(
                    isDark ? Color.cyan.opacity(0.22) : Color(red: 0.88, green: 0.95, blue: 1.0)
                )
                .cornerRadius(5)
        } else {
            let isRunning = context.state.status == "RUNNING"
            Text(isRunning ? String(localized: "Agent 执行中") : String(localized: "已就绪"))
                .font(.system(size: 11.5, weight: .bold))
                .foregroundColor(
                    isRunning
                        ? (isDark ? Color(red: 0.62, green: 0.74, blue: 1.0) : Color(red: 0.16, green: 0.30, blue: 0.82))
                        : (isDark ? Color(red: 0.45, green: 0.90, blue: 0.55) : Color(red: 0.08, green: 0.55, blue: 0.25))
                )
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(
                    isRunning
                        ? (isDark ? Color.indigo.opacity(0.22) : Color(red: 0.91, green: 0.93, blue: 1.0))
                        : (isDark ? Color.green.opacity(0.22) : Color(red: 0.89, green: 0.96, blue: 0.91))
                )
                .cornerRadius(5)
        }
    }
    
    // MARK: - Composite Running Tasks + Agent Action Block
    @ViewBuilder
    private var compositeRunningTasksBlock: some View {
        let displayedTask: AgentTaskSnapshot = context.state.runningTasks.first
            ?? AgentTaskSnapshot(id: "fallback", title: context.state.activeTaskTitle ?? String(localized: "后台任务"), command: context.state.activeTaskCommand)
        let extraCount = max(0, context.state.runningTaskCount - 1)
        
        VStack(alignment: .leading, spacing: 3.5) {
            // Row 1: Compact Task Chip
            HStack(spacing: 5) {
                Image(systemName: displayedTask.isWaiting ? "hourglass" : "terminal.fill")
                    .font(.system(size: 9.5))
                    .foregroundColor(
                        displayedTask.isWaiting
                            ? (isDark ? Color.orange : Color(red: 0.85, green: 0.35, blue: 0.0))
                            : (isDark ? Color(red: 0.20, green: 0.78, blue: 1.0) : Color(red: 0.0, green: 0.44, blue: 0.75))
                    )
                
                if let cmd = displayedTask.command, !cmd.isEmpty {
                    Text(redactLiveActivityCommand(cmd))
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .lineLimit(1)
                        .foregroundColor(titleColor)
                        .privacySensitive()
                } else {
                    Text(displayedTask.title)
                        .font(.system(size: 11, weight: .medium))
                        .lineLimit(1)
                        .foregroundColor(titleColor)
                }
                
                Spacer(minLength: 4)
                
                if extraCount > 0 {
                    Text("+\(extraCount)")
                        .font(.system(size: 9, weight: .bold, design: .monospaced))
                        .foregroundColor(secondaryTextColor)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 1)
                        .background(isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.05))
                        .cornerRadius(3)
                }
                
                Text(displayedTask.isWaiting ? String(localized: "等待") : String(localized: "运行中"))
                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                    .lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
                    .foregroundColor(
                        displayedTask.isWaiting
                            ? (isDark ? Color(red: 1.0, green: 0.70, blue: 0.40) : Color(red: 0.78, green: 0.28, blue: 0.0))
                            : (isDark ? Color(red: 0.20, green: 0.78, blue: 1.0) : Color(red: 0.0, green: 0.44, blue: 0.75))
                    )
                    .padding(.horizontal, 4.5)
                    .padding(.vertical, 1.5)
                    .background(
                        displayedTask.isWaiting
                            ? (isDark ? Color.orange.opacity(0.18) : Color(red: 1.0, green: 0.93, blue: 0.86))
                            : (isDark ? Color.cyan.opacity(0.18) : Color(red: 0.88, green: 0.95, blue: 1.0))
                    )
                    .cornerRadius(3)
            }
            .padding(.horizontal, 7)
            .padding(.vertical, 3.5)
            .background(
                isDark ? Color.white.opacity(0.08) : Color(red: 0.94, green: 0.95, blue: 0.97)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 6, style: .continuous)
                    .stroke(
                        isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.06),
                        lineWidth: 0.6
                    )
            )
            .cornerRadius(6)
            
            // Row 2: Agent Thought / Action with Markdown
            if !context.state.latestAction.isEmpty {
                HStack(alignment: .center, spacing: 5) {
                    Image(systemName: "sparkles")
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(accentColor)
                    
                    Text(renderMarkdownText(context.state.latestAction, defaultColor: actionTextColor, isDark: isDark))
                        .font(.system(size: 12, weight: .medium))
                        .lineLimit(1)
                }
                .padding(.leading, 2)
            }
        }
    }
}

// MARK: - Helper Functions

private func dynamicActionIcon(for text: String, hasPendingAction: Bool, hasRunningTasks: Bool = false) -> (name: String, color: Color) {
    if hasPendingAction {
        return ("exclamationmark.circle.fill", .orange)
    }
    if hasRunningTasks {
        return ("bolt.fill", Color(red: 0.20, green: 0.78, blue: 1.0))
    }
    let lower = text.lowercased()
    if lower.contains("error") || lower.contains("fail") || text.contains("失败") || text.contains("错误") {
        return ("xmark.circle.fill", Color.red)
    }
    if lower.contains("success") || lower.contains("finish") || text.contains("完成") || text.contains("修复") || text.contains("成功") {
        return ("checkmark.circle.fill", Color(red: 0.25, green: 0.85, blue: 0.45))
    }
    if lower.contains("build") || lower.contains("compile") || lower.contains("git") || text.contains("编译") || text.contains("终端") || text.contains("命令") {
        return ("terminal.fill", Color(red: 0.35, green: 0.75, blue: 1.0))
    }
    return ("sparkles", Color(red: 0.42, green: 0.68, blue: 1.0))
}

private func autoCloseInlineMarkdown(_ text: String) -> String {
    var result = text
    let boldCount = result.components(separatedBy: "**").count - 1
    if boldCount % 2 != 0 {
        result += "**"
    }
    let codeCount = result.components(separatedBy: "`").count - 1
    if codeCount % 2 != 0 {
        result += "`"
    }
    let singleAsteriskCount = result.replacingOccurrences(of: "**", with: "").components(separatedBy: "*").count - 1
    if singleAsteriskCount % 2 != 0 {
        result += "*"
    }
    let strikeCount = result.components(separatedBy: "~~").count - 1
    if strikeCount % 2 != 0 {
        result += "~~"
    }
    return result
}

private func renderMarkdownText(_ rawText: String, defaultColor: Color, isDark: Bool) -> AttributedString {
    let trimmed = rawText.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else {
        return AttributedString("")
    }
    
    // Clean block markers (titles, quotes, list bullets)
    var cleaned = trimmed
    cleaned = cleaned.replacingOccurrences(of: #"(?m)^#{1,6}\s+"#, with: "", options: .regularExpression)
    cleaned = cleaned.replacingOccurrences(of: #"(?m)^>\s+"#, with: "", options: .regularExpression)
    cleaned = cleaned.replacingOccurrences(of: #"(?m)^[-*+]\s+"#, with: "", options: .regularExpression)
    cleaned = cleaned.replacingOccurrences(of: #"\n{2,}"#, with: "\n", options: .regularExpression)
    cleaned = autoCloseInlineMarkdown(cleaned)
    
    var options = AttributedString.MarkdownParsingOptions()
    options.interpretedSyntax = .inlineOnlyPreservingWhitespace
    
    guard var attr = try? AttributedString(markdown: cleaned, options: options) else {
        var fallback = AttributedString(cleaned)
        fallback.foregroundColor = defaultColor
        return fallback
    }
    
    let codeBgColor = isDark ? Color(white: 0.22) : Color(red: 0.92, green: 0.93, blue: 0.96)
    let codeFgColor = isDark ? Color(red: 1.0, green: 0.82, blue: 0.45) : Color(red: 0.75, green: 0.40, blue: 0.05)
    let boldFgColor = isDark ? Color(red: 0.98, green: 0.98, blue: 1.0) : Color(red: 0.05, green: 0.06, blue: 0.08)
    
    for run in attr.runs {
        let runRange = run.range
        if let intent = run.inlinePresentationIntent {
            if intent.contains(.code) {
                attr[runRange].foregroundColor = codeFgColor
                attr[runRange].backgroundColor = codeBgColor
                attr[runRange].font = .system(size: 12.5, weight: .medium, design: .monospaced)
            } else if intent.contains(.stronglyEmphasized) {
                attr[runRange].foregroundColor = boldFgColor
                attr[runRange].font = .system(size: 13.5, weight: .bold)
            } else if intent.contains(.emphasized) {
                attr[runRange].font = .system(size: 13.5).italic()
            }
        } else {
            if attr[runRange].foregroundColor == nil {
                attr[runRange].foregroundColor = defaultColor
            }
        }
    }
    return attr
}

private func resolvedTitle(context: ActivityViewContext<AgentActivityAttributes>) -> String {
    let dynamicTitle = context.state.conversationTitle.trimmingCharacters(in: .whitespacesAndNewlines)
    if !dynamicTitle.isEmpty {
        return dynamicTitle
    }
    let attrTitle = context.attributes.conversationTitle.trimmingCharacters(in: .whitespacesAndNewlines)
    if !attrTitle.isEmpty {
        return attrTitle
    }
    return "Multigravity Agent"
}
