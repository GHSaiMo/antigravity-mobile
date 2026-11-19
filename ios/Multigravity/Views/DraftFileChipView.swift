import SwiftUI

func formatFileSize(_ bytes: Int64) -> String {
    if bytes >= 1 << 20 { return String(format: "%.1f MB", Double(bytes) / 1_048_576.0) }
    if bytes >= 1 << 10 { return "\(Int((Double(bytes) / 1024.0).rounded())) KB" }
    return "\(bytes) B"
}

/// Square badge for a file type: the app's file icon when one exists, otherwise the extension on a tinted tile.
struct FileTypeBadge: View {
    let fileName: String
    var size: CGFloat = 36
    
    var body: some View {
        Group {
            if FileIconResolver.isWeChatChatArchive(fileName), UIImage(named: "file_icon_wechat") != nil {
                Image("file_icon_wechat")
                    .resizable()
                    .scaledToFill()
            } else if let icon = FileIconResolver.resolveIcon(for: fileName), UIImage(named: icon) != nil {
                Image(icon)
                    .resizable()
                    .scaledToFit()
                    .padding(size * 0.15)
            } else {
                Text(extLabel)
                    .font(.system(size: size * 0.28, weight: .bold))
                    .foregroundColor(.indigo)
                    .lineLimit(1)
                    .minimumScaleFactor(0.6)
            }
        }
        .frame(width: size, height: size)
        .background(FileIconResolver.isWeChatChatArchive(fileName) ? Color(red: 0x07/255.0, green: 0xC1/255.0, blue: 0x60/255.0) : Color.indigo.opacity(0.12))
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
    
    private var extLabel: String {
        let ext = (fileName as NSString).pathExtension.uppercased()
        return ext.isEmpty ? "FILE" : String(ext.prefix(4))
    }
}

/// Chip for a file attached to the draft: type badge, name, size / upload state, remove button.
struct DraftFileChipView: View {
    let file: DraftFile
    let onRemove: () -> Void
    let onRetry: () -> Void
    
    private var isBusy: Bool { file.state == .pending || file.state == .uploading }
    
    var body: some View {
        ZStack(alignment: .topTrailing) {
            HStack(spacing: 8) {
                ZStack {
                    FileTypeBadge(fileName: file.name, size: 36)
                    if isBusy {
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .fill(Color.black.opacity(0.50))
                            .frame(width: 36, height: 36)
                        UploadProgressRing(
                            progress: file.state == .uploading && file.progress > 0 ? file.progress : nil,
                            size: 24,
                            lineWidth: 2.8
                        )
                    } else if file.state == .failed {
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .fill(Color.black.opacity(0.50))
                            .frame(width: 36, height: 36)
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 15, weight: .semibold))
                            .foregroundColor(.white)
                    }
                }
                VStack(alignment: .leading, spacing: 1) {
                    Text(file.name)
                        .font(.system(size: 13, weight: .medium))
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .foregroundColor(.primary)
                    Text(subtitle)
                        .font(.system(size: 11))
                        .monospacedDigit()
                        .foregroundColor(file.state == .failed ? .red : .secondary)
                        .lineLimit(1)
                }
                .padding(.trailing, 10)
            }
            .padding(.horizontal, 8)
            .frame(height: 52)
            .frame(maxWidth: 240, alignment: .leading)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(file.state == .failed ? Color.red : Color.secondary.opacity(0.25), lineWidth: 1)
            )
            .contentShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .onTapGesture {
                if file.state == .failed { onRetry() }
            }
            
            Button(action: onRemove) {
                Image(systemName: "xmark.circle.fill")
                    .font(.system(size: 16))
                    .foregroundColor(.white)
                    .background(Circle().fill(Color.black.opacity(0.65)))
            }
            .buttonStyle(.plain)
            .offset(x: 4, y: -4)
        }
        .padding(.top, 4)
        .padding(.trailing, 4)
    }
    
    private var subtitle: String {
        let sizeText = formatFileSize(file.size)
        let prefix = FileIconResolver.isWeChatChatArchive(file.name) ? "微信聊天记录 · \(sizeText)" : sizeText
        switch file.state {
        case .failed:
            return String(localized: "上传失败，点按重试")
        case .done:
            return prefix
        case .uploading where file.progress > 0:
            let percent = max(1, min(99, Int((file.progress * 100).rounded())))
            return "\(prefix) · \(percent)%"
        default:
            return String(localized: "\(prefix) · 上传中")
        }
    }
}

/// Circular upload progress ring aligned with Android's Material3 CircularProgressIndicator:
/// - Determinate (`progress > 0`): translucent white track + clockwise white arc from 12 o'clock.
/// - Indeterminate (`progress == nil` or `<= 0`): subtle track + smoothly rotating connecting arc.
private struct UploadProgressRing: View {
    let progress: Double?
    var size: CGFloat = 24
    var lineWidth: CGFloat = 2.8

    var body: some View {
        if let progress, progress > 0 {
            let clamped = min(max(progress, 0.03), 1.0)
            ZStack {
                Circle()
                    .stroke(Color.white.opacity(0.28), lineWidth: lineWidth)
                Circle()
                    .trim(from: 0, to: clamped)
                    .stroke(
                        Color.white,
                        style: StrokeStyle(lineWidth: lineWidth, lineCap: .round)
                    )
                    .rotationEffect(.degrees(-90))
                    .animation(.linear(duration: 0.10), value: clamped)
            }
            .frame(width: size, height: size)
        } else {
            IndeterminateUploadRing(size: size, lineWidth: lineWidth)
        }
    }
}

private struct IndeterminateUploadRing: View {
    let size: CGFloat
    let lineWidth: CGFloat
    @State private var rotation: Double = 0

    var body: some View {
        ZStack {
            Circle()
                .stroke(Color.white.opacity(0.14), lineWidth: lineWidth)
            Circle()
                .trim(from: 0, to: 0.24)
                .stroke(
                    Color.white.opacity(0.9),
                    style: StrokeStyle(lineWidth: lineWidth, lineCap: .round)
                )
                .rotationEffect(.degrees(rotation))
        }
        .frame(width: size, height: size)
        .onAppear {
            rotation = 0
            withAnimation(.linear(duration: 0.9).repeatForever(autoreverses: false)) {
                rotation = 360
            }
        }
    }
}

/// File card shown in a user message bubble for files attached to that message.
struct MessageFileCardView: View {
    let file: AttachmentRules.ParsedFile
    let onTap: () -> Void
    
    private var typeLabel: String {
        FileIconResolver.isWeChatChatArchive(file.name) ? "微信聊天记录" : file.ext.uppercased()
    }
    
    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 10) {
                FileTypeBadge(fileName: file.name, size: 36)
                VStack(alignment: .leading, spacing: 1) {
                    Text(file.name)
                        .font(.system(size: 13, weight: .medium))
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .foregroundColor(.primary)
                    Text("\(typeLabel) · \(file.sizeLabel)")
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .frame(maxWidth: 280, alignment: .leading)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .stroke(Color.secondary.opacity(0.2), lineWidth: 0.5)
            )
        }
        .buttonStyle(.plain)
    }
}
