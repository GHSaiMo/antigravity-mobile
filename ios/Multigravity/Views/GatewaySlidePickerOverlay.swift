import SwiftUI
import UIKit

@MainActor
@Observable
public final class GatewaySlidePickerState {
    public var isPresented: Bool = false
    public var isInteractiveTapMode: Bool = false
    /// Anchor rect of the Hero title in the overlay's own coordinate space (top-left origin).
    public var anchorRect: CGRect = .zero
    public var containerSize: CGSize = .zero
    /// 0..<profiles.count = host row, profiles.count = "+ 添加主机" row
    public var hoveredRowIndex: Int? = nil
    public var hasEnteredMenu: Bool = false

    private let selectionHaptic = UISelectionFeedbackGenerator()

    public enum Layout {
        static let hostRowHeight: CGFloat = 46
        static let addRowHeight: CGFloat = 42
        static let rowSpacing: CGFloat = 4
        static let dividerBlockHeight: CGFloat = 9
        static let cardPadding: CGFloat = 6
        static let cardMinWidth: CGFloat = 264
        static let cardMaxWidth: CGFloat = 324
        static let horizontalMargin: CGFloat = 14
        static let gapBelowTitle: CGFloat = 8
        static let bottomSafeMargin: CGFloat = 28
    }

    public init() {}

    public func open(anchorRect: CGRect) {
        self.anchorRect = anchorRect
        self.hoveredRowIndex = nil
        self.hasEnteredMenu = false
        self.isInteractiveTapMode = false
        self.selectionHaptic.prepare()
        withAnimation(.spring(response: 0.22, dampingFraction: 0.85)) {
            self.isPresented = true
        }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        GatewayProfileStore.shared.refreshInactiveSummaries(force: false, minInterval: 8.0)
    }

    public func dismiss() {
        withAnimation(.easeOut(duration: 0.14)) {
            isPresented = false
        }
        isInteractiveTapMode = false
        hoveredRowIndex = nil
        hasEnteredMenu = false
    }

    public func cardWidth() -> CGFloat {
        guard containerSize.width > 0 else { return 296 }
        let available = containerSize.width - Layout.horizontalMargin * 2
        return min(Layout.cardMaxWidth, max(min(Layout.cardMinWidth, available), min(available, containerSize.width * 0.84)))
    }

    public func cardHeight(profileCount: Int) -> CGFloat {
        let hostCount = max(0, profileCount)
        let hostsHeight = CGFloat(hostCount) * Layout.hostRowHeight
            + CGFloat(max(0, hostCount - 1)) * Layout.rowSpacing
        let dividerPart = hostCount > 0 ? Layout.dividerBlockHeight : 0
        let natural = Layout.cardPadding * 2 + hostsHeight + dividerPart + Layout.addRowHeight
        let maxAvail = containerSize.height > 160
            ? max(160, containerSize.height - (anchorRect.maxY + Layout.gapBelowTitle) - Layout.bottomSafeMargin)
            : 440
        return min(natural, maxAvail)
    }

    public func cardRect(profileCount: Int) -> CGRect {
        let width = cardWidth()
        let height = cardHeight(profileCount: profileCount)

        let rawMinX: CGFloat
        if containerSize.width > 0 && abs(anchorRect.midX - containerSize.width / 2) < 48 && anchorRect.minX > 44 {
            // Scrolled inline centered title: center the popover under the title
            rawMinX = anchorRect.midX - width / 2
        } else {
            // Expanded large title (leading aligned)
            rawMinX = max(Layout.horizontalMargin, anchorRect.minX - 2)
        }
        let maxMinX = containerSize.width > 0
            ? max(Layout.horizontalMargin, containerSize.width - width - Layout.horizontalMargin)
            : rawMinX
        let minX = min(max(Layout.horizontalMargin, rawMinX), maxMinX)

        let preferredY = max(54, anchorRect.maxY + Layout.gapBelowTitle)
        let maxMinY = containerSize.height > 0
            ? max(54, containerSize.height - height - Layout.bottomSafeMargin)
            : preferredY
        let minY = min(preferredY, maxMinY)

        return CGRect(x: minX, y: minY, width: width, height: height)
    }

    public func updateDrag(at point: CGPoint, profileCount: Int) {
        guard isPresented else { return }
        let rect = cardRect(profileCount: profileCount)

        // Finger still resting on the Hero title trigger: keep hovered nil so releasing enters tap mode
        if anchorRect.insetBy(dx: -16, dy: -10).contains(point) {
            if hoveredRowIndex != nil {
                hoveredRowIndex = nil
            }
            return
        }

        let hitRect = CGRect(
            x: rect.minX - 24,
            y: rect.minY - 8,
            width: rect.width + 48,
            height: rect.height + 24
        )
        guard hitRect.contains(point) else {
            if hoveredRowIndex != nil {
                hoveredRowIndex = nil
            }
            return
        }

        hasEnteredMenu = true
        let localY = point.y - (rect.minY + Layout.cardPadding)
        let hostCount = max(0, profileCount)
        let hostsTotalHeight = CGFloat(hostCount) * Layout.hostRowHeight
            + CGFloat(max(0, hostCount - 1)) * Layout.rowSpacing

        let targetIdx: Int
        if hostCount == 0 {
            targetIdx = 0
        } else if localY < hostsTotalHeight + Layout.dividerBlockHeight * 0.45 {
            let step = Layout.hostRowHeight + Layout.rowSpacing
            targetIdx = min(max(0, Int(floor(max(0, localY) / step))), hostCount - 1)
        } else {
            targetIdx = hostCount
        }

        if hoveredRowIndex != targetIdx {
            hoveredRowIndex = targetIdx
            selectionHaptic.selectionChanged()
        }
    }

    public func endDrag(
        profiles: [GatewayProfile],
        onSelectProfile: (GatewayProfile) -> Void,
        onAddGateway: () -> Void
    ) {
        guard isPresented else { return }
        if let idx = hoveredRowIndex {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            if idx == profiles.count {
                dismiss()
                onAddGateway()
                return
            } else if profiles.indices.contains(idx) {
                let target = profiles[idx]
                dismiss()
                onSelectProfile(target)
                return
            }
        }

        if !hasEnteredMenu {
            isInteractiveTapMode = true
        } else {
            dismiss()
        }
    }
}

public struct GatewaySlidePickerOverlay: View {
    @Bindable var state: GatewaySlidePickerState
    let onSelectProfile: (GatewayProfile) -> Void
    let onAddGateway: () -> Void
    let onRenameProfile: (GatewayProfile) -> Void
    let onRemoveProfile: (GatewayProfile) -> Void

    private var store: GatewayProfileStore { GatewayProfileStore.shared }

    public var body: some View {
        if state.isPresented {
            let profiles = store.profiles
            let rect = state.cardRect(profileCount: profiles.count)

            ZStack(alignment: .topLeading) {
                Color.black
                    .opacity(state.isInteractiveTapMode ? 0.10 : 0.04)
                    .ignoresSafeArea()
                    .contentShape(Rectangle())
                    .onTapGesture {
                        state.dismiss()
                    }

                popoverCard(profiles: profiles, size: rect.size)
                    .position(x: rect.midX, y: rect.midY)
                    .transition(
                        .asymmetric(
                            insertion: .opacity.combined(with: .scale(scale: 0.94, anchor: .topLeading)),
                            removal: .opacity
                        )
                    )
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .ignoresSafeArea()
            .zIndex(550)
        }
    }

    @ViewBuilder
    private func popoverCard(profiles: [GatewayProfile], size: CGSize) -> some View {
        let hostCount = profiles.count
        let naturalHeight = GatewaySlidePickerState.Layout.cardPadding * 2
            + CGFloat(hostCount) * GatewaySlidePickerState.Layout.hostRowHeight
            + CGFloat(max(0, hostCount - 1)) * GatewaySlidePickerState.Layout.rowSpacing
            + (hostCount > 0 ? GatewaySlidePickerState.Layout.dividerBlockHeight : 0)
            + GatewaySlidePickerState.Layout.addRowHeight
        let needsScroll = naturalHeight > size.height + 1

        let content = VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: GatewaySlidePickerState.Layout.rowSpacing) {
                ForEach(Array(profiles.enumerated()), id: \.element.id) { index, profile in
                    hostRow(profile: profile, index: index)
                }
            }

            if !profiles.isEmpty {
                Rectangle()
                    .fill(Color.primary.opacity(0.09))
                    .frame(height: 0.75)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 4)
            }

            addGatewayRow(index: profiles.count)
        }
        .padding(GatewaySlidePickerState.Layout.cardPadding)

        Group {
            if needsScroll {
                ScrollView(.vertical, showsIndicators: false) {
                    content
                }
            } else {
                content
            }
        }
        .frame(width: size.width, height: size.height)
        .background(cardSurface)
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.16), radius: 16, x: 0, y: 8)
    }

    @ViewBuilder
    private func hostRow(profile: GatewayProfile, index: Int) -> some View {
        let isActive = store.activeProfileId == profile.id
        let isHovered = state.hoveredRowIndex == index
        let summary = store.summaries[profile.id]

        HStack(spacing: 8) {
            Button {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                state.dismiss()
                onSelectProfile(profile)
            } label: {
                HStack(spacing: 8) {
                    ZStack {
                        RoundedRectangle(cornerRadius: 7, style: .continuous)
                            .fill((isActive || isHovered) ? Color.indigo.opacity(0.16) : Color.primary.opacity(0.06))
                            .frame(width: 26, height: 26)
                        Image(systemName: profile.platformIconName)
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor((isActive || isHovered) ? .indigo : .secondary)
                    }

                    VStack(alignment: .leading, spacing: 1.5) {
                        HStack(spacing: 4) {
                            Text(profile.resolvedDisplayName)
                                .font(.system(size: 13.5, weight: (isActive || isHovered) ? .semibold : .medium))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                                .truncationMode(.tail)
                            if isActive {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 10, weight: .bold))
                                    .foregroundColor(.indigo)
                            }
                        }
                    }

                    Spacer(minLength: 4)

                    statusPills(summary: summary, isActive: isActive)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            if state.isInteractiveTapMode {
                Menu {
                    Button {
                        state.dismiss()
                        onRenameProfile(profile)
                    } label: {
                        Label(String(localized: "重命名主机"), systemImage: "pencil")
                    }
                    Button(role: .destructive) {
                        state.dismiss()
                        onRemoveProfile(profile)
                    } label: {
                        Label(String(localized: "移除此主机"), systemImage: "trash")
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.secondary)
                        .frame(width: 24, height: 28)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 9)
        .frame(maxWidth: .infinity)
        .frame(height: GatewaySlidePickerState.Layout.hostRowHeight)
        .background(rowBackground(isHovered: isHovered, isActive: isActive))
        .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 9, style: .continuous)
                .stroke(rowBorderColor(isHovered: isHovered, isActive: isActive), lineWidth: isHovered ? 1.25 : 1)
        )
    }

    @ViewBuilder
    private func statusPills(summary: GatewaySummarySnapshot?, isActive: Bool) -> some View {
        if let summary {
            if !summary.isReachable && !isActive {
                Text(String(localized: "离线"))
                    .font(.system(size: 10.5, weight: .medium))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Capsule().fill(Color.primary.opacity(0.06)))
            } else if summary.isRefreshing && summary.updatedAt == .distantPast && !isActive {
                ProgressView()
                    .controlSize(.mini)
                    .tint(.secondary)
            } else if summary.actionCount > 0 || summary.runningCount > 0 || summary.unreadCount > 0 {
                HStack(spacing: 4) {
                    if summary.actionCount > 0 {
                        compactPill(
                            text: String(localized: "\(summary.actionCount) 待确认"),
                            color: .orange
                        )
                    }
                    if summary.runningCount > 0 {
                        compactPill(
                            text: String(localized: "\(summary.runningCount) 工作中"),
                            color: .blue
                        )
                    }
                    if summary.unreadCount > 0 && summary.actionCount == 0 {
                        compactPill(
                            text: String(localized: "\(summary.unreadCount) 未读"),
                            color: .indigo
                        )
                    }
                }
            } else if summary.updatedAt != .distantPast || isActive {
                Text(String(localized: "空闲"))
                    .font(.system(size: 10.5, weight: .medium))
                    .foregroundColor(.secondary.opacity(0.8))
            }
        }
    }

    private func compactPill(text: String, color: Color) -> some View {
        Text(text)
            .font(.system(size: 10.5, weight: .semibold).monospacedDigit())
            .foregroundColor(color)
            .lineLimit(1)
            .fixedSize()
            .padding(.horizontal, 6)
            .padding(.vertical, 2.5)
            .background(Capsule().fill(color.opacity(0.14)))
    }

    @ViewBuilder
    private func addGatewayRow(index: Int) -> some View {
        let isHovered = state.hoveredRowIndex == index
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            state.dismiss()
            onAddGateway()
        } label: {
            HStack(spacing: 8) {
                Image(systemName: "plus.circle.fill")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.indigo)
                    .frame(width: 26, height: 26)
                Text(String(localized: "添加主机"))
                    .font(.system(size: 13.5, weight: isHovered ? .semibold : .medium))
                    .foregroundColor(.indigo)
                Spacer(minLength: 4)
                Text(String(localized: "扫码配对"))
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(.secondary)
            }
            .padding(.horizontal, 9)
            .frame(maxWidth: .infinity)
            .frame(height: GatewaySlidePickerState.Layout.addRowHeight)
            .background(isHovered ? Color.indigo.opacity(0.18) : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .stroke(isHovered ? Color.indigo.opacity(0.75) : Color.clear, lineWidth: 1.2)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    @ViewBuilder
    private var cardSurface: some View {
        ZStack {
            Rectangle().fill(.ultraThinMaterial)
            Color(uiColor: .systemBackground).opacity(0.82)
        }
    }

    private func rowBackground(isHovered: Bool, isActive: Bool) -> Color {
        if isHovered {
            return Color.indigo.opacity(0.22)
        }
        if isActive {
            return Color.indigo.opacity(0.10)
        }
        return Color.clear
    }

    private func rowBorderColor(isHovered: Bool, isActive: Bool) -> Color {
        if isHovered {
            return Color.indigo.opacity(0.75)
        }
        if isActive {
            return Color.indigo.opacity(0.36)
        }
        return Color.clear
    }
}
