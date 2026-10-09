package com.antigravity.mobile.data.service

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ConversationEventsClientTest {
    @Test
    fun parsesHelloAndChangedFrames() {
        assertEquals(ConversationEventFrame("hello", 0), parseConversationEventFrame("""{"type":"hello","rev":0}"""))
        assertEquals(ConversationEventFrame("changed", 7), parseConversationEventFrame("""{"type":"changed","rev":7}""" + "\n"))
    }

    @Test
    fun missingRevDefaultsToZero() {
        assertEquals(ConversationEventFrame("changed", 0), parseConversationEventFrame("""{"type":"changed"}"""))
    }

    @Test
    fun ignoresMalformedOrTypelessFrames() {
        assertNull(parseConversationEventFrame("not json"))
        assertNull(parseConversationEventFrame("""{"rev":3}"""))
        assertNull(parseConversationEventFrame("[]"))
    }

    @Test
    fun unknownFutureFieldsAreIgnored() {
        assertEquals(
            ConversationEventFrame("changed", 2),
            parseConversationEventFrame("""{"type":"changed","rev":2,"cascades":["a","b"]}""")
        )
    }
}
