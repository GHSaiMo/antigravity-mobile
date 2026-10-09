package com.antigravity.mobile.ui.util

import com.antigravity.mobile.data.model.SearchMatchRange

/**
 * 把 language_server 返回的命中区间（按 Unicode 码点计）换算成 Kotlin 字符串的 UTF-16 区间。
 * 越界的区间会被裁剪或丢弃，空/反向区间丢弃，结果按起点排序且互不重叠。
 */
internal fun highlightRanges(snippet: String, ranges: List<SearchMatchRange>): List<IntRange> {
    if (snippet.isEmpty() || ranges.isEmpty()) return emptyList()
    val total = snippet.codePointCount(0, snippet.length)
    val out = ArrayList<IntRange>(ranges.size)
    var lastEnd = 0
    for (r in ranges.sortedBy { it.startOffset }) {
        val start = r.startOffset.coerceIn(0, total)
        val end = r.endOffsetExclusive.coerceIn(0, total)
        if (end <= start) continue
        val s = snippet.offsetByCodePoints(0, start)
        val e = snippet.offsetByCodePoints(0, end)
        if (s < lastEnd) continue
        out.add(s until e)
        lastEnd = e
    }
    return out
}
