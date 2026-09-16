package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class PairingInfo(
    val host: String,
    val port: Int,
    val code: String,
    val ssl: Boolean = false,
    val lanHost: String? = null,
    val ipv6Host: String? = null,
    val ddnsHost: String? = null,
    val relayHost: String? = null
) {
    fun candidateBaseUrls(): List<String> {
        val list = mutableListOf<String>()
        lanHost?.takeIf { it.isNotBlank() }?.let { list.add(formatUrl(it, port, ssl)) }
        val primary = formatUrl(host, port, ssl)
        if (!list.contains(primary)) list.add(primary)
        relayHost?.takeIf { it.isNotBlank() }?.let {
            val u = formatUrl(it, port, ssl)
            if (!list.contains(u)) list.add(u)
        }
        ipv6Host?.takeIf { it.isNotBlank() }?.let {
            val u = formatUrl(it, port, ssl)
            if (!list.contains(u)) list.add(u)
        }
        ddnsHost?.takeIf { it.isNotBlank() }?.let {
            val u = formatUrl(it, port, ssl)
            if (!list.contains(u)) list.add(u)
        }
        return list
    }

    companion object {
        fun formatUrl(host: String, port: Int, ssl: Boolean): String {
            val scheme = if (ssl) "https://" else "http://"
            var formattedHost = host.trim()
            if (!formattedHost.startsWith("[") && formattedHost.count { it == ':' } >= 2) {
                formattedHost = "[$formattedHost]"
            }
            return "$scheme$formattedHost:$port"
        }

        fun parseFromUri(uriString: String): PairingInfo? {
            val trimmed = uriString.trim()
            if (!trimmed.startsWith("agy://pair", ignoreCase = true)) return null

            val queryPart = trimmed.substringAfter('?', "")
            if (queryPart.isEmpty()) return null

            val params = queryPart.split("&").associate {
                val pair = it.split("=", limit = 2)
                val key = pair[0].lowercase()
                val value = if (pair.size > 1) pair[1] else ""
                key to java.net.URLDecoder.decode(value, "UTF-8")
            }

            val host = params["host"] ?: return null
            val port = params["port"]?.toIntOrNull() ?: return null
            val code = params["code"] ?: return null
            val ssl = params["ssl"] == "1" || params["ssl"].equals("true", ignoreCase = true)
            val lan = params["lan"]
            val ipv6 = params["ipv6"]
            val ddns = params["ddns"]
            val relay = params["relay"]

            return PairingInfo(
                host = host,
                port = port,
                code = code,
                ssl = ssl,
                lanHost = lan,
                ipv6Host = ipv6,
                ddnsHost = ddns,
                relayHost = relay
            )
        }
    }
}

@Serializable
data class PairRequest(
    @SerialName("pairing_code") val pairingCode: String,
    @SerialName("device_name") val deviceName: String,
    @SerialName("platform") val platform: String = "android"
)

@Serializable
data class EndpointInfo(
    val type: String,
    val url: String
)

@Serializable
data class PairResponse(
    @SerialName("device_id") val deviceId: String,
    @SerialName("device_token") val deviceToken: String,
    val endpoints: List<EndpointInfo>? = null
)
