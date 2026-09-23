import SwiftUI
import WidgetKit
import ActivityKit

public struct AgentActivityWidget: Widget {
    public init() {}
    
    public var body: some WidgetConfiguration {
        ActivityConfiguration(for: AgentActivityAttributes.self) { context in
            // Lock Screen Live Activity View (JD-style prominent layout & Light/Dark adaptive)
            AgentLockScreenView(context: context)
                .widgetURL(URL(string: "antigravity://cascade/\(context.attributes.cascadeId)"))
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
            .widgetURL(URL(string: "antigravity://cascade/\(context.attributes.cascadeId)"))
        }
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
            Text(isRunning ? "Agent 执行中" : "已就绪")
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
                    : [AgentTaskSnapshot(id: "fallback", title: context.state.activeTaskTitle ?? "后台任务", command: context.state.activeTaskCommand)]
                
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(islandTasks) { task in
                        HStack(spacing: 6) {
                            Image(systemName: task.isWaiting ? "hourglass" : "terminal.fill")
                                .font(.system(size: 10))
                                .foregroundColor(task.isWaiting ? .orange : .cyan)
                            
                            if let cmd = task.command, !cmd.isEmpty {
                                Text(cmd)
                                    .font(.system(size: 11, design: .monospaced))
                                    .lineLimit(1)
                                    .foregroundColor(.cyan.opacity(0.95))
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
                HStack(spacing: 5) {
                    Image(systemName: context.state.hasPendingAction ? "exclamationmark.circle.fill" : "sparkles")
                        .font(.system(size: 12))
                        .foregroundColor(context.state.hasPendingAction ? .orange : Color(red: 0.6, green: 0.72, blue: 1.0))
                    
                    Text(context.state.latestAction)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.white)
                        .lineLimit(1)
                }
                .padding(.vertical, 2)
            }
        }
        .padding(.horizontal, 4)
        .padding(.top, 4)
    }
}

// MARK: - Lock Screen Live Activity View (JD-Style Layout)

struct AgentLockScreenView: View {
    let context: ActivityViewContext<AgentActivityAttributes>
    @Environment(\.colorScheme) var colorScheme
    
    private var isDark: Bool {
        colorScheme == .dark
    }
    
    private var accentColor: Color {
        isDark
            ? Color(red: 0.55, green: 0.68, blue: 1.0)
            : Color(red: 0.28, green: 0.38, blue: 0.95)
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
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    
                    Spacer(minLength: 6)
                    
                    statusBadge
                }
                
                // Middle row: Current Action / Subtask
                if context.state.runningTaskCount > 0 {
                    runningTasksBlock
                } else {
                    HStack(alignment: .center, spacing: 6) {
                        Image(systemName: context.state.hasPendingAction ? "exclamationmark.circle.fill" : "sparkles")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(context.state.hasPendingAction ? .orange : accentColor)
                        
                        Text(context.state.latestAction)
                            .font(.system(size: 13.5, weight: .medium))
                            .foregroundColor(isDark ? Color(white: 0.9) : Color(white: 0.25))
                            .lineLimit(1)
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
                        .foregroundColor(.secondary)
                    
                    Text(context.state.lastUpdated, style: .relative)
                        .font(.system(size: 11.5))
                        .foregroundColor(.secondary)
                    
                    if context.state.runningTaskCount > 0 {
                        Text("·")
                            .font(.system(size: 11.5))
                            .foregroundColor(.secondary)
                        Text("\(context.state.runningTaskCount) 个任务")
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(.secondary)
                    }
                    
                    Spacer()
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .activityBackgroundTint(Color("LiveActivityBackground"))
        .activitySystemActionForegroundColor(Color("LiveActivitySystemAction"))
    }
    
    // MARK: - Avatar Badge
    @ViewBuilder
    private var avatarBadge: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(
                    isDark
                        ? Color.white.opacity(0.12)
                        : Color(red: 0.95, green: 0.96, blue: 0.98)
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(
                            isDark ? Color.white.opacity(0.15) : Color.black.opacity(0.06),
                            lineWidth: 0.8
                        )
                )
            
            if context.state.hasPendingAction {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.system(size: 26))
                    .foregroundColor(.orange)
            } else if context.state.runningTaskCount > 0 {
                Image(systemName: "terminal.fill")
                    .font(.system(size: 24))
                    .foregroundColor(Color(red: 0.0, green: 0.65, blue: 0.9))
            } else if context.state.status == "COMPLETED" {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 26))
                    .foregroundColor(.green)
            } else {
                Image("AppLogoTransparent")
                    .renderingMode(.original)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 36, height: 23)
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
                .foregroundColor(.orange)
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(Color.orange.opacity(isDark ? 0.22 : 0.12))
                .cornerRadius(5)
        } else if context.state.runningTaskCount > 0 {
            Text("\(context.state.runningTaskCount) 任务运行中")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(Color(red: 0.0, green: 0.65, blue: 0.9))
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(Color.cyan.opacity(isDark ? 0.22 : 0.12))
                .cornerRadius(5)
        } else {
            let isRunning = context.state.status == "RUNNING"
            Text(isRunning ? "Agent 执行中" : "已就绪")
                .font(.system(size: 11.5, weight: .bold))
                .foregroundColor(
                    isRunning
                        ? (isDark ? Color(red: 0.6, green: 0.72, blue: 1.0) : Color(red: 0.28, green: 0.38, blue: 0.95))
                        : .green
                )
                .padding(.horizontal, 7)
                .padding(.vertical, 3)
                .background(
                    (isRunning ? Color.indigo : Color.green).opacity(isDark ? 0.22 : 0.12)
                )
                .cornerRadius(5)
        }
    }
    
    // MARK: - Multi-task Block
    @ViewBuilder
    private var runningTasksBlock: some View {
        let displayedTasks: [AgentTaskSnapshot] = !context.state.runningTasks.isEmpty
            ? Array(context.state.runningTasks.prefix(2))
            : [AgentTaskSnapshot(id: "fallback", title: context.state.activeTaskTitle ?? "后台任务", command: context.state.activeTaskCommand)]
        
        VStack(alignment: .leading, spacing: 4) {
            ForEach(displayedTasks) { task in
                HStack(spacing: 6) {
                    Image(systemName: task.isWaiting ? "hourglass" : "terminal.fill")
                        .font(.system(size: 10))
                        .foregroundColor(task.isWaiting ? .orange : Color(red: 0.0, green: 0.65, blue: 0.9))
                    
                    if let cmd = task.command, !cmd.isEmpty {
                        Text(cmd)
                            .font(.system(size: 11.5, weight: .medium, design: .monospaced))
                            .lineLimit(1)
                            .foregroundColor(.primary)
                    } else {
                        Text(task.title)
                            .font(.system(size: 11.5, weight: .medium))
                            .lineLimit(1)
                            .foregroundColor(.primary)
                    }
                    
                    Spacer(minLength: 4)
                    
                    if task.isWaiting {
                        Text("等待中")
                            .font(.system(size: 9.5, weight: .bold))
                            .foregroundColor(.orange)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1.5)
                            .background(Color.orange.opacity(isDark ? 0.2 : 0.12))
                            .cornerRadius(3)
                    } else {
                        Text("运行中")
                            .font(.system(size: 9.5, weight: .bold, design: .monospaced))
                            .foregroundColor(Color(red: 0.0, green: 0.65, blue: 0.9))
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1.5)
                            .background(Color.cyan.opacity(isDark ? 0.2 : 0.12))
                            .cornerRadius(3)
                    }
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4.5)
                .background(
                    isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.04)
                )
                .cornerRadius(6)
            }
            
            let remainingCount = max(0, context.state.runningTaskCount - displayedTasks.count)
            if remainingCount > 0 {
                HStack(spacing: 4) {
                    Image(systemName: "ellipsis.circle")
                        .font(.system(size: 10))
                        .foregroundColor(.secondary)
                    Text("另有 \(remainingCount) 个任务在队列中...")
                        .font(.system(size: 10.5))
                        .foregroundColor(.secondary)
                }
                .padding(.leading, 4)
            }
        }
    }
}

// MARK: - Helper Functions

private func resolvedTitle(context: ActivityViewContext<AgentActivityAttributes>) -> String {
    let dynamicTitle = context.state.conversationTitle.trimmingCharacters(in: .whitespacesAndNewlines)
    if !dynamicTitle.isEmpty {
        return dynamicTitle
    }
    let attrTitle = context.attributes.conversationTitle.trimmingCharacters(in: .whitespacesAndNewlines)
    if !attrTitle.isEmpty {
        return attrTitle
    }
    return "Antigravity Agent"
}
