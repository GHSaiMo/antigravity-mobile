import SwiftUI
import WidgetKit
import ActivityKit

public struct AgentActivityWidget: Widget {
    public init() {}
    
    public var body: some WidgetConfiguration {
        ActivityConfiguration(for: AgentActivityAttributes.self) { context in
            // Lock Screen / Banner UI
            lockScreenView(context: context)
                .widgetURL(URL(string: "antigravity://cascade/\(context.attributes.cascadeId)"))
        } dynamicIsland: { context in
            DynamicIsland {
                // Expanded UI
                DynamicIslandExpandedRegion(.leading) {
                    HStack(spacing: 6) {
                        if context.state.runningTaskCount > 0 {
                            Image(systemName: "terminal.fill")
                                .font(.system(size: 15))
                                .foregroundColor(.cyan)
                        } else {
                            Image("AppLogoTransparent")
                                .renderingMode(.original)
                                .resizable()
                                .aspectRatio(contentMode: .fit)
                                .frame(width: 18, height: 18)
                        }
                        Text(resolvedTitle(context: context))
                            .font(.system(size: 14, weight: .bold))
                            .lineLimit(1)
                    }
                    .padding(.leading, 4)
                }
                
                DynamicIslandExpandedRegion(.trailing) {
                    statusBadgeView(context: context)
                }
                
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 5) {
                        HStack {
                            Text("步骤 \(context.state.stepCount)")
                                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                .foregroundColor(.secondary)
                            Spacer()
                            Text(context.state.lastUpdated, style: .relative)
                                .font(.system(size: 11))
                                .foregroundColor(.secondary)
                        }
                        
                        if context.state.runningTaskCount > 0 {
                            let islandTasks: [AgentTaskSnapshot] = !context.state.runningTasks.isEmpty
                                ? Array(context.state.runningTasks.prefix(2))
                                : [AgentTaskSnapshot(id: "fallback", title: context.state.activeTaskTitle ?? "后台任务", command: context.state.activeTaskCommand)]
                            
                            VStack(alignment: .leading, spacing: 3) {
                                ForEach(islandTasks) { task in
                                    HStack(spacing: 5) {
                                        Image(systemName: task.isWaiting ? "hourglass" : "terminal.fill")
                                            .font(.system(size: 9))
                                            .foregroundColor(task.isWaiting ? .orange : .cyan)
                                        
                                        if let cmd = task.command, !cmd.isEmpty {
                                            Text(cmd)
                                                .font(.system(size: 10, design: .monospaced))
                                                .lineLimit(1)
                                                .foregroundColor(.cyan.opacity(0.95))
                                        } else {
                                            Text(task.title)
                                                .font(.system(size: 10))
                                                .lineLimit(1)
                                                .foregroundColor(.primary)
                                        }
                                        
                                        Spacer()
                                    }
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 3)
                                    .background(Color.white.opacity(0.08))
                                    .cornerRadius(5)
                                }
                                
                                let islandRemaining = max(0, context.state.runningTaskCount - islandTasks.count)
                                if islandRemaining > 0 {
                                    HStack {
                                        Spacer()
                                        Text("+\(islandRemaining) 个任务排队中")
                                            .font(.system(size: 9, weight: .medium))
                                            .foregroundColor(.secondary)
                                    }
                                }
                            }
                        } else {
                            Text(context.state.latestAction)
                                .font(.system(size: 12))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                        }
                    }
                    .padding(.horizontal, 4)
                    .padding(.top, 4)
                }
            } compactLeading: {
                if context.state.hasPendingAction {
                    Image(systemName: "exclamationmark.circle.fill")
                        .font(.system(size: 12))
                        .foregroundColor(.orange)
                } else if context.state.runningTaskCount > 0 {
                    Image(systemName: "terminal.fill")
                        .font(.system(size: 11))
                        .foregroundColor(.cyan)
                } else {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 14, height: 14)
                }
            } compactTrailing: {
                if context.state.hasPendingAction {
                    Text("待确认")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(.orange)
                } else if context.state.runningTaskCount > 0 {
                    HStack(spacing: 2) {
                        Image(systemName: "bolt.fill")
                            .font(.system(size: 9))
                        Text("\(context.state.runningTaskCount)")
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                    }
                    .foregroundColor(.cyan)
                } else {
                    Text("\(context.state.stepCount)")
                        .font(.system(size: 12, weight: .bold, design: .monospaced))
                        .foregroundColor(.indigo)
                }
            } minimal: {
                if context.state.hasPendingAction {
                    Image(systemName: "exclamationmark.circle.fill")
                        .font(.system(size: 12))
                        .foregroundColor(.orange)
                } else if context.state.runningTaskCount > 0 {
                    Image(systemName: "terminal.fill")
                        .font(.system(size: 11))
                        .foregroundColor(.cyan)
                } else {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 14, height: 14)
                }
            }
            .widgetURL(URL(string: "antigravity://cascade/\(context.attributes.cascadeId)"))
        }
    }
    
    @ViewBuilder
    private func statusBadgeView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        if context.state.hasPendingAction {
            Text("待审批")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(.orange)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(Color.orange.opacity(0.18))
                .cornerRadius(4)
        } else if context.state.runningTaskCount > 0 {
            Text("\(context.state.runningTaskCount) 任务运行中")
                .font(.system(size: 10, weight: .bold))
                .foregroundColor(.cyan)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(Color.cyan.opacity(0.18))
                .cornerRadius(4)
        } else {
            let isRunning = context.state.status == "RUNNING"
            Text(isRunning ? "Agent 执行中" : "已就绪")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(isRunning ? .indigo : .green)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background((isRunning ? Color.indigo : Color.green).opacity(0.15))
                .cornerRadius(4)
        }
    }
    
    private func resolvedTitle(context: ActivityViewContext<AgentActivityAttributes>) -> String {
        let dynamicTitle = context.state.conversationTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        if !dynamicTitle.isEmpty {
            return dynamicTitle
        }
        return context.attributes.conversationTitle
    }
    
    @ViewBuilder
    private func lockScreenView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            // Header Row: App Identity + Conversation Title + Status Badge
            HStack(spacing: 6) {
                if context.state.runningTaskCount > 0 {
                    Image(systemName: "terminal.fill")
                        .font(.system(size: 13))
                        .foregroundColor(.cyan)
                } else {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 16, height: 16)
                }
                
                Text(resolvedTitle(context: context))
                    .font(.system(size: 14, weight: .bold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                
                Spacer(minLength: 8)
                
                statusBadgeView(context: context)
            }
            
            // Middle Area: Multi-task list or rich action view
            if context.state.runningTaskCount > 0 {
                let displayedTasks: [AgentTaskSnapshot] = !context.state.runningTasks.isEmpty
                    ? Array(context.state.runningTasks.prefix(3))
                    : [AgentTaskSnapshot(id: "fallback", title: context.state.activeTaskTitle ?? "后台任务", command: context.state.activeTaskCommand)]
                
                VStack(alignment: .leading, spacing: 5) {
                    ForEach(displayedTasks) { task in
                        HStack(spacing: 6) {
                            Image(systemName: task.isWaiting ? "hourglass" : "terminal.fill")
                                .font(.system(size: 10))
                                .foregroundColor(task.isWaiting ? .orange : .cyan)
                            
                            if let cmd = task.command, !cmd.isEmpty {
                                Text(cmd)
                                    .font(.system(size: 11, weight: .medium, design: .monospaced))
                                    .lineLimit(1)
                                    .foregroundColor(.primary)
                            } else {
                                Text(task.title)
                                    .font(.system(size: 11))
                                    .lineLimit(1)
                                    .foregroundColor(.primary)
                            }
                            
                            Spacer(minLength: 4)
                            
                            if task.isWaiting {
                                Text("等待中")
                                    .font(.system(size: 9, weight: .bold))
                                    .foregroundColor(.orange)
                                    .padding(.horizontal, 4)
                                    .padding(.vertical, 1)
                                    .background(Color.orange.opacity(0.15))
                                    .cornerRadius(3)
                            } else {
                                Text("运行中")
                                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                                    .foregroundColor(.cyan)
                                    .padding(.horizontal, 4)
                                    .padding(.vertical, 1)
                                    .background(Color.cyan.opacity(0.15))
                                    .cornerRadius(3)
                            }
                        }
                        .padding(.horizontal, 9)
                        .padding(.vertical, 5)
                        .background(Color.white.opacity(0.06))
                        .cornerRadius(6)
                    }
                    
                    let remainingCount = max(0, context.state.runningTaskCount - displayedTasks.count)
                    if remainingCount > 0 {
                        HStack(spacing: 4) {
                            Image(systemName: "ellipsis.circle")
                                .font(.system(size: 10))
                                .foregroundColor(.secondary)
                            Text("另有 \(remainingCount) 个任务在队列中...")
                                .font(.system(size: 10))
                                .foregroundColor(.secondary)
                        }
                        .padding(.leading, 4)
                        .padding(.top, 1)
                    }
                }
            } else {
                HStack(alignment: .top, spacing: 6) {
                    Image(systemName: context.state.hasPendingAction ? "exclamationmark.circle.fill" : "sparkles")
                        .font(.system(size: 12))
                        .foregroundColor(context.state.hasPendingAction ? .orange : .indigo)
                        .padding(.top, 1)
                    
                    Text(context.state.latestAction)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.primary)
                        .lineLimit(2)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.vertical, 2)
            }
            
            // Footer Row: Step Counter + Relative Time
            HStack {
                Text("当前第 \(context.state.stepCount) 步")
                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                    .foregroundColor(context.state.runningTaskCount > 0 ? .cyan : .indigo)
                
                if context.state.runningTaskCount > 0 && !context.state.latestAction.isEmpty {
                    Text("·")
                        .foregroundColor(.secondary)
                    Text(context.state.latestAction)
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                
                Spacer()
                
                Text(context.state.lastUpdated, style: .relative)
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .activityBackgroundTint(Color.black.opacity(0.85))
    }
}
