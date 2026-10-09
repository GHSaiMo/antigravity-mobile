package com.antigravity.mobile.data.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SlashCommandFilterTest {
    private val cmds = listOf(
        SlashCommandOption(name = "plan", title = "plan", description = "Plan carefully before executing a task.", kind = "system"),
        SlashCommandOption(name = "goal", title = "goal", description = "Run until the specified goal is completely finished.", kind = "system"),
        SlashCommandOption(name = "ppt-master", title = "ppt-master", description = "AI-driven presentation workflow", kind = "skill"),
        SlashCommandOption(name = "tax-invoice-verifier", title = "tax-invoice-verifier", description = "增值税发票查验", kind = "skill")
    )

    @Test
    fun queryOnlyWhileTypingTheCommandWord() {
        assertEquals("", SlashCommandFilter.queryOf("/"))
        assertEquals("pl", SlashCommandFilter.queryOf("/pl"))
        assertNull(SlashCommandFilter.queryOf("/plan 帮我"))
        assertNull(SlashCommandFilter.queryOf("/plan\n"))
        assertNull(SlashCommandFilter.queryOf("hello /plan"))
        assertNull(SlashCommandFilter.queryOf(""))
    }

    @Test
    fun filterPrefersNamePrefixThenAnyMatch() {
        assertEquals(cmds.map { it.name }, SlashCommandFilter.filter(cmds, "").map { it.name })
        // "p" 前缀命中 plan、ppt-master；goal 的描述里有 "specified"/"completely" 里含 p，排在后面
        val names = SlashCommandFilter.filter(cmds, "p").map { it.name }
        assertEquals(listOf("plan", "ppt-master"), names.take(2))
        assertEquals(listOf("tax-invoice-verifier"), SlashCommandFilter.filter(cmds, "发票").map { it.name })
        assertEquals(listOf("goal"), SlashCommandFilter.filter(cmds, "GOAL").map { it.name })
        assertEquals(emptyList<String>(), SlashCommandFilter.filter(cmds, "zzz").map { it.name })
    }

    @Test
    fun splitPrefixRestoresCommandAndText() {
        val (c1, t1) = SlashCommandFilter.splitPrefix("/plan 帮我规划一下", cmds)
        assertEquals("plan", c1?.name); assertEquals("帮我规划一下", t1)
        val (c2, t2) = SlashCommandFilter.splitPrefix("/goal", cmds)
        assertEquals("goal", c2?.name); assertEquals("", t2)
        val (c3, t3) = SlashCommandFilter.splitPrefix("/unknown 文本", cmds)
        assertNull(c3); assertEquals("/unknown 文本", t3)
        val (c4, t4) = SlashCommandFilter.splitPrefix("/plan 文本", emptyList())
        assertNull(c4); assertEquals("/plan 文本", t4)
        val (c5, t5) = SlashCommandFilter.splitPrefix("普通文字", cmds)
        assertNull(c5); assertEquals("普通文字", t5)
        // 不能把 "/planning" 误判成 "/plan"
        val (c6, _) = SlashCommandFilter.splitPrefix("/planning 文本", cmds)
        assertNull(c6)
    }
}
