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

/// 输入框里键入 "/" 后浮在会话界面底部的命令气泡弹窗（系统命令 + 技能），与桌面端的斜杠菜单同源。
struct SlashCommandPickerView: View {
    let commands: [SlashCommandOption]
    let isLoading: Bool
    var maxListHeight: CGFloat = 280
    let onSelect: (SlashCommandOption) -> Void
    let onDismiss: () -> Void

    @State private var contentHeight: CGFloat = 0

    private var effectiveListHeight: CGFloat {
        let fallback = CGFloat(max(commands.count, 1)) * 66
        let measured = contentHeight > 0 ? contentHeight : fallback
        return min(measured, max(maxListHeight, 150))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // 顶部标题栏 + 右上角关闭按钮
            HStack(spacing: 8) {
                Image(systemName: "slash.circle.fill")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.indigo)

                Text("斜杠命令与技能")
                    .font(.system(size: 13.5, weight: .semibold))
                    .foregroundColor(.primary)

                if !commands.isEmpty {
                    Text("\(commands.count)")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.indigo)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1.5)
                        .background(Color.indigo.opacity(0.14))
                        .clipShape(Capsule())
                }

                Spacer(minLength: 8)

                Button(action: onDismiss) {
                    Image(systemName: "xmark")
                        .font(.system(size: 11.5, weight: .bold))
                        .foregroundColor(.secondary)
                        .frame(width: 26, height: 26)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Circle())
                        .contentShape(Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("关闭斜杠命令气泡")
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)

            Divider()
                .opacity(0.6)

            if commands.isEmpty {
                HStack(spacing: 10) {
                    if isLoading { ProgressView().scaleEffect(0.85) }
                    Text(isLoading ? "正在读取命令与技能..." : "没有匹配的命令或技能")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                    Spacer()
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 20)
            } else {
                ScrollView(.vertical, showsIndicators: true) {
                    VStack(spacing: 0) {
                        ForEach(Array(commands.enumerated()), id: \.element.id) { index, command in
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                onSelect(command)
                            } label: {
                                row(command)
                            }
                            .buttonStyle(.plain)

                            if index < commands.count - 1 {
                                Divider()
                                    .padding(.leading, 56)
                                    .opacity(0.45)
                            }
                        }
                    }
                    .onGeometryChange(for: CGFloat.self) { proxy in
                        proxy.size.height
                    } action: { newHeight in
                        if newHeight > 0 && abs(contentHeight - newHeight) > 0.5 {
                            contentHeight = newHeight
                        }
                    }
                }
                .scrollBounceBehavior(.basedOnSize)
                .frame(height: effectiveListHeight)
            }
        }
        .background(
            ZStack {
                Color(uiColor: .secondarySystemGroupedBackground)
                Rectangle().fill(.regularMaterial).opacity(0.55)
            }
        )
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .stroke(Color.primary.opacity(0.14), lineWidth: 0.8)
        )
        .shadow(color: Color.black.opacity(0.32), radius: 18, x: 0, y: 6)
    }

    private func row(_ command: SlashCommandOption) -> some View {
        let isSkill = (command.kind == "skill")
        let accentColor: Color = isSkill ? .indigo : .blue

        return HStack(alignment: .top, spacing: 12) {
            ZStack {
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .fill(accentColor.opacity(0.13))
                    .frame(width: 32, height: 32)
                SlashCommandIconView(name: command.name, icon: command.icon, kind: command.kind, size: 16)
                    .foregroundColor(accentColor)
            }
            .padding(.top, 1)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text("/\(command.name)")
                        .font(.system(size: 14.5, weight: .semibold, design: .monospaced))
                        .foregroundColor(.primary)
                        .lineLimit(1)

                    Text(isSkill ? "技能" : "系统")
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(accentColor)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1.5)
                        .background(accentColor.opacity(0.12))
                        .clipShape(Capsule())
                }

                let summary = command.description.split(whereSeparator: \.isNewline).first.map(String.init) ?? ""
                if !summary.isEmpty {
                    Text(summary)
                        .font(.system(size: 12.5))
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .contentShape(Rectangle())
    }
}

/// 已选中的命令标签：与 Gemini 模型按钮同高同行的胶囊按钮，右上角带小叉，发送时随消息一起带上。
struct SlashCommandChipView: View {
    let command: SlashCommandOption
    let onClear: () -> Void

    var body: some View {
        Button(action: onClear) {
            HStack(spacing: 5) {
                SlashCommandIconView(name: command.name, icon: command.icon, kind: command.kind, size: 14)
                    .opacity(0.9)
                Text(command.name)
                    .font(.system(size: 13, weight: .medium))
            }
            .foregroundColor(.indigo)
            .padding(.horizontal, 11)
            .frame(height: 32)
            .background(Color.indigo.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(Color.indigo.opacity(0.35), lineWidth: 1)
            )
            .overlay(alignment: .topTrailing) {
                ZStack {
                    Circle()
                        .fill(Color(uiColor: .systemBackground))
                        .frame(width: 16, height: 16)
                    Image(systemName: "xmark.circle.fill")
                        .font(.system(size: 14))
                        .foregroundStyle(.white, Color.indigo.opacity(0.85))
                }
                .offset(x: 5, y: -5)
            }
        }
        .buttonStyle(.plain)
        .padding(.trailing, 4)
        .accessibilityLabel("移除命令 \(command.name)")
    }
}
