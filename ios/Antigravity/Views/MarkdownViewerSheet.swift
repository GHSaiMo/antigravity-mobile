import SwiftUI

public struct MarkdownViewerSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let data: MarkdownFileViewerData
    public let onDismiss: () -> Void
    public let onProceed: (() -> Void)?
    public let onRetry: (() -> Void)?
    
    @State private var isCopied: Bool = false
    
    public init(
        data: MarkdownFileViewerData,
        onDismiss: @escaping () -> Void,
        onProceed: (() -> Void)? = nil,
        onRetry: (() -> Void)? = nil
    ) {
        self.data = data
        self.onDismiss = onDismiss
        self.onProceed = onProceed
        self.onRetry = onRetry
    }
    
    private var fileIcon: String? {
        FileIconResolver.resolveIcon(for: data.title) ?? FileIconResolver.resolveIcon(for: data.uri)
    }
    
    public var body: some View {
        NavigationStack {
            ZStack(alignment: .bottom) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        // Summary Card (Desktop Parity for Planning Mode Artifacts)
                        if let summary = data.summary, !summary.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(spacing: 6) {
                                    Image(systemName: "sparkles")
                                        .font(.system(size: 13, weight: .semibold))
                                        .foregroundColor(.blue)
                                    Text("方案目标与摘要")
                                        .font(.system(size: 12.5, weight: .semibold))
                                        .foregroundColor(.primary)
                                    Spacer()
                                    if data.canProceed {
                                        Text("待确认")
                                            .font(.system(size: 11, weight: .semibold))
                                            .foregroundColor(.blue)
                                            .padding(.horizontal, 7)
                                            .padding(.vertical, 2.5)
                                            .background(Color.blue.opacity(0.12))
                                            .clipShape(Capsule())
                                    }
                                }
                                Text(summary)
                                    .font(.system(size: 13.5))
                                    .foregroundColor(.secondary)
                                    .lineSpacing(3)
                            }
                            .padding(14)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .stroke(Color.blue.opacity(0.2), lineWidth: 1)
                            )
                        }
                        
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
                            MarkdownContentView(content: data.content)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.top, 12)
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
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("关闭") {
                        onDismiss()
                        dismiss()
                    }
                    .font(.system(size: 15, weight: .regular))
                }
                
                ToolbarItem(placement: .principal) {
                    HStack(spacing: 6) {
                        if let icon = fileIcon {
                            Image(icon)
                                .resizable()
                                .scaledToFit()
                                .frame(width: 16, height: 16)
                        } else {
                            Image(systemName: "doc.text.fill")
                                .font(.system(size: 13))
                                .foregroundColor(.blue)
                        }
                        Text(data.title)
                            .font(.system(size: 15, weight: .semibold))
                            .lineLimit(1)
                    }
                }
                
                ToolbarItem(placement: .primaryAction) {
                    Button(action: {
                        guard !data.content.isEmpty else { return }
                        UIPasteboard.general.string = data.content
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        isCopied = true
                        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                            isCopied = false
                        }
                    }) {
                        HStack(spacing: 4) {
                            Image(systemName: isCopied ? "checkmark" : "doc.on.doc")
                                .font(.system(size: 13, weight: .medium))
                            Text(isCopied ? "已复制" : "复制")
                                .font(.system(size: 14, weight: .medium))
                        }
                        .foregroundColor(isCopied ? .green : .blue)
                    }
                    .disabled(data.content.isEmpty)
                }
            }
        }
    }
}
