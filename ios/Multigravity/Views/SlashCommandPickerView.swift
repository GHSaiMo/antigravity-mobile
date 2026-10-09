import SwiftUI

/// 输入框里键入 "/" 后弹出的命令列表（系统命令 + 技能），与桌面端的斜杠菜单同源。
struct SlashCommandPickerView: View {
    let commands: [SlashCommandOption]
    let isLoading: Bool
    let onSelect: (SlashCommandOption) -> Void

    var body: some View {
        Group {
            if commands.isEmpty {
                HStack(spacing: 10) {
                    if isLoading { ProgressView().scaleEffect(0.8) }
                    Text(isLoading ? "正在读取命令..." : "没有匹配的命令")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                    Spacer()
                }
                .padding(14)
            } else {
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(commands) { command in
                            Button {
                                onSelect(command)
                            } label: {
                                row(command)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
                .frame(maxHeight: 264)
            }
        }
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(Color.secondary.opacity(0.25), lineWidth: 0.5)
        )
    }

    private func row(_ command: SlashCommandOption) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: command.kind == "skill" ? "sparkles" : "bolt.fill")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(command.kind == "skill" ? .orange : .indigo)
                .frame(width: 18)
                .padding(.top, 2)
            VStack(alignment: .leading, spacing: 2) {
                Text("/\(command.name)")
                    .font(.system(size: 14.5, weight: .semibold, design: .monospaced))
                    .foregroundColor(.primary)
                let summary = command.description.split(whereSeparator: \.isNewline).first.map(String.init) ?? ""
                if !summary.isEmpty {
                    Text(summary)
                        .font(.system(size: 12.5))
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .contentShape(Rectangle())
    }
}

/// 已选中的命令标签："/plan ✕"，发送时随消息一起带上。
struct SlashCommandChipView: View {
    let command: SlashCommandOption
    let onClear: () -> Void

    var body: some View {
        Button(action: onClear) {
            HStack(spacing: 6) {
                Text("/\(command.name)")
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .bold))
            }
            .foregroundColor(.indigo)
            .padding(.leading, 10)
            .padding(.trailing, 8)
            .padding(.vertical, 5)
            .background(Color.indigo.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(Color.indigo.opacity(0.35), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
        .accessibilityLabel("移除命令 \(command.name)")
    }
}
