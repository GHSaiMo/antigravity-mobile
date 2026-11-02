import SwiftUI

public struct AudioRecordingCardView: View {
    @ObservedObject var recorder: AudioRecorderManager
    let onCancel: () -> Void
    let onDone: () -> Void
    var onSend: (() -> Void)? = nil
    
    public init(
        recorder: AudioRecorderManager,
        onCancel: @escaping () -> Void,
        onDone: @escaping () -> Void,
        onSend: (() -> Void)? = nil
    ) {
        self.recorder = recorder
        self.onCancel = onCancel
        self.onDone = onDone
        self.onSend = onSend
    }
    
    public var body: some View {
        HStack(spacing: 10) {
            // Recording status & timer
            HStack(spacing: 6) {
                Circle()
                    .fill(recorder.isPaused ? Color.orange : Color.red)
                    .frame(width: 8, height: 8)
                
                Text(formatDuration(recorder.duration))
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                    .foregroundColor(.primary)
            }
            .frame(width: 60, alignment: .leading)
            
            // Audio dynamic waveform bars
            HStack(spacing: 2) {
                ForEach(0..<recorder.soundLevels.count, id: \.self) { index in
                    let level = recorder.soundLevels[index]
                    RoundedRectangle(cornerRadius: 1.5)
                        .fill(recorder.isPaused ? Color.secondary.opacity(0.4) : Color.indigo)
                        .frame(width: 3, height: max(4, level * 26))
                        .animation(.easeOut(duration: 0.08), value: level)
                }
            }
            .frame(maxWidth: .infinity)
            .frame(height: 28)
            
            // Control buttons
            HStack(spacing: 8) {
                // Cancel (Trash)
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onCancel()
                } label: {
                    Image(systemName: "trash")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.secondary)
                        .frame(width: 28, height: 28)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
                
                // Pause / Resume
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    recorder.togglePause()
                } label: {
                    Image(systemName: recorder.isPaused ? "play.fill" : "pause.fill")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.primary)
                        .frame(width: 28, height: 28)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
                
                // Done (Checkmark - Save to draft)
                Button {
                    UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    onDone()
                } label: {
                    Image(systemName: "checkmark")
                        .font(.system(size: 13, weight: .bold))
                        .foregroundColor(.white)
                        .frame(width: 28, height: 28)
                        .background(Color.secondary.opacity(0.6))
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
                
                // Send immediately (Arrow up - Stop & Send)
                if let onSend = onSend {
                    Button {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        onSend()
                    } label: {
                        Image(systemName: "arrow.up")
                            .font(.system(size: 13, weight: .bold))
                            .foregroundColor(.white)
                            .frame(width: 28, height: 28)
                            .background(Color.indigo)
                            .clipShape(Circle())
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .fill(.ultraThinMaterial)
                .overlay(
                    RoundedRectangle(cornerRadius: 14, style: .continuous)
                        .stroke(Color.primary.opacity(0.08), lineWidth: 0.5)
                )
                .shadow(color: Color.black.opacity(0.06), radius: 6, x: 0, y: 2)
        )
        .padding(.horizontal, 12)
        .padding(.bottom, 6)
    }
    
    private func formatDuration(_ duration: TimeInterval) -> String {
        let totalSeconds = Int(duration)
        let minutes = totalSeconds / 60
        let seconds = totalSeconds % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }
}
