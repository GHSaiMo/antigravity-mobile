import SwiftUI

// MARK: - Markdown Carousel View (Interactive Photo Carousel)

public struct MarkdownCarouselView: View {
    public let slides: [MarkdownCarouselSlide]
    public let onImageTap: ((URL) -> Void)?
    
    @State private var currentIndex: Int = 0
    @State private var slideDirection: CGFloat = 1 // 1 for next (swipe right-to-left), -1 for prev
    
    public init(slides: [MarkdownCarouselSlide], onImageTap: ((URL) -> Void)? = nil) {
        self.slides = slides
        self.onImageTap = onImageTap
    }
    
    private var safeIndex: Int {
        guard !slides.isEmpty else { return 0 }
        return min(max(0, currentIndex), slides.count - 1)
    }
    
    public var body: some View {
        if slides.isEmpty {
            EmptyView()
        } else {
            let currentSlide = slides[safeIndex]
            
            VStack(alignment: .leading, spacing: 0) {
                // Header Bar
                HStack(spacing: 8) {
                    HStack(spacing: 6) {
                        Image(systemName: "photo.stack.fill")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.blue)
                        
                        Text("照片轮播")
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor(.primary)
                        
                        Text("\(safeIndex + 1) / \(slides.count)")
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(.secondary)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 2.5)
                            .background(Color.secondary.opacity(0.12))
                            .clipShape(Capsule())
                    }
                    
                    Spacer()
                    
                    // Desktop-style interactive Previous / Next navigation buttons
                    HStack(spacing: 6) {
                        Button {
                            goToPrevious()
                        } label: {
                            HStack(spacing: 3) {
                                Image(systemName: "chevron.left")
                                    .font(.system(size: 10.5, weight: .bold))
                                Text("上一张")
                                    .font(.system(size: 11.5, weight: .medium))
                            }
                            .foregroundColor(safeIndex > 0 ? .primary : .secondary.opacity(0.35))
                            .padding(.horizontal, 9)
                            .padding(.vertical, 5)
                            .background(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .fill(safeIndex > 0 ? Color(uiColor: .tertiarySystemFill) : Color.clear)
                            )
                            .overlay(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .stroke(Color.secondary.opacity(safeIndex > 0 ? 0.25 : 0.12), lineWidth: 0.8)
                            )
                        }
                        .disabled(safeIndex == 0)
                        .buttonStyle(.plain)
                        .accessibilityLabel("上一张照片")
                        
                        Button {
                            goToNext()
                        } label: {
                            HStack(spacing: 3) {
                                Text("下一张")
                                    .font(.system(size: 11.5, weight: .medium))
                                Image(systemName: "chevron.right")
                                    .font(.system(size: 10.5, weight: .bold))
                            }
                            .foregroundColor(safeIndex < slides.count - 1 ? .primary : .secondary.opacity(0.35))
                            .padding(.horizontal, 9)
                            .padding(.vertical, 5)
                            .background(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .fill(safeIndex < slides.count - 1 ? Color(uiColor: .tertiarySystemFill) : Color.clear)
                            )
                            .overlay(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .stroke(Color.secondary.opacity(safeIndex < slides.count - 1 ? 0.25 : 0.12), lineWidth: 0.8)
                            )
                        }
                        .disabled(safeIndex >= slides.count - 1)
                        .buttonStyle(.plain)
                        .accessibilityLabel("下一张照片")
                    }
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(Color(uiColor: .tertiarySystemBackground).opacity(0.85))
                
                Divider()
                    .background(Color.secondary.opacity(0.2))
                
                // Current Slide Body Content
                VStack(alignment: .leading, spacing: 8) {
                    MarkdownContentView(content: currentSlide.content, onImageTap: onImageTap)
                        .id(currentSlide.id)
                        .transition(.asymmetric(
                            insertion: .opacity.combined(with: .offset(x: slideDirection * 20)),
                            removal: .opacity.combined(with: .offset(x: -slideDirection * 20))
                        ))
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
                .gesture(
                    DragGesture(minimumDistance: 20)
                        .onEnded { value in
                            let horizontal = value.translation.width
                            let vertical = value.translation.height
                            if abs(horizontal) > abs(vertical) * 1.4 {
                                if horizontal < -35 {
                                    goToNext()
                                } else if horizontal > 35 {
                                    goToPrevious()
                                }
                            }
                        }
                )
                
                // Bottom Pagination Dots (if multiple slides)
                if slides.count > 1 {
                    HStack(spacing: 5) {
                        ForEach(0..<slides.count, id: \.self) { idx in
                            Capsule()
                                .fill(idx == safeIndex ? Color.blue : Color.secondary.opacity(0.25))
                                .frame(width: idx == safeIndex ? 18 : 6, height: 6)
                                .onTapGesture {
                                    guard idx != safeIndex else { return }
                                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                    slideDirection = idx > safeIndex ? 1 : -1
                                    withAnimation(.spring(response: 0.32, dampingFraction: 0.82)) {
                                        currentIndex = idx
                                    }
                                }
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .center)
                    .padding(.top, 4)
                    .padding(.bottom, 10)
                }
            }
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .stroke(Color.secondary.opacity(0.2), lineWidth: 1)
            )
        }
    }
    
    private func goToPrevious() {
        guard safeIndex > 0 else { return }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        slideDirection = -1
        withAnimation(.spring(response: 0.32, dampingFraction: 0.82)) {
            currentIndex = safeIndex - 1
        }
    }
    
    private func goToNext() {
        guard safeIndex < slides.count - 1 else { return }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        slideDirection = 1
        withAnimation(.spring(response: 0.32, dampingFraction: 0.82)) {
            currentIndex = safeIndex + 1
        }
    }
}
