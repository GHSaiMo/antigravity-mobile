import Foundation
import Observation

/// 全球经典文学阶梯折算结果（用于新建对话启动台与 Token 历史看板双处联动）。
public struct TokenBookEquivalent: Sendable, Equatable {
    public let periodLabel: String
    public let formattedCount: String
    public let unit: String
    public let bookTitle: String
    public let authorOrigin: String
    public let sentence: String

    private struct BookEntry {
        let tokensPerUnit: Double
        let titleZh: String
        let titleEn: String
        let titleJa: String
        let unitZh: String
        let unitEnSingular: String
        let unitEnPlural: String
        let unitJa: String
        let originZh: String
        let originEn: String
        let originJa: String
    }

    private enum UILanguage {
        case zh, en, ja

        static var current: UILanguage {
            let preferred = Locale.preferredLanguages.first?.lowercased() ?? "zh"
            if preferred.hasPrefix("ja") { return .ja }
            if preferred.hasPrefix("en") { return .en }
            return .zh
        }
    }

    // 6 档全球经典文学书目阶梯（横跨法/美/英/捷/巴/哥/西/俄/中）
    private static let tiers: [(maxTokens: Int64, books: [BookEntry])] = [
        (150_000, [
            BookEntry(tokensPerUnit: 25_000, titleZh: "《小王子》", titleEn: "The Little Prince", titleJa: "『星の王子さま』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "法国 · 圣埃克苏佩里", originEn: "France · Antoine de Saint-Exupéry", originJa: "フランス · サン＝テグジュペリ"),
            BookEntry(tokensPerUnit: 30_000, titleZh: "《变形记》", titleEn: "The Metamorphosis", titleJa: "『変身』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "捷克/奥地利 · 卡夫卡", originEn: "Bohemia · Franz Kafka", originJa: "チェコ · フランツ・カフカ"),
            BookEntry(tokensPerUnit: 35_000, titleZh: "《老人与海》", titleEn: "The Old Man and the Sea", titleJa: "『老人と海』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "美国 · 海明威", originEn: "USA · Ernest Hemingway", originJa: "アメリカ · ヘミングウェイ")
        ]),
        (1_500_000, [
            BookEntry(tokensPerUnit: 40_000, titleZh: "《动物农场》", titleEn: "Animal Farm", titleJa: "『動物農場』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "英国 · 乔治·奥威尔", originEn: "UK · George Orwell", originJa: "イギリス · ジョージ・オーウェル"),
            BookEntry(tokensPerUnit: 55_000, titleZh: "《牧羊少年奇幻之旅》", titleEn: "The Alchemist", titleJa: "『アルケミスト』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "巴西 · 保罗·柯艾略", originEn: "Brazil · Paulo Coelho", originJa: "ブラジル · パウロ・コエーリョ"),
            BookEntry(tokensPerUnit: 65_000, titleZh: "《了不起的盖茨比》", titleEn: "The Great Gatsby", titleJa: "『グレート・ギャツビー』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "美国 · 菲茨杰拉德", originEn: "USA · F. Scott Fitzgerald", originJa: "アメリカ · フィッツジェラルド")
        ]),
        (15_000_000, [
            BookEntry(tokensPerUnit: 130_000, titleZh: "《一九八四》", titleEn: "1984", titleJa: "『1984年』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "英国 · 乔治·奥威尔", originEn: "UK · George Orwell", originJa: "イギリス · ジョージ・オーウェル"),
            BookEntry(tokensPerUnit: 190_000, titleZh: "《百年孤独》", titleEn: "One Hundred Years of Solitude", titleJa: "『百年の孤独』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "哥伦比亚 · 加西亚·马尔克斯", originEn: "Colombia · Gabriel García Márquez", originJa: "コロンビア · ガルシア＝マルケス"),
            BookEntry(tokensPerUnit: 250_000, titleZh: "《沙丘》", titleEn: "Dune", titleJa: "『デューン』", unitZh: "本", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "冊", originZh: "美国 · 弗兰克·赫伯特", originEn: "USA · Frank Herbert", originJa: "アメリカ · フランク・ハーバート")
        ]),
        (80_000_000, [
            BookEntry(tokensPerUnit: 520_000, titleZh: "《堂吉诃德》", titleEn: "Don Quixote", titleJa: "『ドン・キホーテ』", unitZh: "部", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "部", originZh: "西班牙 · 塞万提斯", originEn: "Spain · Miguel de Cervantes", originJa: "スペイン · セルバンテス"),
            BookEntry(tokensPerUnit: 620_000, titleZh: "《指环王》全套", titleEn: "The Lord of the Rings", titleJa: "『指輪物語』全巻", unitZh: "套", unitEnSingular: "set of", unitEnPlural: "sets of", unitJa: "セット", originZh: "英国 · 托尔金", originEn: "UK · J.R.R. Tolkien", originJa: "イギリス · トールキン"),
            BookEntry(tokensPerUnit: 730_000, titleZh: "《悲惨世界》", titleEn: "Les Misérables", titleJa: "『レ・ミゼラブル』", unitZh: "部", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "部", originZh: "法国 · 维克多·雨果", originEn: "France · Victor Hugo", originJa: "フランス · ヴィクトル・ユーゴー")
        ]),
        (300_000_000, [
            BookEntry(tokensPerUnit: 780_000, titleZh: "《战争与和平》", titleEn: "War and Peace", titleJa: "『戦争と平和』", unitZh: "部", unitEnSingular: "copy of", unitEnPlural: "copies of", unitJa: "部", originZh: "俄国 · 列夫·托尔斯泰", originEn: "Russia · Leo Tolstoy", originJa: "ロシア · レフ・トルストイ"),
            BookEntry(tokensPerUnit: 850_000, titleZh: "《三体》全套", titleEn: "The Three-Body Trilogy", titleJa: "『三体』三部作", unitZh: "套", unitEnSingular: "set of", unitEnPlural: "sets of", unitJa: "セット", originZh: "中国 · 刘慈欣", originEn: "China · Liu Cixin", originJa: "中国 · 劉慈欣")
        ]),
        (Int64.max, [
            BookEntry(tokensPerUnit: 1_700_000, titleZh: "《追忆似水年华》全套", titleEn: "In Search of Lost Time", titleJa: "『失われた時を求めて』全巻", unitZh: "套", unitEnSingular: "set of", unitEnPlural: "sets of", unitJa: "セット", originZh: "法国 · 马塞尔·普鲁斯特", originEn: "France · Marcel Proust", originJa: "フランス · マルセル・プルースト"),
            BookEntry(tokensPerUnit: 55_000_000, titleZh: "《大英百科全书》", titleEn: "Encyclopædia Britannica", titleJa: "『ブリタニカ百科事典』", unitZh: "套", unitEnSingular: "set of", unitEnPlural: "sets of", unitJa: "セット", originZh: "全球经典参考全书", originEn: "Global Reference Classic", originJa: "世界定番の百科事典")
        ])
    ]

    /// 根据 Token 历史自动匹配阶梯书目（优先近 7 日，无数据时回退累计总 Token；低于 3,000 返回 nil）。
    public static func resolve(from history: TokenHistoryResponse, now: Date = Date()) -> TokenBookEquivalent? {
        let s = history.summary
        let useLast7d = s.last7dTokens >= 3_000
        let tokens = useLast7d ? s.last7dTokens : s.totalTokens
        guard tokens >= 3_000 else { return nil }

        let lang = UILanguage.current
        let tierBooks: [BookEntry]
        if let matched = tiers.first(where: { tokens < $0.maxTokens }) {
            if matched.maxTokens == Int64.max && tokens < 600_000_000 {
                tierBooks = [matched.books[0]]
            } else {
                tierBooks = matched.books
            }
        } else {
            tierBooks = tiers.last!.books
        }

        let daySeed = max(0, Int(now.timeIntervalSince1970 / 86_400))
        let book = tierBooks[daySeed % tierBooks.count]

        let rawCount = Double(tokens) / book.tokensPerUnit
        let formattedCount: String
        if rawCount < 10.0 {
            let rounded = (rawCount * 10.0).rounded() / 10.0
            if abs(rounded - rounded.rounded()) < 0.05 {
                formattedCount = "\(max(1, Int(rounded.rounded())))"
            } else {
                formattedCount = String(format: "%.1f", max(0.1, rounded))
            }
        } else {
            formattedCount = "\(Int(rawCount.rounded()))"
        }

        let isSingle = rawCount < 1.05
        switch lang {
        case .en:
            let period = useLast7d ? "Last 7d" : "Total"
            let unit = isSingle ? book.unitEnSingular : book.unitEnPlural
            let sentence = "\(period) tokens ≈ \(formattedCount) \(unit) \(book.titleEn)"
            return TokenBookEquivalent(
                periodLabel: period,
                formattedCount: formattedCount,
                unit: unit,
                bookTitle: book.titleEn,
                authorOrigin: book.originEn,
                sentence: sentence
            )
        case .ja:
            let period = useLast7d ? "直近7日間の" : "累計"
            let sentence = "\(period)トークン消費は\(book.titleJa) \(formattedCount) \(book.unitJa)分に相当"
            return TokenBookEquivalent(
                periodLabel: period,
                formattedCount: formattedCount,
                unit: book.unitJa,
                bookTitle: book.titleJa,
                authorOrigin: book.originJa,
                sentence: sentence
            )
        case .zh:
            let period = useLast7d ? "近 7 日" : "累计"
            let sentence = "\(period)用量相当于 \(formattedCount) \(book.unitZh)\(book.titleZh)"
            return TokenBookEquivalent(
                periodLabel: period,
                formattedCount: formattedCount,
                unit: book.unitZh,
                bookTitle: book.titleZh,
                authorOrigin: book.originZh,
                sentence: sentence
            )
        }
    }

    /// 计算缓存命中率百分比（0..100）：优先今日有输入记录的日统计，否则回退最近 7 条或全量统计。
    public static func cacheHitRatePercent(from history: TokenHistoryResponse) -> Int? {
        let matrix = history.dailyMatrix
        guard !matrix.isEmpty else { return nil }

        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.dateFormat = "yyyy-MM-dd"
        let todayKey = df.string(from: Date())

        if let todayRec = matrix.last(where: { $0.date == todayKey }) {
            let promptTotal = todayRec.input + todayRec.cacheRead
            if promptTotal > 0 {
                return Int((Double(todayRec.cacheRead) / Double(promptTotal) * 100.0).rounded())
            }
        }

        let recent = matrix.suffix(7)
        let recentInp = recent.reduce(Int64(0)) { $0 + $1.input }
        let recentCache = recent.reduce(Int64(0)) { $0 + $1.cacheRead }
        if recentInp + recentCache > 0 {
            return Int((Double(recentCache) / Double(recentInp + recentCache) * 100.0).rounded())
        }

        let allInp = matrix.reduce(Int64(0)) { $0 + $1.input }
        let allCache = matrix.reduce(Int64(0)) { $0 + $1.cacheRead }
        if allInp + allCache > 0 {
            return Int((Double(allCache) / Double(allInp + allCache) * 100.0).rounded())
        }
        return nil
    }
}

/// 跨页面共享的 Token 历史轻量缓存（供新建对话启动台零闪烁读取，并由首页/配额弹窗同步写入）。
@Observable
@MainActor
public final class TokenHistoryCache {
    public static let shared = TokenHistoryCache()

    public private(set) var history: TokenHistoryResponse? = nil
    private var lastFetchedAt: Date = .distantPast
    private var lastSessionKey: String? = nil
    private var isFetching: Bool = false
    private let refreshInterval: TimeInterval = 3600

    private init() {}

    public func update(_ response: TokenHistoryResponse?, sessionKey: String? = nil) {
        self.history = response
        if response != nil {
            self.lastFetchedAt = Date()
            self.lastSessionKey = sessionKey ?? AppSettings.shared.serverURL?.absoluteString
        } else {
            self.lastFetchedAt = .distantPast
            self.lastSessionKey = nil
        }
    }

    public func clear() {
        history = nil
        lastFetchedAt = .distantPast
        lastSessionKey = nil
    }

    /// 静默预热缓存（已有新鲜缓存或正在请求时直接跳过，失败静默忽略绝不打扰界面）。
    public func prefetchIfNeeded() async {
        guard let url = AppSettings.shared.serverURL else { return }
        let key = url.absoluteString
        if history != nil && lastSessionKey == key && Date().timeIntervalSince(lastFetchedAt) < refreshInterval {
            return
        }
        guard !isFetching else { return }
        isFetching = true
        defer { isFetching = false }
        if let fetched = try? await APIClient.shared.fetchTokenHistory(baseURL: url, force: false) {
            update(fetched, sessionKey: key)
        }
    }
}
