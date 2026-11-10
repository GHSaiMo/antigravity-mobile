import SwiftUI

public struct VoiceCallCardView: View {
    @ObservedObject var voiceCall: VoiceCallManager
    let isAgentRunning: Bool
    let onStopAgentTask: () -> Void

    public init(
        voiceCall: VoiceCallManager,
        isAgentRunning: Bool,
        onStopAgentTask: @escaping () -> Void
    ) {
        self.voiceCall = voiceCall
        self.isAgentRunning = isAgentRunning
        self.onStopAgentTask = onStopAgentTask
    }

    private var effectiveTranscriptText: String {
        if !voiceCall.liveTranscriptText.isEmpty {
            return voiceCall.liveTranscriptText
        }
        switch voiceCall.callState {
        case .connecting:
            return "正在准备麦克风与连接语音服务..."
        case .reconnecting:
            return "语音服务重连中..."
        default:
            return ""
        }
    }

    private var effectiveTranscriptRole: VoiceCallManager.TranscriptRole {
        if !voiceCall.liveTranscriptText.isEmpty {
            return voiceCall.liveTranscriptRole
        }
        return .ai
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            if !effectiveTranscriptText.isEmpty {
                HStack(alignment: .top, spacing: 6) {
                    Text(effectiveTranscriptRole == .ai ? "AI" : "我")
                        .font(.system(size: 10, weight: .bold))
                        .foregroundColor(.white)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 1.5)
                        .background(
                            Capsule()
                                .fill(effectiveTranscriptRole == .ai ? Color.cyan : Color.indigo)
                        )
                        .padding(.top, 1.5)

                    Text(effectiveTranscriptText)
                        .font(.system(size: 13, weight: .regular))
                        .foregroundColor(.primary.opacity(0.92))
                        .lineLimit(2)
                        .lineSpacing(3)
                        .multilineTextAlignment(.leading)
                        .frame(maxWidth: .infinity, minHeight: 36, alignment: .leading)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.top, 2)
                .padding(.bottom, 2)
                .transition(.opacity.combined(with: .move(edge: .bottom)))
            }

            HStack(spacing: 10) {
                // Status dot + MM:SS timer
                HStack(spacing: 6) {
                    Circle()
                        .fill(statusDotColor)
                        .frame(width: 8, height: 8)

                    Text(statusTimerLabel)
                        .font(.system(size: 12.5, weight: .semibold, design: .monospaced))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                }
                .frame(minWidth: 56, alignment: .leading)

                // Wide dual-color dynamic waveform
                HStack(spacing: 2.5) {
                    ForEach(0..<voiceCall.soundLevels.count, id: \.self) { index in
                        let level = voiceCall.soundLevels[index]
                        RoundedRectangle(cornerRadius: 1.5)
                            .fill(waveformBarColor)
                            .frame(width: 3, height: max(4, level * 24))
                            .animation(.easeOut(duration: 0.08), value: level)
                    }
                }
                .frame(maxWidth: .infinity)
                .frame(height: 26)

                // Controls: Speed pill, Mic Mute, Speaker Mute, optional Stop Agent Task
                HStack(spacing: 7) {
                    Button {
                        voiceCall.cyclePlaybackSpeed()
                    } label: {
                        Text(formatSpeed(voiceCall.playbackSpeed))
                            .font(.system(size: 11, weight: .bold, design: .rounded))
                            .foregroundColor(.primary)
                            .padding(.horizontal, 7)
                            .frame(height: 28)
                            .background(Color(uiColor: .tertiarySystemFill))
                            .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("语速 \(formatSpeed(voiceCall.playbackSpeed))")

                    Button {
                        voiceCall.toggleMicMute()
                    } label: {
                        Image(systemName: voiceCall.isMicMuted ? "mic.slash.fill" : "mic.fill")
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor(voiceCall.isMicMuted ? .white : .primary)
                            .frame(width: 28, height: 28)
                            .background(voiceCall.isMicMuted ? Color.red : Color(uiColor: .tertiarySystemFill))
                            .clipShape(Circle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(voiceCall.isMicMuted ? "取消静音麦克风" : "静音麦克风")

                    Button {
                        voiceCall.toggleSpeakerMute()
                    } label: {
                        Image(systemName: voiceCall.isSpeakerMuted ? "speaker.slash.fill" : "speaker.wave.2.fill")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(voiceCall.isSpeakerMuted ? .white : .primary)
                            .frame(width: 28, height: 28)
                            .background(voiceCall.isSpeakerMuted ? Color.orange : Color(uiColor: .tertiarySystemFill))
                            .clipShape(Circle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(voiceCall.isSpeakerMuted ? "开启AI外放" : "静音AI外放")

                    if isAgentRunning {
                        Button {
                            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                            onStopAgentTask()
                        } label: {
                            Image(systemName: "stop.fill")
                                .font(.system(size: 11, weight: .bold))
                                .foregroundColor(.white)
                                .frame(width: 28, height: 28)
                                .background(Color.red.opacity(0.88))
                                .clipShape(Circle())
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("停止当前任务")
                    }
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .background(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .fill(.ultraThinMaterial)
                .overlay(
                    RoundedRectangle(cornerRadius: 16, style: .continuous)
                        .stroke(
                            voiceCall.isAgentSpeaking
                                ? Color.cyan.opacity(0.35)
                                : Color.indigo.opacity(0.22),
                            lineWidth: 0.75
                        )
                )
                .shadow(color: Color.black.opacity(0.06), radius: 6, x: 0, y: 2)
        )
        .padding(.horizontal, 12)
        .onAppear {
            UIApplication.shared.isIdleTimerDisabled = true
        }
        .onDisappear {
            if !voiceCall.isInCall {
                UIApplication.shared.isIdleTimerDisabled = false
            }
        }
        .padding(.bottom, 6)
        .animation(.spring(response: 0.25, dampingFraction: 0.82), value: effectiveTranscriptText.isEmpty)
    }

    private var statusDotColor: Color {
        switch voiceCall.callState {
        case .connecting, .reconnecting:
            return .orange
        case .listening:
            return voiceCall.isAgentSpeaking ? .cyan : .green
        case .failed:
            return .red
        case .idle:
            return .secondary
        }
    }

    private var waveformBarColor: Color {
        if voiceCall.isMicMuted && !voiceCall.isAgentSpeaking {
            return Color.secondary.opacity(0.35)
        }
        return voiceCall.isAgentSpeaking ? Color.cyan : Color.indigo
    }

    private var statusTimerLabel: String {
        switch voiceCall.callState {
        case .connecting:
            return "连接中"
        case .reconnecting:
            return "重连中"
        case .listening, .idle, .failed:
            let totalSeconds = Int(voiceCall.duration)
            let minutes = totalSeconds / 60
            let seconds = totalSeconds % 60
            return String(format: "%02d:%02d", minutes, seconds)
        }
    }

    private func formatSpeed(_ speed: Double) -> String {
        if speed == floor(speed) {
            return String(format: "%.1fx", speed)
        } else if (speed * 10) == floor(speed * 10) {
            return String(format: "%.1fx", speed)
        } else {
            return String(format: "%.2fx", speed)
        }
    }
}
