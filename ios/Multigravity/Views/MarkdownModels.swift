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

public enum MarkdownBlock: Identifiable {
    case frontmatter(id: String, rawContent: String, lineCount: Int)
    case heading(id: String, level: Int, text: String)
    case divider(id: String)
    case codeBlock(id: String, lang: String, code: String)
    case table(id: String, headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = [])
    case list(id: String, items: [String])
    case orderedList(id: String, startIndex: Int, items: [String])
    case paragraph(id: String, text: String)
    case image(id: String, alt: String, url: String)
    
    public var id: String {
        switch self {
        case .frontmatter(let id, _, _): return id
        case .heading(let id, _, _): return id
        case .divider(let id): return id
        case .codeBlock(let id, _, _): return id
        case .table(let id, _, _, _): return id
        case .list(let id, _): return id
        case .orderedList(let id, _, _): return id
        case .paragraph(let id, _): return id
        case .image(let id, _, _): return id
        }
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
