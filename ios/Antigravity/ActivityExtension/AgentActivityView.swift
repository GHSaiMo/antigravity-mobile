import SwiftUI
import WidgetKit
import ActivityKit

public struct AgentActivityWidget: Widget {
    public init() {}
    
    public var body: some WidgetConfiguration {
        ActivityConfiguration(for: AgentActivityAttributes.self) { context in
            // Lock Screen / Banner UI
            lockScreenView(context: context)
        } dynamicIsland: { context in
            DynamicIsland {
                // Expanded UI
                DynamicIslandExpandedRegion(.leading) {
                    HStack(spacing: 6) {
                        Image(systemName: "brain.head.profile")
                            .font(.system(size: 18))
                            .foregroundColor(.blue)
                        Text(context.attributes.conversationTitle)
                            .font(.system(size: 14, weight: .bold))
                            .lineLimit(1)
                    }
                    .padding(.leading, 4)
                }
                
                DynamicIslandExpandedRegion(.trailing) {
                    Text(context.state.status)
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(context.state.status == "RUNNING" ? .blue : .green)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(
                            (context.state.status == "RUNNING" ? Color.blue : Color.green).opacity(0.15)
                        )
                        .cornerRadius(4)
                }
                
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text("步骤 \(context.state.stepCount)")
                                .font(.system(size: 12, weight: .semibold, design: .monospaced))
                                .foregroundColor(.secondary)
                            Spacer()
                            Text(context.state.lastUpdated, style: .time)
                                .font(.system(size: 11))
                                .foregroundColor(.secondary)
                        }
                        
                        Text(context.state.latestAction)
                            .font(.system(size: 13))
                            .foregroundColor(.primary)
                            .lineLimit(2)
                    }
                    .padding(.horizontal, 4)
                    .padding(.top, 4)
                }
            } compactLeading: {
                Image(systemName: "brain.head.profile")
                    .font(.system(size: 12))
                    .foregroundColor(.blue)
            } compactTrailing: {
                Text("\(context.state.stepCount)")
                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                    .foregroundColor(.blue)
            } minimal: {
                Image(systemName: "brain.head.profile")
                    .font(.system(size: 12))
                    .foregroundColor(.blue)
            }
        }
    }
    
    @ViewBuilder
    private func lockScreenView(context: ActivityViewContext<AgentActivityAttributes>) -> some View {
        HStack(alignment: .center, spacing: 14) {
            ZStack {
                Circle()
                    .fill(Color.blue.opacity(0.15))
                    .frame(width: 44, height: 44)
                Image(systemName: "brain.head.profile")
                    .font(.system(size: 22))
                    .foregroundColor(.blue)
            }
            
            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(context.attributes.conversationTitle)
                        .font(.system(size: 15, weight: .bold))
                        .lineLimit(1)
                    Spacer()
                    Text(context.state.status)
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(context.state.status == "RUNNING" ? .blue : .green)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(
                            (context.state.status == "RUNNING" ? Color.blue : Color.green).opacity(0.15)
                        )
                        .cornerRadius(4)
                }
                
                Text(context.state.latestAction)
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                
                HStack {
                    Text("当前第 \(context.state.stepCount) 步")
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .foregroundColor(.blue)
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
