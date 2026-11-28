package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.L10n
import kotlinx.coroutines.flow.MutableStateFlow
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.math.abs
import kotlin.math.roundToInt

/**
 * 全球经典文学阶梯折算结果（用于新建对话启动台与 Token 历史看板双处联动）。
 */
data class TokenBookEquivalent(
    val periodLabel: String,
    val formattedCount: String,
    val unit: String,
    val bookTitle: String,
    val authorOrigin: String,
    val sentence: String
) {
    private data class BookEntry(
        val tokensPerUnit: Double,
        val titleZh: String,
        val titleEn: String,
        val titleJa: String,
        val unitZh: String,
        val unitEnSingular: String,
        val unitEnPlural: String,
        val unitJa: String,
        val originZh: String,
        val originEn: String,
        val originJa: String
    )

    private data class Tier(val maxTokens: Long, val books: List<BookEntry>)

    companion object {
        // 11 档人类经典名著与知识库阶梯（横跨短篇名著至百亿级全球科研文献，每档倍数收敛在 0.6~20×）
        private val TIERS = listOf(
            Tier(
                200_000L, listOf(
                    BookEntry(25_000.0, "《小王子》", "The Little Prince", "『星の王子さま』", "本", "copy of", "copies of", "冊", "法国 · 圣埃克苏佩里", "France · Antoine de Saint-Exupéry", "フランス · サン＝テグジュペリ"),
                    BookEntry(30_000.0, "《变形记》", "The Metamorphosis", "『変身』", "本", "copy of", "copies of", "冊", "捷克/奥地利 · 卡夫卡", "Bohemia · Franz Kafka", "チェコ · フランツ・カフカ"),
                    BookEntry(35_000.0, "《老人与海》", "The Old Man and the Sea", "『老人と海』", "本", "copy of", "copies of", "冊", "美国 · 海明威", "USA · Ernest Hemingway", "アメリカ · ヘミングウェイ")
                )
            ),
            Tier(
                1_000_000L, listOf(
                    BookEntry(40_000.0, "《动物农场》", "Animal Farm", "『動物農場』", "本", "copy of", "copies of", "冊", "英国 · 乔治·奥威尔", "UK · George Orwell", "イギリス · ジョージ・オーウェル"),
                    BookEntry(55_000.0, "《牧羊少年奇幻之旅》", "The Alchemist", "『アルケミスト』", "本", "copy of", "copies of", "冊", "巴西 · 保罗·柯艾略", "Brazil · Paulo Coelho", "ブラジル · パウロ・コエーリョ"),
                    BookEntry(65_000.0, "《了不起的盖茨比》", "The Great Gatsby", "『グレート・ギャツビー』", "本", "copy of", "copies of", "冊", "美国 · 菲茨杰拉德", "USA · F. Scott Fitzgerald", "アメリカ · フィッツジェラルド")
                )
            ),
            Tier(
                4_000_000L, listOf(
                    BookEntry(130_000.0, "《一九八四》", "1984", "『1984年』", "本", "copy of", "copies of", "冊", "英国 · 乔治·奥威尔", "UK · George Orwell", "イギリス · ジョージ・オーウェル"),
                    BookEntry(190_000.0, "《百年孤独》", "One Hundred Years of Solitude", "『百年の孤独』", "本", "copy of", "copies of", "冊", "哥伦比亚 · 加西亚·马尔克斯", "Colombia · Gabriel García Márquez", "コロンビア · ガルシア＝マルケス"),
                    BookEntry(250_000.0, "《沙丘》", "Dune", "『デューン』", "本", "copy of", "copies of", "冊", "美国 · 弗兰克·赫伯特", "USA · Frank Herbert", "アメリカ · フランク・ハーバート")
                )
            ),
            Tier(
                15_000_000L, listOf(
                    BookEntry(520_000.0, "《堂吉诃德》", "Don Quixote", "『ドン・キホーテ』", "部", "copy of", "copies of", "部", "西班牙 · 塞万提斯", "Spain · Miguel de Cervantes", "スペイン · セルバンテス"),
                    BookEntry(620_000.0, "《指环王》全套", "The Lord of the Rings", "『指輪物語』全巻", "套", "set of", "sets of", "セット", "英国 · 托尔金", "UK · J.R.R. Tolkien", "イギリス · トールキン"),
                    BookEntry(730_000.0, "《悲惨世界》", "Les Misérables", "『レ・ミゼラブル』", "部", "copy of", "copies of", "部", "法国 · 维克多·雨果", "France · Victor Hugo", "フランス · ヴィクトル・ユーゴー"),
                    BookEntry(780_000.0, "《战争与和平》", "War and Peace", "『戦争と平和』", "部", "copy of", "copies of", "部", "俄国 · 列夫·托尔斯泰", "Russia · Leo Tolstoy", "ロシア · レフ・トルストイ"),
                    BookEntry(850_000.0, "《三体》全套", "The Three-Body Trilogy", "『三体』三部作", "套", "set of", "sets of", "セット", "中国 · 刘慈欣", "China · Liu Cixin", "中国 · 劉慈欣")
                )
            ),
            Tier(
                45_000_000L, listOf(
                    BookEntry(1_700_000.0, "《追忆似水年华》全套", "In Search of Lost Time", "『失われた時を求めて』全巻", "套", "set of", "sets of", "セット", "法国 · 马塞尔·普鲁斯特", "France · Marcel Proust", "フランス · マルセル・プルースト"),
                    BookEntry(2_300_000.0, "《冰与火之歌》全套", "A Song of Ice and Fire", "『氷と炎の歌』全巻", "套", "set of", "sets of", "セット", "美国 · 乔治·R·R·马丁", "USA · George R.R. Martin", "アメリカ · ジョージ・R・R・マーティン")
                )
            ),
            Tier(
                350_000_000L, listOf(
                    BookEntry(55_000_000.0, "《大英百科全书》", "Encyclopædia Britannica", "『ブリタニカ百科事典』", "套", "set of", "sets of", "セット", "全球经典参考全书 · 32 卷", "Global Reference Classic · 32 Vols", "世界定番の百科事典 · 全32巻")
                )
            ),
            Tier(
                1_200_000_000L, listOf(
                    BookEntry(400_000_000.0, "《永乐大典》", "Yongle Encyclopedia", "『永楽大典』", "部", "edition of", "editions of", "部", "中国明代 · 22,877 卷巨型类书", "Ming Dynasty · 22,877-Volume Encyclopedia", "明代中国 · 全22,877巻の類書")
                )
            ),
            Tier(
                4_000_000_000L, listOf(
                    BookEntry(1_000_000_000.0, "《四库全书》", "Siku Quanshu", "『四庫全書』", "套", "set of", "sets of", "セット", "中国清代 · 79,338 卷官修丛书", "Qing Dynasty · 79,338-Volume Imperial Library", "清代中国 · 全79,338巻の叢書")
                )
            ),
            Tier(
                20_000_000_000L, listOf(
                    BookEntry(4_500_000_000.0, "《古登堡计划》全库", "Project Gutenberg Archive", "『プロジェクト・グーテンベルク』全書庫", "座", "archive of", "archives of", "個", "7 万+部人类公版名著全集", "70,000+ Public-Domain Classics", "7万冊超のパブリックドメイン書庫"),
                    BookEntry(7_000_000_000.0, "《英文维基百科》全站", "English Wikipedia", "英語版ウィキペディア", "部", "edition of", "editions of", "版", "全球开源百科 · 720 万+条目", "Global Open Encyclopedia · 7.2M+ Articles", "世界最大の百科事典 · 720万記事超")
                )
            ),
            Tier(
                120_000_000_000L, listOf(
                    BookEntry(24_000_000_000.0, "《全语种维基百科》", "All-Language Wikipedia", "全言語版ウィキペディア", "部", "edition of", "editions of", "版", "300+ 语言 · 6,600 万+条目知识库", "300+ Languages · 66M+ Articles", "300言語超 · 6,600万記事超の知識庫"),
                    BookEntry(28_000_000_000.0, "《arXiv 学术论文全库》", "arXiv Scientific Archive", "arXiv 学術論文全アーカイブ", "座", "archive of", "archives of", "基", "全球前沿科研预印本全库", "Global Open Scientific Preprint Archive", "世界の最先端科学論文アーカイブ")
                )
            ),
            Tier(
                Long.MAX_VALUE, listOf(
                    BookEntry(70_000_000_000.0, "《Semantic Scholar 论文库》", "Semantic Scholar Corpus", "Semantic Scholar 論文コーパス", "座", "archive of", "archives of", "基", "3,880 万篇全球科研文献全集", "38.8M Global Research Papers", "世界3,880万報の研究論文コーパス")
                )
            )
        )

        fun resolve(history: TokenHistoryResponse, nowMs: Long = System.currentTimeMillis()): TokenBookEquivalent? {
            val s = history.summary
            val useLast7d = s.last7dTokens >= 3_000L
            val tokens = if (useLast7d) s.last7dTokens else s.totalTokens
            if (tokens < 3_000L) return null

            val matched = TIERS.firstOrNull { tokens < it.maxTokens } ?: TIERS.last()
            val tierBooks = matched.books

            val daySeed = ((nowMs / 86_400_000L).toInt()).coerceAtLeast(0)
            val book = tierBooks[daySeed % tierBooks.size]

            val rawCount = tokens.toDouble() / book.tokensPerUnit
            val formattedCount = if (rawCount < 10.0) {
                val rounded = (rawCount * 10.0).roundToInt() / 10.0
                if (abs(rounded - rounded.roundToInt()) < 0.05) {
                    rounded.roundToInt().coerceAtLeast(1).toString()
                } else {
                    String.format(Locale.US, "%.1f", rounded.coerceAtLeast(0.1))
                }
            } else {
                rawCount.roundToInt().toString()
            }

            val isSingle = rawCount < 1.05
            return when (L10n.currentLang) {
                L10n.AppLang.EN -> {
                    val period = if (useLast7d) "Last 7d" else "Total"
                    val unit = if (isSingle) book.unitEnSingular else book.unitEnPlural
                    TokenBookEquivalent(
                        periodLabel = period,
                        formattedCount = formattedCount,
                        unit = unit,
                        bookTitle = book.titleEn,
                        authorOrigin = book.originEn,
                        sentence = "$period tokens ≈ $formattedCount $unit ${book.titleEn}"
                    )
                }
                L10n.AppLang.JA -> {
                    val period = if (useLast7d) "直近7日間の" else "累計"
                    TokenBookEquivalent(
                        periodLabel = period,
                        formattedCount = formattedCount,
                        unit = book.unitJa,
                        bookTitle = book.titleJa,
                        authorOrigin = book.originJa,
                        sentence = "${period}トークン消費は${book.titleJa} $formattedCount ${book.unitJa}分に相当"
                    )
                }
                L10n.AppLang.ZH -> {
                    val period = if (useLast7d) "近 7 日" else "累计"
                    TokenBookEquivalent(
                        periodLabel = period,
                        formattedCount = formattedCount,
                        unit = book.unitZh,
                        bookTitle = book.titleZh,
                        authorOrigin = book.originZh,
                        sentence = "${period}用量相当于 $formattedCount ${book.unitZh}${book.titleZh}"
                    )
                }
            }
        }

        fun cacheHitRatePercent(history: TokenHistoryResponse): Int? {
            val matrix = history.dailyMatrix
            if (matrix.isEmpty()) return null

            val todayKey = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())
            matrix.lastOrNull { it.date == todayKey }?.let { rec ->
                val promptTotal = rec.input + rec.cacheRead
                if (promptTotal > 0L) {
                    return ((rec.cacheRead.toDouble() / promptTotal.toDouble()) * 100.0).roundToInt()
                }
            }

            val recent = matrix.takeLast(7)
            val recentInp = recent.sumOf { it.input }
            val recentCache = recent.sumOf { it.cacheRead }
            if (recentInp + recentCache > 0L) {
                return ((recentCache.toDouble() / (recentInp + recentCache).toDouble()) * 100.0).roundToInt()
            }

            val allInp = matrix.sumOf { it.input }
            val allCache = matrix.sumOf { it.cacheRead }
            if (allInp + allCache > 0L) {
                return ((allCache.toDouble() / (allInp + allCache).toDouble()) * 100.0).roundToInt()
            }
            return null
        }
    }
}

/** 跨页面共享的 Token 历史轻量缓存（供新建对话启动台零闪烁读取）。 */
object TokenHistoryCache {
    val history = MutableStateFlow<TokenHistoryResponse?>(null)
}
