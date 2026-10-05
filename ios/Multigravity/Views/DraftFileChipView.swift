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
            if let icon = FileIconResolver.resolveIcon(for: fileName), UIImage(named: icon) != nil {
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
        .background(Color.indigo.opacity(0.12))
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
                            .fill(Color.black.opacity(0.35))
                            .frame(width: 36, height: 36)
                        if file.state == .uploading && file.progress > 0 {
                            ProgressView(value: file.progress)
                                .progressViewStyle(.circular)
                                .tint(.white)
                                .scaleEffect(0.7)
                        } else {
                            ProgressView()
                                .tint(.white)
                                .scaleEffect(0.7)
                        }
                    } else if file.state == .failed {
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .fill(Color.black.opacity(0.45))
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
                        .foregroundColor(file.state == .failed ? .red : .secondary)
                        .lineLimit(1)
                }
                .padding(.trailing, 10)
            }
            .padding(.horizontal, 8)
            .frame(height: 52)
            .frame(maxWidth: 220, alignment: .leading)
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
        switch file.state {
        case .failed: return "上传失败，点按重试"
        case .done: return formatFileSize(file.size)
        default: return "\(formatFileSize(file.size)) · 上传中"
        }
    }
}

/// File card shown in a user message bubble for files attached to that message.
struct MessageFileCardView: View {
    let file: AttachmentRules.ParsedFile
    let onTap: () -> Void
    
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
                    Text("\(file.ext.uppercased()) · \(file.sizeLabel)")
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
