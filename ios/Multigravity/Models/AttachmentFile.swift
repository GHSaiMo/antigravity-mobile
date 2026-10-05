import Foundation

public enum UploadState: String, Codable, Sendable {
    case pending, uploading, done, failed
}

/// A non-image file attached to the draft (document, archive, source file).
///
/// The file is copied into app storage (`fileName` inside the draft's directory) as soon as it is
/// picked and uploaded to the gateway in the background. Once `state == .done`, `attachmentId` and
/// `line` come from the gateway and are what the message actually references.
public struct DraftFile: Identifiable, Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var size: Int64
    /// File name of the local copy inside the draft directory (relative: container paths change across updates).
    public var fileName: String
    public var state: UploadState
    public var progress: Double
    public var attachmentId: String?
    public var line: String?
    public var error: String?
    
    public init(
        id: String = UUID().uuidString,
        name: String,
        size: Int64,
        fileName: String,
        state: UploadState = .pending,
        progress: Double = 0,
        attachmentId: String? = nil,
        line: String? = nil,
        error: String? = nil
    ) {
        self.id = id
        self.name = name
        self.size = size
        self.fileName = fileName
        self.state = state
        self.progress = progress
        self.attachmentId = attachmentId
        self.line = line
        self.error = error
    }
    
    public var isUploaded: Bool { state == .done && attachmentId != nil }
}

/// Response of POST /api/v1/attachments.
public struct UploadedAttachment: Codable, Sendable {
    public let id: String
    public let path: String
    public let name: String
    public let size: Int64
    public let mime: String
    public let sha256: String
    public let kind: String
    public let line: String
}

/// Reference sent in SendUserCascadeMessage; the gateway resolves it to a path block in the text.
public struct AttachmentRef: Codable, Sendable {
    public let id: String
}

/// Client-side mirror of the gateway's attachment rules (internal/proxy/attachments.go), so that
/// obviously unsupported files are rejected before any bytes are uploaded. The gateway remains the
/// source of truth.
public enum AttachmentRules {
    public static let maxFileBytes: Int64 = 50 * 1024 * 1024
    public static let maxFilesPerMessage = 5
    public static let maxTotalBytes: Int64 = 100 * 1024 * 1024
    public static let blockHeader = "📎 附件（已上传到本机，可直接读取）："
    
    private static let allowedExtensions: Set<String> = [
        // documents
        "doc", "docx", "dot", "dotx", "rtf", "odt", "pages", "pdf", "md", "markdown", "txt", "log", "epub",
        // spreadsheets
        "xls", "xlsx", "xlsm", "csv", "tsv", "ods", "numbers",
        // presentations
        "ppt", "pptx", "pps", "ppsx", "odp", "key",
        // archives
        "zip", "tar", "gz", "tgz", "7z",
        // source & config
        "py", "js", "mjs", "cjs", "ts", "tsx", "jsx", "go", "rs", "java", "kt", "kts", "swift", "m", "mm",
        "c", "cc", "cpp", "cxx", "h", "hpp", "cs", "rb", "php", "lua", "dart", "scala", "r", "pl",
        "sh", "bash", "zsh", "bat", "ps1", "sql", "vue", "svelte", "gradle", "proto",
        "json", "jsonl", "yaml", "yml", "toml", "ini", "cfg", "conf", "xml", "html", "htm", "css", "scss",
        "less", "ipynb", "dockerfile", "makefile"
    ]
    
    public static func isAllowedName(_ name: String) -> Bool {
        let lower = name.lowercased()
        let ext = lower.contains(".") ? (lower as NSString).pathExtension : lower
        return allowedExtensions.contains(ext)
    }
    
    /// Returns a user-facing rejection reason, or nil when the file may be attached.
    public static func validate(name: String, size: Int64, existing: [DraftFile]) -> String? {
        guard isAllowedName(name) else {
            let ext = (name as NSString).pathExtension
            return "不支持的文件类型（\(ext.isEmpty ? "无扩展名" : ext)）：\(name)"
        }
        if size <= 0 { return "文件为空：\(name)" }
        if size > maxFileBytes { return "文件超过 \(maxFileBytes / 1024 / 1024)MB：\(name)" }
        if existing.count >= maxFilesPerMessage { return "一条消息最多附带 \(maxFilesPerMessage) 个文件" }
        if existing.reduce(0, { $0 + $1.size }) + size > maxTotalBytes {
            return "附件总大小超过 \(maxTotalBytes / 1024 / 1024)MB"
        }
        return nil
    }
    
    /// Same text the gateway appends to the user's message, used for optimistic bubbles.
    public static func formatBlock(_ files: [DraftFile]) -> String {
        let lines = files.compactMap { $0.line }.filter { !$0.isEmpty }
        guard !lines.isEmpty else { return "" }
        return blockHeader + "\n" + lines.joined(separator: "\n")
    }
    
    public static func appendBlock(to text: String, files: [DraftFile]) -> String {
        let block = formatBlock(files)
        guard !block.isEmpty else { return text }
        if text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { return block }
        var base = text
        while base.hasSuffix("\n") { base.removeLast() }
        return base + "\n\n" + block
    }
    
    public struct ParsedFile: Identifiable, Hashable, Sendable {
        public var id: String { path }
        public let path: String
        public let name: String
        public let ext: String
        public let sizeLabel: String
    }
    
    private static let lineRegex = try! NSRegularExpression(pattern: #"^- (.+) \(([^,()]*), ([^()]+)\)$"#)
    
    /// Splits a user message into its body text and the attachment lines the gateway appended.
    public static func parseBlock(_ text: String) -> (body: String, files: [ParsedFile]) {
        guard let headerRange = text.range(of: blockHeader, options: .backwards) else { return (text, []) }
        let tail = text[headerRange.upperBound...].trimmingCharacters(in: .newlines)
        var files: [ParsedFile] = []
        for raw in tail.components(separatedBy: "\n") {
            let line = raw.trimmingCharacters(in: .whitespaces)
            let ns = line as NSString
            guard let m = lineRegex.firstMatch(in: line, range: NSRange(location: 0, length: ns.length)), m.numberOfRanges == 4 else { continue }
            let path = ns.substring(with: m.range(at: 1))
            var name = (path as NSString).lastPathComponent
            // strip the "<id16>-" prefix the gateway adds to avoid collisions
            if name.count > 17 {
                let idx = name.index(name.startIndex, offsetBy: 16)
                let prefix = name[name.startIndex..<idx]
                if name[idx] == "-" && prefix.allSatisfy({ $0.isHexDigit && !$0.isUppercase }) {
                    name = String(name[name.index(after: idx)...])
                }
            }
            files.append(ParsedFile(path: path, name: name, ext: ns.substring(with: m.range(at: 2)), sizeLabel: ns.substring(with: m.range(at: 3))))
        }
        guard !files.isEmpty else { return (text, []) }
        let body = String(text[text.startIndex..<headerRange.lowerBound]).trimmingCharacters(in: .whitespacesAndNewlines)
        return (body, files)
    }
}
