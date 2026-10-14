import SwiftUI

public enum TableColumnAlignment: Sendable, Equatable {
    case leading
    case center
    case trailing
    
    public var swiftUIAlignment: Alignment {
        switch self {
        case .leading: return .leading
        case .center: return .center
        case .trailing: return .trailing
        }
    }
    
    public var textAlignment: TextAlignment {
        switch self {
        case .leading: return .leading
        case .center: return .center
        case .trailing: return .trailing
        }
    }
}

public enum MarkdownAlertType: String, Sendable, CaseIterable {
    case note
    case tip
    case important
    case warning
    case caution
    
    public var title: String {
        switch self {
        case .note: return "Note"
        case .tip: return "Tip"
        case .important: return "Important"
        case .warning: return "Warning"
        case .caution: return "Caution"
        }
    }
    
    public var systemImage: String {
        switch self {
        case .note: return "info.circle.fill"
        case .tip: return "lightbulb.fill"
        case .important: return "exclamationmark.circle.fill"
        case .warning: return "exclamationmark.triangle.fill"
        case .caution: return "octagon.fill"
        }
    }
    
    public var accentColor: Color {
        switch self {
        case .note: return Color(red: 10/255, green: 132/255, blue: 255/255)       // #0A84FF
        case .tip: return Color(red: 48/255, green: 209/255, blue: 88/255)         // #30D158
        case .important: return Color(red: 191/255, green: 90/255, blue: 242/255) // #BF5AF2
        case .warning: return Color(red: 255/255, green: 159/255, blue: 10/255)    // #FF9F0A
        case .caution: return Color(red: 255/255, green: 69/255, blue: 58/255)     // #FF453A
        }
    }
}

public enum MarkdownBlock: Identifiable {
    case frontmatter(id: String, rawContent: String, lineCount: Int)
    case heading(id: String, level: Int, text: String)
    case divider(id: String)
    case codeBlock(id: String, lang: String, code: String)
    case table(id: String, headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = [])
    case list(id: String, items: [String])
    case orderedList(id: String, startIndex: Int, items: [String])
    case alert(id: String, type: MarkdownAlertType, content: String)
    case blockquote(id: String, content: String)
    case paragraph(id: String, text: String)
    case image(id: String, alt: String, url: String)
    case agentEmbed(id: String, src: String)
    case carousel(id: String, slides: [MarkdownCarouselSlide])
    
    public var id: String {
        switch self {
        case .frontmatter(let id, _, _): return id
        case .heading(let id, _, _): return id
        case .divider(let id): return id
        case .codeBlock(let id, _, _): return id
        case .table(let id, _, _, _): return id
        case .list(let id, _): return id
        case .orderedList(let id, _, _): return id
        case .alert(let id, _, _): return id
        case .blockquote(let id, _): return id
        case .paragraph(let id, _): return id
        case .image(let id, _, _): return id
        case .agentEmbed(let id, _): return id
        case .carousel(let id, _): return id
        }
    }
}

public struct MarkdownCarouselSlide: Identifiable, Sendable, Equatable {
    public let id: String
    public let title: String?
    public let content: String
    
    public init(id: String, title: String? = nil, content: String) {
        self.id = id
        self.title = title
        self.content = content
    }
}

final class MarkdownBlockCache: @unchecked Sendable {
    static let shared = MarkdownBlockCache()
    private let lock = NSLock()
    private var storage: [Int: [MarkdownBlock]] = [:]
    
    func blocks(for text: String) -> [MarkdownBlock] {
        let hash = text.hashValue
        lock.lock()
        if let cached = storage[hash] {
            lock.unlock()
            return cached
        }
        lock.unlock()
        
        let parsed = MarkdownParser.parse(text)
        lock.lock()
        if storage.count > 150 {
            storage.removeAll(keepingCapacity: true)
        }
        storage[hash] = parsed
        lock.unlock()
        return parsed
    }
}
