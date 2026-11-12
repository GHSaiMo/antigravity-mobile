import SwiftUI

public struct SwarmBannerView: View {
    public let brief: SwarmBrief
    public let onTap: () -> Void

    public init(brief: SwarmBrief, onTap: @escaping () -> Void) {
        self.brief = brief
        self.onTap = onTap
    }

    public var body: some View {
        Button(action: {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            onTap()
        }) {
            HStack(spacing: 8) {
                SwarmPixelBeeView(
                    name: "coordinator",
                    pose: brief.workingCount > 0 ? .working : .landed,
                    size: 28
                )

                VStack(alignment: .leading, spacing: 2) {
                    HStack(spacing: 6) {
                        Text(brief.title.isEmpty ? "蜂群协同 (Swarm)" : brief.title)
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)

                        if !brief.phase.isEmpty {
                            Text(brief.phase)
                                .font(.system(size: 10, weight: .bold))
                                .foregroundColor(SwarmTheme.accent)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 1.5)
                                .background(SwarmTheme.accent.opacity(0.12))
                                .clipShape(Capsule())
                        }
                    }

                    HStack(spacing: 6) {
                        Text("\(brief.workingCount)/\(brief.totalMembers) 活跃")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)

                        Text("·")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)

                        Text("#\(brief.latestSeq) 看板")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)

                        if !brief.headline.isEmpty {
                            Text("· \(brief.headline)")
                                .font(.system(size: 11))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                        }
                    }
                }

                Spacer()

                if brief.attentionCount > 0 {
                    HStack(spacing: 3) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 10))
                        Text("\(brief.attentionCount) 待关注")
                            .font(.system(size: 11, weight: .medium))
                    }
                    .foregroundColor(SwarmTheme.warnAmber)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 3)
                    .background(SwarmTheme.warnAmber.opacity(0.12))
                    .clipShape(Capsule())
                }

                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.secondary)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(.ultraThinMaterial)
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .stroke(SwarmTheme.accent.opacity(0.28), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)
    }
}
