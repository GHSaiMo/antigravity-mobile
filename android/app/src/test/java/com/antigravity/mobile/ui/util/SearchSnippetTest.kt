package com.antigravity.mobile.ui.util

import com.antigravity.mobile.data.model.SearchMatchRange
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SearchSnippetTest {

    private fun r(s: Int, e: Int) = SearchMatchRange(s, e)

    @Test
    fun asciiRange() {
        val text = "... 502 Bad Gateway)"
        val got = highlightRanges(text, listOf(r(12, 19)))
        assertEquals(listOf(12 until 19), got)
        assertEquals("Gateway", text.substring(got[0].first, got[0].last + 1))
    }

    @Test
    fun cjkRange() {
        val text = "... 江苏省税局后端网关宕机"
        // 码点偏移：'网关' 位于第 11、12 个字符之后
        val start = text.indexOf("网关")
        val got = highlightRanges(text, listOf(r(start, start + 2)))
        assertEquals("网关", text.substring(got[0].first, got[0].last + 1))
    }

    @Test
    fun astralCharactersShiftUtf16Offsets() {
        // 😀 是 1 个码点 = 2 个 UTF-16 单元；服务端按码点给偏移，必须换算
        val text = "😀😀abc"
        val got = highlightRanges(text, listOf(r(2, 5)))
        assertEquals("abc", text.substring(got[0].first, got[0].last + 1))
    }

    @Test
    fun outOfBoundsAndInvalidRangesAreSafe() {
        val text = "hello"
        assertTrue(highlightRanges(text, listOf(r(10, 20))).isEmpty())
        assertTrue(highlightRanges(text, listOf(r(3, 3))).isEmpty())
        assertTrue(highlightRanges(text, listOf(r(4, 2))).isEmpty())
        assertEquals(listOf(3 until 5), highlightRanges(text, listOf(r(3, 99))))
        assertTrue(highlightRanges("", listOf(r(0, 1))).isEmpty())
    }

    @Test
    fun overlappingRangesAreDropped() {
        val got = highlightRanges("abcdefgh", listOf(r(0, 4), r(2, 6), r(5, 7)))
        assertEquals(listOf(0 until 4, 5 until 7), got)
    }
}
