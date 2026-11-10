package com.antigravity.mobile.ui.components

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class VoiceTranscriptCleanerTest {

    @Test
    fun testStripInstructionsAndExtractDialogue() {
        val raw = """
            <voice_instructions>
            With , a live voice handles casual chat, sends tasks here, and speaks your reply. Without it, reply in markdown.

            1. Share one persona with the voice: continue from its last line with no filler, restatements, or mentions of the voice or instructions, correcting any hedge or small-talk slip cleanly.

            2. Fulfill any running task and the new request unless it cancels, replaces, or narrows the first; on stop, cancel, or ignore, stop immediately and reply briefly.

            3. Act only on the current request, using prior lines only as context.

            lists timestamped user and talker lines oldest first (trailing hyphen marks cutoffs), matched in order to attached audio/wav clips. Check audio if speech looks misheard or clipped, honor self-corrections, and ask one short question instead of guessing unclear targets.
            Claim finished or logged only after tool
            </voice_instructions>
            <talker_agent_context>
            [2026-11-09T00:34:10+08:00] user: "帮我分析一下 Palantir Ontology"
            [2026-11-09T00:34:15+08:00] talker: "好的，我已经开始为您整理关于 Palantir Ontology 的报告。"
            </talker_agent_context>
        """.trimIndent()

        val cleaned = VoiceTranscriptCleaner.clean(raw)
        val expected = "用户: 帮我分析一下 Palantir Ontology\n\n智能体: 好的，我已经开始为您整理关于 Palantir Ontology 的报告。"
        assertEquals(expected, cleaned)
    }

    @Test
    fun testUnclosedInstructionsTags() {
        val raw = """
            <voice_instructions>
            1. Share persona
            lists timestamped user
            <talker_agent_context>
            [2026-11-09T00:34:10+08:00] user: "帮我分析一下 Palantir Ontology"
            [2026-11-09T00:34:15+08:00] talker: "好的，我已经开始为您整理关于 Palantir Ontology 的报告。"
        """.trimIndent()

        val cleaned = VoiceTranscriptCleaner.clean(raw)
        val expected = "用户: 帮我分析一下 Palantir Ontology\n\n智能体: 好的，我已经开始为您整理关于 Palantir Ontology 的报告。"
        assertEquals(expected, cleaned)
    }

    @Test
    fun testOnlyInstructionsReturnsEmpty() {
        val raw = """
            <voice_instructions>
            With , a live voice handles casual chat.
            1. Share one persona
            </voice_instructions>
        """.trimIndent()

        val cleaned = VoiceTranscriptCleaner.clean(raw)
        assertTrue(cleaned.isEmpty())
    }

    @Test
    fun testChineseRolesAndTimestamps() {
        val raw = """
            [12:30:05] 用户: "测试问题"
            [12:30:10] 智能体: "测试回答"
        """.trimIndent()

        val cleaned = VoiceTranscriptCleaner.clean(raw)
        val expected = "用户: 测试问题\n\n智能体: 测试回答"
        assertEquals(expected, cleaned)
    }

    @Test
    fun testNormalizeCJKSpacing() {
        val raw = "我 想 问 一 下 关 于 这 个 项 目 的 进 展"
        val expected = "我想问一下关于这个项目的进展"
        assertEquals(expected, VoiceTranscriptCleaner.normalizeCJKSpacing(raw))

        val mixed = "你 好   ，   我 是 智 能 体 ， 请 问 Hello World 怎么写？"
        val expectedMixed = "你好，我是智能体，请问 Hello World 怎么写？"
        assertEquals(expectedMixed, VoiceTranscriptCleaner.normalizeCJKSpacing(mixed))

        val englishPreserved = "Hello World this is Antigravity 2.31.0"
        assertEquals(englishPreserved, VoiceTranscriptCleaner.normalizeCJKSpacing(englishPreserved))
    }
}
