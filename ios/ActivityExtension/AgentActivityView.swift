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
                        Image(systemName: context.state.runningTaskCount > 0 ? "terminal.fill" : "brain.head.profile")
                            .font(.system(size: 16))
                            .foregroundColor(context.state.runningTaskCount > 0 ? .cyan : .indigo)
                        Text(context.attributes.conversationTitle)
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
                        
                        Text(context.state.latestAction)
                            .font(.system(size: 12))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                        
                        // Dedicated Background Running Task Section
                        if context.state.runningTaskCount > 0 {
                            HStack(spacing: 6) {
                                Image(systemName: "terminal.fill")
                                    .font(.system(size: 10))
                                    .foregroundColor(.cyan)
                                
                                if let cmd = context.state.activeTaskCommand, !cmd.isEmpty {
                                    Text(cmd)
                                        .font(.system(size: 11, design: .monospaced))
                                        .lineLimit(1)
                                        .foregroundColor(.cyan.opacity(0.95))
                                } else if let title = context.state.activeTaskTitle, !title.isEmpty {
                                    Text(title)
                                        .font(.system(size: 11))
                                        .lineLimit(1)
                                        .foregroundColor(.primary)
                                } else {
                                    Text("后台任务执行中...")
                                        .font(.system(size: 11))
                                        .foregroundColor(.secondary)
                                }
                                
                                Spacer()
                                
                                if context.state.runningTaskCount > 1 {
                                    Text("+\(context.state.runningTaskCount - 1)")
                                        .font(.system(size: 10, weight: .bold, design: .monospaced))
                                        .foregroundColor(.cyan)
                                        .padding(.horizontal, 4)
                                        .padding(.vertical, 1)
                                        .background(Color.cyan.opacity(0.2))
                                        .cornerRadius(4)
                                }
                            }
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .background(Color.white.opacity(0.08))
                            .cornerRadius(6)
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
                    Image(systemName: "brain.head.profile")
                        .font(.system(size: 12))
                        .foregroundColor(.indigo)
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
                    Image(systemName: "brain.head.profile")
                        .font(.system(size: 12))
                        .foregroundColor(.indigo)
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
    
    @ViewBuilder
    private func lockScreenView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        HStack(alignment: .center, spacing: 14) {
            ZStack {
                Circle()
                    .fill((context.state.runningTaskCount > 0 ? Color.cyan : Color.indigo).opacity(0.15))
                    .frame(width: 44, height: 44)
                Image(systemName: context.state.runningTaskCount > 0 ? "terminal.fill" : "brain.head.profile")
                    .font(.system(size: 20))
                    .foregroundColor(context.state.runningTaskCount > 0 ? .cyan : .indigo)
            }
            
            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(context.attributes.conversationTitle)
                        .font(.system(size: 15, weight: .bold))
                        .lineLimit(1)
                    Spacer()
                    statusBadgeView(context: context)
                }
                
                Text(context.state.latestAction)
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                
                if context.state.runningTaskCount > 0 {
                    HStack(spacing: 6) {
                        Image(systemName: "terminal.fill")
                            .font(.system(size: 10))
                            .foregroundColor(.cyan)
                        if let cmd = context.state.activeTaskCommand, !cmd.isEmpty {
                            Text(cmd)
                                .font(.system(size: 11, design: .monospaced))
                                .lineLimit(1)
                                .foregroundColor(.primary)
                        } else if let title = context.state.activeTaskTitle, !title.isEmpty {
                            Text(title)
                                .font(.system(size: 11))
                                .lineLimit(1)
                                .foregroundColor(.primary)
                        }
                        Spacer()
                        if context.state.runningTaskCount > 1 {
                            Text("+\(context.state.runningTaskCount - 1) 个任务")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.cyan)
                        }
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 3)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .cornerRadius(5)
                }
                
                HStack {
                    Text("当前第 \(context.state.stepCount) 步")
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .foregroundColor(context.state.runningTaskCount > 0 ? .cyan : .indigo)
                    Spacer()
                    Text(context.state.lastUpdated, style: .relative)
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                }
            }
        }
        .padding(16)
        .activityBackgroundTint(Color.black.opacity(0.85))
    }
}
