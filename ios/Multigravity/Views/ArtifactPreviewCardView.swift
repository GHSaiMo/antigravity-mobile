import SwiftUI

// MARK: - Artifact Preview Card (Markdown 一键预览卡片)

public struct ArtifactPreviewCardView: View {
    @Environment(\.openURL) private var openURL
    public let artifact: ArtifactItem
    
    public init(artifact: ArtifactItem) {
        self.artifact = artifact
    }
    
    public var body: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if let url = URL(string: artifact.uri) {
                openURL(url)
            }
        } label: {
            VStack(alignment: .leading, spacing: 6) {
                let isVideo = isVideoURL(artifact.uri) || isVideoURL(artifact.title)
                // Header: Document Icon + Title
                HStack(alignment: .center, spacing: 7) {
                    Text(isVideo ? "🎬" : "📄")
                        .font(.system(size: 15))
                    
                    Text(artifact.title.isEmpty ? "文档详情" : artifact.title)
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    
                    Spacer()
                    
                    if isVideo {
                        Text("MP4 视频")
                            .font(.system(size: 10.5, weight: .semibold, design: .rounded))
                            .foregroundColor(.secondary)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.secondary.opacity(0.12))
                            .clipShape(Capsule())
                    }
                }
                
                // Summary text if available
                if let summary = artifact.summary, !summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    Text(summary)
                        .font(.system(size: 12.5))
                        .foregroundColor(.secondary)
                        .lineSpacing(2.5)
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(.horizontal, 13)
            .padding(.vertical, 11)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(uiColor: .tertiarySystemBackground).opacity(0.95))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .stroke(Color.secondary.opacity(0.18), lineWidth: 0.8)
            )
        }
        .buttonStyle(ArtifactCardButtonStyle())
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(artifact.title), \(artifact.summary ?? "点击一键预览")")
    }
}

private struct ArtifactCardButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .opacity(configuration.isPressed ? 0.78 : 1.0)
            .scaleEffect(configuration.isPressed ? 0.99 : 1.0)
            .animation(.easeInOut(duration: 0.12), value: configuration.isPressed)
    }
}
