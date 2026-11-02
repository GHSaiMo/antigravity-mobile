import SwiftUI
import WidgetKit
import ActivityKit

public struct AudioRecordingActivityWidget: Widget {
    public init() {}
    
    public var body: some WidgetConfiguration {
        ActivityConfiguration(for: AudioRecordingActivityAttributes.self) { context in
            AudioRecordingLockScreenView(context: context)
                .widgetURL(URL(string: "antigravity://cascade/\(context.attributes.cascadeId)"))
        } dynamicIsland: { context in
            DynamicIsland {
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
    
    // MARK: - Dynamic Island Views
    
    @ViewBuilder
    private func compactLeadingView(context: ActivityViewContext<AudioRecordingActivityAttributes>) -> some View {
        Image("AppLogoTransparent")
            .renderingMode(.original)
            .resizable()
            .aspectRatio(contentMode: .fit)
            .frame(width: 20, height: 13)
    }
    
    @ViewBuilder
    private func compactTrailingView(context: ActivityViewContext<AudioRecordingActivityAttributes>) -> some View {
        if context.state.isPaused {
            Text(formatRecordingDuration(context.state.pausedDuration))
                .font(.system(size: 13, weight: .semibold, design: .rounded))
                .monospacedDigit()
                .foregroundColor(.orange)
                .frame(width: context.state.pausedDuration >= 3600 ? 62 : 44, alignment: .trailing)
        } else {
            let isOverHour = Date().timeIntervalSince(context.state.recordingStartDate) >= 3600
            Text(isOverHour ? "0:00:00" : "00:00")
                .font(.system(size: 13, weight: .semibold, design: .rounded))
                .monospacedDigit()
                .hidden()
                .overlay(
                    Text(context.state.recordingStartDate, style: .timer)
                        .font(.system(size: 13, weight: .semibold, design: .rounded))
                        .monospacedDigit()
                        .foregroundColor(.white),
                    alignment: .trailing
                )
                .frame(width: isOverHour ? 62 : 44, alignment: .trailing)
        }
    }
    
    @ViewBuilder
    private func minimalView(context: ActivityViewContext<AudioRecordingActivityAttributes>) -> some View {
        Image("AppLogoTransparent")
            .renderingMode(.original)
            .resizable()
            .aspectRatio(contentMode: .fit)
            .frame(width: 18, height: 12)
    }
    
    @ViewBuilder
    private func expandedLeadingView(context: ActivityViewContext<AudioRecordingActivityAttributes>) -> some View {
        HStack(spacing: 8) {
            ZStack {
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .fill(Color.white.opacity(0.12))
                Image("AppLogoTransparent")
                    .renderingMode(.original)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 18, height: 12)
            }
            .frame(width: 28, height: 28)
            
            Text(resolvedRecordingTitle(context.state.conversationTitle))
                .font(.system(size: 15, weight: .bold))
                .foregroundColor(.white)
                .lineLimit(1)
        }
        .padding(.leading, 4)
    }
    
    @ViewBuilder
    private func expandedTrailingView(context: ActivityViewContext<AudioRecordingActivityAttributes>) -> some View {
        Group {
            if context.state.isPaused {
                Text(formatRecordingDuration(context.state.pausedDuration))
                    .font(.system(size: 15, weight: .semibold, design: .rounded))
                    .monospacedDigit()
                    .foregroundColor(.orange)
            } else {
                Text(context.state.recordingStartDate, style: .timer)
                    .font(.system(size: 15, weight: .semibold, design: .rounded))
                    .monospacedDigit()
                    .foregroundColor(Color.white.opacity(0.95))
            }
        }
        .padding(.trailing, 4)
    }
    
    @ViewBuilder
    private func expandedBottomView(context: ActivityViewContext<AudioRecordingActivityAttributes>) -> some View {
        HStack(spacing: 12) {
            Link(destination: URL(string: "antigravity://recording?action=stop&cascadeId=\(context.state.cascadeId)")!) {
                HStack(spacing: 6) {
                    Image(systemName: "stop.circle.fill")
                        .font(.system(size: 15, weight: .semibold))
                    Text("结束录音")
                        .font(.system(size: 14.5, weight: .semibold))
                }
                .foregroundColor(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 42)
                .background(Color.white.opacity(0.18))
                .clipShape(Capsule())
            }
            
            Link(destination: URL(string: "antigravity://recording?action=send&cascadeId=\(context.state.cascadeId)")!) {
                HStack(spacing: 6) {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.system(size: 15, weight: .semibold))
                    Text("结束并发送")
                        .font(.system(size: 14.5, weight: .semibold))
                }
                .foregroundColor(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 42)
                .background(Color.white.opacity(0.24))
                .clipShape(Capsule())
            }
        }
        .padding(.horizontal, 4)
        .padding(.top, 6)
        .padding(.bottom, 2)
    }
}

// MARK: - Lock Screen Live Activity View

public struct AudioRecordingLockScreenView: View {
    let context: ActivityViewContext<AudioRecordingActivityAttributes>
    
    public init(context: ActivityViewContext<AudioRecordingActivityAttributes>) {
        self.context = context
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            // Top Row: Logo badge + Title + Live Timer
            HStack(alignment: .center, spacing: 10) {
                ZStack {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .fill(Color.white.opacity(0.12))
                        .overlay(
                            RoundedRectangle(cornerRadius: 10, style: .continuous)
                                .stroke(Color.white.opacity(0.1), lineWidth: 0.8)
                        )
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 24, height: 16)
                }
                .frame(width: 36, height: 36)
                
                Text(resolvedRecordingTitle(context.state.conversationTitle))
                    .font(.system(size: 16, weight: .bold))
                    .foregroundColor(.white)
                    .lineLimit(1)
                
                Spacer(minLength: 8)
                
                if context.state.isPaused {
                    Text(formatRecordingDuration(context.state.pausedDuration))
                        .font(.system(size: 16, weight: .semibold, design: .rounded))
                        .monospacedDigit()
                        .foregroundColor(.orange)
                } else {
                    Text(context.state.recordingStartDate, style: .timer)
                        .font(.system(size: 16, weight: .semibold, design: .rounded))
                        .monospacedDigit()
                        .foregroundColor(Color.white.opacity(0.9))
                }
            }
            
            // Bottom Row: Two buttons side by side
            HStack(spacing: 12) {
                Link(destination: URL(string: "antigravity://recording?action=stop&cascadeId=\(context.state.cascadeId)")!) {
                    HStack(spacing: 6) {
                        Image(systemName: "stop.circle.fill")
                            .font(.system(size: 16, weight: .semibold))
                        Text("结束录音")
                            .font(.system(size: 15, weight: .semibold))
                    }
                    .foregroundColor(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 44)
                    .background(Color.white.opacity(0.18))
                    .clipShape(Capsule())
                }
                
                Link(destination: URL(string: "antigravity://recording?action=send&cascadeId=\(context.state.cascadeId)")!) {
                    HStack(spacing: 6) {
                        Image(systemName: "arrow.up.circle.fill")
                            .font(.system(size: 16, weight: .semibold))
                        Text("结束并发送")
                            .font(.system(size: 15, weight: .semibold))
                    }
                    .foregroundColor(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 44)
                    .background(Color.white.opacity(0.24))
                    .clipShape(Capsule())
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 14)
        .activityBackgroundTint(Color(red: 0.12, green: 0.12, blue: 0.13))
        .activitySystemActionForegroundColor(.white)
    }
}

// MARK: - Helpers

private func resolvedRecordingTitle(_ title: String) -> String {
    let trimmed = title.trimmingCharacters(in: .whitespacesAndNewlines)
    if trimmed.isEmpty {
        return "录音中"
    }
    return "录音中 · \(trimmed)"
}

private func formatRecordingDuration(_ duration: TimeInterval) -> String {
    let totalSeconds = max(0, Int(duration))
    let minutes = totalSeconds / 60
    let seconds = totalSeconds % 60
    if totalSeconds >= 3600 {
        let hours = totalSeconds / 3600
        return String(format: "%02d:%02d:%02d", hours, minutes, seconds)
    }
    return String(format: "%02d:%02d", minutes, seconds)
}
