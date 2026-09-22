package com.antigravity.mobile.data.service

import kotlinx.serialization.json.Json

object JsonConfig {
    val instance: Json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = true
        coerceInputValues = true
    }
}
