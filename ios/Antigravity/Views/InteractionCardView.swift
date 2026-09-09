import SwiftUI

public struct InteractionCardView: View {
    public let interaction: PendingInteraction
    public let isSubmitting: Bool
    public let onSubmit: (String, String?, String?) -> Void
    public let onSkip: () -> Void
    
    @State private var selectedOptionId: String
    @State private var writeInText: String = ""
    @State private var targetText: String
    
    public init(
        interaction: PendingInteraction,
        isSubmitting: Bool,
        onSubmit: @escaping (String, String?, String?) -> Void,
        onSkip: @escaping () -> Void
    ) {
        self.interaction = interaction
        self.isSubmitting = isSubmitting
        self.onSubmit = onSubmit
        self.onSkip = onSkip
        _selectedOptionId = State(initialValue: interaction.defaultOptionId ?? interaction.options.first?.id ?? "1")
        _targetText = State(initialValue: interaction.target ?? "")
    }
    
    private var headerIconName: String {
        switch interaction.type {
        case "permission", "file_permission":
            return "hand.raised.fill"
        case "ask_question":
            return "questionmark.bubble.fill"
        case "run_command":
            return "terminal.fill"
        default:
            return "questionmark.circle.fill"
        }
    }
    
    private var headerIconColor: Color {
        switch interaction.type {
        case "permission", "file_permission":
            return .blue
        case "ask_question":
            return .purple
        case "run_command":
            return .orange
        default:
            return .blue
        }
    }
    
    private var isDenySelected: Bool {
        if let opt = interaction.options.first(where: { $0.id == selectedOptionId }) {
            return opt.isDeny == true || opt.id == "5" || opt.id == "__write_in__"
        }
        return selectedOptionId == "5" || selectedOptionId == "__write_in__"
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Header: Icon + Question Title
            HStack(alignment: .center, spacing: 8) {
                Image(systemName: headerIconName)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(headerIconColor)
                
                Text(interaction.title)
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .fixedSize(horizontal: false, vertical: true)
                
                Spacer()
            }
            
            // Target path / command preview box
            if let target = interaction.target, !target.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text(target)
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.secondary)
                        .lineLimit(3)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 7)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
            }
            
            // Radio Options Group
            VStack(spacing: 5) {
                ForEach(interaction.options) { option in
                    let isSelected = selectedOptionId == option.id
                    
                    Button {
                        UISelectionFeedbackGenerator().selectionChanged()
                        withAnimation(.easeInOut(duration: 0.15)) {
                            selectedOptionId = option.id
                        }
                    } label: {
                        HStack(alignment: .center, spacing: 9) {
                            // Number badge [1], [2], etc.
                            Text(option.id)
                                .font(.system(size: 11, weight: .bold, design: .monospaced))
                                .foregroundColor(isSelected ? .white : .secondary)
                                .frame(width: 20, height: 20)
                                .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                            
                            // Option text
                            Text(option.text)
                                .font(.system(size: 13, weight: isSelected ? .medium : .regular))
                                .foregroundColor(isSelected ? .primary : .secondary)
                                .multilineTextAlignment(.leading)
                                .lineLimit(2)
                            
                            Spacer()
                            
                            // Radio indicator
                            Image(systemName: isSelected ? "largecircle.fill.circle" : "circle")
                                .font(.system(size: 15))
                                .foregroundColor(isSelected ? .blue : Color(uiColor: .tertiaryLabel))
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 7.5)
                        .background(
                            RoundedRectangle(cornerRadius: 9, style: .continuous)
                                .fill(isSelected ? Color.blue.opacity(0.08) : Color(uiColor: .secondarySystemBackground))
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 9, style: .continuous)
                                .stroke(isSelected ? Color.blue.opacity(0.4) : Color.clear, lineWidth: 1)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
            
            // Inline Write-In Input for Deny or Custom input
            if interaction.hasWriteIn == true && isDenySelected {
                VStack(alignment: .leading, spacing: 4) {
                    TextField(
                        interaction.writeInPlaceholder ?? "(告诉 Agent 应该怎么做)",
                        text: $writeInText,
                        axis: .vertical
                    )
                    .font(.system(size: 12.5))
                    .padding(.horizontal, 10)
                    .padding(.vertical, 7)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                    )
                }
                .transition(.opacity.combined(with: .move(edge: .top)))
            }
            
            // Bottom Action Bar: Skip & Submit (Blue Background)
            HStack(spacing: 10) {
                Spacer()
                
                // Skip Button
                Button(action: onSkip) {
                    Text("Skip")
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 7)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
                .buttonStyle(.plain)
                .disabled(isSubmitting)
                
                // Submit Button (Blue Background)
                Button {
                    let text = isDenySelected ? writeInText : nil
                    onSubmit(selectedOptionId, text, interaction.target)
                } label: {
                    HStack(spacing: 5) {
                        if isSubmitting {
                            ProgressView()
                                .tint(.white)
                                .scaleEffect(0.8)
                        } else {
                            Text("Submit")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(.white)
                            Text("↵")
                                .font(.system(size: 12, weight: .regular))
                                .foregroundColor(.white.opacity(0.7))
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 7)
                    .background(Color.blue)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .shadow(color: Color.blue.opacity(0.3), radius: 3, x: 0, y: 1.5)
                }
                .buttonStyle(.plain)
                .disabled(isSubmitting)
            }
            .padding(.top, 2)
        }
        .padding(12)
        .background(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .fill(Color(uiColor: .secondarySystemGroupedBackground))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .stroke(Color.primary.opacity(0.08), lineWidth: 1)
        )
        .shadow(color: Color.black.opacity(0.10), radius: 10, x: 0, y: 4)
        .padding(.horizontal, 12)
        .padding(.bottom, 6)
    }
}
