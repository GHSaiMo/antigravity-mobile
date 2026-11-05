import SwiftUI

public struct SlashCommandShape: Shape {
    public let iconKey: String

    public func path(in rect: CGRect) -> Path {
        let cgPath = SlashCommandIconCatalog.cachedCGPath(for: iconKey)
        let scale = min(rect.width, rect.height) / 960.0
        var transform = CGAffineTransform(scaleX: scale, y: scale)
        let offsetX = (rect.width - 960.0 * scale) / 2.0
        let offsetY = (rect.height - 960.0 * scale) / 2.0
        transform = transform.concatenating(CGAffineTransform(translationX: offsetX, y: offsetY))
        if let transformed = cgPath.copy(using: &transform) {
            return Path(transformed)
        }
        return Path()
    }
}

public struct SlashCommandIconView: View {
    public let name: String
    public var icon: String? = nil
    public var kind: String? = nil
    public var size: CGFloat = 16

    public init(name: String, icon: String? = nil, kind: String? = nil, size: CGFloat = 16) {
        self.name = name
        self.icon = icon
        self.kind = kind
        self.size = size
    }

    public var body: some View {
        let key = SlashCommandIconCatalog.resolveIcon(name: name, icon: icon, kind: kind)
        SlashCommandShape(iconKey: key)
            .fill(style: FillStyle(eoFill: false))
            .frame(width: size, height: size)
    }
}

public enum SlashCommandTextRenderer {
    public static func render(
        text: String,
        mentions: [SlashCommandMention] = [],
        availableCommands: [SlashCommandOption] = [],
        iconColor: UIColor = .white,
        iconSize: CGFloat = 15
    ) -> Text {
        let segments = SlashCommandTextTokenizer.tokenize(
            text: text,
            mentions: mentions,
            availableCommands: availableCommands
        )
        guard !segments.isEmpty else { return Text(text) }

        var result = Text("")
        for seg in segments {
            switch seg {
            case .text(let str):
                result = result + Text(str)
            case .slash(let mention):
                let iconKey = SlashCommandIconCatalog.resolveIcon(name: mention.name, icon: mention.icon, kind: mention.kind)
                let img = SlashCommandIconCatalog.cachedImage(iconKey: iconKey, size: iconSize, color: iconColor)
                let iconText = Text(Image(uiImage: img)).baselineOffset(-1.5)
                let nameText = Text(verbatim: " \(mention.name)").fontWeight(.semibold)
                result = result + iconText + nameText
            }
        }
        return result
    }
}

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
            SlashCommandIconView(name: command.name, icon: command.icon, kind: command.kind, size: 16)
                .foregroundColor(.secondary)
                .frame(width: 18)
                .padding(.top, 2)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text("/\(command.name)")
                        .font(.system(size: 14.5, weight: .semibold, design: .monospaced))
                        .foregroundColor(.primary)
                    Text(command.kind == "skill" ? "技能" : "系统")
                        .font(.system(size: 10.5, weight: .medium))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Capsule())
                }
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
                SlashCommandIconView(name: command.name, icon: command.icon, kind: command.kind, size: 14)
                    .opacity(0.85)
                Text(command.name)
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .bold))
            }
            .foregroundColor(.indigo)
            .padding(.leading, 8)
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
