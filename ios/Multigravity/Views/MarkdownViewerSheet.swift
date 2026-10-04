import SwiftUI
import PhotosUI
import AVFoundation

// MARK: - Markdown Viewer Sheet (Implementation Plan & Markdown Artifacts)
public struct MarkdownViewerSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let data: MarkdownFileViewerData
    public let onDismiss: (() -> Void)?
    public let onProceed: (() -> Void)?
    public let onRetry: (() -> Void)?
    public let onRefresh: (() -> Void)?
    
    @State private var previewImage: IdentifiableImage? = nil
    @State private var isShowingDocumentPicker: Bool = false
    @State private var toastSuccessMessage: String? = nil
    
    public init(
        data: MarkdownFileViewerData,
        onDismiss: (() -> Void)? = nil,
        onProceed: (() -> Void)? = nil,
        onRetry: (() -> Void)? = nil,
        onRefresh: (() -> Void)? = nil
    ) {
        self.data = data
        self.onDismiss = onDismiss
        self.onProceed = onProceed
        self.onRetry = onRetry
        self.onRefresh = onRefresh
    }
    
    private var shareURL: URL? {
        if let fileURL = data.cachedFileURL, FileManager.default.fileExists(atPath: fileURL.path) {
            return fileURL
        }
        guard !data.content.isEmpty else { return nil }
        let baseName = data.title.isEmpty ? "document" : data.title
        let safeName = baseName.replacingOccurrences(of: "/", with: "_")
        let fileName = safeName.hasSuffix(".md") ? safeName : "\(safeName).md"
        let tempURL = FileManager.default.temporaryDirectory.appendingPathComponent(fileName)
        if !FileManager.default.fileExists(atPath: tempURL.path) {
            try? data.content.write(to: tempURL, atomically: true, encoding: .utf8)
        }
        return tempURL
    }
    
    public var body: some View {
        NavigationStack {
            ZStack(alignment: .bottom) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        // Main Markdown Content Body
                        if data.isLoading {
                            VStack(spacing: 12) {
                                ProgressView()
                                    .scaleEffect(1.1)
                                Text("正在从电脑端拉取文档...")
                                    .font(.system(size: 13.5))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity, minHeight: 240)
                        } else if let err = data.errorMessage {
                            VStack(spacing: 12) {
                                Image(systemName: "exclamationmark.triangle.fill")
                                    .font(.system(size: 32))
                                    .foregroundColor(.orange)
                                Text(err)
                                    .font(.system(size: 14))
                                    .foregroundColor(.secondary)
                                    .multilineTextAlignment(.center)
                                    .padding(.horizontal, 24)
                                if let retry = onRetry {
                                    Button("重新加载", action: retry)
                                        .buttonStyle(.borderedProminent)
                                        .padding(.top, 4)
                                }
                            }
                            .frame(maxWidth: .infinity, minHeight: 240)
                        } else if data.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                            VStack(spacing: 8) {
                                Image(systemName: "doc.text")
                                    .font(.system(size: 32))
                                    .foregroundColor(.secondary)
                                Text("文档内容为空")
                                    .font(.system(size: 14))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity, minHeight: 200)
                        } else {
                            MarkdownContentView(content: data.content, onImageTap: { url in
                                previewImage = IdentifiableImage(url: url)
                            })
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.top, 14)
                    .padding(.bottom, data.canProceed ? 90 : 32)
                }
                
                // Bottom Fixed Proceed Bar (when planning mode artifact needs approval)
                if data.canProceed {
                    VStack(spacing: 0) {
                        Divider()
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text("方案查阅完毕")
                                    .font(.system(size: 11.5, weight: .medium))
                                    .foregroundColor(.secondary)
                                Text("点击立即进入自动化执行")
                                    .font(.system(size: 12.5, weight: .semibold))
                                    .foregroundColor(.primary)
                            }
                            
                            Spacer()
                            
                            Button(action: {
                                onProceed?()
                            }) {
                                HStack(spacing: 6) {
                                    Image(systemName: "play.fill")
                                        .font(.system(size: 12, weight: .bold))
                                    Text("确认执行 (Proceed)")
                                        .font(.system(size: 13.5, weight: .semibold))
                                }
                                .foregroundColor(.white)
                                .padding(.horizontal, 16)
                                .padding(.vertical, 9)
                                .background(Color.blue)
                                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                .shadow(color: Color.blue.opacity(0.35), radius: 5, x: 0, y: 2.5)
                            }
                            .buttonStyle(.plain)
                        }
                        .padding(.horizontal, 16)
                        .padding(.vertical, 10)
                        .background(Color(uiColor: .systemBackground).opacity(0.95))
                    }
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .navigationTitle(data.title.isEmpty ? "文档详情" : data.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: {
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                        if shareURL != nil {
                            isShowingDocumentPicker = true
                        }
                    }) {
                        Image(systemName: "square.and.arrow.down")
                            .font(.system(size: 16, weight: .semibold))
                    }
                    .disabled(shareURL == nil)
                    .accessibilityLabel("存储到“文件”")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    if let url = shareURL {
                        ShareLink(item: url, preview: SharePreview(data.title.isEmpty ? "文档详情" : data.title)) {
                            Image(systemName: "square.and.arrow.up")
                                .font(.system(size: 16, weight: .semibold))
                        }
                        .accessibilityLabel("分享")
                    }
                }
            }
            .sheet(isPresented: $isShowingDocumentPicker) {
                if let url = shareURL {
                    DocumentExporterRepresentable(url: url) { _ in
                        UINotificationFeedbackGenerator().notificationOccurred(.success)
                        toastSuccessMessage = "已保存至“文件”"
                        DispatchQueue.main.asyncAfter(deadline: .now() + 2.6) {
                            toastSuccessMessage = nil
                        }
                    }
                }
            }
        }
        .presentationDragIndicator(.visible)
        .overlay(alignment: .center) {
            if let msg = toastSuccessMessage {
                VStack(spacing: 12) {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 40, weight: .semibold))
                        .foregroundColor(.green)
                    Text(msg)
                        .font(.system(size: 14.5, weight: .medium))
                        .foregroundColor(.primary)
                        .multilineTextAlignment(.center)
                }
                .padding(.horizontal, 24)
                .padding(.vertical, 20)
                .background(.ultraThinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
                )
                .shadow(color: Color.black.opacity(0.18), radius: 20, x: 0, y: 8)
                .transition(.scale(scale: 0.85).combined(with: .opacity))
            }
        }
        .animation(.spring(response: 0.3, dampingFraction: 0.78), value: toastSuccessMessage)
        .fullScreenCover(item: $previewImage) { item in
            ImageViewerSheet(item: item)
                .presentationBackground(.clear)
                .ignoresSafeArea()
        }
    }
}
